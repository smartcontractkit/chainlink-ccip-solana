use anchor_lang::prelude::*;
use ccip_common::messages::FinalityConfig;
use ccip_common::types::{ByteString, CrossChainGas, UsdCents};

use crate::state::{LombardChain, LombardVerifierPath, RemoteAdapter, SupportedToken};

#[event]
pub struct RemoteChainLombardConfigChanged {
    pub config: LombardChain,
}

#[event]
pub struct CcipLombardMessageSentEvent {
    // Seeds for the CCTP message sent event account
    pub original_sender: Pubkey,
    pub remote_chain_selector: u64,
    pub msg_total_nonce: u64,

    // Actual event account address, derived from the seeds above
    pub event_address: Pubkey,

    // CCTP values identifying the message
    pub source_domain: u32, // The source chain domain ID, which for Solana is always 5
    pub bridge_nonce: u64,

    // CCTP message bytes, used to get the attestation offchain and receive the message on dest
    pub message_sent_bytes: Vec<u8>,
}

#[event]
pub struct CcipLombardMessageEventAccountClosed {
    // Seeds for the CCTP message sent event account
    original_sender: Pubkey,
    remote_chain_selector: u64,
    msg_total_nonce: u64,
    // Actual event account address, derived from the seeds above
    address: Pubkey,
}

#[event]
pub struct AltEnabled {
    pub enabled: bool,
}

// --- CCIP 2.0 / verifier additions ---

#[event]
pub struct VerifierOwnershipTransferRequested {
    pub from: Pubkey,
    pub to: Pubkey,
}

#[event]
pub struct VerifierOwnershipTransferred {
    pub from: Pubkey,
    pub to: Pubkey,
}

#[event]
pub struct VerifierStorageLocationAdminTransferRequested {
    pub from: Pubkey,
    pub to: Pubkey,
}

#[event]
pub struct VerifierStorageLocationAdminTransferred {
    pub from: Pubkey,
    pub to: Pubkey,
}

#[event]
pub struct VerifierAllowlistAdminSet {
    pub old: Pubkey,
    pub new: Pubkey,
}

#[event]
pub struct VerifierFeeAggregatorSet {
    pub old: Pubkey,
    pub new: Pubkey,
}

#[event]
pub struct VerifierLookupTableSet {
    pub remote_chain_selector: u64,
    pub old: Option<Pubkey>,
    pub new: Option<Pubkey>,
}

#[event]
pub struct VerifierAllowedFinalityConfigSet {
    pub old: FinalityConfig,
    pub new: FinalityConfig,
}

#[event]
pub struct VerifierChainConfigSet {
    pub remote_chain_selector: u64,
    pub router_signer: Pubkey,
    pub fee: UsdCents,
    pub gas_for_verification: CrossChainGas,
    pub payload_size_bytes: u32,
}

#[event]
pub struct VerifierAllowlistSet {
    pub dest_chain_selector: u64,
    pub enabled: bool,
    pub allowed_senders: Vec<Pubkey>,
}

#[event]
pub struct VerifierStorageLocationsSet {
    pub locations: Vec<ByteString>,
}

#[event]
pub struct VerifierPathSet {
    pub remote_chain_selector: u64,
    pub path: LombardVerifierPath,
}

#[event]
pub struct VerifierPathRemoved {
    pub remote_chain_selector: u64,
    pub path: LombardVerifierPath,
}

#[event]
pub struct VerifierSupportedTokensUpdated {
    pub tokens_removed: Vec<Pubkey>,
    pub tokens_set: Vec<SupportedToken>,
}

#[event]
pub struct VerifierRemoteAdaptersSet {
    pub remote_chain_selector: u64,
    pub remote_adapters: Vec<RemoteAdapter>,
}
