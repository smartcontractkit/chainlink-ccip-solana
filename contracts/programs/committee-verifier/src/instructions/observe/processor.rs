use anchor_lang::prelude::*;

use ccip_common::messages::FinalityConfig;
use ccip_common::observe::OBSERVATION_VERSION;
use ccip_common::seed;
use ccip_common::types::{CrossChainGas, UsdCents};

use crate::state::{valid_version, CodeVersion, Config, RemoteChainConfig, MAX_CONFIG_V};
use crate::CommitteeVerifierError;

/// Global configuration of this CCV, as observed by off-chain consumers.
///
/// Storage locations are not included here: they are unbounded in size and already exposed by
/// `get_storage_locations`.
#[derive(AnchorSerialize, AnchorDeserialize, Clone, Debug)]
pub struct CommitteeVerifierConfigObservation {
    pub observation_version: u16,
    pub type_version: String,

    pub version: u8,
    pub default_code_version: CodeVersion,
    pub owner: Pubkey,
    pub proposed_owner: Pubkey,
    pub storage_location_admin: Pubkey,
    pub proposed_storage_location_admin: Pubkey,
    pub allowlist_admin: Pubkey,
    pub fee_aggregator: Pubkey,
    pub allowed_finality_config: FinalityConfig,
}

/// Per-source-chain configuration of this CCV, as observed by off-chain consumers.
///
/// The allow list itself is reported as a count only, as its size is unbounded by configuration.
#[derive(AnchorSerialize, AnchorDeserialize, Clone, Debug)]
pub struct CommitteeVerifierRemoteChainObservation {
    pub observation_version: u16,
    pub type_version: String,

    pub version: u8,
    pub remote_chain_selector: u64,

    // admin config
    pub router_signer: Pubkey,
    pub fee: UsdCents,
    pub gas_for_verification: CrossChainGas,
    pub payload_size_bytes: u32,

    // allowlist config
    pub allowlist_enabled: bool,
    pub allowed_senders_count: u32,

    // signer config: `signer_threshold` is `None` when no signer set is configured for this lane.
    pub signer_threshold: Option<u8>,
    pub signers: Vec<crate::state::Signer>,
}

pub fn observe_config(ctx: Context<ObserveConfig>) -> Result<CommitteeVerifierConfigObservation> {
    let config = &ctx.accounts.config;

    Ok(CommitteeVerifierConfigObservation {
        observation_version: OBSERVATION_VERSION,
        type_version: env!("CCIP_BUILD_TYPE_VERSION").to_string(),

        version: config.version,
        default_code_version: config.default_code_version,
        owner: config.owner,
        proposed_owner: config.proposed_owner,
        storage_location_admin: config.storage_location_admin,
        proposed_storage_location_admin: config.proposed_storage_location_admin,
        allowlist_admin: config.allowlist_admin,
        fee_aggregator: config.fee_aggregator,
        allowed_finality_config: config.allowed_finality_config,
    })
}

pub fn observe_remote_chain(
    ctx: Context<ObserveRemoteChain>,
    _remote_chain_selector: u64,
) -> Result<CommitteeVerifierRemoteChainObservation> {
    let remote_chain_config = &ctx.accounts.remote_chain_config;

    Ok(CommitteeVerifierRemoteChainObservation {
        observation_version: OBSERVATION_VERSION,
        type_version: env!("CCIP_BUILD_TYPE_VERSION").to_string(),

        version: remote_chain_config.version,
        remote_chain_selector: remote_chain_config.remote_chain_selector,

        router_signer: remote_chain_config.admin_config.router_signer,
        fee: remote_chain_config.admin_config.fee,
        gas_for_verification: remote_chain_config.admin_config.gas_for_verification,
        payload_size_bytes: remote_chain_config.admin_config.payload_size_bytes,

        allowlist_enabled: remote_chain_config.allowlist_config.allowlist_enabled,
        allowed_senders_count: remote_chain_config
            .allowlist_config
            .allowed_senders
            .len()
            .try_into()
            .unwrap_or(u32::MAX),

        signer_threshold: remote_chain_config.signer_config.threshold(),
        signers: remote_chain_config.signer_config.signers().to_vec(),
    })
}

#[derive(Accounts)]
pub struct ObserveConfig<'info> {
    #[account(
        seeds = [seed::CONFIG],
        bump = config.bump,
        constraint = valid_version(config.version, MAX_CONFIG_V) @ CommitteeVerifierError::InvalidVersion,
    )]
    pub config: Account<'info, Config>,
}

#[derive(Accounts)]
#[instruction(remote_chain_selector: u64)]
pub struct ObserveRemoteChain<'info> {
    #[account(
        seeds = [seed::REMOTE_CHAIN, &remote_chain_selector.to_le_bytes()],
        bump = remote_chain_config.bump,
        constraint = valid_version(remote_chain_config.version, MAX_CONFIG_V) @ CommitteeVerifierError::InvalidVersion,
    )]
    pub remote_chain_config: Account<'info, RemoteChainConfig>,
}
