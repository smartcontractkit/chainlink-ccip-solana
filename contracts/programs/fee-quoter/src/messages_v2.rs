use anchor_lang::prelude::*;
use ccip_common::types::{CrossChainGas, Usd18Decimals, UsdCents};

#[derive(AnchorDeserialize, AnchorSerialize, Clone, Debug)]
pub struct GetFallbackTokenTransferFeeV2Params {
    pub dest_chain_selector: u64,
    pub mint: Pubkey,
}

// This reuses CCIPv1 state for V2 fallback fees. For legacy token pools, we don't
// support BPS fees despite them being defined in TokenTransferFeeConfig. Instead,
// we use the `min_fee_usdcents` to encode the flat fee for a legacy token transfer.
//
// This is a wire type, so the flat fee uses the protocol-unified u32 USD cents
// (matching `min_fee_usdcents` and `Receipt.fee_usd_cents`) instead of UsdCents,
// which would silently truncate fees above u16::MAX cents ($655.35).
#[derive(AnchorDeserialize, AnchorSerialize, Clone, Debug)]
pub struct FallbackTokenTransferFeeV2 {
    pub flat_fee: UsdCents,
    pub dest_gas_limit: CrossChainGas,
    pub dest_bytes_overhead: u32,
}

#[derive(AnchorDeserialize, AnchorSerialize, Clone, Debug)]
pub struct QuoteGasForExecV2Params {
    pub dest_chain_selector: u64,
    pub fee_token_mint: Pubkey,
    pub gas_limit_sum: CrossChainGas,
    pub bytes_overhead_sum: u32,
}

#[derive(AnchorDeserialize, AnchorSerialize, Clone, Debug)]
pub struct QuoteGasForExecV2Result {
    pub total_gas: CrossChainGas,
    pub total_gas_cost: UsdCents,
    pub premium_percent_multiplier: u64,
}

#[derive(AnchorDeserialize, AnchorSerialize, Clone, Debug)]
pub struct ValidatedTokenPriceV2 {
    pub price: Usd18Decimals,
    pub billing_enabled: bool,
}
