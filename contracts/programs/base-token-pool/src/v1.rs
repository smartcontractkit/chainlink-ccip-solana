use anchor_lang::prelude::*;

use crate::errors::*;
use crate::rate_limiter::RateLimitTokenBucket;
use crate::state::RemoteAddress;

pub use ccip_common::v2::CrossChainAmount;

#[derive(Clone, AnchorSerialize, AnchorDeserialize, Debug)]
pub struct LockOrBurnInV1 {
    pub receiver: Vec<u8>, //  The recipient of the tokens on the destination chain
    pub remote_chain_selector: u64, // The chain ID of the destination chain
    pub original_sender: Pubkey, // The original sender of the tx on the source chain
    pub amount: u64, // local solana amount to lock/burn,  The amount of tokens to lock or burn, denominated in the source token's decimals
    pub local_token: Pubkey, //  The address on this chain of the token to lock or burn

    /// DEPRECATED. The onramp's full nonce for this message, keyed by `original_sender` and
    /// `remote_chain_selector`. Still populated with a real value by the 1.6 onramp
    /// (`ccip-router`'s `instructions_v1/v1/onramp.rs`), but hard-coded to `0` by `ccip_send_v2`,
    /// which no longer tracks per-sender nonces.
    ///
    /// The only pool that reads it is `cctp-token-pool`, which uses it as a PDA seed for the
    /// `cctp_message_sent_event` account (see that program's `lib.rs` and `derive.rs`). A constant
    /// `0` would therefore collide across messages — so the `0` is only sound because a CCTP v1 pool
    /// can never be reached from `ccip_send_v2`:
    ///
    /// - CCTP and Lombard are the only v1 pools that set `implements_account_resolution`.
    /// - `TokenTransferContext::new` (the router's `instructions/common.rs`) verifies the
    ///   `TokenAdminRegistry` PDA and then rejects `interface_version == 1` &&
    ///   `implements_account_resolution` with `UnsupportedPoolV1ResolutionOnLaneV2`. That runs in
    ///   `ccip_send_v2` before any pool CPI, so it is the authoritative gate; the account-resolution
    ///   stage (`ccip_send/resolve_accounts/token_transfer_static.rs`) rejects the same combination
    ///   earlier, but is only advisory since callers can build the transaction themselves.
    /// - `set_pool_interface_implementation` additionally refuses to register
    ///   `implements_account_resolution` below `interface_version` 2. The runtime check still
    ///   matters because registries migrated from 1.6 can carry the v1/resolution combination.
    ///
    /// Removing the field outright is a breaking change to the v1 pool interface, so it stays until
    /// the 1.6 lane is retired. Anything that starts consuming it must not assume it is meaningful
    /// on the v2 lane.
    pub msg_total_nonce: u64,
}

#[derive(Clone, AnchorSerialize, AnchorDeserialize)]
pub struct LockOrBurnOutV1 {
    pub dest_token_address: RemoteAddress,
    // Currently encodes the local decimals as there's a chance they differ.
    pub dest_pool_data: Vec<u8>,
}

#[derive(Clone, AnchorSerialize, AnchorDeserialize)]
pub struct ReleaseOrMintInV1 {
    pub original_sender: RemoteAddress, //          The original sender of the tx on the source chain
    pub remote_chain_selector: u64,     // ─╮ The chain ID of the source chain
    pub receiver: Pubkey, // ───────────╯ The recipient of the tokens on the destination chain.
    pub amount: [u8; 32], // u256, incoming cross-chain amount - The amount of tokens to release or mint, denominated in the source token's decimals
    pub local_token: Pubkey, //            The address on this chain of the token to release or mint
    /// @dev WARNING: sourcePoolAddress should be checked prior to any processing of funds. Make sure it matches the
    /// expected pool address for the given remoteChainSelector.
    pub source_pool_address: RemoteAddress, //       The address of the source pool, abi encoded in the case of EVM chains
    pub source_pool_data: Vec<u8>, //          The data received from the source pool to process the release or mint
    /// @dev WARNING: offchainTokenData is untrusted data.
    pub offchain_token_data: Vec<u8>, //       The offchain data to process the release or mint
}

#[derive(Clone, AnchorSerialize, AnchorDeserialize)]
pub struct ReleaseOrMintOutV1 {
    // The number of tokens released or minted on the destination chain, denominated in the local token's decimals.
    // This value is expected to be equal to the ReleaseOrMintInV1.amount in the case where the source and destination
    // chain have the same number of decimals.
    pub destination_amount: u64, // token amounts local to solana
}

