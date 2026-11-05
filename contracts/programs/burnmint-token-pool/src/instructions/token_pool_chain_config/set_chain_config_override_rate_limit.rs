use anchor_lang::prelude::*;
use base_token_pool::{constants::*, errors::*, events::*};

use crate::state::*;
use crate::{ChainConfigOverride, SetChainConfigOverrideRateLimitParams, State};

#[event_cpi]
#[derive(Accounts)]
#[instruction(params: SetChainConfigOverrideRateLimitParams)]
pub struct SetChainConfigOverrideRateLimit<'info> {
    #[account(
        seeds = [POOL_STATE_SEED, params.mint.as_ref()],
        bump,
        constraint = valid_version(state.version, MAX_POOL_STATE_V) @ CcipTokenPoolError::InvalidVersion,
    )]
    pub state: Account<'info, State>,

    #[account(
        mut,
        seeds = [POOL_OVERRIDE_CHAINCONFIG_SEED, params.remote_chain_selector.to_le_bytes().as_ref(), params.mint.key().as_ref(), params.override_type.as_bytes()],
        bump = chain_config_override.bump,
        constraint = valid_version(chain_config_override.version, MAX_POOL_OVERRIDE_V) @ CcipTokenPoolError::InvalidVersion,
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
