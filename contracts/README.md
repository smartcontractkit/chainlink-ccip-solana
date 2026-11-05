# Chainlink Solana contracts (programs)

## Prerequisites

Install Rust, Solana & Anchor. See https://solana.com/docs/intro/installation

## Build

To build on the host:

```
anchor build
```

To build inside a docker environment:

```bash
anchor build --verifiable
```

To build for a specific network, specify via a cargo feature:

```bash
anchor build -- --features mainnet
```

Available networks with declared IDs:

- mainnet
- testnet
- devnet
- localnet (default)

### Build with different program_ids (e.g. staging)

```bash
# If not reusing program_ids, delete/move existing keypairs first
# rm target/deploy/*-keypair.json
# Generate new IDs
./scripts/regenerate.sh
# Rebuild programs (needs docker and solana-verify)
./scripts/verify_build.sh
```

## Test

Make sure to run `pnpm i` to fetch mocha and other test dependencies.

Start a dockerized shell that contains Solana and Anchor:

```bash
./scripts/anchor-shell.sh
```

Next, generate a keypair for anchor:

```bash
solana-keygen new -o id.json
```

### Run anchor TypeScript tests (automatically tests against a local node)

```bash
anchor test
```

### Run GoLang tests (automatically tests against a local node)

Pre-requisites:

- Have the `solana-test-validator` command installed
- Run `anchor build` if there have been any changes to the program under test.

```bash
make go-tests-contracts
```

#### `anchor-go` bindings generation

Install `https://github.com/smartcontractkit/anchor-go`

To install `anchor-go` locally so that you can use the `anchor-go` command globally, follow these steps:

1. **Clone the repository:**

   ```bash
   git clone https://github.com/smartcontractkit/anchor-go.git
   cd anchor-go
   ```

2. **Install the command globally:**

   Run the following command to install the `anchor-go` command globally:

   ```bash
   go install
   ```

   This will install the `anchor-go` binary to your `$GOPATH/bin` directory. Make sure that this directory is included in your system's `PATH` environment variable.

3. **Then run the following command to generate the Go bindings:**

   ```bash
   make anchor-go-gen
   ```

## Release

Programs are released by release-please as the `contracts` package, tagged `contracts/vX.Y.Z`. All programs share `[workspace.package] version` in `Cargo.toml`, which is the only line release-please bumps. `main` carries the last released version, with no `-dev` suffix.

1. Merging `feat:`/`fix:` PRs that touch `contracts/` opens or updates a "chore: release contracts X.Y.Z" PR.
2. When ready to release, check out that PR's branch and run `make contracts-release-sync`. It updates `Cargo.lock` and the version in `target/idl/*.json` and `target/types/*.ts`. Commit and push; CI's `check artifacts` job fails until this is done.
3. Merge the release PR. release-please rebuilds its branch when `main` moves, so redo step 2 if anything lands on `main` in between.
4. The `contracts/vX.Y.Z` tag triggers `.github/workflows/contract-release.yml`, which checks the versions, builds each program with `scripts/verify_build.sh` and publishes the artifacts.

## Notes

### Usage

> [!WARNING]
> Contracts in `dev/` directories or with a typeAndVersion ending in `-dev` are under active development
> and are likely unaudited.
> Please refrain from using these in production applications.
