---
name: solana-devenv-e2e
description: 'Understand and work with the Solana CCIP devenv e2e test environment. Use when: asking about devenv architecture, chain initialization, contract deployment flow, token pool setup, message lifecycle, how env-phased.toml or env-out.toml work, how to add e2e tests, how to add token pools, how verifiers/executors/aggregator/indexer interact, debugging devenv failures, running smoke or load tests against a deployed environment (private staging), understanding CCV framework integration, CLDF operations, or any question about devenv/ directory.'
---

# Solana Devenv E2E

On-demand reference and procedural guide for the Solana CCIP devenv — local development environment with EVM and Solana chains, verifiers, executors, aggregator, and indexer.

## When to Use

- Questions about devenv architecture or how chains, contracts, tokens are initialized
- Understanding the e2e test flow (send → verify → aggregate → execute → confirm)
- Adding or modifying e2e tests
- Adding new token pool types
- Debugging devenv startup or test failures
- Understanding how `env-phased.toml` / `env-out.toml` / CLDF datastore work
- Understanding CCV framework integration (chainreg, ImplFactory, deployment changesets)

## Procedure

### Step 1: Load the Architecture Reference

Read the full architecture document for detailed knowledge:

```
Read file: devenv/README.md
```

Read devenv/README.md for usage and startup modes. For the topics below, work from the source files in Step 2:
- **Repos involved** — chainlink-ccip-solana, chainlink-ccv, chainlink-ccip and their roles
- **Bootstrap flow** — `make start-devenv` → phased startup → contract deployment → services launch
- **Contract initialization** — FeeQuoter, Router, OffRamp, Executor, CommitteeVerifier (sequence + params)
- **Token flow** — GetSupportedPools → ComputeTokenCombinations → GetTokenExpansionConfigs → DeployTokensAndPools
- **E2E test lifecycle** — setup → build message → send → verify → aggregate → execute → confirm
- **Solana-specific messaging** — ccip_send_v2 account resolution, LUTs, CPI event extraction
- **Key files map** — every relevant file with its purpose

### Step 2: Examine Relevant Source Files

Based on the user's question, read the appropriate source files:

**For chain initialization / contract deployment:**
- `devenv/deployment.go` — `PostDeployContractsForSelector()`, all CLDF operations
- `devenv/impl.go` — `ImplFactory`, how Chain instances are created
- `devenv/ccvregistry/solana.go` — CLDF provider factory, chain config loader

**For token pools:**
- `devenv/tokens.go` — `GetSupportedPools()`, `GetTokenExpansionConfigs()`, `GetTokenTransferConfigs()`
- `devenv/tests/utils/tokens.go` — test-time token utilities (liquidity provision)

**For messaging / test flow:**
- `devenv/messaging.go` — `BuildChainMessage()`, `SendChainMessage()`
- `devenv/chain.go` — `Chain` struct, `ConfirmSendOnSource()`, `ConfirmExecOnDest()`, `GetTokenBalance()`

**For test structure:**
- `devenv/tests/smoke/evm2solana_test.go` — EVM→Solana functional smoke
- `devenv/tests/smoke/solana2evm_test.go` — Solana→EVM functional smoke
- `devenv/README.md` ("Testing a deployed environment") — private-staging smoke against a deployed env: `SMOKE_TEST_CONFIG` attach config, `TARGET_EVM_SELECTOR`, CI secrets, `Smoke (Deployed Env)` workflow and the `smoke:private-staging` PR label
- `devenv/tests/load/README.md` — load test architecture and CI workflow
- `devenv/tests/load/setup.go` — load env bootstrap
- `devenv/tests/load/gun.go` — `SolanaTXGun` for Sol→EVM
- `devenv/tests/load/harness.go` — wasp load profile harness
- `devenv/tests/load/sol2evm_test.go` — `TestSolana2EVMLoad`
- `devenv/tests/load/evm2sol_test.go` — `TestEVM2SolanaLoad`

**For environment config:**
- `devenv/env-monolith.toml` / `devenv/env-phased.toml` — topology (chains, verifiers, executors, aggregator, indexer)
- `devenv/cmd/ccv/main.go` — CLI entry point, chainreg registration

**For deployment operations:**
- `deployment/v2_0_0/operations/` — per-contract CLDF ops
- `deployment/v2_0_0/adapters/` — CCV adapter registrations

### Step 3: For Common Tasks

**Adding a new e2e test:**
1. Create test function in appropriate file (`evm2solana_test.go`, etc.)
2. Follow the setup pattern: register Solana → load env-out.toml → build chains → get clients → create test context
3. Build message with `BuildChainMessage()`, send with `SendChainMessage()`
4. Assert with `testCtx.AssertMessage()` and `ConfirmExecOnDest()`

**Adding a new token pool type:**
1. Add pool capability to `GetSupportedPools()` in `devenv/tokens.go`
2. Ensure pool program is listed in `devenv/env-phased.toml` under `[blockchains.solana_programs]`
3. Add pool program ID to OffRamp LUT in `PostDeployContractsForSelector()` (`devenv/deployment.go`)
4. Handle pool-specific deployment in `GetTokenExpansionConfigs()` if needed
5. Update transfer config in `GetTokenTransferConfigs()` if needed

**Debugging devenv startup:**
1. Check `make start-devenv` output for Docker build errors
2. Verify `contracts/target/deploy/*.so` exist (`make docker-build-contracts`)
3. Check container logs: `docker logs <container-name>`
4. Inspect aggregator DB: `psql "postgresql://aggregator:aggregator@localhost:7432/aggregator"`
5. Inspect indexer DB: `psql "postgresql://indexer:indexer@localhost:6432/indexer"`
6. See `devenv/README.md` for operational details
