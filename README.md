# CCIP Solana

- `contracts/` - Solana programs (Rust) + integration tests (Go) built on Anchor
  - `programs/` - Solana programs (15 programs including router, offramp, fee quoter, token pools, etc.)
  - `crates/` - Shared Rust crates (`anchor_discriminator`, `arrayvec`, `build-commit`,
    `suffix-eater-allocator`, `probing-bump-allocator`) and `allocator-demo`, a standalone
    demo of the allocator
  - `tests/` - Go integration tests (CCIP, CCTP, examples, test utilities)
  - `gobindings/v2/` - Auto-generated Go bindings for contracts using `anchor-go` (Go module `…/contracts/gobindings/v2`, versioned with the contracts)
  - `utils/` - Go utility packages (ccip, common, eth, fees, state, tokens)
  - `target/` - Build output (IDLs, types, deployed artifacts, vendor)
- `pkg/` - Go packages (devenv)
- [`docs/`](docs/README.md) - Guides for contracts, the executor, and the devenv.
- `scripts/` - Utility scripts for generating artifacts

## Dependencies

- [Rust](https://www.rust-lang.org/tools/install) (edition 2021)
- [Go](https://go.dev/doc/install) >= 1.27.1
- [Solana CLI](https://docs.anza.xyz/cli/install) 3.1.10 (recommended version for Anchor 1.0.0, see [release notes](https://www.anchor-lang.com/docs/updates/release-notes/1-0-0#recommended-solana-version))
- [Anchor](https://www.anchor-lang.com/docs/installation) 1.0.0

## Development

```bash
# install anchor-go if needed
go install github.com/smartcontractkit/anchor-go@v1.0.0

# build contracts + IDL
make build-contracts

# go bindings need to be regenerated if contract changes were made
make anchor-go-gen

# test contracts
make rust-tests go-tests-contracts
```

The custom heap allocators have extra verification that is too slow for CI and is run by
hand:

- `probing-bump-allocator`, the non-reclaiming one installed in `ccip-router`,
  `ccip-offramp` and `test-ccip-receiver`: `make miri-probing-bump` and
  `make fuzz-probing-bump`. See
  [`contracts/crates/probing-bump-allocator/README.md`](contracts/crates/probing-bump-allocator/README.md).
- `suffix-eater-allocator`, the reclaiming alternative, kept for comparison:
  `make miri-allocator`, `make fuzz-allocator` and `make test-allocator-surfpool`. See
  [`contracts/crates/suffix-eater-allocator/README.md`](contracts/crates/suffix-eater-allocator/README.md).

## Agent Skills

Reusable agent skills live under `.agents/skills/` (e.g. `solana-ccip-invariants`,
`solana-devenv-e2e`, `debug-devenv`) so they are version-controlled and shared across agent
tools. Claude Code only auto-discovers skills from `.claude/skills/`, which is gitignored, so
each developer links them in locally once:

```bash
# Symlink every skill in .agents/skills/ into Claude Code's discovery path
mkdir -p .claude/skills
for skill in .agents/skills/*/; do
  name=$(basename "$skill")
  ln -sfn "../../.agents/skills/$name" ".claude/skills/$name"
done
```

The symlinks are local-only (gitignored) and Claude Code loads the skills automatically in every
session in this repo. Re-run the loop after cloning or when new skills are added.

## CI

Two workflows live under `.github/workflows/`:

- `solana-ci.yml` — runs on every PR and push to main. `contracts-build` produces the `.so` cache, then five siblings fan out in parallel: `contracts-rust-tests`, `contracts-go-tests`, `verify-gobindings`, `integration-tests` (matrix), `devenv-pinned`. Per-job paths-filter gating skips work that doesn't apply to the change. The `ci-result` job aggregates all six and is the single required status check.
- `solana-ci-nightly.yml` — runs on a cron against the `tracking` ccv ref (typically `main`). Informational only; never required for branch protection. Failure path exists for Slack notification + auto-bump PR (TODOs in the workflow).

The `devenv-test` composite action (`.github/actions/devenv-test/`) is shared by both — validates `ccv-ref.yaml` vs go.mod, pins deps, builds images, spins up devenv, runs the e2e test.

## Devenv

Local development environment for running CCIP across Solana (and optionally EVM) chains. Assumes `chainlink-ccv` is checked out at `../chainlink-ccv`.

```bash
# Build CCV Docker images (verifier, aggregator, indexer, etc.) with :latest tags
make build-ccv-images

# Build the Solana-specific verifier Docker image
make build-solana-verifier-image

# Start Solana-to-Solana devenv (builds images first, then brings up the environment)
make start-devenv

# Tear down the devenv
make stop-devenv
```
