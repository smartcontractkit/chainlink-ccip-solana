use std::fmt::Display;

use anchor_lang::prelude::*;
use ccip_common::types::{CrossChainGas, UsdCents};

use crate::messages::SVM2AnyMessage;

pub use ccip_common::state::CodeVersion;

#[account]
#[derive(InitSpace, Debug)]
pub struct Config {
    pub version: u8,

    pub default_code_version: CodeVersion,

    pub svm_chain_selector: u64,
    pub owner: Pubkey,
    pub proposed_owner: Pubkey,
    pub fee_quoter: Pubkey,
    pub rmn_remote: Pubkey,
    pub link_token_mint: Pubkey,
    pub fee_aggregator: Pubkey, // Allowed address to withdraw billed fees to (will use ATAs derived from it)
}

// PDA seeds: [REFERENCE_ADDRESSES]
// Addresses the router refers to outside of its `Config`, mirroring the offramp's account of the
// same name. New fields are added by bumping `version`.
#[account]
#[derive(InitSpace, Debug)]
pub struct ReferenceAddresses {
    pub version: u8,
    pub bump: u8,
    // Returned by `ccip_send_v2` / `get_fee_v2` account resolution. Only read during resolution,
    // so it is never an account of those instructions. Pubkey::default() means no table is returned.
    pub onramp_lookup_table: Pubkey,
}

#[derive(Clone, AnchorSerialize, AnchorDeserialize, InitSpace, Debug, PartialEq, Eq, Copy)]
pub enum RestoreOnAction {
    None,     // initial case, there's no saved message number to restore
    Upgrade, // after a rollback, the saved message number must be restored on the following upgrade
    Rollback, // after an upgrade, the saved message number must be restored on the following rollback
}

impl Display for RestoreOnAction {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        match self {
            RestoreOnAction::None => write!(f, "None"),
            RestoreOnAction::Upgrade => write!(f, "Upgrade"),
            RestoreOnAction::Rollback => write!(f, "Rollback"),
        }
    }
}

#[derive(Clone, AnchorSerialize, AnchorDeserialize, InitSpace, Debug, PartialEq, Eq, Copy)]
pub struct DestChainState {
    // Named after EVM 1.7's `OnRamp.DestChainConfig.messageNumber`. The legacy 1.6 onramp shares
    // this counter and still exposes it as `sequence_number` on `RampMessageHeader`, matching what
    // EVM 1.6 called it.
    pub message_number: u64, // The last used message number. On upgrades, this is reset to 0.

    // The following property is used to support one rollback operation, in which the message number of
    // the previous OnRamp version is restored. The upgrade/rollback is done per-lane (i.e. per dest chain).
    // If it's 0, that means that it is not possible to rollback to the previous version. As version upgrades
    // are not often, we won't need to rollback multiple versions.
    pub message_number_to_restore: u64, // The last used message number in the previous onramp version
    pub restore_on_action: RestoreOnAction,
}

#[derive(Clone, AnchorSerialize, AnchorDeserialize, InitSpace, Debug)]
pub struct DestChainConfig {
    // The code version of the lane, in case we need to shift traffic to new logic for a single lane to test an upgrade
    pub lane_code_version: CodeVersion,

    // list of senders authorized to send messages to this destination chain.
    // Note: The attribute name `max_len` is slightly misleading: it is not in any
    // way limiting the actual length of the vector during initialization; it just
    // helps the InitSpace derive macro work out the initial space. We can leave it at
    // zero and calculate the actual length in the instruction context.
    #[max_len(0)]
    pub allowed_senders: Vec<Pubkey>,
    pub allow_list_enabled: bool,
}

impl DestChainConfig {
    pub fn space(&self) -> usize {
        Self::INIT_SPACE + self.dynamic_space()
    }

    pub fn dynamic_space(&self) -> usize {
        self.allowed_senders.len() * std::mem::size_of::<Pubkey>()
    }
}

#[derive(Clone, AnchorSerialize, AnchorDeserialize, InitSpace, Debug)]
pub struct DestChainConfigCcipV2 {
    // The code version of the lane, in case we need to shift traffic to new logic for a single lane to test an upgrade
    pub lane_code_version: CodeVersion,

    pub address_bytes_length: u8, // The length of an address on this chain in bytes, e.g. 20 for EVM, 32 for SVM, etc.

    pub token_receiver_allowed: bool, // Whether specifying `tokenReceiver` in V3 extra args is allowed for this destination chain.

