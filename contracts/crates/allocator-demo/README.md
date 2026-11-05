# allocator-demo

A deployable native Solana program that installs
[`suffix-eater-allocator`](../suffix-eater-allocator) as its `#[global_allocator]` and exercises it.

Its instruction runs 8 phases that each grow, checksum, and drop a `Vec<u64>`. The maximum workload
(4096 items) is deliberately sized so its final vector plus allocator metadata is a few bytes larger
than Solana's default 32 KiB heap. The Surfpool test uses that boundary to prove the allocator
really consumes the heap frame the *transaction* requests: the same transaction fails with a heap
access violation on the default heap, and succeeds once `request_heap_frame(64 KiB)` is added.

## Running the test

```sh
./scripts/test-surfpool.sh          # or: make test-allocator-surfpool, from the repo root
```

Requires `cargo-build-sbf` from the Solana/Agave CLI toolchain. Surfpool runs in-process via the
`surfpool-sdk` crate, so the standalone `surfpool` CLI is not needed. The first run compiles the
full Agave dependency graph and takes several minutes.

The host-only unit tests need neither:

```sh
cargo test --locked
```

## Why this is a separate workspace

This directory is listed in `exclude` in `contracts/Cargo.toml` and carries its own `Cargo.lock`.
The `surfpool-tests` feature pins the modular Solana crates to the exact versions in Surfpool
1.5.0's release lockfile — necessary because later Solana 4.x minors use `wincode` 0.6 while
Surfpool's Agave 4.1 runtime uses 0.5. Keeping it out of the contracts workspace stops those exact
pins from constraining the CCIP programs. Use `--locked`.

**This test is not run in CI.** It is a local-only tool.
