use anchor_lang::prelude::*;

use base_token_pool::constants::valid_version;
use base_token_pool::state::{
    BaseChain, BaseConfig, BaseOverrideConfig, CCVConfig, TokenTransferFeeConfig,
};
use ccip_common::messages::FinalityConfig;
use ccip_common::types::{ByteString, CrossChainGas, UsdCents};
use ccip_common::CommonCcipError;

use crate::constants::MAX_POOL_OVERRIDE_V;
use crate::errors::LombardTokenPoolError;

#[account]
#[derive(InitSpace)]
pub struct State {
    pub version: u8,
    pub config: BaseConfig,
    pub bridge: Pubkey,      // bridge address
    pub alt: Option<Pubkey>, // bridge address
}

#[derive(AnchorSerialize, AnchorDeserialize, Clone, Copy, InitSpace, Debug)]
pub struct LombardChain {
    pub destination_chain_id: [u8; 32],
    pub destination_caller: [u8; 32],
}

impl LombardChain {
    /// A zero destination caller lets anyone claim the deposit on the destination, before the
    /// remote pool can.
    pub fn validate(&self) -> Result<()> {
        require_keys_neq!(
            Pubkey::new_from_array(self.destination_chain_id),
            Pubkey::default(),
            LombardTokenPoolError::InvalidPath
        );
        require_keys_neq!(
            Pubkey::new_from_array(self.destination_caller),
            Pubkey::default(),
            LombardTokenPoolError::InvalidPath
        );
        Ok(())
    }
}

#[account]
#[derive(InitSpace)]
pub struct PoolConfig {
    // This is mainly a placeholder in case we ever want to include global configs that apply
    // to how all pools behave.
    // For now, it is pretty much empty.
    pub version: u8,
}

#[account]
#[derive(InitSpace)]
pub struct ChainConfig {
    pub version: u8,
    pub base: BaseChain,
    pub bridge: LombardChain,
}

/// Optional per-(mint, remote chain, override type) rate-limit override (currently only `FTF`).
/// The override type is part of the PDA seeds so future override kinds get their own PDA without
/// migrating existing accounts.
/// PDA seeds: `[POOL_OVERRIDE_CHAINCONFIG_SEED, remote_chain_selector, mint, override_type]`.
#[account]
#[derive(InitSpace)]
pub struct ChainConfigOverride {
    pub bump: u8,
    pub version: u8,
    pub base: BaseOverrideConfig,
}

impl ChainConfigOverride {
    /// Attempts to load a `ChainConfigOverride` from an `UncheckedAccount`. Used for optional
    /// override accounts where the PDA may or may not be initialized yet - returns `Ok(None)`
    /// rather than erroring in that case. A borrow, deserialization, or version failure is
    /// reported as an `Err` so that a corrupted or version-mismatched override never silently
    /// falls back to the default rate-limit bucket. Only validates owner, data integrity, and
    /// version - not PDA derivation, which callers must still enforce via Anchor `seeds`
    /// constraints.
    pub fn try_load(account_info: &AccountInfo) -> Result<Option<Self>> {
        if *account_info.owner != Self::owner() {
            return Ok(None); // Account not initialized
        }

        let Ok(data) = account_info.try_borrow_data() else {
            return Err(CommonCcipError::InvalidAccountData.into());
        };

        let Ok(config) = Self::try_deserialize(&mut &data[..]) else {
            return Err(CommonCcipError::InvalidAccountData.into());
        };

        if !valid_version(config.version, MAX_POOL_OVERRIDE_V) {
            return Err(CommonCcipError::InvalidAccountVersion.into());
        }

        Ok(Some(config))
    }
}

// --- CCIP 2.0 additions ---

pub const MAX_CHAIN_CONFIG_V2_V: u8 = 1;
pub const MAX_VERIFIER_CONFIG_V: u8 = 1;
pub const MAX_VERIFIER_REMOTE_CHAIN_CONFIG_V: u8 = 1;
pub const MAX_VERIFIER_STORAGE_LOCATIONS_V: u8 = 1;