#[derive(Debug, Default, PartialEq, Eq, Clone, AnchorDeserialize, AnchorSerialize)]
pub struct DeriveAccountsResponse {
    pub ask_again_with: Vec<CcipAccountMeta>,
    pub accounts_to_save: Vec<CcipAccountMeta>,
    pub look_up_tables_to_save: Vec<Pubkey>,
    pub current_stage: String,
    pub next_stage: String,
}

// We can't use anchor's `AccountMeta` since it doesn't implement
// AnchorSerialize/AnchorDeserialize, and it's too small to warrant wrapping.
#[derive(Debug, Default, PartialEq, Eq, Clone, AnchorDeserialize, AnchorSerialize)]
pub struct CcipAccountMeta {
    pub pubkey: Pubkey,
    pub is_signer: bool,
    pub is_writable: bool,
}

pub trait ToMeta {
    fn readonly(self) -> CcipAccountMeta;
    fn writable(self) -> CcipAccountMeta;
    fn signer(self) -> CcipAccountMeta;
}

impl From<AccountMeta> for CcipAccountMeta {
    fn from(meta: AccountMeta) -> Self {
        Self {
            pubkey: meta.pubkey,
            is_signer: meta.is_signer,
            is_writable: meta.is_writable,
        }
    }
}

impl ToMeta for Pubkey {
    fn readonly(self) -> CcipAccountMeta {
        CcipAccountMeta {
            pubkey: self,
            is_signer: false,
            is_writable: false,
        }
    }

    fn writable(self) -> CcipAccountMeta {
        CcipAccountMeta {
            pubkey: self,
            is_signer: false,
            is_writable: true,
        }
    }

    fn signer(self) -> CcipAccountMeta {
        CcipAccountMeta {
            pubkey: self,
            is_signer: true,
            is_writable: false,
        }
    }
}

impl ToMeta for CcipAccountMeta {
    fn readonly(self) -> CcipAccountMeta {
        CcipAccountMeta {
            pubkey: self.pubkey,
            is_signer: self.is_signer,
            is_writable: false,
        }
    }

    fn writable(self) -> CcipAccountMeta {
        CcipAccountMeta {
            pubkey: self.pubkey,
            is_signer: self.is_signer,
            is_writable: true,
        }
    }

    fn signer(self) -> CcipAccountMeta {
        CcipAccountMeta {
            pubkey: self.pubkey,
            is_signer: true,
            is_writable: self.is_writable,
        }
    }
}

// validate_lock_or_burn checks for correctness on inputs
// - token & pool is correct for chain
// - rate limiting
#[allow(clippy::too_many_arguments)]
pub fn validate_lock_or_burn(
    lock_or_burn_in: &LockOrBurnInV1,
    config_mint: Pubkey,
    outbound_rate_limit: &mut RateLimitTokenBucket,
    allow_list_enabled: bool,
    allow_list: &[Pubkey],
) -> Result<()> {
    // validate token matches configured pool token
    require_keys_eq!(
        config_mint,
        lock_or_burn_in.local_token,
        CcipTokenPoolError::InvalidToken
    );
    require!(
        !allow_list_enabled || allow_list.contains(&lock_or_burn_in.original_sender),
        CcipTokenPoolError::InvalidSender
    );
    outbound_rate_limit.consume::<Clock>(lock_or_burn_in.amount)
}

// validate_release_or_mint checks for correctness on inputs
// - token & pool is correct for chain
// - rate limiting
#[allow(clippy::too_many_arguments)]
pub fn validate_release_or_mint(
    release_or_mint_in: &ReleaseOrMintInV1,
    parsed_amount: u64,
    config_mint: Pubkey,
    pool_addresses: &[RemoteAddress],
    inbound_rate_limit: &mut RateLimitTokenBucket,
) -> Result<()> {
    require_keys_eq!(
        config_mint,
        release_or_mint_in.local_token,
        CcipTokenPoolError::InvalidToken
    );
    require!(
        !pool_addresses.is_empty()
            && !release_or_mint_in.source_pool_address.is_empty()
            && pool_addresses.contains(&release_or_mint_in.source_pool_address),
        CcipTokenPoolError::InvalidSourcePoolAddress
    );
    inbound_rate_limit.consume::<Clock>(parsed_amount)
}
