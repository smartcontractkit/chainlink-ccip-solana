# Token Pool Invariants — Solana

> **Base invariants**: [TOKEN_POOL_INVARIANTS.md](https://github.com/smartcontractkit/chainlink-ccip/blob/main/chains/evm/contracts/invariants/TOKEN_POOL_INVARIANTS.md)
> All base invariants (INV-POOL-1 through INV-POOL-21, INV-RL-1 through INV-RL-11, INV-PCFG-1 through INV-PCFG-3, INV-PVAL-1/2) apply to Solana unless noted below.
> This file documents Solana-specific invariants and how the base invariants are implemented.

---

## Solana-Specific Invariants

### INV-SOL-AL-1 — Sender Allow-List

Token pools may enforce a sender allow-list on outbound operations. When enabled, only addresses in the allow-list may initiate `lockOrBurn`.

The allow-list is a `Vec<Pubkey>` stored in the pool's `State` account. The `allow_list_enabled` flag controls enforcement. During `validate_lock_or_burn`, if the flag is enabled, the sender's address is checked against the list.

### INV-SOL-AL-2 — Allow-List Administration

The pool owner controls the allow-list: adding/removing addresses and enabling/disabling enforcement.

### INV-SOL-AL-3 — Allow-List Disabled Behavior

When the allow-list is disabled, any sender may initiate outbound operations (subject to rate limits and other checks).

### INV-SOL-TID-1 — Token Identity Validation

The pool must validate that the token being operated on matches the pool's configured token. A pool configured for token A must reject operations on token B.

The `config_mint` from the pool's `BaseConfig` is compared against the `local_token` (the mint account passed to the instruction). If they do not match, the operation reverts. This prevents cross-token attacks where a malicious caller might attempt to use a pool configured for one token to operate on a different token.

---

## Implementation Notes

### Section 1 — Pool Types

**INV-POOL-2 / INV-POOL-3**: Three pool types exist — `burnmint-token-pool`, `lockrelease-token-pool`, and `cctp-token-pool`. All share the `base-token-pool` crate for common logic (rate limiting, validation, remote config). The pool token account is a PDA-derived Associated Token Account owned by the pool program.

### Section 2 — Access Control

**INV-POOL-4**: Outbound operations require a `router_authority` PDA signer. The pool validates that the CPI call originates from the router by checking the PDA derivation against the router's program ID. The PDA is seeded deterministically, making it unforgeable.

**INV-POOL-5**: Inbound operations require an `offramp_authority` PDA signer. The pool validates that the CPI call originates from a registered offramp by checking the `allowed_offramp` PDA (seeded by `[ALLOWED_OFFRAMP, source_chain_selector, offramp_pubkey]`). Unregistered offramps cannot produce a valid PDA, so the instruction fails at account validation.

**INV-POOL-6**: Remote chain configuration is stored in `RemoteConfig` PDAs. The PDA must exist and be initialized before any token operation. If the PDA does not exist, Anchor's account deserialization fails.

### Section 3 — V1 vs V2 Interface

**INV-POOL-7**: Solana is a v1 CCIP chain; V1 is legacy and must be supported.

**INV-POOL-9**: Both V1 and V2 operations exist side by side (e.g., `lock_tokens_v1` / `lock_tokens_v2`, `burn_tokens_v1` / `burn_tokens_v2`). V1 operations use simpler context structures without finality parameters. The code version routing determines which path is used.

### Section 5 — Decimal Handling

**INV-POOL-14 through INV-POOL-18**: Decimal conversion uses `U256` arithmetic to safely handle cross-chain amount scaling. The local token's decimals are read from the SPL mint metadata (`mint.decimals`). The conversion validates that the result fits in `u64` before proceeding with SPL token operations. Overflow in the `U256` → `u64` downcast reverts the instruction.

### Section 7 — Rate Limiting

**INV-RL-1 through INV-RL-4**: The `base-token-pool` crate implements the token bucket in the rate limiter module. On each operation, `refill()` is called first (using `Clock::get()` for the current timestamp), then `consume()` validates capacity and deducts tokens. Errors: `RLRateLimitReached` (insufficient tokens — includes the minimum wait time in the error), `RLMaxCapacityExceeded` (single request exceeds bucket capacity).

**INV-RL-8**: Validated at configuration time — `capacity >= rate` when the bucket is enabled.

**INV-RL-10**: The pool owner can set a separate `rate_limit_admin` who has independent authority to configure rate limits without full pool ownership. Both the pool owner and the rate limit admin can update rate limit parameters.

### Section 8 — Pool Configuration

**INV-PCFG-1**: Remote chain configuration is managed through `RemoteConfig` PDAs. Initialization requires a non-empty `token_address` (a `RemoteAddress`). The `pool_addresses` vector starts empty and is populated separately.

**INV-PCFG-2**: `RemoteConfig.pool_addresses` is a `Vec` that supports appending new addresses or overwriting the entire list (with account reallocation when the vec grows).

### Section 9 — Source Pool Validation

**INV-PVAL-1**: During `validate_release_or_mint`, the incoming `source_pool_address` is checked against `remote_config.pool_addresses`. If the address is not found in the configured set, the operation reverts.
