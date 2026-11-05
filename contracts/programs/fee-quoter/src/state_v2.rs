use anchor_lang::prelude::*;
use ccip_common::types::{CrossChainGas, TimestampedPackedU224, UsdCents};

use crate::state::CodeVersion;

#[account]
#[derive(InitSpace, Debug)]
pub struct DestChainV2 {
    // Config for SVM2Any
    pub version: u8,
    pub bump: u8,
    pub chain_selector: u64,       // Chain selector used for the seed
    pub state: DestChainStateV2,   // values that are updated automatically
    pub config: DestChainConfigV2, // values configured by an admin
}

#[derive(Clone, AnchorSerialize, AnchorDeserialize, InitSpace, Debug)]
pub struct DestChainStateV2 {
    // Currently unused until the price updater v2 is developed. The field is left here
    // for future PDA compatibility. Instead of this value, the fee quoter retrieves the
    // usd_per_unit_gas from 1.6 DestChain state for now, as it's updated by the 1.6 offramp.
    pub _usd_per_unit_gas: TimestampedPackedU224,
}

#[derive(Clone, AnchorSerialize, AnchorDeserialize, InitSpace, Debug)]
pub struct DestChainConfigV2 {
    pub is_enabled: bool, // Whether this destination chain is enabled

    // The code version of the lane, in case we need to shift traffic to new logic for a single lane to test an upgrade
    pub lane_code_version: CodeVersion,

    pub max_data_bytes: u32, // Maximum payload data size in bytes
    pub max_per_msg_gas_limit: CrossChainGas, // Maximum gas limit for messages targeting EVMs
    pub dest_gas_overhead: CrossChainGas, // LEGACY: Gas charged on top of the gasLimit to cover destination chain costs
    pub dest_gas_per_payload_byte_base: u32, // Base gas cost per byte up to threshold
    pub chain_family_selector: [u8; 4], // Selector that identifies the destination chain's family. Used to determine the correct validations to perform for the dest chain.

    // The following three properties are defaults, they can be overridden by setting the TokenTransferFeeConfig for a token
    pub default_token_fee_usd_cents: u16, // Default token fee charged per token transfer. U16 for legacy reasons.
    pub default_token_dest_gas_overhead: CrossChainGas, //  Default gas charged to execute the token transfer on the destination chain
    pub default_tx_gas_limit: CrossChainGas,            // Default gas limit for a tx
    pub network_fee: UsdCents, // LEGACY: Flat network fee to charge for messages, multiples of 0.01 USD
    pub link_fee_multiplier_percent: u8, // Percentage multiplier to apply when fee is paid in LINK. 90 = 10% discount
}
