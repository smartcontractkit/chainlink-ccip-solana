use anchor_lang::prelude::*;

use ccip_common::observe::OBSERVATION_VERSION;
use ccip_common::seed;
use ccip_common::state::AccountSchemaVersion;

use crate::error::CcipOfframpError;
use crate::state::{Config, ReferenceAddresses, SourceChain};

/// Global configuration of the offramp, as observed by off-chain consumers. Combines the `config`
/// and `reference_addresses` PDAs, which are always initialized together.
#[derive(AnchorSerialize, AnchorDeserialize, Clone, Debug)]
pub struct OffRampConfigObservation {
    pub observation_version: u16,
    pub type_version: String,

    pub version: u8,
    pub svm_chain_selector: u64,
    pub owner: Pubkey,
    pub proposed_owner: Pubkey,

    // reference addresses
    pub reference_addresses_version: u8,
    pub router: Pubkey,
    pub fee_quoter: Pubkey,
    pub offramp_lookup_table: Pubkey,
    pub rmn_remote: Pubkey,
}

/// Per-source-chain configuration of the offramp, as observed by off-chain consumers.
#[derive(AnchorSerialize, AnchorDeserialize, Clone, Debug)]
pub struct OffRampSourceChainObservation {
    pub observation_version: u16,
    pub type_version: String,

    pub version: u8,
    pub chain_selector: u64,
    pub is_enabled: bool,
    /// OnRamp addresses allowed to send from the source chain, as raw bytes (not ABI encoded).
    pub on_ramps: Vec<Vec<u8>>,
    pub default_ccvs: Vec<Pubkey>,
    pub lane_mandated_ccvs: Vec<Pubkey>,
}

pub fn observe_config(ctx: Context<ObserveConfig>) -> Result<OffRampConfigObservation> {
    let config = &ctx.accounts.config;
    let reference_addresses = &ctx.accounts.reference_addresses;

    Ok(OffRampConfigObservation {
        observation_version: OBSERVATION_VERSION,
        type_version: env!("CCIP_BUILD_TYPE_VERSION").to_string(),

        version: config.version,
        svm_chain_selector: config.svm_chain_selector,
        owner: config.owner,
        proposed_owner: config.proposed_owner,

        reference_addresses_version: reference_addresses.version,
        router: reference_addresses.router,
        fee_quoter: reference_addresses.fee_quoter,
        offramp_lookup_table: reference_addresses.offramp_lookup_table,
        rmn_remote: reference_addresses.rmn_remote,
    })
}

pub fn observe_source_chain(
    ctx: Context<ObserveSourceChain>,
    _source_chain_selector: u64,
) -> Result<OffRampSourceChainObservation> {
    let source_chain = &ctx.accounts.source_chain;

    Ok(OffRampSourceChainObservation {
        observation_version: OBSERVATION_VERSION,
        type_version: env!("CCIP_BUILD_TYPE_VERSION").to_string(),

        version: source_chain.version,
        chain_selector: source_chain.chain_selector,
        is_enabled: source_chain.config.is_enabled,
        on_ramps: source_chain
            .config
            .on_ramps
            .iter()
            .map(|on_ramp| on_ramp.bytes().to_vec())
            .collect(),
        default_ccvs: source_chain.config.default_ccvs.clone(),
        lane_mandated_ccvs: source_chain.config.lane_mandated_ccvs.clone(),
    })
}

#[derive(Accounts)]
pub struct ObserveConfig<'info> {
    #[account(
        seeds = [seed::CONFIG],
        bump = config.bump,
        constraint = config.version == Config::MAX_V @ CcipOfframpError::InvalidVersion,
    )]
    pub config: Account<'info, Config>,

    #[account(
        seeds = [seed::REFERENCE_ADDRESSES],
        bump = reference_addresses.bump,
        constraint = reference_addresses.version == ReferenceAddresses::MAX_V @ CcipOfframpError::InvalidVersion,
    )]
    pub reference_addresses: Account<'info, ReferenceAddresses>,
}

#[derive(Accounts)]
#[instruction(source_chain_selector: u64)]
pub struct ObserveSourceChain<'info> {
    #[account(
        seeds = [seed::SOURCE_CHAIN, source_chain_selector.to_le_bytes().as_ref()],
        bump = source_chain.bump,
        constraint = source_chain.version == SourceChain::MAX_V @ CcipOfframpError::InvalidVersion,
    )]
    pub source_chain: Account<'info, SourceChain>,
}
