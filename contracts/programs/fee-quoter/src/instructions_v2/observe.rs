use anchor_lang::prelude::*;

use ccip_common::observe::OBSERVATION_VERSION;
use ccip_common::seed;
use ccip_common::types::{CrossChainGas, TimestampedPackedU224, UsdCents};

use crate::context::{valid_version, MAX_CHAINSTATE_V as MAX_CHAINSTATE_V_LEGACY};
use crate::instructions_v2::{V2_MAX_CHAINSTATE_V, V2_MAX_CONFIG_V};
use crate::state::{CodeVersion, Config, DestChain};
use crate::state_v2::DestChainV2;
use crate::FeeQuoterError;

/// Global configuration of the fee quoter, as observed by off-chain consumers.
#[derive(AnchorSerialize, AnchorDeserialize, Clone, Debug)]
pub struct FeeQuoterConfigObservation {
    pub observation_version: u16,
    pub type_version: String,

    pub version: u8,
    pub default_code_version: CodeVersion,
    pub owner: Pubkey,
    pub proposed_owner: Pubkey,
    /// Maximum fee per message, with 18 decimals of precision (LINK juels).
    pub max_fee_juels_per_msg: u128,
    pub link_token_mint: Pubkey,
    pub link_token_local_decimals: u8,
    pub onramp: Pubkey,
}

/// Per-destination-chain CCIP 2.0 fee configuration and price state, as observed by off-chain
/// consumers. Read from the `dest_chain_v2` PDA.
///
/// Note that unlike the 1.6 `dest_chain` config, this one carries no gas price staleness
/// threshold: there is no 2.0 equivalent to report.
#[derive(AnchorSerialize, AnchorDeserialize, Clone, Debug)]
pub struct FeeQuoterDestChainV2Observation {
    pub observation_version: u16,
    pub type_version: String,

    pub version: u8,
    pub chain_selector: u64,

    // automatically updated state
    pub usd_per_unit_gas: TimestampedPackedU224,

    // admin-configured values
    pub is_enabled: bool,
    pub lane_code_version: CodeVersion,
    pub max_data_bytes: u32,
    pub max_per_msg_gas_limit: CrossChainGas,
    pub dest_gas_overhead: CrossChainGas,
    pub dest_gas_per_payload_byte_base: u32,
    pub chain_family_selector: [u8; 4],
    pub default_token_fee_usd_cents: u16,
    pub default_token_dest_gas_overhead: CrossChainGas,
    pub default_tx_gas_limit: CrossChainGas,
    pub network_fee: UsdCents,
    pub link_fee_multiplier_percent: u8,
}

pub fn observe_config(ctx: Context<ObserveConfig>) -> Result<FeeQuoterConfigObservation> {
    let config = &ctx.accounts.config;

    Ok(FeeQuoterConfigObservation {
        observation_version: OBSERVATION_VERSION,
        type_version: env!("CCIP_BUILD_TYPE_VERSION").to_string(),

        version: config.version,
        default_code_version: config.default_code_version,
        owner: config.owner,
        proposed_owner: config.proposed_owner,
        max_fee_juels_per_msg: config.max_fee_juels_per_msg,
        link_token_mint: config.link_token_mint,
        link_token_local_decimals: config.link_token_local_decimals,
        onramp: config.onramp,
    })
}

pub fn observe_dest_chain_v2(
    ctx: Context<ObserveDestChainV2>,
    _chain_selector: u64,
) -> Result<FeeQuoterDestChainV2Observation> {
    let dest_chain = &ctx.accounts.dest_chain;
    let dest_chain_legacy = &ctx.accounts.dest_chain_legacy;

    Ok(FeeQuoterDestChainV2Observation {
        observation_version: OBSERVATION_VERSION,
        type_version: env!("CCIP_BUILD_TYPE_VERSION").to_string(),

        version: dest_chain.version,
        chain_selector: dest_chain.chain_selector,

        // NOTE: This is NOT coming from the v2 PDA but the legacy one: we retrieve
        // usd gas prices from 1.6 state until the 2.0 price updater exists.
        usd_per_unit_gas: dest_chain_legacy.state.usd_per_unit_gas.clone(),

        is_enabled: dest_chain.config.is_enabled,
        lane_code_version: dest_chain.config.lane_code_version,
        max_data_bytes: dest_chain.config.max_data_bytes,
        max_per_msg_gas_limit: dest_chain.config.max_per_msg_gas_limit,
        dest_gas_overhead: dest_chain.config.dest_gas_overhead,
        dest_gas_per_payload_byte_base: dest_chain.config.dest_gas_per_payload_byte_base,
        chain_family_selector: dest_chain.config.chain_family_selector,
        default_token_fee_usd_cents: dest_chain.config.default_token_fee_usd_cents,
        default_token_dest_gas_overhead: dest_chain.config.default_token_dest_gas_overhead,
        default_tx_gas_limit: dest_chain.config.default_tx_gas_limit,
        network_fee: dest_chain.config.network_fee,
        link_fee_multiplier_percent: dest_chain.config.link_fee_multiplier_percent,
    })
}

#[derive(Accounts)]
pub struct ObserveConfig<'info> {
    #[account(
        seeds = [seed::CONFIG],
        bump,
        constraint = valid_version(config.version, V2_MAX_CONFIG_V) @ FeeQuoterError::InvalidVersion,
    )]
    pub config: Account<'info, Config>,
}

#[derive(Accounts)]
#[instruction(chain_selector: u64)]
pub struct ObserveDestChainV2<'info> {
    #[account(
        seeds = [seed::DEST_CHAIN_V2, chain_selector.to_le_bytes().as_ref()],
        bump = dest_chain.bump,
        constraint = valid_version(dest_chain.version, V2_MAX_CHAINSTATE_V) @ FeeQuoterError::InvalidVersion,
    )]
    pub dest_chain: Account<'info, DestChainV2>,

    #[account(
        seeds = [seed::DEST_CHAIN, chain_selector.to_le_bytes().as_ref()],
        bump,
        constraint = valid_version(dest_chain_legacy.version, MAX_CHAINSTATE_V_LEGACY) @ FeeQuoterError::InvalidVersion,
    )]
    pub dest_chain_legacy: Account<'info, DestChain>,
}
