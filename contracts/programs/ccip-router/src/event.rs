use anchor_lang::prelude::*;

use crate::{DestChainConfig, Receipt, SVM2AnyRampMessage};

pub mod events {
    use super::*;

    #[event]
    pub struct PdaUpgraded {
        pub address: Pubkey,
        pub old_version: u8,
        pub new_version: u8,
        pub name: String,
        pub seeds: Vec<u8>,
    }

    #[event]
    pub struct ReceiverRegistered {
        pub receiver_program: Pubkey,
        pub receiver_version: u8,
    }

    pub mod on_ramp {
        use ccip_common::v2::ProtocolAmount;

        use super::*;
        #[event]
        pub struct CCIPMessageSent {
            pub dest_chain_selector: u64,
            pub sequence_number: u64,
            pub message: SVM2AnyRampMessage,
        }

        /// CCIP 2.0 CCIPMessageSent event with receipts and verifier blobs.
        #[event]
        pub struct CCIPMessageSentV2 {
            pub dest_chain_selector: u64,
            pub sender: Pubkey,
            pub message_id: [u8; 32],
            pub fee_token: Pubkey,
            pub token_amount_before_token_pool_fees: ProtocolAmount,
            pub encoded_message: Vec<u8>,
            pub receipts: Vec<Receipt>,
            pub verifier_blobs: Vec<Vec<u8>>,
        }
    }

    pub mod admin {
        use crate::state::DestChainConfigCcipV2;

        use super::*;

        // NOTE: ownership update events can be found in OwnershipTransferRequested, OwnershipTransferred
        #[event]
        pub struct ConfigSet {
            pub svm_chain_selector: u64,
            pub fee_quoter: Pubkey,
            pub rmn_remote: Pubkey,
            pub link_token_mint: Pubkey,
            pub fee_aggregator: Pubkey,
        }

        #[event]
        #[allow(dead_code)]
        pub struct FeeTokenAdded {
            pub fee_token: Pubkey,
            pub enabled: bool,
        }

        #[event]
        #[allow(dead_code)]
        pub struct FeeTokenEnabled {
            pub fee_token: Pubkey,
        }

        #[event]
        #[allow(dead_code)]
        pub struct FeeTokenDisabled {
            pub fee_token: Pubkey,
        }

        #[event]
        #[allow(dead_code)]
        pub struct FeeTokenRemoved {
            pub fee_token: Pubkey,
        }

        #[event]
        pub struct DestChainConfigUpdated {
            pub dest_chain_selector: u64,
            pub dest_chain_config: DestChainConfig,
        }

        #[event]
        pub struct DestChainConfigUpdatedV2 {
            pub dest_chain_selector: u64,
            pub dest_chain_config: DestChainConfigCcipV2,
        }

        #[event]
        pub struct DestChainAdded {
            pub dest_chain_selector: u64,
            pub dest_chain_config: DestChainConfig,
        }

        #[event]
        pub struct DestChainAddedV2 {
            pub dest_chain_selector: u64,
            pub dest_chain_config: DestChainConfigCcipV2,
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
        pub struct OfframpAdded {
            pub source_chain_selector: u64,
            pub offramp: Pubkey,
        }

        #[event]
        pub struct OfframpRemoved {
            pub source_chain_selector: u64,
            pub offramp: Pubkey,
        }

        #[event]
        pub struct CcipVersionForDestChainVersionBumped {
            pub dest_chain_selector: u64,
            pub previous_sequence_number: u64,
            pub new_sequence_number: u64,
        }

        #[event]
        pub struct CcipVersionForDestChainVersionRolledBack {
            pub dest_chain_selector: u64,
            pub previous_sequence_number: u64,
            pub new_sequence_number: u64,
        }

        #[event]
        pub struct ReferenceAddressesSet {
            pub onramp_lookup_table: Pubkey,
        }
    }

    pub mod token_admin_registry {
        use super::*;
        #[event]
        pub struct PoolSet {
            pub token: Pubkey,
            pub previous_pool_lookup_table: Pubkey,
            pub new_pool_lookup_table: Pubkey,
        }

        #[event]
        pub struct AdministratorTransferRequested {
            pub token: Pubkey,
            pub current_admin: Pubkey,
            pub new_admin: Pubkey,
        }

        #[event]
        pub struct AdministratorTransferred {
            pub token: Pubkey,
            pub new_admin: Pubkey,
        }

        #[event]
        pub struct PoolInterfaceImplementationUpdated {
            pub token: Pubkey,
            pub interface_version: u8,
            pub account_resolution: ccip_common::router_accounts::ResolutionType,
        }
    }
}

/// Anchor 0.32 only includes types in the IDL when they are referenced by an instruction
/// argument, an #[account] context, or an #[event] struct. Types that are deserialized from
/// raw bytes at runtime (not part of any instruction signature) would be missing from the IDL
/// and Go bindings. This cfg-guarded event forces Anchor to include those types. It is never
/// emitted at runtime — it only exists during `anchor idl build`.
#[cfg(feature = "idl-build")]
#[event]
pub struct IdlBuildTypeExport {
    pub svm2any_ramp_message: crate::messages::SVM2AnyRampMessage,
    pub svm2any_token_transfer: crate::messages::SVM2AnyTokenTransfer,
    pub ramp_message_header: crate::messages::RampMessageHeader,
    pub cross_chain_amount: crate::messages::CrossChainAmount,
    pub nonce: crate::state::Nonce,
    pub receipt: crate::messages::Receipt,
}
