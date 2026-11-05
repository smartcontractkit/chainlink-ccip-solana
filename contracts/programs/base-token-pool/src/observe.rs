//! Payload types and builders shared by every token pool's `observe_*` instructions.
//!
//! The pool programs each declare their own Anchor account contexts (their account types must be
//! owned by their own program ID), then delegate to the builders here so that all pools report the
//! same shape. See `ccip_common::observe` for the conventions these follow.
use anchor_lang::prelude::*;
use ccip_common::messages::FinalityConfig;
use ccip_common::observe::OBSERVATION_VERSION;
use ccip_common::state::PoolChainConfigOverrideType;

use crate::rate_limiter::{RateLimitTokenBucket, Timestamper};
use crate::state::{BaseChain, BaseConfig, BaseOverrideConfig, CCVConfig};

/// State of one rate limiter bucket, as of the moment of observation.
#[derive(AnchorSerialize, AnchorDeserialize, Clone, Debug)]
pub struct RateLimiterObservation {
    pub is_enabled: bool,
    pub capacity: u64,
    pub rate: u64,
    /// Tokens available now: the stored balance plus the refill accrued since the bucket was last
    /// touched, capped at `capacity`. This is the value EVM's `getCurrent*RateLimiterState`
    /// returns, not the raw stored balance.
    pub tokens: u64,
    /// Timestamp the `tokens` value corresponds to (the observation time, for an enabled bucket).
    pub last_updated: u64,
}

/// Per-mint configuration of a token pool, as observed by off-chain consumers.
///
/// The allow list is reported as a count only, as its size is unbounded by configuration.
#[derive(AnchorSerialize, AnchorDeserialize, Clone, Debug)]
pub struct PoolStateObservation {
    pub observation_version: u16,
    pub type_version: String,

    pub version: u8,

    // token
    pub token_program: Pubkey,
    pub mint: Pubkey,
    pub decimals: u8,
    pub pool_signer: Pubkey,
    pub pool_token_account: Pubkey,

    // ownership and administration
    pub owner: Pubkey,
    pub proposed_owner: Pubkey,
    pub rate_limit_admin: Pubkey,

    // routing
    pub router: Pubkey,
    pub router_onramp_authority: Pubkey,
    pub rmn_remote: Pubkey,

    /// Rebalancer, only meaningful for lock/release pools. The default pubkey means rebalancing is
    /// not allowed.
    pub rebalancer: Pubkey,

    // lock-or-burn allow list
    pub list_enabled: bool,
    pub allow_list_count: u32,
}

/// Per-remote-chain configuration of a token pool, as observed by off-chain consumers.
#[derive(AnchorSerialize, AnchorDeserialize, Clone, Debug)]
pub struct PoolChainConfigObservation {
    pub observation_version: u16,
    pub type_version: String,

    pub remote_chain_selector: u64,
    pub mint: Pubkey,

    /// Token address on the remote chain, as raw bytes.
    pub remote_token_address: Vec<u8>,
    pub remote_decimals: u8,
    /// Pool addresses on the remote chain, as raw bytes. More than one entry means earlier pool
    /// versions are still accepted as sources.
    pub remote_pool_addresses: Vec<Vec<u8>>,

    pub inbound_rate_limit: RateLimiterObservation,
    pub outbound_rate_limit: RateLimiterObservation,
}

/// Per-remote-chain, per-override-type rate limit override of a token pool, as observed by
/// off-chain consumers.
///
/// Override PDAs are optional: when a lane has no override of the requested type, the observing
/// instruction fails, which callers can read as "not overridden".
#[derive(AnchorSerialize, AnchorDeserialize, Clone, Debug)]
pub struct PoolChainConfigOverrideObservation {
    pub observation_version: u16,
    pub type_version: String,

    pub version: u8,
    pub remote_chain_selector: u64,
    pub mint: Pubkey,
    /// Which flavour of transfer this override applies to (e.g. fast transfers).
    pub override_type: PoolChainConfigOverrideType,

    pub inbound_rate_limit: RateLimiterObservation,
    pub outbound_rate_limit: RateLimiterObservation,
}

/// Per-remote-chain CCIP 2.0 configuration of a token pool, as observed by off-chain consumers.
#[derive(AnchorSerialize, AnchorDeserialize, Clone, Debug)]
pub struct PoolChainConfigV2Observation {
    pub observation_version: u16,
    pub type_version: String,

