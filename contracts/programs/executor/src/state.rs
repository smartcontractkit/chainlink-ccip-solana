use anchor_lang::prelude::{pubkey::PUBKEY_BYTES, *};

use ccip_common::messages::FinalityConfig;
pub use ccip_common::types::UsdCents;

pub const ANCHOR_DISCRIMINATOR: usize = 8;

pub const MAX_CONFIG_V: u8 = 1;
pub const MAX_CHAIN_CONFIG_V: u8 = 1;

/// Capacity of [`Config::allowed_ccvs`]. The config account is allocated once at `initialize` with
/// room for exactly this many entries and is never realloc'd, so the allowlist instructions reject
/// anything larger up front instead of failing opaquely when the account is serialized back.
pub const MAX_ALLOWED_CCVS: usize = 64;

pub const fn valid_version(v: u8, max_version: u8) -> bool {
    !uninitialized(v) && v <= max_version
}

pub const fn uninitialized(v: u8) -> bool {
    v == 0
}

#[derive(InitSpace)]
#[account]
pub struct Config {
    pub version: u8,
    pub bump: u8,
    pub max_ccvs_per_msg: u8,
    pub owner: Pubkey,
    pub proposed_owner: Pubkey,
    pub fee_aggregator: Pubkey,
    pub allowed_finality_config: FinalityConfig,
    pub ccv_allowlist_enabled: bool,
    #[max_len(MAX_ALLOWED_CCVS)]
    pub allowed_ccvs: Vec<Pubkey>,
}

impl Config {
    pub fn dynamic_len(allowed_ccvs_len: usize) -> usize {
        Self::INIT_SPACE + allowed_ccvs_len * PUBKEY_BYTES
    }
}

// PDA seeds: [REMOTE_CHAIN, chain_selector] — `chain_selector` is stored so the account is
// self-describing: it can be checked against the PDA it was derived from without consulting
// the caller's parameters.
#[derive(InitSpace)]
#[account]
pub struct ChainConfig {
    pub version: u8,
    pub bump: u8,
    pub chain_selector: u64,
    pub usd_cents_fee: UsdCents,
    pub enabled: bool,
}
