use anchor_lang::prelude::*;

use ccip_common::{
    messages::FinalityConfig,
    types::{ByteString, CrossChainGas, UsdCents},
};

use crate::state::CodeVersion;

#[event]
pub struct AllowlistSet {
    pub dest_chain_selector: u64,
    pub enabled: bool,
    pub allowed_senders: Vec<Pubkey>,
}

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
pub struct DefaultCodeVersionSet {
    pub old: CodeVersion,
    pub new: CodeVersion,
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
pub struct StorageLocationAdminTransferRequested {
    pub from: Pubkey,
    pub to: Pubkey,
}

#[event]
pub struct StorageLocationAdminTransferred {
    pub from: Pubkey,
    pub to: Pubkey,
}

#[event]
pub struct ChainConfigSet {
    pub remote_chain_selector: u64,
    pub router: Pubkey,
    pub fee: UsdCents,
    pub gas_for_verification: CrossChainGas,
    pub payload_size_bytes: u32,
}

#[event]
pub struct StorageLocationsSet {
    pub locations: Vec<ByteString>,
}

#[event]
pub struct SignerConfigSet {
    pub source_chain_selector: u64,
    pub signers: Vec<[u8; 20]>,
    pub threshold: u8,
}