    pub version: u8,
    pub remote_chain_selector: u64,
    pub mint: Pubkey,

    /// Transfer amount above which the threshold CCVs are additionally required. Zero disables the
    /// threshold lists.
    pub threshold_amount: u64,
    pub outbound_ccvs: Vec<Pubkey>,
    pub threshold_outbound_ccvs: Vec<Pubkey>,
    pub inbound_ccvs: Vec<Pubkey>,
    pub threshold_inbound_ccvs: Vec<Pubkey>,

    /// Allowed finality policy for outbound (lock_or_burn) transfers on this lane.
    pub allowed_finality_config: FinalityConfig,
}

pub fn rate_limiter_observation<C: Timestamper>(
    bucket: &RateLimitTokenBucket,
) -> Result<RateLimiterObservation> {
    let observed = bucket.as_of_now::<C>()?;
    let cfg = observed.config();

    Ok(RateLimiterObservation {
        is_enabled: cfg.enabled,
        capacity: cfg.capacity,
        rate: cfg.rate,
        tokens: observed.tokens,
        last_updated: observed.last_updated,
    })
}

pub fn pool_state_observation(
    type_version: String,
    version: u8,
    config: &BaseConfig,
) -> PoolStateObservation {
    PoolStateObservation {
        observation_version: OBSERVATION_VERSION,
        type_version,

        version,

        token_program: config.token_program,
        mint: config.mint,
        decimals: config.decimals,
        pool_signer: config.pool_signer,
        pool_token_account: config.pool_token_account,

        owner: config.owner,
        proposed_owner: config.proposed_owner,
        rate_limit_admin: config.rate_limit_admin,

        router: config.router,
        router_onramp_authority: config.router_onramp_authority,
        rmn_remote: config.rmn_remote,

        rebalancer: config.rebalancer,

        list_enabled: config.list_enabled,
        allow_list_count: config.allow_list.len().try_into().unwrap_or(u32::MAX),
    }
}

pub fn pool_chain_observation<C: Timestamper>(
    type_version: String,
    remote_chain_selector: u64,
    mint: Pubkey,
    base: &BaseChain,
) -> Result<PoolChainConfigObservation> {
    Ok(PoolChainConfigObservation {
        observation_version: OBSERVATION_VERSION,
        type_version,

        remote_chain_selector,
        mint,

        remote_token_address: base.remote.token_address.address.clone(),
        remote_decimals: base.remote.decimals,
        remote_pool_addresses: base
            .remote
            .pool_addresses
            .iter()
            .map(|pool| pool.address.clone())
            .collect(),

        inbound_rate_limit: rate_limiter_observation::<C>(&base.inbound_rate_limit)?,
        outbound_rate_limit: rate_limiter_observation::<C>(&base.outbound_rate_limit)?,
    })
}

pub fn pool_chain_override_observation<C: Timestamper>(
    type_version: String,
    version: u8,
    base: &BaseOverrideConfig,
) -> Result<PoolChainConfigOverrideObservation> {
    Ok(PoolChainConfigOverrideObservation {
        observation_version: OBSERVATION_VERSION,
        type_version,

        version,
        remote_chain_selector: base.remote_chain_selector,
        mint: base.mint,
        override_type: base.override_type,

        inbound_rate_limit: rate_limiter_observation::<C>(&base.inbound_rate_limit)?,
        outbound_rate_limit: rate_limiter_observation::<C>(&base.outbound_rate_limit)?,
    })
}

pub fn pool_chain_v2_observation(
    type_version: String,
    version: u8,
    remote_chain_selector: u64,
    mint: Pubkey,
    ccv_config: &CCVConfig,
    allowed_finality_config: FinalityConfig,
) -> PoolChainConfigV2Observation {
    PoolChainConfigV2Observation {
        observation_version: OBSERVATION_VERSION,
        type_version,

        version,
        remote_chain_selector,
        mint,

        threshold_amount: ccv_config.threshold_amount,
        outbound_ccvs: ccv_config.outbound_ccvs.clone(),
        threshold_outbound_ccvs: ccv_config.threshold_outbound_ccvs.clone(),
        inbound_ccvs: ccv_config.inbound_ccvs.clone(),
        threshold_inbound_ccvs: ccv_config.threshold_inbound_ccvs.clone(),

        allowed_finality_config,
    }
}