/// Per-(mint, remote chain) v2 lane config
#[account]
#[derive(InitSpace)]
pub struct ChainConfigV2 {
    pub version: u8,
    pub bump: u8,
    pub remote_chain_selector: u64,
    pub mint: Pubkey,
    pub allowed_finality_config: FinalityConfig,
    pub token_transfer_fee_config: TokenTransferFeeConfig,
    pub ccv_config: CCVConfig,
}

impl ChainConfigV2 {
    pub fn dynamic_space(ccv_config: &CCVConfig) -> usize {
        CCVConfig::dynamic_space(
            &ccv_config.outbound_ccvs,
            &ccv_config.threshold_outbound_ccvs,
            &ccv_config.inbound_ccvs,
            &ccv_config.threshold_inbound_ccvs,
        )
    }

    pub fn try_load(account_info: &AccountInfo) -> Result<Option<Self>> {
        if *account_info.owner != Self::owner() {
            return Ok(None); // Account not initialized
        }

        let Ok(data) = account_info.try_borrow_data() else {
            return Err(CommonCcipError::InvalidAccountData.into());
        };

        let Ok(config) = Self::try_deserialize(&mut &data[..]) else {
            return Err(CommonCcipError::InvalidAccountData.into());
        };

        if !valid_version(config.version, MAX_CHAIN_CONFIG_V2_V) {
            return Err(CommonCcipError::InvalidAccountVersion.into());
        }

        Ok(Some(config))
    }
}

/// A local token this verifier supports, with an optional adapter substitution.
/// `local_adapter` is `None` when the token itself is used directly with the Lombard bridge.
#[derive(AnchorSerialize, AnchorDeserialize, Clone, Copy, InitSpace, Debug, PartialEq, Eq)]
pub struct SupportedToken {
    pub local_token: Pubkey,
    pub local_adapter: Option<Pubkey>,
}

/// Global verifier config.
#[account]
#[derive(InitSpace)]
pub struct VerifierConfig {
    pub version: u8,
    pub bump: u8,
    pub owner: Pubkey,
    pub proposed_owner: Pubkey,
    pub storage_location_admin: Pubkey,
    pub proposed_storage_location_admin: Pubkey,
    pub allowlist_admin: Pubkey,
    pub fee_aggregator: Pubkey,
    pub allowed_finality_config: FinalityConfig,
    /// This verifier's own version tag, mirroring EVM's immutable `versionTag()`
    pub version_tag: [u8; 4],
    #[max_len(0)]
    pub supported_tokens: Vec<SupportedToken>,
}

/// Per-remote-chain verifier admin config
#[derive(AnchorSerialize, AnchorDeserialize, Clone, Copy, InitSpace, Debug)]
pub struct VerifierRemoteChainAdminConfig {
    pub router_signer: Pubkey,
    pub fee: UsdCents,
    pub gas_for_verification: CrossChainGas,
    pub payload_size_bytes: u32,
}

/// Per-dest-chain sender allowlist for the outbound path
/// `RemoteChainAllowlistConfig` / EVM's `BaseVerifier` allowlist.
#[derive(AnchorSerialize, AnchorDeserialize, Clone, InitSpace, Debug)]
pub struct VerifierRemoteChainAllowlistConfig {
    pub allowlist_enabled: bool,
    #[max_len(0)]
    pub allowed_senders: Vec<Pubkey>,
}

/// Lombard GMP path config for a remote chain
/// (`allowedCaller`, `lChainId`, `remoteBridgeSender`).
#[derive(AnchorSerialize, AnchorDeserialize, Clone, Copy, InitSpace, Debug, Default)]
pub struct LombardVerifierPath {
    pub allowed_caller: [u8; 32],
    pub l_chain_id: [u8; 32],
    pub remote_bridge_sender: [u8; 32],
}

impl LombardVerifierPath {
    pub fn is_set(&self) -> bool {
        self.l_chain_id != [0u8; 32]
    }
}

/// A remote adapter override for one local token on one remote chain
/// `LombardVerifier.RemoteAdapterArgs` / `s_remoteAdapters[destChainSelector][token]`.
#[derive(AnchorSerialize, AnchorDeserialize, Clone, Copy, InitSpace, Debug, PartialEq, Eq)]
pub struct RemoteAdapter {
    pub token: Pubkey,
    pub remote_adapter: [u8; 32],
}

