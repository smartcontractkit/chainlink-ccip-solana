# suffix-eater-allocator

An upward-growing bump allocator with suffix reclamation. See the crate docs in
`src/lib.rs` for the design; this file covers the two heavier verification tools, both of
which are local-only and deliberately not part of CI.

The programs (`ccip-router`, `ccip-offramp`, `test-ccip-receiver`) now install
[`probing-bump-allocator`](../probing-bump-allocator) instead. This crate is kept, with its
whole test suite, as the reclaiming alternative to compare against.

## Layout

| Path | What it is |
| --- | --- |
| `src/allocator.rs` | The allocator, plus its unit tests |
| `src/types.rs` | `Address` / `Footer` / `Block` |
| `src/harness.rs` | Op-sequence driver + invariants, shared by the fuzzer and Miri |
| `fuzz/` | cargo-fuzz crate (its own workspace) |
| `testdata/corpus/` | Committed inputs replayed by `cargo test` and Miri |

## Fuzzing

```sh
make fuzz-allocator                                  # from the repo root, 60s by default
FUZZ_TIME=3600 FUZZ_JOBS=8 make fuzz-allocator       # a real run
```

Needs `cargo install cargo-fuzz` and `rustup toolchain install nightly`.

`fuzz_targets/ops.rs` decodes the input into a sequence of `alloc` / `dealloc` / `realloc`
calls and runs them against a fresh arena, asserting after every call that:

1. a returned pointer is aligned as requested;
2. the payload lies inside the arena and clear of its cursor word;
3. the payload does not overlap any other live block;
4. every live block still holds the byte tag it was filled with — the oracle that catches
   corruption an address comparison would miss;
5. `realloc` preserves the `min(old, new)` prefix, and a null return leaves the original
   block live and intact;
6. a failed `alloc` or `realloc` leaves the cursor exactly where it was;
7. once every block has been freed, in whatever order, the cursor is back to zero — the
   headline property of suffix reclamation.

AddressSanitizer is on by default, and the harness backs each arena with an exactly sized
heap allocation so that a write past the arena end lands in a redzone rather than in
slack.

### Turning a finding into a regression

```sh
cp fuzz/artifacts/ops/crash-<sha> testdata/corpus/<descriptive-name>.bin
```

then add one line to `CORPUS` in `src/harness.rs`. The input now replays under plain
`make rust-tests` and under Miri. To iterate on a single input
without adding it:

```sh
make fuzz-allocator-repro ARTIFACT=contracts/crates/suffix-eater-allocator/fuzz/artifacts/ops/crash-<sha>
```

## Miri

```sh
make miri-allocator
```

Needs `rustup toolchain install nightly` and `rustup +nightly component add miri`. Runs
the unit tests and the corpus replay under both Stacked Borrows and Tree Borrows; takes a
couple of minutes.

Miri is the only tool here that checks the raw pointer work in `src/types.rs` for
undefined behaviour, and it runs under `-Zmiri-strict-provenance`.

One test, `fixed_address_allocator_uses_zero_offset_for_an_empty_arena`, is
`#[cfg_attr(miri, ignore)]`. It is the only one that goes through
`with_fixed_address_range`, whose address is a bare integer with no provenance — which is
also why the on-chain instantiation is inherently outside Miri's reach (see below).

### A note on `Sync`

Carrying a pointer costs `Allocator` its automatic `Sync`, which `#[global_allocator]`
requires, so there is now an explicit `unsafe impl Sync for Allocator`. That impl is a
promise the type cannot keep by itself: the cursor word and footers are non-atomic, so
sharing one allocator across threads is a data race. What makes it sound is the
`target_os = "solana"` gate in `install_global_allocator!`, which stops the allocator ever
being installed as a global on a multi-threaded host.

## What the harness deliberately cannot cover

`run_ops` always gives `Allocator::with_fixed_address_range` a `len` equal to the arena's
real length. On chain the program instead configures the 256KB maximum while the runtime
may map as little as 32KB, and *relies* on the footer write past the mapped end aborting
the transaction — that is the allocator's bounds check. On a host that write is genuine
out-of-bounds UB, so it cannot be fuzzed here. `make test-allocator-surfpool` covers it
on-chain instead. Please don't "fix" the harness by fuzzing that configuration.
