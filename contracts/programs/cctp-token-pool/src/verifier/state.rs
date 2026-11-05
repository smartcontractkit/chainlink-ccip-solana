use anchor_lang::prelude::*;

use base_token_pool::state::RemoteAddress;
use ccip_common::messages::FinalityConfig;
pub use ccip_common::state::AccountSchemaVersion;
use ccip_common::types::{ByteString, CrossChainGas, UsdCents};

// A separate module with verifier-specific seeds. These are not shared with the common library for this verifier,
// as this one is a special case that shares the same program with the token pool, so we need seeds to be unique
// across the pool & verifier (e.g. because "Config" means a different PDA in each case).
pub mod verifier_seed {
    pub const CCTPV2_NONCE: &[u8] = b"ccip_cctpv2_nonce";
    pub const CCTPV2_EVENT: &[u8] = b"ccip_cctpv2_event";

    pub const REMOTE_CHAIN: &[u8] = b"verifier_remote_chain";

    pub use ccip_common::seed::{CONFIG, FEE_VAULT, POOL_SIGNER_SEED, STORAGE_LOCATIONS};
}

/// The division factor for basis points. This also represents the maximum bps fee.
pub const BPS_DIVIDER: u16 = 10_000;

#[derive(InitSpace)]
#[account]
pub struct Config {
    pub version: u8,
    pub bump: u8,

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

    // Basis points charged for fast finality when Solana is source. The value is independent of the destination,
    // and the fees are deducted on the destination side (based on the source).
    // https://developers.circle.com/cctp/concepts/fees#fee-tables
    pub fast_finality_bps: u16,
}

impl AccountSchemaVersion for Config {
    const MAX_V: u8 = 1;

    fn version(&self) -> u8 {
        self.version
    }
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

impl AccountSchemaVersion for StorageLocations {
    const MAX_V: u8 = 1;

    fn version(&self) -> u8 {
        self.version
    }
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
    /// The address of the Solana router program
    pub router: Pubkey,
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

// PDA seeds: [REMOTE_CHAIN, chain_selector]
#[derive(InitSpace, Debug)]
#[account]
pub struct RemoteChainConfig {
    pub version: u8,
    pub bump: u8,
    pub remote_chain_selector: u64,
    pub admin_config: RemoteChainAdminConfig,
    pub allowlist_config: RemoteChainAllowlistConfig,
    pub cctp: RemoteCctpConfig,
}

impl AccountSchemaVersion for RemoteChainConfig {
    const MAX_V: u8 = 1;

    fn version(&self) -> u8 {
        self.version
    }
}

impl RemoteChainConfig {
    pub fn dynamic_space(senders: &[Pubkey], supported_tokens: &[RemoteLocalPair]) -> usize {
        std::mem::size_of_val(senders) + supported_tokens.len() * RemoteLocalPair::INIT_SPACE
    }
}

#[derive(AnchorSerialize, AnchorDeserialize, InitSpace, Debug, Clone)]
pub struct RemoteCctpConfig {
    pub enabled: bool,          // Whether or not the domain is enabled.
    pub domain_identifier: u32, // Unique domain ID used across CCTP (https://developers.circle.com/cctp/concepts/supported-chains-and-domains#domain-identifiers)

    // The following addresses are limited to 32 bytes by Circle's CCTP message format. They are encoded into the
    // Pubkey type as the CCTP contracts expect them that way, but it's effectively a (padded) 32-byte slice.
    // See https://developers.circle.com/cctp/references/technical-guide#message-format
    pub allowed_caller_receive: Pubkey, // Address allowed to call receiveMessage on the domain.
    pub allowed_caller_deposit: Pubkey, // Address allowed to call depositForBurn on the domain.
    pub mint_recipient_on_dest: Option<Pubkey>, // Address to mint USDC to on the destination chain, for chains that proxy the funds through an intermediate account.

    // Other fields specific to the CCTP CCV
    pub lookup_table: Option<Pubkey>, // an ALT to be used for compressing txs
    #[max_len(0)]
    pub supported_tokens: Vec<RemoteLocalPair>, // List of supported tokens for this domain.
}

#[derive(AnchorSerialize, AnchorDeserialize, InitSpace, Debug, Clone)]
pub struct RemoteLocalPair {
    pub local: Pubkey,
    /// The remote address has to always be trimmed to the remote chain's length already, matching the
    /// router's dest_chain_state.config.address_bytes_length
    pub remote: RemoteAddress,
}

impl RemoteLocalPair {
    pub fn matches(&self, local: &Pubkey, remote: &[u8]) -> bool {
        let expected_len = self.remote.len();
        let actual_len = remote.len();

        if self.local != *local || expected_len > actual_len {
            return false;
        }

        let (prefix, trimmed) = remote.split_at(actual_len - expected_len);
        self.remote.address.as_slice() == trimmed && prefix.iter().all(|&b| b == 0)
    }
}

#[derive(InitSpace, Debug)]
#[account]
pub struct Nonce {
    pub version: u8,
    pub bump: u8,

    pub sender: Pubkey,
    pub mint: Pubkey,
    pub remote_chain_selector: u64,

    // Used as a seed for the CCTP event account.
    // Must be unique across as many dimensions as possible to avoid collisions between senders.
    pub nonce: u64,
}

impl AccountSchemaVersion for Nonce {
    const MAX_V: u8 = 1;

    fn version(&self) -> u8 {
        self.version
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_remote_local_pair_without_leading_zeros() {
        let local = Pubkey::new_unique();
        let remote = RemoteAddress {
            address: vec![1, 2, 3],
        };
        let pair = RemoteLocalPair {
            local,
            remote: remote.clone(),
        };

        assert!(pair.matches(&local, &remote.address));
        assert!(!pair.matches(&Pubkey::new_unique(), &remote.address));
        assert!(!pair.matches(&local, &[0u8; 32]));
        assert!(!pair.matches(&local, &[1u8; 20]));
        assert!(!pair.matches(&local, &[1, 1, 1]));
        assert!(!pair.matches(&local, &[2, 3]));
    }

    #[test]
    fn test_remote_local_leading_zeros() {
        let local = Pubkey::new_unique();
        let remote = RemoteAddress {
            address: vec![0, 1], // has a leading zero, to check the trimmed/prefix matching logic
        };
        let pair = RemoteLocalPair { local, remote };

        // matches
        assert!(pair.matches(&local, &[0, 1]));
        assert!(pair.matches(&local, &[0, 0, 0, 0, 0, 1]));

        // different
        assert!(!pair.matches(&local, &[1, 0, 1]));
        assert!(!pair.matches(&local, &[0, 1, 1]));
        assert!(!pair.matches(&local, &[1, 1]));
        assert!(!pair.matches(&local, &[1])); // missing 1 leading zero that is part of the expected local address
    }
}
