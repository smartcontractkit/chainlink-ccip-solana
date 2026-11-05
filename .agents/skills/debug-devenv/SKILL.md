---
name: Debug Devenv
description: Debug the Solana CCIP devenv by inspecting Docker container logs, querying databases, and correlating errors with source code across chainlink-ccip-solana and chainlink-ccv repos.
user_invocable: true
argument: "[test-name] — optional test function name (e.g. TestEVM2Solana or TestSolana2EVM). If omitted, just diagnose the running devenv."
---

## Debug Devenv

Diagnose issues in the Solana CCIP devenv. The devenv runs multiple Docker containers
(verifiers, aggregator, indexer, executor, databases, blockchains) orchestrated via
`make start-devenv`. The topology is 1 EVM (Anvil) chain + 2 Solana chains, supporting
**EVM→Solana** and **Solana→EVM** directions. Issues can originate in:

- **This repo** (`pkg/`, `devenv/`, `cmd/executor/`)
- **chainlink-ccv** (`../chainlink-ccv`) — verifier, aggregator, indexer, executor framework, devenv framework
- **On-chain programs** (`contracts/programs/`)

### Important: command execution notes

- **Container names are dynamic.** Always discover names with `docker ps --format '{{.Names}}' | grep <pattern>` rather than hardcoding.
- **Network sandbox.** `curl localhost:*` may be blocked. Use `docker exec` against a blockchain container as alternative.
- **rtk hook interference.** If docker fails with "No such file or directory (os error 2)", use `/usr/local/bin/docker` as an absolute path.

### Key containers

| Container grep pattern | Role | Direction |
|---|---|---|
| `evm-verifier` (exclude `db`) | Reads CCIPMessageSent events from EVM source chain | EVM→Solana |
| `solana-verifier` (exclude `db`) | Reads CCIPMessageSent events from Solana source chains | Solana→EVM |
| `aggregator` (exclude `db`, `redis`, `nginx`) | Aggregates verifier results, stores in DB | Both |
| `indexer` (exclude `db`) | Indexes aggregated messages for executor consumption | Both |
| `solana-executor` (exclude `db`) | Submits `execute_v2` txns to Solana destination | EVM→Solana |
| `default-executor` (exclude `db`) | Submits execute txns to EVM destination | Solana→EVM |
| `blockchain` | Solana test validators + Anvil EVM | Both |

### Database endpoints

| DB | Connection string | Key tables |
|---|---|---|
| Aggregator | `postgresql://aggregator:aggregator@localhost:7432/aggregator` | `commit_aggregated_reports`, `commit_verification_records` |
| Indexer | `postgresql://indexer:indexer@localhost:6432/indexer` | `indexer.messages` |

### Source code for correlation

**This repo:** `pkg/transmitter/`, `pkg/sourcereader/`, `pkg/logpoller/`, `pkg/destinationreader/`, `devenv/`, `cmd/executor/`

**chainlink-ccv:** `../chainlink-ccv/verifier/`, `../chainlink-ccv/aggregator/`, `../chainlink-ccv/indexer/`, `../chainlink-ccv/executor/`, `../chainlink-ccv/build/devenv/`

**chainlink-solana:** `../chainlink-solana/pkg/solana/` (Txm, RPC, logpoller)

---

## Key principle: keep local chainlink-ccv up to date

The devenv Docker images (verifier, aggregator, indexer, executor) are built from the **local** `../chainlink-ccv` checkout. If your local branch is stale or missing recent fixes from `main`, you'll hit bugs that have already been resolved upstream. This is the single most common class of devenv failure.

**Always check first:**
```bash
cd ../chainlink-ccv && git log --oneline main..HEAD | head -5  # what's on your branch but not main
cd ../chainlink-ccv && git log --oneline HEAD..main | head -5  # what's on main but not your branch
```

If `main` has commits your branch is missing, rebase or cherry-pick, rebuild images, and restart.

### Version split between verifier images and other services

The verifier Docker image may also be pinned via this repo's `go.mod`. If it diverges from the local `../chainlink-ccv` used to build aggregator/indexer/executor, wire format mismatches can occur (e.g. field type changes in `protocol/message_types.go`). Symptom: `no valid signers found` in aggregator with 0 verification records. Fix: align the versions.

---

## Debugging procedure

Follow these tiers IN ORDER. Stop as soon as you identify the root cause.

### Tier 0: Quick health check (always run first)

Run these in parallel:

1. **Container status:**
   ```bash
   docker ps --format "table {{.Names}}\t{{.Status}}\t{{.Ports}}" | grep -E "solana|aggregator|indexer|verifier|executor|blockchain"
   ```
   Look for containers that are restarting, exited, or unhealthy.

2. **Error scan across all key containers:**
   ```bash
   for c in $(docker ps --format '{{.Names}}' | grep -E 'executor|verifier|aggregator|indexer' | grep -v -E 'db|redis|nginx'); do echo "=== $c ==="; docker logs "$c" 2>&1 | grep -iE "panic|fatal|error|fail|validation failed" | grep -v "tls: bad certificate" | tail -10; echo; done
   ```
   **Important:** Do NOT use `--tail 50`. Some errors (especially verification task failures) are logged only once and may be far back. Scan the full log. Filter out `tls: bad certificate` which is transient startup noise.