/// Per-remote-chain verifier config
#[account]
#[derive(InitSpace)]
pub struct VerifierRemoteChainConfig {
    pub version: u8,
    pub bump: u8,
    pub remote_chain_selector: u64,
    pub admin_config: VerifierRemoteChainAdminConfig,
    pub allowlist_config: VerifierRemoteChainAllowlistConfig,
    pub path: LombardVerifierPath,
    /// Address lookup table holding this remote chain's own resolution accounts.
    pub lookup_table: Option<Pubkey>,
    #[max_len(0)]
    pub remote_adapters: Vec<RemoteAdapter>,
}

impl VerifierRemoteChainConfig {
    pub fn dynamic_space(allowed_senders: &[Pubkey], remote_adapters: &[RemoteAdapter]) -> usize {
        std::mem::size_of_val(allowed_senders) + remote_adapters.len() * RemoteAdapter::INIT_SPACE
    }
}

/// Off-chain proof-lookup locations
#[account]
#[derive(InitSpace)]
pub struct VerifierStorageLocations {
    pub version: u8,
    pub bump: u8,
    /// Opaque location identifiers — URLs in practice, but the program only stores and echoes them,
    /// so they are held as [`ByteString`] rather than `String` to avoid paying for UTF-8 validation
    /// on every load. Borsh-identical to `Vec<String>`, so this needs no account migration.
    #[max_len(0)]
    pub locations: Vec<ByteString>,
}

impl VerifierStorageLocations {
    pub fn dynamic_space(locations: &[ByteString]) -> usize {
        locations.iter().map(|l| 4 + l.payload_len()).sum()
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    fn chain_config_v2(version: u8) -> ChainConfigV2 {
        ChainConfigV2 {
            version,
            bump: 255,
            remote_chain_selector: 1,
            mint: Pubkey::new_unique(),
            allowed_finality_config: FinalityConfig::default(),
            token_transfer_fee_config: TokenTransferFeeConfig::default(),
            ccv_config: CCVConfig {
                threshold_amount: 0,
                outbound_ccvs: vec![],
                threshold_outbound_ccvs: vec![],
                inbound_ccvs: vec![],
                threshold_inbound_ccvs: vec![],
            },
        }
    }

    fn serialized(config: &ChainConfigV2) -> Vec<u8> {
        let mut data = Vec::new();
        config.try_serialize(&mut data).unwrap();
        data
    }

    fn try_load_with(owner: Pubkey, mut data: Vec<u8>) -> Result<Option<ChainConfigV2>> {
        let key = Pubkey::new_unique();
        let mut lamports = 0;
        let account_info =
            AccountInfo::new(&key, false, false, &mut lamports, &mut data, &owner, false);
        ChainConfigV2::try_load(&account_info)
    }

    #[test]
    fn try_load_returns_none_for_uninitialized_account() {
        let loaded = try_load_with(Pubkey::default(), vec![]).unwrap();
        assert!(loaded.is_none());
    }

    #[test]
    fn try_load_returns_valid_config() {
        let config = chain_config_v2(MAX_CHAIN_CONFIG_V2_V);
        let loaded = try_load_with(crate::ID, serialized(&config))
            .unwrap()
            .unwrap();
        assert_eq!(loaded.version, config.version);
        assert_eq!(loaded.mint, config.mint);
    }

    #[test]
    fn try_load_fails_on_undecodable_data() {
        let mut data = serialized(&chain_config_v2(MAX_CHAIN_CONFIG_V2_V));
        data.truncate(data.len() - 1);
        let err = try_load_with(crate::ID, data).err().unwrap();
        assert_eq!(err, CommonCcipError::InvalidAccountData.into());
    }

    #[test]
    fn try_load_fails_on_invalid_version() {
        for version in [0, MAX_CHAIN_CONFIG_V2_V + 1] {
            let data = serialized(&chain_config_v2(version));
            let err = try_load_with(crate::ID, data).err().unwrap();
            assert_eq!(err, CommonCcipError::InvalidAccountVersion.into());
        }
    }
}
