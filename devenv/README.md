# Solana Devenv

Local development environment for Chainlink CCIP with Solana as both source and destination chain, using dummy (mock) offchain components.

## Prerequisites

- Docker
- Go
- [`just`](https://github.com/casey/just) (used by the CCV image build step)
- `chainlink-ccv` checked out at `../chainlink-ccv` relative to this repo root

## First-time setup

Build the contract binaries so CTF can pre-load them into the local validators:

```sh
make docker-build-contracts
```

This only needs to run once after a fresh clone, or after changing contract Rust source code.

## Starting the devenv

You must have built the Anchor programs first so `contracts/target/deploy/` exists and contains `*.so` files (see [First-time setup](#first-time-setup)). Otherwise Docker will fail when binding that directory, or `make start-devenv` will stop early with a pointer to `make docker-build-contracts`.

From the **repo root**:

```sh
make start-devenv
```

This will:
1. Build the CCV Docker images (aggregator, indexer, verifier, etc.) from `../chainlink-ccv`
2. Build the Solana-specific verifier Docker image (`cmd/committeeverifier/Dockerfile` --> `solana-verifier:latest`)
3. Build the Solana executor Docker image (`cmd/executor/Dockerfile` --> `solana-executor:latest`)
4. Tear down any existing containers
5. Spin up the devenv in **phased mode** (`ccv up devenv/env-phased.toml --env-mode phased`): 1 EVM (Anvil) + 1 Solana validator carrying two Solana chain selectors, with verifiers and executors for both directions (Solana→EVM and EVM→Solana)

During startup, the devenv pre-loads the router and offramp programs onto each validator via CTF (`--upgradeable-program`), then initializes them via CLDF operations.

Once running, the environment config is written to `env-phased-out.toml` at the repo root (the output file is `<config>-out.toml`, derived from the config name). The containers (verifiers, aggregator, indexer, DBs, executor, etc.) will keep running in the background.

### Startup modes (phased vs. legacy)

`make start-devenv` runs **phased mode**, the default and forward path. The CCV CLI's `--env-mode` flag selects the orchestrator:

| Mode | Flag | Config file | Output file |
| --- | --- | --- | --- |
| Phased (default) | `--env-mode phased` | `devenv/env-phased.toml` | `env-phased-out.toml` |
| Legacy / monolith | `--env-mode legacy` | `devenv/env-monolith.toml` | `env-monolith-out.toml` |

Phased mode (`NewPhasedEnvironment`) brings the stack up in ordered phases — blockchains → JD + node registration → standalone verifiers → contract deploy → lane config → aggregator/indexer → job-spec generation → write output TOML. The legacy monolith path is being retired; new work should target phased.

## Stopping the devenv

```sh
make stop-devenv
```

## Executor

The executor binary lives at [`cmd/executor/main.go`](../cmd/executor/main.go). It uses the CCV **bootstrap** + **executor service factory** pattern: `bootstrap.Run` with the shared executor factory, which wires the coordinator, indexer stream, leader election, and HTTP health endpoint. The Solana accessor registered from [`pkg/accessors`](../pkg/accessors/factory.go) builds a [`DestinationReader`](../pkg/destinationreader/destination_reader.go) per Solana destination chain and creates the [`SolanaContractTransmitter`](../pkg/transmitter/transmitter.go) after CCV injects the bootstrap keystore.

`make start-devenv` builds the executor image and the devenv framework starts the executor container from the `[[executor]]` block in `devenv/env-phased.toml` (Job Distributor + bootstrap). Transaction fee-payer signing uses the bootstrap-managed Solana Ed25519 transmitter key declared by [`cmd/executor/main.go`](../cmd/executor/main.go): `transmitter.DefaultKeyName` (`solana/tx/executor_solana_transmitter_key`). The CCV devenv fetches that public key from the executor bootstrap server and funds it on each configured Solana chain.

### Configuration

- **Input:** `devenv/env-phased.toml` — `[[executor]]`, `environment_topology` (NOP + executor pools), and the rest of the topology.
- **Generated:** After `up`, see `env-phased-out.toml` for resolved ports, container names, and materialized executor settings.

### Viewing executor logs

Use the container name from the output TOML (often prefixed `solana-`, e.g. `solana-solana-executor-1`). For example:

```sh
docker logs -f <executor-container-name>
```

### Running manually (development / override)

Run the binary with bootstrap config and JD as in the Docker image, or use a local bootstrap TOML if you have one. The bootstrap keystore creates or loads the Solana transmitter key declared by the executor binary; executor job config is normally supplied by the Job Distributor, not a hand-written `env-executor.toml`.

**Transmitter funding:** The CCV devenv launch flow funds the transmitter key automatically via `fundExecutorTransmitters`. If you run the executor outside that path (manual binary, harness, or custom topology), you must fund the bootstrap-managed Solana transmitter key yourself. The key name is `transmitter.DefaultKeyName` (`solana/tx/executor_solana_transmitter_key`). On startup the executor logs its public key at info level (`solana executor fee payer ready`); fund that address with SOL on each Solana destination chain before expecting execution txs to succeed. A zero-balance WARN is logged if the account is unfunded.

## Running the e2e tests

The e2e tests expect the devenv to already be running. With the devenv up, run from the repo root:

```sh
SMOKE_TEST_CONFIG=../../env-phased-out.toml \
  go test -timeout 30m -v -count 1 ./devenv/tests/smoke/... -run 'TestSolana2EVM|TestEVM2Solana'
```

Requires `make start-devenv` (or equivalent) so contracts, aggregator, and executors are running. The e2e tests read `../../env-phased-out.toml` (the phased output) for connection details.

### Testing a deployed environment

Deployed-environment tests use a complete CCV attach config. The committed example
contains the public private-staging topology; its deployed addresses and RPC URLs are
left as placeholders.

```sh
cp devenv/env-private-staging.deployed.example.toml \
  devenv/env-private-staging.deployed.toml
# Fill the address and RPC placeholders in the ignored .deployed.toml file.

set -a
source devenv/.env # PRIVATE_KEY and SOLANA_PRIVATE_KEY only
set +a

SMOKE_TEST_CONFIG="$PWD/devenv/env-private-staging.deployed.toml" \
  make test-smoke CASES='TestEVM2Solana/ArbMessaging$$'
```

Files matching `devenv/env-*.deployed.toml` are gitignored. The test does not merge
environment variables into the config: `SMOKE_TEST_CONFIG` must point to a complete,
valid file.

If the manifest has more than one EVM chain, set `TARGET_EVM_SELECTOR` to the
selector of the chain to test. The run fails when several EVM chains exist and
the variable is not set.

Set `USE_TEST_ROUTER=true` when the 2.0 lane is on the EVM TestRouter, not on the
production Router. EVM-source sends and the token allowance then use the TestRouter.
The `Smoke (Deployed Env)` workflow sets it for `CCV_PROD_TESTNET`.

For CI, create a protected GitHub Environment with these secrets:

- `SMOKE_TEST_CONFIG`: complete contents of the `.deployed.toml` file
- `E2E_EVM_PRIVATE_KEY`: funded EVM test signer
- `E2E_SOLANA_PRIVATE_KEY`: funded Solana test signer

The GitHub secret holds file contents; the workflow materializes it and reuses
`SMOKE_TEST_CONFIG` as the config path passed to the Go test process.

Run the `Smoke (Deployed Env)` workflow and select that environment. Each matrix job
materializes the config under `$RUNNER_TEMP`, runs one direction, and removes the file.
For a pull request, apply the `smoke:private-staging` label. Add another explicit
label-to-environment condition when a second PR-targeted environment is introduced.

## Load tests

Sustained CCIP message load using [wasp](https://github.com/smartcontractkit/chainlink-testing-framework/tree/main/wasp) and shared `load.AssertMessagesAsync` verification from chainlink-ccv. Requires a running devenv (`make start-devenv`). The load harness still defaults to `../../env-out.toml`, so with the phased default point it at the phased output via `LOAD_TEST_OUT_FILE=../../env-phased-out.toml`. See [tests/load/README.md](tests/load/README.md) for how setup, guns, harness, and metrics fit together.

```sh
# Solana → EVM (~5 RPS × 30s)
go test -timeout 10m -v -count=1 ./devenv/tests/load/... -run TestSolana2EVMLoad

# EVM → Solana (~5 RPS × 30s; uses CCV EVMTXGun)
go test -timeout 10m -v -count=1 ./devenv/tests/load/... -run TestEVM2SolanaLoad

# Both directions
go test -timeout 15m -v -count=1 ./devenv/tests/load/... -run 'TestSolana2EVMLoad|TestEVM2SolanaLoad'
```

Assertions: all sent messages reach `ConfirmExecOnDest` on the destination chain; P90 end-to-end latency ≤ 1m.

Steps:

1. The mock source reader emits a deterministic message to the aggregator.
2. The test polls the aggregator until the message is fully verified and appears in the indexer.
3. The executor picks up verified work and submits execution on the destination chain.
4. The test waits for `ExecutionStateChangedV2` on the destination. The execution tx fee payer is the bootstrap-managed Solana Ed25519 transmitter key, not CCV's EVM transmitter key.

Requires `make start-devenv` (or equivalent) so the **Solana executor container** from `[[executor]]` is running.

If a prior failed run left Solana slots pruned, restart the devenv before re-running.

> The load test reads `../../env-out.toml` (relative to the test file) by default for aggregator connection details, so the devenv must have been started at least once to produce an output file. Since the phased default writes `env-phased-out.toml`, set `LOAD_TEST_OUT_FILE=../../env-phased-out.toml`.

## How programs are deployed

Both contract tests and the devenv use CTF to pre-load program binaries onto the local Solana validator at startup via `--upgradeable-program`. This forces each program to its canonical address (from `Anchor.toml`), matching the `declare_id!()` in the compiled binary. No BPF deploy or keypair management is needed.

The devenv then goes a step further: it initializes the router and offramp on-chain via CLDF operations (`routerops.Initialize`, `offrampops.Initialize`, `offrampops.InitializeConfig`). This creates the PDA accounts and config state needed for transaction processing. Other contracts (fee quoter, RMN remote, token pools) still use placeholder addresses.

## Upstream dependency tracking (chainlink-ccv)

The ccv ref is managed via `scripts/ci/ccv-ref.yaml`:

```yaml
pinned: e5f4463c...   # required to pass for merge
tracking: main         # PR + nightly, informational
```

| Workflow | When | Ref | Gates merge? |
| --- | --- | --- | --- |
| `solana-ci.yml` → `devenv-pinned` job | PR / push to main | pinned | yes |
| `test-load-devenv.yaml` → `devenv-load-pinned` | PR label `load-test:devenv` | pinned | no |
| `solana-ci-nightly.yml` → `devenv-tracking` job | nightly cron + manual | tracking | no (informational) |

Smoke and chaos use `devenv-test` (smoke is required). Load uses `run-load-test` (both directions on one devenv, opt-in via `load-test:devenv` label; metrics in job summary + PR comment). See `devenv/tests/load/README.md`.

### Repinning

When tracking goes red or the pinned SHA disappears (upstream squash-merge):

1. Run `make pin-ccv REF=<new-sha>` (updates yaml, go.mod, and local ccv checkout)
2. Commit the changes

## Inspecting the aggregator DB

The aggregator Postgres instance is exposed on port `7432`. You can query it directly with `psql`:

```sh
psql "postgresql://aggregator:aggregator@localhost:7432/aggregator" \
  -c "SELECT message_id, created_at FROM commit_aggregated_reports ORDER BY created_at DESC LIMIT 10;"
```

Other useful tables:

| Table | Contents |
| --- | --- |
| `commit_aggregated_reports` | Fully aggregated messages (quorum reached) |
| `commit_verification_records` | Individual verifier observations |
| `commit_aggregated_report_verifications` | Per-report verification entries |

## Inspecting the indexer DB

The indexer Postgres instance is exposed on port `6432`. The indexer subscribes to the aggregator and stores one record per aggregated message. Query it with `psql`:

```sh
psql "postgresql://indexer-1:indexer-1@localhost:6432/indexer-1" \
  -c "SELECT message_id, status, source_chain_selector, dest_chain_selector, ingestion_timestamp FROM indexer.messages ORDER BY ingestion_timestamp DESC LIMIT 10;"
```

Key columns:

| Column | Description |
| --- | --- |
| `message_id` | Hex message ID |
| `status` | Processing status |
| `source_chain_selector` | Source chain |
| `dest_chain_selector` | Destination chain |
| `ingestion_timestamp` | When the indexer received the record |