3. **Pipeline health — DB row counts:**
   ```bash
   echo "=== Aggregator ===" && psql "postgresql://aggregator:aggregator@localhost:7432/aggregator" -t -c "SELECT 'verification_records: ' || COUNT(*) FROM commit_verification_records UNION ALL SELECT 'aggregated_reports: ' || COUNT(*) FROM commit_aggregated_reports;" && echo "=== Indexer ===" && psql "postgresql://indexer:indexer@localhost:6432/indexer" -t -c "SELECT 'messages: ' || COUNT(*) FROM indexer.messages;"
   ```

**Interpreting results — where is the pipeline stuck?**

| DB state | Pipeline stage stuck | What to check next |
|---|---|---|
| 0 verification_records | Verifiers not producing results | Verifier logs (full scan for errors) |
| Records but 0 aggregated_reports | Aggregator not reaching quorum | Aggregator logs, threshold config |
| Reports but 0 indexed messages | Indexer not consuming | Indexer logs |
| Messages indexed but no execution | Executor not picking up or tx failing | Executor logs |

### Tier 1: Targeted verifier diagnostic (if 0 verification_records)

When the pipeline is stuck at verifiers, the error is often a one-time task validation failure logged much earlier. Run:

```bash
for c in $(docker ps --format '{{.Names}}' | grep -E 'evm-verifier|solana-verifier' | grep -v db); do echo "=== $c ==="; docker logs "$c" 2>&1 | grep -iE "validation failed|verification failed|batch verification completed.*errorCount" | tail -5; echo; done
```

If this finds errors, the verifier is finding events but rejecting them at the task processing stage. The error message will tell you which validation is failing. Most commonly this is a fix missing from the local `../chainlink-ccv` branch — check `git log --all --oneline -- verifier/pkg/commit/validation.go` to see recent changes.

If NO errors at all, the verifier isn't finding events. Check for "Found CCIPMessageSent" in the logs to confirm event detection is working.

### Tier 2: State inspection (if Tier 0 doesn't pinpoint it)

1. **DB detail queries:**
   ```bash
   psql "postgresql://aggregator:aggregator@localhost:7432/aggregator" -c "SELECT message_id, created_at FROM commit_aggregated_reports ORDER BY created_at DESC LIMIT 5;"
   psql "postgresql://indexer:indexer@localhost:6432/indexer" -c "SELECT message_id, status, source_chain_selector, dest_chain_selector FROM indexer.messages ORDER BY ingestion_timestamp DESC LIMIT 5;"
   ```

2. **Executor logs (full scan):**
   ```bash
   for c in $(docker ps --format '{{.Names}}' | grep executor | grep -v db); do echo "=== $c ==="; docker logs "$c" 2>&1 | grep -iE "ERROR|WARN|panic|fatal" | tail -15; echo; done
   ```

3. **Verifier activity check** — confirm events are being found and finality is advancing:
   ```bash
   docker logs <evm-verifier-name> 2>&1 | grep -E "Found CCIPMessageSent|Finality check|Successfully published" | tail -10
   ```

### Tier 3: Deep analysis

1. **Full container logs with timestamps** for cross-container timeline reconstruction:
   ```bash
   for c in $(docker ps --format '{{.Names}}' | grep -E 'executor|verifier|aggregator|indexer' | grep -v -E 'db|redis|nginx'); do echo "=== $c ==="; docker logs --tail 200 --timestamps "$c" 2>&1 | tail -50; echo; done
   ```

2. **Container restarts:**
   ```bash
   docker inspect --format='{{.Name}} restarts={{.RestartCount}} started={{.State.StartedAt}}' $(docker ps -q)
   ```

3. **On-chain state:** Use `solana` CLI or RPC calls to inspect program accounts if errors point to on-chain issues (wrong PDA, missing accounts).

4. **If the issue traces to chainlink-ccv code:** Identify the specific file/function, make changes in `../chainlink-ccv`, rebuild images, and retest.

---

## Running a test

If a test name argument is provided (e.g. `TestEVM2Solana` or `TestSolana2EVM`):

1. First verify the devenv is running (Tier 0 container check).
2. Run the test:
   ```bash
   cd <path-to>/chainlink-ccip-solana && \
   go test -timeout 5m -v -count 1 ./devenv/tests/smoke/... -run <test-name> 2>&1
   ```
3. If the test fails, capture the failure output, then run the debugging tiers above.
4. Correlate the test failure (timeout location, assertion, error message) with container logs and DB state.

---

## Common failure patterns (examples)

| Symptom | Likely cause | Quick check |
|---|---|---|
| 0 verification_records + verifier ERROR with `validation failed` | Local chainlink-ccv branch missing a fix from `main` | `cd ../chainlink-ccv && git log --oneline HEAD..main -- verifier/` |
| `no valid signers found` in aggregator | Wire format mismatch between verifier and aggregator (different chainlink-ccv versions) | Align versions — rebase local chainlink-ccv or update go.mod pin |
| `Block X cleaned up, does not exist on node` | Finality checker stuck on pruned Solana blocks | Restart devenv (short-term) |
| `connection refused` on RPC | Blockchain container not ready or crashed | Check `blockchain-*` container logs |
| Executor logs show activity but test times out | Test polling wrong slot range, or logpoller not finding `ExecutionStateChangedV2` | Check test's `fromSlot` logic and `pkg/logpoller/` |
| `tls: bad certificate` on wsrpc | Job Distributor TLS — usually transient at startup | Wait a few seconds and check if it resolves |
