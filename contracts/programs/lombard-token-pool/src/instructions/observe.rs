use anchor_lang::prelude::*;
use base_token_pool::constants::{
    valid_version, POOL_CHAINCONFIG_SEED, POOL_OVERRIDE_CHAINCONFIG_SEED, POOL_STATE_SEED,
};
use base_token_pool::errors::CcipTokenPoolError;
use base_token_pool::observe::{
    pool_chain_observation, pool_chain_override_observation, pool_chain_v2_observation,
    pool_state_observation, PoolChainConfigObservation, PoolChainConfigOverrideObservation,
    PoolChainConfigV2Observation, PoolStateObservation,
};
use ccip_common::state::PoolChainConfigOverrideType;

use crate::constants::{MAX_POOL_OVERRIDE_V, MAX_POOL_STATE_V, POOL_CHAINCONFIG_SEED_2_0};
use crate::state::{ChainConfig, ChainConfigOverride, ChainConfigV2, State, MAX_CHAIN_CONFIG_V2_V};
use crate::TOKEN_POOL_TYPE_AND_VERSION;

pub fn observe_state(ctx: Context<ObserveState>, _mint: Pubkey) -> Result<PoolStateObservation> {
    let state = &ctx.accounts.state;

    Ok(pool_state_observation(
        TOKEN_POOL_TYPE_AND_VERSION.to_string(),
        state.version,
        &state.config,
    ))
}

pub fn observe_chain_config(
    ctx: Context<ObserveChainConfig>,
    remote_chain_selector: u64,
    mint: Pubkey,
) -> Result<PoolChainConfigObservation> {
    pool_chain_observation::<Clock>(
        TOKEN_POOL_TYPE_AND_VERSION.to_string(),
        remote_chain_selector,
        mint,
        &ctx.accounts.chain_config.base,
    )
}

pub fn observe_chain_config_override(
    ctx: Context<ObserveChainConfigOverride>,
    _remote_chain_selector: u64,
    _mint: Pubkey,
    _override_type: PoolChainConfigOverrideType,
) -> Result<PoolChainConfigOverrideObservation> {
    let chain_config_override = &ctx.accounts.chain_config_override;

    pool_chain_override_observation::<Clock>(
        TOKEN_POOL_TYPE_AND_VERSION.to_string(),
        chain_config_override.version,
        &chain_config_override.base,
    )
}

pub fn observe_chain_config_v2(
    ctx: Context<ObserveChainConfigV2>,
    remote_chain_selector: u64,
    mint: Pubkey,
) -> Result<PoolChainConfigV2Observation> {
    let chain_config_v2 = &ctx.accounts.chain_config_v2;

    Ok(pool_chain_v2_observation(
        TOKEN_POOL_TYPE_AND_VERSION.to_string(),
        chain_config_v2.version,
        remote_chain_selector,
        mint,
        &chain_config_v2.ccv_config,
        chain_config_v2.allowed_finality_config,
    ))
}

#[derive(Accounts)]
#[instruction(mint: Pubkey)]
pub struct ObserveState<'info> {
    #[account(
        seeds = [POOL_STATE_SEED, mint.as_ref()],
        bump,
        constraint = valid_version(state.version, MAX_POOL_STATE_V) @ CcipTokenPoolError::InvalidVersion,
    )]
    pub state: Account<'info, State>,
}

#[derive(Accounts)]
#[instruction(remote_chain_selector: u64, mint: Pubkey)]
pub struct ObserveChainConfig<'info> {
    #[account(
        seeds = [POOL_CHAINCONFIG_SEED, remote_chain_selector.to_le_bytes().as_ref(), mint.as_ref()],
        bump,
    )]
    pub chain_config: Account<'info, ChainConfig>,
}

#[derive(Accounts)]
#[instruction(remote_chain_selector: u64, mint: Pubkey)]
pub struct ObserveChainConfigV2<'info> {
    #[account(
        seeds = [POOL_CHAINCONFIG_SEED_2_0, remote_chain_selector.to_le_bytes().as_ref(), mint.as_ref()],
        bump = chain_config_v2.bump,
        constraint = valid_version(chain_config_v2.version, MAX_CHAIN_CONFIG_V2_V) @ CcipTokenPoolError::InvalidVersion,
    )]
    pub chain_config_v2: Account<'info, ChainConfigV2>,
}

#[derive(Accounts)]
#[instruction(remote_chain_selector: u64, mint: Pubkey, override_type: PoolChainConfigOverrideType)]
pub struct ObserveChainConfigOverride<'info> {
    #[account(
        seeds = [POOL_OVERRIDE_CHAINCONFIG_SEED, remote_chain_selector.to_le_bytes().as_ref(), mint.as_ref(), override_type.as_bytes()],
        bump = chain_config_override.bump,
        constraint = valid_version(chain_config_override.version, MAX_POOL_OVERRIDE_V) @ CcipTokenPoolError::InvalidVersion,
    )]
    pub chain_config_override: Account<'info, ChainConfigOverride>,
}
