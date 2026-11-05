# Finality / Faster-Than-Finality (FTF) Invariants — Solana

> **Base invariants**: [FINALITY_INVARIANTS.md](https://github.com/smartcontractkit/chainlink-ccip/blob/main/chains/evm/contracts/invariants/FINALITY_INVARIANTS.md)
> All base invariants (INV-FIN-ENC-1 through INV-FIN-ENC-4, INV-FIN-SRC-1 through INV-FIN-SRC-4, INV-FIN-DST-1 through INV-FIN-DST-5, INV-FIN-POOL-1 through INV-FIN-POOL-7, INV-FIN-EXEC-1/2, INV-FIN-CCV-1 through INV-FIN-CCV-3, INV-FIN-REORG-1/2, INV-FIN-OPTIN-1/2) apply to Solana without deviation.
> This file documents how they are implemented.

---

## Implementation Notes

### Section 2 — Source Side

**INV-FIN-SRC-4**: V1 pool operations (`lock_tokens_v1`, `burn_tokens_v1`) do not accept finality parameters. V2 operations include finality in the context, enabling FTF-aware pools.

### Section 4.2 — Pool Outbound/Inbound Validation

**INV-FIN-POOL-3**: V1 operations (e.g., `lock_tokens_v1`, `release_tokens_v1`) do not include finality parameters, so they implicitly operate under `WAIT_FOR_FINALITY_FLAG`. V2 operations include finality and validate it against the pool's allowed finality configuration.

### Section 4.3 — Rate Limiting

**INV-FIN-POOL-4 / INV-FIN-POOL-5**: The base-token-pool's rate limiter maintains per-chain, per-direction token buckets. Rate limit configuration is stored in chain-specific PDAs with separate inbound/outbound parameters.

### Section 8 — Opt-in

**INV-FIN-OPTIN-2**: V1 pool operations and receivers without explicit finality configuration default to `WAIT_FOR_FINALITY_FLAG`, automatically rejecting FTF requests.
