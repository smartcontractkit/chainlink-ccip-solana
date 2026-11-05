// 20-byte EVM-style address for committee signers.
pub type Signer = [u8; 20];

use anchor_lang::prelude::*;

pub use ccip_common::state::CodeVersion;
use ccip_common::{
    messages::FinalityConfig,
    types::{ByteString, CrossChainGas, UsdCents},
};

use crate::error::CommitteeVerifierError;

pub const ANCHOR_DISCRIMINATOR: usize = 8;

pub const MAX_CONFIG_V: u8 = 1;
pub const MAX_STORAGE_LOCATIONS_V: u8 = 1;

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
    pub default_code_version: CodeVersion,

    // Both of these default to initializer, and need to be
    // set by a two-stage proposal/acceptance process.
    pub owner: Pubkey,
    pub storage_location_admin: Pubkey,

    // These are managed by `owner` and can be set authoritatively
    // with no proposal.
    pub allowlist_admin: Pubkey,
    pub fee_aggregator: Pubkey,

    pub proposed_owner: Pubkey,
    pub proposed_storage_location_admin: Pubkey,
    pub allowed_finality_config: FinalityConfig,
}

#[derive(InitSpace)]
#[account]
pub struct StorageLocations {
    pub version: u8,
    pub bump: u8,
    /// Opaque location identifiers — URLs in practice, but the program only stores and echoes them,
    /// so they are held as [`ByteString`] rather than `String` to avoid paying for UTF-8 validation
    /// on every load. Borsh-identical to `Vec<String>`, so this needs no account migration.
    #[max_len(0)]
    pub locations: Vec<ByteString>,
}

impl StorageLocations {
    /// Compute the dynamic Borsh-serialized byte length for a set of storage locations.
    /// Each entry contributes 4 bytes (Borsh length prefix) + its byte length; `INIT_SPACE`
    /// already accounts for the outer vec's own prefix.
    pub fn dynamic_space(locations: &[ByteString]) -> usize {
        locations.iter().map(|s| 4 + s.payload_len()).sum()
    }
}

#[derive(AnchorSerialize, AnchorDeserialize, InitSpace, Debug, Clone)]
pub struct RemoteChainAdminConfig {
    /// PDA of the router allowed to sign the start of a verification process.
    pub router_signer: Pubkey,
    pub fee: UsdCents,
    pub gas_for_verification: CrossChainGas,
    pub payload_size_bytes: u32,
}

#[derive(AnchorSerialize, AnchorDeserialize, InitSpace, Debug, Clone)]
pub struct RemoteChainAllowlistConfig {
    pub allowlist_enabled: bool,

    #[max_len(0)]
    pub allowed_senders: Vec<Pubkey>,
}

#[derive(AnchorSerialize, AnchorDeserialize, InitSpace, Debug, Clone)]
pub struct RemoteChainSignerConfig {
    threshold: u8,

    #[max_len(0)]
    signers: Vec<[u8; 20]>,
}

impl RemoteChainSignerConfig {
    pub fn threshold(&self) -> Option<u8> {
        self.is_enabled().then_some(self.threshold)
    }

    pub fn signers(&self) -> &[[u8; 20]] {
        &self.signers
    }

    pub fn is_enabled(&self) -> bool {
        self.threshold > 0
    }

    pub fn new_disabled() -> Self {
        Self {
            threshold: 0,
            signers: vec![],
        }
    }

    pub fn new(threshold: u8, signers: Vec<[u8; 20]>) -> Result<Self> {
        require_gt!(threshold, 0, CommitteeVerifierError::InvalidSignerThreshold);
        require_gte!(
            signers.len(),
            threshold as usize,
            CommitteeVerifierError::InvalidSignerThreshold
        );

        Ok(Self { threshold, signers })
    }
}

// PDA seeds: [REMOTE_CHAIN, chain_selector]
#[derive(InitSpace, Debug)]
#[account]
pub struct RemoteChainConfig {
    pub version: u8,
    pub bump: u8,
    pub remote_chain_selector: u64,
    pub admin_config: RemoteChainAdminConfig,
    pub allowlist_config: RemoteChainAllowlistConfig,
    pub signer_config: RemoteChainSignerConfig,
}

impl RemoteChainConfig {
    pub fn dynamic_space(senders: &[Pubkey], signers: &[Signer]) -> usize {
        std::mem::size_of_val(senders) + std::mem::size_of_val(signers)
    }
}
