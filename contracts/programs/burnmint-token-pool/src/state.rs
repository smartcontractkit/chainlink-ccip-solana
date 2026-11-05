use anchor_lang::prelude::*;
use base_token_pool::v2::SetChainConfigV2Params;
use base_token_pool::{constants::valid_version, state::*};
use ccip_common::messages::FinalityConfig;
use ccip_common::CommonCcipError;

pub const MAX_POOL_STATE_V: u8 = 1;
pub const MAX_POOL_CONFIG_V: u8 = 1;
pub const MAX_POOL_OVERRIDE_V: u8 = 1;
pub const MAX_V2_CONFIG_V: u8 = 1;
pub const MAX_TOKEN_TRANSFER_FEE_CONFIG_V: u8 = 1;

// ============================================================
// Account types
//
// Each of these wraps a type from the base-token-pool crate.
// They must be re-declared here so that Anchor assigns the
// correct program ID during ownership checks — importing the
// base type directly would use the base-token-pool program ID
// instead of this program's ID.
// ============================================================

// PoolConfig is the global configuration of the program itself.
// It stores shared defaults (router, RMN Remote) used when
// initializing new token pool instances.
#[account]
#[derive(InitSpace)]
pub struct PoolConfig {
    pub version: u8,
    pub self_served_allowed: bool,

    // Default address values applied when a new token pool is initialized
    pub router: Pubkey,
    pub rmn_remote: Pubkey,
}

// State is the per-token-pool configuration for a single mint.
// Each mint gets its own State PDA that holds ownership,
// rate-limit admin, allow-list, and routing details.
#[account]
#[derive(InitSpace)]
pub struct State {
    pub version: u8,
    pub config: BaseConfig,
}

// ChainConfig is the per-token-pool, per-chain configuration.
// For every remote chain that a pool supports there is one
// ChainConfig PDA storing the remote token address, allowed
// remote pool addresses, and per-direction rate limits.
#[account]
#[derive(InitSpace)]
pub struct ChainConfig {
    pub base: BaseChain,
}

// ChainConfigOverride stores optional per-chain override
// configuration. The override type (e.g. FTF) is part of the PDA
// seeds so that future override kinds get their own PDA without
// migrating existing accounts.
// PDA seeds: [POOL_OVERRIDE_CHAINCONFIG_SEED, chain_selector, mint, override_type]
#[account]
#[derive(InitSpace)]
pub struct ChainConfigOverride {
    pub bump: u8,
    pub version: u8,
    pub base: BaseOverrideConfig,
}

// ChainConfigV2 is the per-token-pool, per-chain v2 configuration. It holds the CCV config,
// allowed finality policy, and token-transfer fee config for outbound transfers on the lane.
// Mirrors lockrelease's `ChainConfigV2` so both pools share the same account layout and CPI shape.
// PDA seeds: [POOL_CHAINCONFIG_SEED_2_0, chain_selector, mint]
#[account]
#[derive(InitSpace)]
pub struct ChainConfigV2 {
    pub bump: u8,
    pub version: u8,
    pub remote_chain_selector: u64,
    pub mint: Pubkey,

    /// Allowed finality policy for *outbound* (lock_or_burn) transfers on this lane.
    /// Mirrors EVM's pool-wide `s_allowedFinalityConfig`. Zero-initialized
    /// (`WAIT_FOR_FINALITY`) means only full-finality transfers are permitted.
    pub allowed_finality_config: FinalityConfig,
    pub token_transfer_fee_config: TokenTransferFeeConfig,
    pub ccv_config: CCVConfig,
}

impl ChainConfigV2 {
    pub fn dynamic_space(params: &SetChainConfigV2Params) -> usize {
        CCVConfig::dynamic_space(
            &params.ccv_config.outbound_ccvs,
            &params.ccv_config.threshold_outbound_ccvs,
            &params.ccv_config.inbound_ccvs,
            &params.ccv_config.threshold_inbound_ccvs,
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

        if !valid_version(config.version, MAX_V2_CONFIG_V) {
            return Err(CommonCcipError::InvalidAccountVersion.into());
        }

        Ok(Some(config))
    }
}

impl ChainConfigOverride {
    /// Attempts to load a `ChainConfigOverride` from an `UncheckedAccount`.
    ///
    /// This is used for optional override accounts (e.g., FTF rate limits) where the PDA
    /// may or may not be initialized.  Performs the same owner and discriminator checks as
    /// `Account::try_from` (via `Owner::owner()` and `try_deserialize`), then additionally
    /// validates the version.
    ///
    /// Note: We cannot delegate to `Account::try_from` directly because its lifetime
    /// requirements (`&'a AccountInfo<'a>`) are incompatible with `UncheckedAccount`
    /// references at the call sites.
    ///
    /// Returns `Ok(None)` only when the account is not initialized (owner mismatch). A borrow,
    /// deserialization, or version failure is reported as an `Err` so that a corrupted or
    /// version-mismatched override never silently falls back to the default rate-limit bucket.
    ///
    /// # Safety
    /// Callers must ensure the PDA address is validated via Anchor seeds constraints.
    /// This function only validates owner, data integrity, and version — not PDA derivation.
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
