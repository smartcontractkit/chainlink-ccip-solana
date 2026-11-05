use anchor_lang::prelude::*;
use ccip_common::seed;
use ccip_common::types::UsdCents;

use crate::{
    context::valid_version,
    instructions::{safe_deserialize, CCIP_LOCK_OR_BURN_V1_RET_BYTES},
    instructions_v2::{V2_MAX_CHAINSTATE_V, V2_MAX_CONFIG_V},
    messages_v2::{FallbackTokenTransferFeeV2, GetFallbackTokenTransferFeeV2Params},
    state::{Config, PerChainPerTokenConfig},
    state_v2::DestChainV2,
    FeeQuoterError,
};

pub fn fallback_get_token_transfer_fee_v2<'info>(
    ctx: Context<'info, GetFallbackTokenTransferFeeV2<'info>>,
    params: GetFallbackTokenTransferFeeV2Params,
) -> Result<FallbackTokenTransferFeeV2> {
    let per_chain_per_token_config = safe_deserialize::per_chain_per_token_config(
        &ctx.accounts.per_chain_per_token_config,
        params.mint,
        params.dest_chain_selector,
    )?;

    let dest_chain_config = &ctx.accounts.dest_chain.config;

    let (flat_fee, dest_gas_limit, dest_bytes_overhead) = match per_chain_per_token_config {
        Some(PerChainPerTokenConfig {
            token_transfer_config: config,
            ..
        }) if config.is_enabled => (
            config.min_fee,
            config.dest_gas_overhead,
            config.dest_bytes_overhead,
        ),
        // If the token has no specific overrides configured, we use the global defaults.
        _ => (
            // SAFETY: `default_token_fee_usd_cents` is a `u16`, so this widens into `UsdCents`'
            // `u32` and cannot lose value.
            UsdCents::new(dest_chain_config.default_token_fee_usd_cents as u32),
            dest_chain_config.default_token_dest_gas_overhead,
            CCIP_LOCK_OR_BURN_V1_RET_BYTES,
        ),
    };

    Ok(FallbackTokenTransferFeeV2 {
        dest_gas_limit,
        dest_bytes_overhead,
        flat_fee,
    })
}

#[derive(Accounts)]
#[instruction(params: GetFallbackTokenTransferFeeV2Params)]
pub struct GetFallbackTokenTransferFeeV2<'info> {
    #[account(
        seeds = [seed::CONFIG],
        bump,
        constraint = valid_version(config.version, V2_MAX_CONFIG_V) @ FeeQuoterError::InvalidVersion,
    )]
    pub config: Account<'info, Config>,

    #[account(
        seeds = [seed::DEST_CHAIN_V2, params.dest_chain_selector.to_le_bytes().as_ref()],
        bump = dest_chain.bump,
        constraint = valid_version(dest_chain.version, V2_MAX_CHAINSTATE_V) @ FeeQuoterError::InvalidVersion,
    )]
    pub dest_chain: Account<'info, DestChainV2>,

    #[account(
        seeds = [seed::PER_CHAIN_PER_TOKEN_CONFIG, params.dest_chain_selector.to_le_bytes().as_ref(), params.mint.as_ref()],
        bump,
    )]
    pub per_chain_per_token_config: UncheckedAccount<'info>,
}
