use anchor_lang::prelude::*;
use base_token_pool::{constants::*, errors::*, events::*, rate_limiter::RateLimitConfig};
use ccip_common::state::PoolChainConfigOverrideType;

use crate::pool::state::*;

#[derive(AnchorSerialize, AnchorDeserialize, Clone)]
pub struct SetChainConfigOverrideRateLimitParams {
    pub remote_chain_selector: u64,
    pub mint: Pubkey,
    pub override_type: PoolChainConfigOverrideType,
    pub inbound: RateLimitConfig,
    pub outbound: RateLimitConfig,
}

#[event_cpi]
#[derive(Accounts)]
#[instruction(params: SetChainConfigOverrideRateLimitParams)]
pub struct SetChainConfigOverrideRateLimit<'info> {
    #[account(
        seeds = [POOL_STATE_SEED, params.mint.as_ref()],
        bump,
        constraint = state.is_latest_version() @ CcipTokenPoolError::InvalidVersion,
    )]
    pub state: Account<'info, State>,

    #[account(
        mut,
        seeds = [POOL_OVERRIDE_CHAINCONFIG_SEED, params.remote_chain_selector.to_le_bytes().as_ref(), params.mint.key().as_ref(), params.override_type.as_bytes()],
        bump = chain_config_override.bump,
        constraint = chain_config_override.is_latest_version() @ CcipTokenPoolError::InvalidVersion,
    )]
    pub chain_config_override: Account<'info, ChainConfigOverride>,

    #[account(constraint = authority.key() == state.config.owner || authority.key() == state.config.rate_limit_admin @ CcipTokenPoolError::Unauthorized)]
    pub authority: Signer<'info>,
}

/// Sets or updates the rate limit configuration for a chain config override.
///
/// Note: When adding new override types, extend the rate-limit bucket selection logic here
/// (currently FTF uses inbound_rate_limit and outbound_rate_limit fields).
pub fn set_chain_config_override_rate_limit(
    ctx: Context<SetChainConfigOverrideRateLimit>,
    params: SetChainConfigOverrideRateLimitParams,
) -> Result<()> {
    ctx.accounts
        .chain_config_override
        .base
        .set_rate_limit(params.inbound.clone(), params.outbound.clone())?;

    emit_cpi!(OverrideChainConfigConfigured {
        chain_selector: params.remote_chain_selector,
        override_type: params.override_type,
        mint: params.mint,
        outbound_rate_limit: params.outbound,
        inbound_rate_limit: params.inbound,
    });

    Ok(())
}