    // list of senders authorized to send messages to this destination chain.
    // Note: The attribute name `max_len` is slightly misleading: it is not in any
    // way limiting the actual length of the vector during initialization; it just
    // helps the InitSpace derive macro work out the initial space. We can leave it at
    // zero and calculate the actual length in the instruction context.
    #[max_len(0)]
    pub allowed_senders: Vec<Pubkey>,
    pub allow_list_enabled: bool,

    // Default CCVs to use for messages to this destination chain.
    #[max_len(0)]
    pub default_ccvs: Vec<Pubkey>,

    // Required CCVs to use for all messages to this destination chain.
    #[max_len(0)]
    pub lane_mandated_ccvs: Vec<Pubkey>,

    // Default executor to use for messages to this destination chain.
    pub default_executor: Pubkey,

    // Destination OffRamp address, NOT abi encoded but raw bytes.
    #[max_len(0)]
    pub offramp: Vec<u8>,

    // Network fee in USD cents for messages with token transfers.
    pub message_network_fee: UsdCents,

    // Network fee in USD cents for messages with token transfers.
    pub token_transfer_network_fee: UsdCents,

    // Base gas cost for executing a message on the destination chain.
    pub base_execution_gas_cost: CrossChainGas,

    // Maximum fee that can be paid for a single message. Unlike in
    // EVM this is configured per-chain for migration reasons, but
    // generally would have the same universal value.
    pub max_fee_per_message: UsdCents,
}

impl DestChainConfigCcipV2 {
    pub fn space(&self) -> usize {
        Self::INIT_SPACE + self.dynamic_space()
    }

    pub fn dynamic_space(&self) -> usize {
        self.allowed_senders.len() * std::mem::size_of::<Pubkey>()
            + self.default_ccvs.len() * std::mem::size_of::<Pubkey>()
            + self.lane_mandated_ccvs.len() * std::mem::size_of::<Pubkey>()
            + self.offramp.len()
    }
}

// PDA seeds: [DEST_CHAIN_STATE, chain_selector]
#[account]
#[derive(InitSpace, Debug)]
pub struct DestChain {
    // Config for SVM2Any
    pub version: u8,
    pub chain_selector: u64,     // Chain selector used for the seed
    pub state: DestChainState,   // values that are updated automatically
    pub config: DestChainConfig, // values configured by an admin
}

// PDA seeds: [DEST_CHAIN_STATE_V2, chain_selector]
#[account]
#[derive(InitSpace, Debug)]
pub struct DestChainCcipV2 {
    pub bump: u8,
    pub version: u8,
    pub chain_selector: u64,           // Chain selector used for the seed
    pub state: DestChainState,         // values that are updated automatically
    pub config: DestChainConfigCcipV2, // values configured by an admin
}

// PDA seeds: [NONCE, dest_chain_selector, authority] — seed fields not stored (lookup-only marker)
#[account]
#[derive(InitSpace)]
pub struct Nonce {
    pub version: u8,        // version to check if nonce account is already initialized
    pub ordered_nonce: u64, // Counter per user and per lane to use as nonce for all the messages to be executed in order
    pub total_nonce: u64, // Counter per user per lane, regardless of whether they are executed in order or not
}

#[derive(Debug, Default, PartialEq, Eq, Clone, AnchorDeserialize, AnchorSerialize)]
pub struct DeriveAccountsResponse {
    /// If this vector is not empty, you must call the `derive_` method again including
    /// exactly these accounts as the `remaining_accounts` field.
    pub ask_again_with: Vec<CcipAccountMeta>,
    /// You must append these accounts at the end of a separate list. When `next_stage`
    /// is finally empty, this separate list will contain all the accounts to use for the
    /// instruction of interest.
    pub accounts_to_save: Vec<CcipAccountMeta>,
    /// Append these look up tables at the end of a list. It will contain all LUTs
    /// that the instruction of interest can use.
    pub look_up_tables_to_save: Vec<Pubkey>,
    /// Identifies the derivation stage.
    pub current_stage: String,
    /// Identifies the next derivation stage. If empty, the derivation is complete.
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

#[derive(Clone, Debug, AnchorDeserialize, AnchorSerialize)]
pub struct DeriveAccountsCcipSendParams {
    pub dest_chain_selector: u64,
    pub ccip_send_caller: Pubkey,
    pub message: SVM2AnyMessage,
}
