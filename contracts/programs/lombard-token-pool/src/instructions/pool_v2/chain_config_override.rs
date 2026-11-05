//! Admin CRUD for the pool's optional per-lane rate-limit override (`ChainConfigOverride`),
//! currently only the `FTF` (Faster Than Finality) override type consumed by
//! `lock_or_burn_tokens_v2`/`release_or_mint_tokens_v2`. Mirrors `burnmint-token-pool`'s own
//! `init_chain_config_override`/`set_chain_config_override_rate_limit`/
//! `delete_chain_config_override`.

use anchor_lang::prelude::*;
use base_token_pool::constants::{
    valid_version, ANCHOR_DISCRIMINATOR, POOL_OVERRIDE_CHAINCONFIG_SEED, POOL_STATE_SEED,
};
use base_token_pool::errors::CcipTokenPoolError;
use base_token_pool::events::{OverrideChainConfigConfigured, OverrideChainConfigRemoved};
use base_token_pool::rate_limiter::RateLimitConfig;
use ccip_common::state::PoolChainConfigOverrideType;

use crate::constants::{MAX_POOL_OVERRIDE_V, MAX_POOL_STATE_V};
use crate::state::{ChainConfigOverride, State};

#[derive(AnchorSerialize, AnchorDeserialize, Clone)]
pub struct InitChainConfigOverrideParams {
    pub remote_chain_selector: u64,
    pub mint: Pubkey,
    pub override_type: PoolChainConfigOverrideType,
    pub inbound: RateLimitConfig,
    pub outbound: RateLimitConfig,
}

#[event_cpi]
#[derive(Accounts)]
#[instruction(params: InitChainConfigOverrideParams)]
pub struct InitializeChainConfigOverride<'info> {
    #[account(
        seeds = [POOL_STATE_SEED, params.mint.as_ref()],
        bump,
        constraint = valid_version(state.version, MAX_POOL_STATE_V) @ CcipTokenPoolError::InvalidVersion,
    )]
    pub state: Account<'info, State>,

    #[account(
        init,
        seeds = [POOL_OVERRIDE_CHAINCONFIG_SEED, params.remote_chain_selector.to_le_bytes().as_ref(), params.mint.key().as_ref(), params.override_type.as_bytes()],
        bump,
        payer = authority,
        space = ANCHOR_DISCRIMINATOR + ChainConfigOverride::INIT_SPACE,
    )]
    pub chain_config_override: Account<'info, ChainConfigOverride>,

    #[account(mut, address = state.config.owner)]
    pub authority: Signer<'info>,

    pub system_program: Program<'info, System>,
}

pub fn init_chain_config_override(
    ctx: Context<InitializeChainConfigOverride>,
    params: InitChainConfigOverrideParams,
) -> Result<()> {
    ctx.accounts.chain_config_override.bump = ctx.bumps.chain_config_override;
    ctx.accounts.chain_config_override.version = MAX_POOL_OVERRIDE_V;
    let override_config = &mut ctx.accounts.chain_config_override.base;
    override_config.remote_chain_selector = params.remote_chain_selector;
    override_config.mint = params.mint;
    override_config.override_type = params.override_type;
    override_config.set_rate_limit(params.inbound.clone(), params.outbound.clone())?;

    emit_cpi!(OverrideChainConfigConfigured {
        chain_selector: params.remote_chain_selector,
        override_type: params.override_type,
        mint: params.mint,
        outbound_rate_limit: params.outbound,
        inbound_rate_limit: params.inbound,
    });

    Ok(())
}

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

    #[account(
        constraint = authority.key() == state.config.owner || authority.key() == state.config.rate_limit_admin @ CcipTokenPoolError::Unauthorized,
    )]
    pub authority: Signer<'info>,
}

/// Sets or updates the rate limit configuration for a chain config override.
///
/// Note: when adding new override types, extend the rate-limit bucket selection logic in
/// `validate_lock_or_burn_v2`/`validate_release_or_mint_v2`'s callers (currently `FTF` uses
/// `inbound_rate_limit`/`outbound_rate_limit`).
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

#[derive(AnchorSerialize, AnchorDeserialize, Clone)]
pub struct DeleteChainConfigOverrideParams {
    pub remote_chain_selector: u64,
    pub mint: Pubkey,
    pub override_type: PoolChainConfigOverrideType,
}

#[event_cpi]
#[derive(Accounts)]
#[instruction(params: DeleteChainConfigOverrideParams)]
pub struct DeleteChainConfigOverride<'info> {
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
        close = authority,
    )]
    pub chain_config_override: Account<'info, ChainConfigOverride>,

    #[account(mut, address = state.config.owner)]
    pub authority: Signer<'info>,
}

pub fn delete_chain_config_override(
    ctx: Context<DeleteChainConfigOverride>,
    params: DeleteChainConfigOverrideParams,
) -> Result<()> {
    emit_cpi!(OverrideChainConfigRemoved {
        chain_selector: params.remote_chain_selector,
        override_type: params.override_type,
        mint: params.mint,
    });
    Ok(())
}
