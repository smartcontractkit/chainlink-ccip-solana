# Go Bindings

Go bindings for the CCIP Solana programs, generated with `anchor-go`. Each contract major version is its own Go module:

| Directory | Module | Contracts | Release tags |
|---|---|---|---|
| `v1/` | `github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings` | CCIP 1.6 (legacy) | `contracts/gobindings/v1.x.y` |
| `v2/` | `github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/v2` | CCIP 2.0 | `contracts/gobindings/v2.x.y` |

Within a module, `latest/` tracks the newest bindings and `vX_Y_Z/` directories are frozen snapshots of released versions, so a consumer can import several versions side by side.

## v1 (legacy)

`v1/latest/` is a frozen copy of the 1.6 bindings, kept for consumers that still decode 1.6 lanes. It is not regenerated.

## v2

`v2/latest/` is generated from `contracts/target/idl` by `make anchor-go-gen`; never edit it by hand. The v2 module is versioned together with the contracts: release-please links the two (`linked-versions` in `release-please-config.json`), so a single release PR produces `contracts/vX.Y.Z` and `contracts/gobindings/vX.Y.Z` at the same version.
