use anchor_lang::prelude::*;

use ccip_common::messages::FinalityConfig;

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
pub struct AllowlistAdminSet {
    pub old: Pubkey,
    pub new: Pubkey,
}

#[event]
pub struct FeeAggregatorSet {
    pub old: Pubkey,
    pub new: Pubkey,
}

#[event]
pub struct AllowedFinalityConfigSet {
    pub old: FinalityConfig,
    pub new: FinalityConfig,
}

#[event]
pub struct FastFinalityBpsSet {
    pub old: u16,
    pub new: u16,
}

#[event]
pub struct StorageLocationAdminTransferRequested {
    pub from: Pubkey,
    pub to: Pubkey,
}

#[event]
pub struct StorageLocationAdminTransferred {
    pub from: Pubkey,
    pub to: Pubkey,
}
