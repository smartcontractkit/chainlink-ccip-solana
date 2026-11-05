use anchor_discriminator::anchor_discriminator;
use anchor_lang::prelude::*;

pub const TOKENPOOL_DERIVE_LOCK_OR_BURN_DISCRIMINATOR: [u8; 8] =
    anchor_discriminator!("derive_accounts_lock_or_burn_tokens");

#[derive(Clone, Copy, AnchorSerialize, AnchorDeserialize)]
// Family-agnostic header for OnRamp & OffRamp messages
// The messageId is not expected to match hash(message), since it may originate from another ramp family
pub struct RampMessageHeader {
    pub message_id: [u8; 32], // Unique identifier for the message, generated with the source chain's encoding scheme
    pub source_chain_selector: u64, // the chain selector of the source chain, note: not chainId
    pub dest_chain_selector: u64, // the chain selector of the destination chain, note: not chainId
    pub sequence_number: u64, // sequence number, not unique across lanes
    pub nonce: u64, // nonce for this lane for this sender, not unique across senders/lanes
}

#[derive(Clone, AnchorSerialize, AnchorDeserialize)]
// Family-agnostic message emitted from the OnRamp
// Note: hash(Any2SVMRampMessage) != hash(SVM2AnyRampMessage) due to encoding & parameter differences
// messageId = hash(SVM2AnyRampMessage) using the source EVM chain's encoding format
pub struct SVM2AnyRampMessage {
    pub header: RampMessageHeader, // Message header
    pub sender: Pubkey,            // sender address on the source chain
    pub data: Vec<u8>,             // arbitrary data payload supplied by the message sender
    pub receiver: Vec<u8>,         // receiver address on the destination chain
    pub extra_args: Vec<u8>, // destination-chain specific extra args, such as the gasLimit for EVM chains
    pub fee_token: Pubkey,
    pub token_amounts: Vec<SVM2AnyTokenTransfer>,
    pub fee_token_amount: CrossChainAmount,
    pub fee_value_juels: CrossChainAmount,
}

#[derive(Debug, Clone, AnchorSerialize, AnchorDeserialize, Default)]
pub struct SVM2AnyTokenTransfer {
    // The source pool address. This value is trusted as it was obtained through the onRamp. It can be relied
    // upon by the destination pool to validate the source pool.
    pub source_pool_address: Pubkey,
    // The address of the destination token.
    // This value is UNTRUSTED as any pool owner can return whatever value they want.
    pub dest_token_address: Vec<u8>,
    // Optional pool data to be transferred to the destination chain. By default this is capped at
    // CCIP_LOCK_OR_BURN_V1_RET_BYTES bytes. If more data is required, the TokenTransferFeeConfig.destBytesOverhead
    // has to be set for the specific token.
    pub extra_data: Vec<u8>,
    pub amount: CrossChainAmount, // LE encoded u256 -  cross-chain token amount is always u256
    // Destination chain data used to execute the token transfer on the destination chain. For an EVM destination, it
    // consists of the amount of gas available for the releaseOrMint and transfer calls made by the offRamp.
    pub dest_exec_data: Vec<u8>,
}

pub use ccip_common::messages::{SVM2AnyMessage, SVMTokenAmount};
pub use ccip_common::v2::{CrossChainAmount, Receipt};

#[derive(AnchorSerialize, AnchorDeserialize, Clone, Copy, Default, Debug)]
pub struct GetFeeResult {
    pub amount: u64,
    pub juels: u128,
    pub token: Pubkey,
}

#[derive(AnchorSerialize, AnchorDeserialize, Clone, Copy, Default, Debug)]
pub struct GetFeeResultV2 {
    pub amount: u64,
    pub token: Pubkey,
}
