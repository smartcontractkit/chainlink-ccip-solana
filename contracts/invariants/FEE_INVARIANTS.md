# Fee Invariants — Solana

> **Base invariants**: [FEE_INVARIANTS.md](https://github.com/smartcontractkit/chainlink-ccip/blob/main/chains/evm/contracts/invariants/FEE_INVARIANTS.md)
> All base invariants (INV-FEE-1 through INV-FEE-23) apply to Solana unless noted below.
> This file documents deviations and how they are implemented.

---

## Deviations from Base

### INV-FEE-3 — Fee Cap Denomination

The base specifies `maxUSDCentsPerMessage` (cap in USD cents). On Solana, the fee cap is denominated in **juels** after conversion: `maxFeeJuelsPerMsg` is a `u128` with 18 decimals. The fee-quoter converts the aggregated USD-cent fee to juels and then validates against this cap. `max_fee_juels_per_msg` is set during initialization (`require!(max_fee_juels_per_msg > 0)`).

### INV-FEE-12 — Network Fee

The base specifies two separate values (`tokenNetworkFeeUSDCents` / `messageNetworkFeeUSDCents`). On Solana, the fee-quoter stores `network_fee_usdcents` as a single flat per-message fee in the destination chain configuration.

### INV-FEE-17 — Price Staleness

The base says "There is no staleness check." On Solana, the fee-quoter supports an **optional** `gas_price_staleness_threshold`. If set to 0, staleness checking is disabled. If non-zero, gas prices older than the threshold are rejected. Price data is stored as `TimestampedPackedU224` (28-byte value + 8-byte timestamp).

> **TODO**: Review whether to backport the removal of the staleness check to v1.6 as well once this goes live.

---

## Implementation Notes

### Section 1.1 — Fee Structure

**INV-FEE-2**: The `fee-quoter` program computes fees via CPI. Fee components are aggregated in USD cents and then converted to the fee token denomination (typically LINK, expressed in juels with 18 decimals) using the configured token price.

### Section 3 — Pool Fee

**INV-FEE-6**: As a v1 legacy chain, Solana must support both V2 pool fee paths and the FeeQuoter fallback. The fee-quoter maintains per-chain-per-token fee configuration in `PerChainPerTokenConfig` PDAs (seeded by `[per_chain_per_token_config, chain_selector, token_mint]`) for fallback fee computation.

> **TODO**: Consider removing the `isEnabled` parameter once all pools (and lanes) are upgraded to V2, so that V2 pools always use custom pool fees. This would simplify the fee path and eliminate the fallback logic.

### Section 4 — Executor Fee

**INV-FEE-10**: The fee-quoter computes execution cost using `dest_gas_overhead` (base), per-byte calldata costs (with EIP-7623 threshold pricing for EVM destinations), and data availability costs (`dest_gas_per_data_availability_byte`, `dest_data_availability_multiplier_bps`, `dest_data_availability_overhead_gas`).

### Section 6 — LINK Discount

**INV-FEE-14**: The fee-quoter applies `premium_multiplier_wei_per_eth` from the billing token configuration when computing final token amounts.

### Section 8 — Fee Distribution

**INV-FEE-18 through INV-FEE-21**: Fee distribution is handled via SPL token transfers from the fee payer's token account to the respective recipient token accounts. The router coordinates these transfers during the `ccip_send` flow.

> **Note**: If the user pays in native SOL, the router is responsible for wrapping it into wSOL. All downstream contracts (fee-quoter, token pools, etc.) only deal with SPL tokens — they never handle native SOL directly.

### Section 10 — Token Transfer Fee Granularity

**INV-FEE-23**: Token transfer fee config is stored in per-chain-per-token PDAs (`[per_chain_per_token_config, chain_selector, token_mint]`). Each PDA contains `min_fee_usdcents`, `max_fee_usdcents`, and basis-point parameters. The fee-quoter validates that `max_fee_usdcents > min_fee_usdcents` at configuration time.
