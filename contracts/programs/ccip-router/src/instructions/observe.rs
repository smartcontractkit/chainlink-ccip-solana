use anchor_lang::prelude::*;

use ccip_common::observe::OBSERVATION_VERSION;
use ccip_common::seed;
use ccip_common::types::{CrossChainGas, UsdCents};

use crate::context::{valid_version, MAX_CHAINSTATECCIPV2_V, MAX_CONFIG_V};
use crate::error::CcipRouterError;
use crate::state::{CodeVersion, Config, DestChainCcipV2, RestoreOnAction};

/// Global configuration of the router, as observed by off-chain consumers.
#[derive(AnchorSerialize, AnchorDeserialize, Clone, Debug)]
pub struct RouterConfigObservation {
    pub observation_version: u16,
    pub type_version: String,

    pub version: u8,
    pub default_code_version: CodeVersion,
    pub svm_chain_selector: u64,
    pub owner: Pubkey,
    pub proposed_owner: Pubkey,
    pub fee_quoter: Pubkey,
    pub rmn_remote: Pubkey,
    pub link_token_mint: Pubkey,
    pub fee_aggregator: Pubkey,
}

/// Per-destination-chain state and configuration of the CCIP 2.0 onramp path, as observed by
/// off-chain consumers. Read from the `dest_chain_state_v2` PDA.
///
/// The list of allowed senders is reported as a count only, as its size is unbounded by
/// configuration.
#[derive(AnchorSerialize, AnchorDeserialize, Clone, Debug)]
pub struct RouterDestChainV2Observation {
    pub observation_version: u16,
    pub type_version: String,

    pub version: u8,
    pub chain_selector: u64,

    // automatically updated state
    pub message_number: u64,
    pub message_number_to_restore: u64,
    pub restore_on_action: RestoreOnAction,

    // admin-configured values
    pub lane_code_version: CodeVersion,
    pub allow_list_enabled: bool,
    pub allowed_senders_count: u32,
    pub default_ccvs: Vec<Pubkey>,
    pub lane_mandated_ccvs: Vec<Pubkey>,
    pub default_executor: Pubkey,
    /// Destination offramp address as raw bytes (not ABI encoded).
    pub offramp: Vec<u8>,
    pub message_network_fee: UsdCents,
    pub token_transfer_network_fee: UsdCents,
    pub base_execution_gas_cost: CrossChainGas,
}

pub fn observe_config(ctx: Context<ObserveConfig>) -> Result<RouterConfigObservation> {
    let config = &ctx.accounts.config;

    Ok(RouterConfigObservation {
        observation_version: OBSERVATION_VERSION,
        type_version: env!("CCIP_BUILD_TYPE_VERSION").to_string(),

        version: config.version,
        default_code_version: config.default_code_version,
        svm_chain_selector: config.svm_chain_selector,
        owner: config.owner,
        proposed_owner: config.proposed_owner,
        fee_quoter: config.fee_quoter,
        rmn_remote: config.rmn_remote,
        link_token_mint: config.link_token_mint,
        fee_aggregator: config.fee_aggregator,
    })
}

pub fn observe_dest_chain_v2(
    ctx: Context<ObserveDestChainV2>,
    _dest_chain_selector: u64,
) -> Result<RouterDestChainV2Observation> {
    let dest_chain = &ctx.accounts.dest_chain_state;

    Ok(RouterDestChainV2Observation {
        observation_version: OBSERVATION_VERSION,
        type_version: env!("CCIP_BUILD_TYPE_VERSION").to_string(),

        version: dest_chain.version,
        chain_selector: dest_chain.chain_selector,

        message_number: dest_chain.state.message_number,
        message_number_to_restore: dest_chain.state.message_number_to_restore,
        restore_on_action: dest_chain.state.restore_on_action,

        lane_code_version: dest_chain.config.lane_code_version,
        allow_list_enabled: dest_chain.config.allow_list_enabled,
        allowed_senders_count: dest_chain
            .config
            .allowed_senders
            .len()
            .try_into()
            .unwrap_or(u32::MAX),
        default_ccvs: dest_chain.config.default_ccvs.clone(),
        lane_mandated_ccvs: dest_chain.config.lane_mandated_ccvs.clone(),
        default_executor: dest_chain.config.default_executor,
        offramp: dest_chain.config.offramp.clone(),
        message_network_fee: dest_chain.config.message_network_fee,
        token_transfer_network_fee: dest_chain.config.token_transfer_network_fee,
        base_execution_gas_cost: dest_chain.config.base_execution_gas_cost,
    })
}

#[derive(Accounts)]
pub struct ObserveConfig<'info> {
    #[account(
        seeds = [seed::CONFIG],
        bump,
        constraint = valid_version(config.version, MAX_CONFIG_V) @ CcipRouterError::InvalidVersion,
    )]
    pub config: Account<'info, Config>,
}

#[derive(Accounts)]
#[instruction(dest_chain_selector: u64)]
pub struct ObserveDestChainV2<'info> {
    #[account(
        seeds = [seed::DEST_CHAIN_STATE_V2, dest_chain_selector.to_le_bytes().as_ref()],
        bump = dest_chain_state.bump,
        constraint = valid_version(dest_chain_state.version, MAX_CHAINSTATECCIPV2_V) @ CcipRouterError::InvalidVersion,
    )]
    pub dest_chain_state: Account<'info, DestChainCcipV2>,
}
