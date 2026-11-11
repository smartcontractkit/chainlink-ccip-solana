use anchor_lang::prelude::*;

use crate::state::MessageExecutionState;

/// Mirrors EVM's `OffRamp.ExecutionStateChanged(sourceChainSelector, messageNumber, messageId,
/// state, returnData)` — same fields, same order.
///
/// `message_number` follows EVM, which names this field `messageNumber` in both the event and
/// `MessageV1Codec.MessageV1`. Stellar calls it `sequence_number`; EVM is the reference.
#[event]
pub struct ExecutionStateChangedV2 {
    pub source_chain_selector: u64,
    pub message_number: u64,
    pub message_id: [u8; 32],
    pub state: MessageExecutionState,
    pub return_data: Vec<u8>,
}

#[event]
pub struct BufferInitialized {
    pub buffer_id: [u8; 32],
    pub authority: Pubkey,
}

#[event]
pub struct BufferClosed {
    pub buffer_id: [u8; 32],
    pub authority: Pubkey,
}

pub mod admin {
    use crate::state::SourceChainConfig;

    use super::*;

    #[event]
    pub struct SourceChainConfigUpdated {
        pub source_chain_selector: u64,
        pub source_chain_config: SourceChainConfig,
    }

    #[event]
    pub struct SourceChainAdded {
        pub source_chain_selector: u64,
        pub source_chain_config: SourceChainConfig,
    }

    #[event]
    pub struct SourceChainDeleted {
        pub source_chain_selector: u64,
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
    pub struct ConfigSet {
        pub svm_chain_selector: u64,
    }

    #[event]
    pub struct ReferenceAddressesSet {
        pub router: Pubkey,
        pub fee_quoter: Pubkey,
        pub offramp_lookup_table: Pubkey,
        pub rmn_remote: Pubkey,
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
    pub cross_chain_amount: crate::messages::CrossChainAmount,
    pub protocol_amount: crate::messages::ProtocolAmount,
}
