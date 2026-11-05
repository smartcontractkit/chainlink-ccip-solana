use anchor_lang::prelude::*;

use ccip_common::messages::FinalityConfig;
use ccip_common::observe::{ObservePageParams, OBSERVATION_VERSION};
use ccip_common::seed;
use ccip_common::types::UsdCents;

use crate::error::ExecutorError;
use crate::state::{valid_version, ChainConfig, Config, MAX_CHAIN_CONFIG_V, MAX_CONFIG_V};

/// Global configuration of this executor, as observed by off-chain consumers.
///
/// The CCV allow list holds up to 64 pubkeys, which does not fit the 1024-byte return data limit,
/// so only its length is reported here. Use `observe_allowed_ccvs` to page through the entries.
#[derive(AnchorSerialize, AnchorDeserialize, Clone, Debug)]
pub struct ExecutorConfigObservation {
    pub observation_version: u16,
    pub type_version: String,

    pub version: u8,
    pub max_ccvs_per_msg: u8,
    pub owner: Pubkey,
    pub proposed_owner: Pubkey,
    pub fee_aggregator: Pubkey,
    pub allowed_finality_config: FinalityConfig,
    pub ccv_allowlist_enabled: bool,
    pub allowed_ccvs_count: u32,
}

/// A page of this executor's CCV allow list.
#[derive(AnchorSerialize, AnchorDeserialize, Clone, Debug)]
pub struct ExecutorAllowedCcvsObservation {
    pub observation_version: u16,

    /// Total number of entries in the allow list, so consumers know when to stop paging.
    pub allowed_ccvs_count: u32,
    /// Offset the returned page starts at.
    pub offset: u32,
    pub allowed_ccvs: Vec<Pubkey>,
}

/// Per-destination-chain configuration of this executor, as observed by off-chain consumers.
#[derive(AnchorSerialize, AnchorDeserialize, Clone, Debug)]
pub struct ExecutorChainConfigObservation {
    pub observation_version: u16,
    pub type_version: String,

    pub version: u8,
    /// Taken from the instruction argument: this account predates storing its own selector.
    pub remote_chain_selector: u64,
    pub usd_cents_fee: UsdCents,
    pub enabled: bool,
}

pub fn observe_config(ctx: Context<ObserveConfig>) -> Result<ExecutorConfigObservation> {
    let config = &ctx.accounts.config;

    Ok(ExecutorConfigObservation {
        observation_version: OBSERVATION_VERSION,
        type_version: env!("CCIP_BUILD_TYPE_VERSION").to_string(),

        version: config.version,
        max_ccvs_per_msg: config.max_ccvs_per_msg,
        owner: config.owner,
        proposed_owner: config.proposed_owner,
        fee_aggregator: config.fee_aggregator,
        allowed_finality_config: config.allowed_finality_config,
        ccv_allowlist_enabled: config.ccv_allowlist_enabled,
        allowed_ccvs_count: config.allowed_ccvs.len().try_into().unwrap_or(u32::MAX),
    })
}

pub fn observe_allowed_ccvs(
    ctx: Context<ObserveConfig>,
    page: ObservePageParams,
) -> Result<ExecutorAllowedCcvsObservation> {
    let config = &ctx.accounts.config;

    Ok(ExecutorAllowedCcvsObservation {
        observation_version: OBSERVATION_VERSION,

        allowed_ccvs_count: config.allowed_ccvs.len().try_into().unwrap_or(u32::MAX),
        offset: page.offset,
        allowed_ccvs: page.apply(&config.allowed_ccvs),
    })
}

pub fn observe_chain_config(
    ctx: Context<ObserveChainConfig>,
    remote_chain_selector: u64,
) -> Result<ExecutorChainConfigObservation> {
    let chain_config = &ctx.accounts.chain_config;

    Ok(ExecutorChainConfigObservation {
        observation_version: OBSERVATION_VERSION,
        type_version: env!("CCIP_BUILD_TYPE_VERSION").to_string(),

        version: chain_config.version,
        remote_chain_selector,
        usd_cents_fee: chain_config.usd_cents_fee,
        enabled: chain_config.enabled,
    })
}

#[derive(Accounts)]
pub struct ObserveConfig<'info> {
    #[account(
        seeds = [seed::CONFIG],
        bump = config.bump,
        constraint = valid_version(config.version, MAX_CONFIG_V) @ ExecutorError::InvalidVersion,
    )]
    pub config: Account<'info, Config>,
}

#[derive(Accounts)]
#[instruction(remote_chain_selector: u64)]
pub struct ObserveChainConfig<'info> {
    #[account(
        seeds = [seed::REMOTE_CHAIN, &remote_chain_selector.to_le_bytes()],
        bump = chain_config.bump,
        constraint = valid_version(chain_config.version, MAX_CHAIN_CONFIG_V) @ ExecutorError::InvalidVersion,
    )]
    pub chain_config: Account<'info, ChainConfig>,
}
