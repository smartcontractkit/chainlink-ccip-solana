use anchor_lang::prelude::*;

pub use ccip_common::state::AccountSchemaVersion;
use ccip_common::{messages::FinalityConfig, CommonCcipError};

use base_token_pool::{
    state::{BaseChain, BaseConfig, BaseOverrideConfig, CCVConfig, TokenTransferFeeConfig},
    v2::SetChainConfigV2Params,
};

pub(super) const MESSAGE_SENT_EVENT_SEED: &[u8] = b"ccip_cctp_message_sent_event";

// NOTE: accounts derivations must be native to program - will cause ownership check issues if imported
// wraps functionality from shared Config and Chain types
#[account]
#[derive(InitSpace)]
pub struct State {
    pub version: u8,
    pub config: BaseConfig,
    pub funding: FundingConfig,
}

impl AccountSchemaVersion for State {
    const MAX_V: u8 = 1;

    fn version(&self) -> u8 {
        self.version
    }
}

#[derive(InitSpace, AnchorDeserialize, AnchorSerialize, Clone, Copy)]
pub struct FundingConfig {
    // Authority allowed to reclaim funds (i.e. from closing the CCTP event PDA, or de-funding an
    // overfunded signer PDA.)
    pub manager: Pubkey,
    // Receiver of funds reclaimed from the signer PDA.
    pub reclaim_destination: Pubkey,
    // Funds in the signer PDA cannot drop under this value when reclaiming.
    pub minimum_signer_funds: u64,
}

#[account]
#[derive(InitSpace)]
// ChainConfig is the per-token-pool, per-chain configuration.
pub struct ChainConfig {
    pub version: u8,
    pub base: BaseChain,
    pub cctp: CctpChain,
}

impl AccountSchemaVersion for ChainConfig {
    const MAX_V: u8 = 1;

    fn version(&self) -> u8 {
        self.version
    }
}

#[derive(AnchorSerialize, AnchorDeserialize, Clone, Copy, InitSpace, Debug)]
pub struct CctpChain {
    // Domain ID for CCTP, used to identify the chain. This is a sequential number starting from 0.
    // Using u32 here because it's what CCTP uses in its Params structs.
    pub domain_id: u32,
    // The allowed caller to invoke CCTP's receive_message on the dest chain, which is the token pool there.
    // It is encoded into a Solana Pubkey, as per
    // https://developers.circle.com/stablecoins/solana-programs#mint-recipient-for-solana-as-source-chain-transfers
    pub destination_caller: Pubkey,
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

impl AccountSchemaVersion for ChainConfigOverride {
    const MAX_V: u8 = 1;

    fn version(&self) -> u8 {
        self.version
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

        if !config.is_latest_version() {
            return Err(CommonCcipError::InvalidAccountVersion.into());
        }

        Ok(Some(config))
    }
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

impl AccountSchemaVersion for ChainConfigV2 {
    const MAX_V: u8 = 1;

    fn version(&self) -> u8 {
        self.version
    }
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

        if !config.is_latest_version() {
            return Err(CommonCcipError::InvalidAccountVersion.into());
        }

        Ok(Some(config))
    }
}
