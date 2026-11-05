use anchor_lang::prelude::*;
use ccip_common::messages::FinalityConfig;

use crate::state::UsdCents;

#[event]
pub struct OwnershipTransferRequested {
    pub from: Pubkey,
    pub to: Pubkey,
}

#[event]
pub struct OwnershipTransferred {
    pub from: Pubkey,
    pub to: Pubkey,
}

#[event]
pub struct ConfigSet {
    pub fee_aggregator: Pubkey,
    pub allowed_finality_config: FinalityConfig,
    pub ccv_allowlist_enabled: bool,
}

#[event]
pub struct ChainConfigSet {
    pub remote_chain_selector: u64,
    pub usd_cents_fee: UsdCents,
    pub enabled: bool,
}

/// Mirrors EVM's `Executor.CCVAdded(address indexed ccv)`.
#[event]
pub struct CCVAdded {
    pub ccv: Pubkey,
}

/// Mirrors EVM's `Executor.CCVRemoved(address indexed ccv)`.
#[event]
pub struct CCVRemoved {
    pub ccv: Pubkey,
}

/// Mirrors EVM's `Executor.CCVAllowlistUpdated(bool enabled)`. Like EVM, only emitted when the
/// flag actually changes.
#[event]
pub struct CCVAllowlistUpdated {
    pub enabled: bool,
}
