# Cross-family load tests

Sustained CCIP arbitrary-message load for Solana ↔ EVM using [wasp](https://github.com/smartcontractkit/chainlink-testing-framework/tree/main/wasp) for traffic generation and chainlink-ccv for e2e verification.

**Prerequisites:** `make start-devenv` (writes `devenv/env-out.toml`). Override config path with `LOAD_TEST_OUT_FILE`.

## Files

| File | Role |
|------|------|
| `setup.go` | Boots the test environment from `env-out.toml` |
| `gun.go` | `SolanaTXGun` — wasp gun for Sol→EVM sends |
| `harness.go` | Shared `runLoadProfile` (load generation + verification + metrics) |
| `interval_driver.go` | Ticker driver for fixed-interval sends (sub-1 RPS schedules wasp cannot express) |
| `sol2evm_test.go` | `TestSolana2EVMLoad` |
| `evm2sol_test.go` | `TestEVM2SolanaLoad` (uses CCV `EVMTXGun`) |

## End-to-end flow

```
  setupCrossFamilyLoadEnv
           │
           ▼
  build gun (SolanaTXGun or CCV EVMTXGun)
           │
           ▼
  runLoadProfile ─────────────────────────────────────────┐
           │                                              │
           │  1. AssertMessagesAsync (CCV) starts         │
           │     reading gun.SentMessages()               │
           │                                              │
           │  2. load generation fires gun.Call(): wasp   │
           │     RPS schedule, or fixed-interval ticker   │
           │     when LOAD_TEST_SEND_INTERVAL is set      │
           │                                              │
           │  3. CCV verifies each message async:         │
           │     ConfirmExecOnDest on destination         │
           │     records latency (send time → exec event) │
           │                                              │
           │  4. wasp profile.Wait()                      │
           │                                              │
           │  5. gun.CloseSentChannel()  ◄── must be last │
           │                                              │
           │  6. wait for verification goroutine          │
           │     → metrics summary + assertions           │
           └──────────────────────────────────────────────┘
```

Verification **must** start before wasp runs, and the sent-message channel **must** close only after wasp finishes. That ordering is required by CCV's `AssertMessagesAsync`.

## Setup (`setupCrossFamilyLoadEnv`)

1. Register the Solana chain family with CCV's `chainreg`.
2. Load `env-out.toml` (devenv output from `make start-devenv`).
3. Build a CLDF operations environment (needed for EVM→Sol: `EVMTXGun` uses EVM chain users and datastore).
4. Create CCV chain impls via `lib.ChainsMap()` — one `CCIP17` per chain in the env.

Returns a detached context (not tied to `t.Context()`) so wasp and verification are not cancelled when the test function returns early.

## Guns

Both guns implement CCV's `load.LoadGun` interface: `Call()` sends one message, `SentMessages()` exposes the async channel, `CloseSentChannel()` signals completion.

**Sol→EVM — `SolanaTXGun` (`gun.go`)**
- Calls `BuildChainMessage` + `SendChainMessage` on the Solana chain impl.
- Round-robin signers via `InitSignerPool` + `SolanaOptions.GetRoundRobinUser()` so concurrent sends do not share one key.
- Round-robin lanes (single lane today: Sol → EVM).

**EVM→Sol — CCV `EVMTXGun` (`evm2sol_test.go`)**
- `NewEVMTransactionGunFromTestConfig` with 50% empty and 50% 1500 B payloads (buffered execution on Solana).
- Pays fees in WETH (`EnsureWETHBalanceAndApproval`).
- Destination load defaults (e.g. 300k compute units) come from Solana's `V3LoadMessageOptions()` in `devenv/load_defaults.go`.

## Harness (`runLoadProfile`)

Shared entry point for both directions. Takes any `load.LoadGun`, RPS, and duration.

| Knob | Default | Notes |
|------|---------|-------|
| RPS / duration | 1 RPS × 30s | via `LOAD_TEST_RPS` / `LOAD_TEST_DURATION` |
| `LOAD_TEST_SEND_INTERVAL` | disabled | Fixed interval between sends (e.g. `30s`); overrides RPS. Fires exactly duration/interval sends |
| `execTimeout` | 30s | Per-send timeout, including source-chain tx confirm; EVM→Sol uses 60s (Sepolia block times) |
| `overallTimeout` | duration + 10m | `AssertMessagesAsync` budget; the 10m grace period after load ends lets late messages land |

After load completes, the harness asserts:
- `TotalSent == TotalReceived` (every gun send reached destination execution)
- `TotalSent > 0` (catches silent wasp failures)
- P90 end-to-end latency ≤ 1m (CCIP-9269 target)

## Metrics

CCV owns aggregation — this repo does not implement custom metrics logic.

1. Each `SentMessage` carries `SentTime` (captured before `SendChainMessage` returns).
2. `AssertMessagesAsync` polls `ConfirmExecOnDest` per message and records receive time when a terminal execution state is observed.
3. `metrics.CalculateMetricsSummary` computes totals, success rate, and latency percentiles.
4. `metrics.PrintMetricsSummary` logs the summary to test output.
5. `writeLoadMetricsReport` writes `load-metrics-<TestName>.json` to `LOAD_TEST_METRICS_DIR` (defaults to `devenv/`).
6. The harness logs gun call stats (wasp send success/failed/timeout — failed sends never reach the sent channel) and each failed message (seqNo + message ID: sent but never executed); both are also in the JSON report as `gun_stats` and `failed_messages`, and CI renders them in the results table (counts) plus a collapsible message-ID list.
7. EVM→Sol reports include `message_profile_mix` (configured payload ratios); CI renders this as **Payload mix** in the PR comment.

## Running locally

```sh
go test -timeout 10m -v -count=1 ./devenv/tests/load/... -run TestSolana2EVMLoad
go test -timeout 10m -v -count=1 ./devenv/tests/load/... -run TestEVM2SolanaLoad
```

Note: Restart the devenv if a prior run left Solana slots pruned.

## CI

Workflow: `Load Test (Devenv)` (`.github/workflows/test-load-devenv.yaml`). Optional — does not gate merge.

**What:** Runs `TestEVM2SolanaLoad` then `TestSolana2EVMLoad` sequentially on one devenv (1 RPS × 30s per direction by default). Same profile and assertions as local runs.

**Why:** E2e smoke sends one message per direction. Load CI exercises sustained throughput, concurrent signers/nonces, and async verification at scale (~150 messages per direction).

**How to run in CI:**

1. Open a PR that changes `contracts`, `pkg`, `devenv`, or `deployment`
2. Add the label **`load-test:devenv`** to the PR (re-runs on push while the label remains)
3. Results appear in the job summary and as an updatable PR comment (metrics table)
4. On failure, download artifact `ccip-load-logs-<ccv_ref>` (logs under `devenv/logs/docker-ccip-load-tests`)
