use anchor_lang::prelude::*;

pub use ccip_common::state::CodeVersion;
use ccip_common::types::{CrossChainGas, TimestampedPackedU224, UsdCents};

#[account]
#[derive(InitSpace, Debug)]
pub struct Config {
    pub version: u8,

    pub owner: Pubkey,
    pub proposed_owner: Pubkey,

    // Static config fields
    //
    // Maximum fee that can be charged for a message.
    // This value is stored with 18 decimals of precision (LINK_JUEL_DECIMALS).
    // 1 LINK = 1,000,000,000,000,000,000 juels (1e18).
    pub max_fee_juels_per_msg: u128,
    pub link_token_mint: Pubkey,
    // local LINK mint may not use 18 decimals as EVM, so we store local decimals
    // to calculate the transfer fees in juels later.
    pub link_token_local_decimals: u8,
    // NOTE: EVM's equivalent config carries a `token_price_staleness_threshold`; there is no
    // Solana counterpart because data feeds are not supported here, so a token price is only
    // checked for existence rather than freshness (see `get_validated_token_price`).
    pub onramp: Pubkey,

    pub default_code_version: CodeVersion,
}

#[derive(Clone, AnchorSerialize, AnchorDeserialize, InitSpace, Debug)]
pub struct DestChainConfig {
    pub is_enabled: bool, // Whether this destination chain is enabled

    // The code version of the lane, in case we need to shift traffic to new logic for a single lane to test an upgrade
    pub lane_code_version: CodeVersion,

    pub max_number_of_tokens_per_msg: u16, // Maximum number of distinct ERC20 token transferred per message
    pub max_data_bytes: u32,               // Maximum payload data size in bytes
    pub max_per_msg_gas_limit: CrossChainGas, // Maximum gas limit for messages targeting EVMs
    pub dest_gas_overhead: CrossChainGas, //  Gas charged on top of the gasLimit to cover destination chain costs
    pub dest_gas_per_payload_byte_base: CrossChainGas, // Base gas cost per byte up to threshold
    pub dest_gas_per_payload_byte_high: CrossChainGas, // Higher gas cost per byte after threshold
    pub dest_gas_per_payload_byte_threshold: u32, // Threshold in bytes for higher gas cost
    pub dest_data_availability_overhead_gas: CrossChainGas, // Extra data availability gas charged on top of the message, e.g. for OCR
    pub dest_gas_per_data_availability_byte: u16, // Amount of gas to charge per byte of message data that needs availability
    pub dest_data_availability_multiplier_bps: u16, // Multiplier for data availability gas, multiples of bps, or 0.0001

    // The following three properties are defaults, they can be overridden by setting the TokenTransferFeeConfig for a token
    pub default_token_fee_usdcents: u16, // Default token fee charged per token transfer. u16 for historical reasons (different layout than UsdCents)
    pub default_token_dest_gas_overhead: CrossChainGas, //  Default gas charged to execute the token transfer on the destination chain
    pub default_tx_gas_limit: CrossChainGas,            // Default gas limit for a tx
    pub gas_multiplier_wei_per_eth: u64, // Multiplier for gas costs, 1e18 based so 11e17 = 10% extra cost.
    pub network_fee: UsdCents, // Flat network fee to charge for messages, multiples of 0.01 USD
    pub gas_price_staleness_threshold: u32, // The amount of time a gas price can be stale before it is considered invalid (0 means disabled)
    pub enforce_out_of_order: bool, // Whether to enforce the allowOutOfOrderExecution extraArg value to be true.
    pub chain_family_selector: [u8; 4], // Selector that identifies the destination chain's family. Used to determine the correct validations to perform for the dest chain.
}

#[derive(Clone, AnchorSerialize, AnchorDeserialize, InitSpace, Debug)]
pub struct DestChainState {
    pub usd_per_unit_gas: TimestampedPackedU224,
}

// PDA seeds: [DEST_CHAIN, chain_selector]
#[account]
#[derive(InitSpace, Debug)]
pub struct DestChain {
    // Config for SVM2Any
    pub version: u8,
    pub chain_selector: u64,     // Chain selector used for the seed
    pub state: DestChainState,   // values that are updated automatically
    pub config: DestChainConfig, // values configured by an admin
}

#[derive(InitSpace, Clone, AnchorSerialize, AnchorDeserialize, Debug)]
pub struct BillingTokenConfig {
    // NOTE: when modifying this struct, make sure to update the version in the wrapper
    pub enabled: bool,
    pub mint: Pubkey,

    // price tracking
    pub usd_per_token: TimestampedPackedU224,
    // billing configs
    pub premium_multiplier_wei_per_eth: u64,
}

// PDA seeds: [FEE_BILLING_TOKEN_CONFIG, mint] — mint stored in config.mint
#[account]
#[derive(InitSpace, Debug)]
pub struct BillingTokenConfigWrapper {
    pub version: u8,
    pub config: BillingTokenConfig,
}

// PDA seeds: [PER_CHAIN_PER_TOKEN_CONFIG, chain_selector, mint]
#[account]
#[derive(InitSpace, Debug)]
pub struct PerChainPerTokenConfig {
    pub version: u8,                                   // schema version
    pub chain_selector: u64,                           // remote chain
    pub mint: Pubkey,                                  // token on solana
    pub token_transfer_config: TokenTransferFeeConfig, // EVM: configurable in router only by ccip admins
}

#[derive(InitSpace, Debug, Clone, AnchorSerialize, AnchorDeserialize)]
pub struct TokenTransferFeeConfig {
    pub min_fee: UsdCents, // Minimum fee to charge per token transfer, multiples of 0.01 USD
    pub max_fee: UsdCents, // Maximum fee to charge per token transfer, multiples of 0.01 USD
    pub deci_bps: u16,     // Basis points charged on token transfers, multiples of 0.1bps, or 1e-5
    pub dest_gas_overhead: CrossChainGas, // Gas charged to execute the token transfer on the destination chain
    // Extra data availability bytes that are returned from the source pool and sent
    pub dest_bytes_overhead: u32, // to the destination pool. Must be >= Pool.CCIP_LOCK_OR_BURN_V1_RET_BYTES
    pub is_enabled: bool,         // Whether this token has custom transfer fees
}

// PDA seeds: [ALLOWED_PRICE_UPDATER, price_updater] — marker account, existence is the data
#[account]
#[derive(Copy, Debug, InitSpace)]
pub struct AllowedPriceUpdater {}
