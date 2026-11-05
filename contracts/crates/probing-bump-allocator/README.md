# probing-bump-allocator

A deliberately minimal Solana program heap allocator: an upward bump allocator that never
reclaims, and that probes the top of every allocation before returning it. See the crate
docs in `src/lib.rs` for the design; this file covers the verification tools.

It is installed as the `#[global_allocator]` of `ccip-router`, `ccip-offramp` and
`test-ccip-receiver`, replacing [`suffix-eater-allocator`](../suffix-eater-allocator), which
is kept for comparison.

## The design in three lines

- The whole allocator state is one word at the base of the heap: the cursor offset.
- `dealloc` is a no-op, `realloc` is always allocate-and-copy. The cursor only rises.
- Every allocation performs a `write_volatile` at the last word of its reservation, before
  the frontier moves and before the pointer is returned.

That last point is why the allocator exists. A program cannot ask Solana how big its heap
frame actually is — the *transaction* chooses it with `request_heap_frame`, anywhere from
32KB to 256KB, and the program only sees the fixed start address. So the allocator is
configured with the 256KB maximum and finds the real end by touching it: a reservation past
the mapped heap aborts the instruction with a heap access violation instead of becoming a
live pointer into unmapped memory. The stored value is meaningless; only the store matters,
which is why it is `write_volatile` and not a plain assignment.

Reservations are padded up to a whole number of words at both ends, so the probe word is
always aligned and always *inside* the reservation — even for a one-byte request. It may
overwrite up to `size_of::<usize>() - 1` bytes past the requested size, which are padding
that no other allocation can ever be handed.

The stored cursor is the word immediately *before* the next allocatable region, which for a
non-empty arena is the last word of the newest reservation — that is, its probe word. That
convention removes the empty-arena special case: the word before the first allocatable
region is the offset word itself, at offset zero, which is what a zeroed heap already holds.
So `alloc` reads the cursor, adds one word, and never has to branch on emptiness.

## Layout

| Path | What it is |
| --- | --- |
| `src/allocator.rs` | The allocator, plus its unit tests |
| `src/types.rs` | `Address` / `Block` |
| `src/harness.rs` | Op-sequence driver + invariants, shared by the fuzzer and Miri |
| `fuzz/` | cargo-fuzz crate (its own workspace) |
| `testdata/corpus/` | Committed inputs replayed by `cargo test` and Miri |

## Does the probe actually trap?

Nothing in this crate tests that. The out-of-bounds write it would need is genuine UB on a
host, so neither the fuzzer nor Miri can exercise it, and there are currently no guard-page
tests. `make test-allocator-surfpool` does not cover it either: the
[`allocator-demo`](../allocator-demo) program it runs installs `suffix-eater-allocator`, not
this allocator. The trapping behaviour rests on Solana aborting the instruction with a heap
access violation for any access past the mapped heap frame.

## Fuzzing

```sh
make fuzz-probing-bump                                  # from the repo root, 60s by default
FUZZ_TIME=3600 FUZZ_JOBS=8 make fuzz-probing-bump       # a real run
```

Needs `cargo install cargo-fuzz` and `rustup toolchain install nightly`.

`fuzz_targets/ops.rs` decodes the input into a sequence of `alloc` / `dealloc` / `realloc`
calls and runs them against a fresh arena, asserting after every call that:

1. a returned pointer is aligned as requested;
2. the payload lies inside the arena and clear of its cursor word;
3. the payload does not overlap any live block, and is never below the high-water mark of
   everything handed out so far;
4. the probing write really happened, at an aligned word inside the new reservation;
5. the cursor landed exactly on that reservation's last word, and strictly advanced;
6. `dealloc` reclaimed nothing, and a failed `alloc`/`realloc` left the cursor untouched;
7. `realloc` never grew in place, preserved the `min(old, new)` prefix, and left the old
   block intact;
8. every block the run has seen still holds its own byte tag — including *freed* ones,
   which is the headline property here: memory this allocator has handed out once is never
   touched again.

Point 8 is the mirror image of `suffix-eater-allocator`'s headline property, where freeing
everything returns the whole arena. Here freeing everything returns nothing, and the
harness asserts exactly that.

The harness keeps a sliding window of the most recent retired blocks rather than all of
them, so that byte tags stay unique across everything it still checks.

AddressSanitizer is on by default, and the harness backs each arena with an exactly sized
heap allocation so that a write past the arena end lands in a redzone rather than in slack.

### Turning a finding into a regression

```sh
cp fuzz/artifacts/ops/crash-<sha> testdata/corpus/<descriptive-name>.bin
```

then add one line to `CORPUS` in `src/harness.rs`. The input now replays under plain
`make rust-tests` and under Miri. To iterate on a single input without adding it:

```sh
make fuzz-probing-bump-repro ARTIFACT=contracts/crates/probing-bump-allocator/fuzz/artifacts/ops/crash-<sha>
```

## Miri

```sh
make miri-probing-bump
```

Needs `rustup toolchain install nightly` and `rustup +nightly component add miri`. Runs the
unit tests and the corpus replay under both Stacked Borrows and Tree Borrows.

Miri is the only tool here that checks the raw pointer work in `src/types.rs` for undefined
behaviour, and it runs under `-Zmiri-strict-provenance`.

One test is `#[cfg_attr(miri, ignore)]`:
`a_fixed_address_allocator_starts_from_a_zero_offset`, the only one that goes through
`with_fixed_address_range`, whose address is a bare integer with no provenance — which is
also why the on-chain instantiation is inherently outside Miri's reach. The harness and the
other tests use `with_owned_region`, which keeps the host arena's provenance.

### A note on `Sync`

Carrying a pointer costs `Allocator` its automatic `Sync`, which `#[global_allocator]`
requires, so there is an explicit `unsafe impl Sync for Allocator`. That impl is a promise
the type cannot keep by itself: the cursor word is non-atomic, so sharing one allocator
across threads is a data race. What makes it sound is the `target_os = "solana"` gate in
`install_global_allocator!`, which stops the allocator ever being installed as a global on a
multi-threaded host.

## What the harness deliberately cannot cover

`run_ops` always gives the allocator a `len` equal to the arena's real length, so the
probing write always lands in mapped memory. Configuring it with more than the arena really
has is the production case, and on a host that write is out-of-bounds UB — so it belongs in
a child process behind a guard page or on chain, not in the fuzzer. Please don't "fix" the
harness by fuzzing that configuration.
