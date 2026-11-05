use std::fmt::Display;

use anchor_lang::prelude::*;
use anchor_spl::token::spl_token::native_mint;
use solana_keccak_hasher as keccak;

use crate::{types::CrossChainGas, v2::ProtocolAmount};

/// Per-message finality configuration. Wire-format: 4 bytes big-endian (flags || block_depth).
/// Mirrors the `bytes4` Finality type in chainlink-ccv's `protocol/finality.go`.
///
/// Layout (MSB first):
///   flags[15:1]  — reserved
///   flags[0]     — S: wait for `safe` head instead of full finality
///   block_depth  — number of block confirmations required (0 = full finality / safe, per flags)
#[derive(
    Clone, Copy, InitSpace, AnchorSerialize, AnchorDeserialize, PartialEq, Eq, Debug, Default,
)]
pub struct FinalityConfig {
    pub flags: u16,
    pub block_depth: u16,
}

pub const WAIT_FOR_FINALITY: FinalityConfig = FinalityConfig {
    flags: 0,
    block_depth: 0,
};

impl FinalityConfig {
    // NOTE: must only be used to validate Finality (message specified), **not** FinalityConfig (multiple allowed forms)
    pub const fn is_valid_finality(&self) -> bool {
        // requested finality can only have flags or block_depth set
        if self.flags != 0 && self.block_depth != 0 {
            return false;
        }

        // validate only one flag set
        if self.flags > 0 && self.flags.count_ones() > 1 {
            return false;
        }

        true
    }

    pub const fn is_wait_for_finality(&self) -> bool {
        // NOTE: `== WAIT_FOR_FINALITY` isn't const...
        self.flags == 0 && self.block_depth == 0
    }

    pub const fn ensure_requested_finality_allowed(
        &self,
        requested_finality: FinalityConfig,
    ) -> bool {
        // always valid
        if requested_finality.is_wait_for_finality() {
            return true;
        }
        if !requested_finality.is_valid_finality() {
            return false;
        }
        // If any of the flags match, the request is allowed only when it has no depth field (flag-only request).
        if requested_finality.flags & self.flags != 0 {
            return true;
        }
        if self.block_depth == 0 || requested_finality.block_depth < self.block_depth {
            return false;
        }

        true
    }
}

/// CCIP 2.0 cross-chain MessageV1. Chain-agnostic wire format.
#[derive(Clone, AnchorSerialize, AnchorDeserialize, PartialEq, Eq, Debug, Default)]
pub struct MessageV1 {
    pub source_chain_selector: u64,
    pub dest_chain_selector: u64,
    /// Named after EVM's `MessageV1Codec.MessageV1.messageNumber`. CCIP 1.6 called the equivalent
    /// header field `sequenceNumber`; 1.7 renamed it, and the 1.6 spelling is still used by the
    /// legacy `RampMessageHeader` in the router.
    pub message_number: u64,
    pub execution_gas_limit: CrossChainGas,
    pub ccip_receive_gas_limit: CrossChainGas,
    pub finality: FinalityConfig,
    pub ccv_and_executor_hash: [u8; 32],
    pub on_ramp_address: Vec<u8>,
    pub off_ramp_address: Vec<u8>,
    pub sender: Vec<u8>,
    pub receiver: Vec<u8>,
    pub dest_blob: Vec<u8>,
    pub token_transfer: Option<TokenTransferV1>,
    pub data: Vec<u8>,
}

/// The subset of [`MessageV1`] that is already known while the onramp resolves accounts, i.e.
/// before the token pool, verifier and executor CPIs that produce the rest of the message have
/// run. Handed to verifiers during nested account resolution so their resolvers can pick accounts
/// based on the message they are about to be asked to verify.
///
/// Deliberately absent, because [`MessageV1`] only gains them after resolution is over:
/// - `message_number`: read off the dest chain state and bumped by the send itself, so a value
///   captured during (read-only, simulated) resolution can be stale by execution time.
/// - `execution_gas_limit`: the sum of every participant's gas receipt, quoted by the fee quoter.
///   It includes this very verifier's `get_verifier_fee` result, which resolution precedes.
/// - `ccv_and_executor_hash`: hashed over the final CCV list, which the onramp is still building
///   as it resolves (lane-mandated and pool-required CCVs are merged in along the way).
#[derive(Clone, AnchorSerialize, AnchorDeserialize, PartialEq, Eq, Debug, Default)]
pub struct MessageV1FieldsAtResolutionTime {
    pub source_chain_selector: u64,
    pub dest_chain_selector: u64,
    pub ccip_receive_gas_limit: CrossChainGas,
    pub finality: FinalityConfig,
    pub on_ramp_address: Vec<u8>,
    pub off_ramp_address: Vec<u8>,
    pub sender: Vec<u8>,
    pub receiver: Vec<u8>,
    pub dest_blob: Vec<u8>,
    pub data: Vec<u8>,
    pub token_transfer: Option<TokenTransferFieldsAtResolutionTime>,
}

/// The subset of [`TokenTransferV1`] that is known at resolution time. Everything else in a token
/// transfer is produced by the pool's `lock_or_burn` CPI, which runs after account resolution:
/// the protocol `amount` net of the pool's in-token fee, `source_pool_address`,
/// `dest_token_address` and `extra_data`.
#[derive(Clone, AnchorSerialize, AnchorDeserialize, PartialEq, Eq, Debug, Default)]
pub struct TokenTransferFieldsAtResolutionTime {
    /// Source-chain mint of the transferred token.
    pub source_token_address: Pubkey,
    /// The amount the sender asked to transfer, before any in-token fee the pool deducts.
    pub amount: u64,
    /// Dest-chain token receiver, normalized to the dest chain's address length.
    pub token_receiver: Vec<u8>,
}

/// Whether a message is delivered without consulting or invoking its receiver, mirroring EVM's
/// `OffRamp._isTokenOnlyTransfer`: either the message names no receiver, or it requests no
/// receiver callback.
///
/// `receiver` is the raw dest-chain address; empty or all-zero means the message names none. Any
/// other value counts, including a malformed one, which the caller rejects when it parses it.
pub fn is_offramp_token_only_transfer(
    receiver: &[u8],
    data_len: usize,
    ccip_receive_gas_limit: CrossChainGas,
) -> bool {
    receiver.is_empty()
        || receiver == Pubkey::default().to_bytes()
        || (data_len == 0 && ccip_receive_gas_limit == CrossChainGas::ZERO)
}

impl MessageV1 {
    /// True only when the message reaches its receiver: it names one, and asks for a callback.
    pub fn is_offramp_arbitrary_message(&self) -> bool {
        !is_offramp_token_only_transfer(
            &self.receiver,
            self.data.len(),
            self.ccip_receive_gas_limit,
        )
    }

    /// True only when the message carries a token transfer.
    pub fn is_token_transfer(&self) -> bool {
        self.token_transfer.is_some()
    }
}

/// Current message format version for CCIP v2.0.
pub const MESSAGE_V1_VERSION: u8 = 1;

pub const MESSAGE_V1_BASE_SIZE: usize = 1 + 8 + 8 + 8 + 4 + 4 + 4 + 32 + 1 + 1 + 1 + 1 + 2 + 2 + 2;
pub const TOKEN_TRANSFER_V1_BASE_SIZE: usize = 1 + 32 + 1 + 1 + 1 + 1 + 2;

/// MESSAGE_V1_BASE_SIZE plus the two source-side addresses (onRamp, sender), which are native
/// 32-byte pubkeys when SVM is the source chain.
/// To be added:
/// - receiver and offRamp are dest chain specific (see MESSAGE_V1_REMOTE_CHAIN_ADDRESSES).
/// - destBlob (== executorArgs) and data are user specified.
/// - token transfer is optional and has variable size fields.
pub const MESSAGE_V1_SVM_SOURCE_BASE_SIZE: usize = MESSAGE_V1_BASE_SIZE + 32 + 32;
/// Number of destination-chain-sized addresses in the message envelope (receiver, offRamp).
pub const MESSAGE_V1_REMOTE_CHAIN_ADDRESSES: usize = 2;
/// TOKEN_TRANSFER_V1_BASE_SIZE plus the two source-side addresses (sourcePool, sourceToken),
/// which are native 32-byte pubkeys when SVM is the source chain.
/// To be added:
/// - destToken and tokenReceiver are dest chain specific.
/// - extraData is a variable length field that is billed separately (token pool receipt).
pub const TOKEN_TRANSFER_V1_SVM_SOURCE_BASE_SIZE: usize = TOKEN_TRANSFER_V1_BASE_SIZE + 32 + 32;

// There is no trait for `::MAX` so we just use a quick macro
macro_rules! push {
    // micro optimization: push instead of extend_from_slice(to_be_bytes) for u8
    ($buf:ident, $value:expr, u8) => {{
        let len = $value.len();
        if len > u8::MAX.into() {
            return Err(Error::InvalidLength);
        }
        // SAFETY: len <= u8::MAX checked above
        $buf.push(len as u8);
        $buf.extend_from_slice($value);
    }};
    ($buf:ident, $value:expr, $typ:tt) => {{
        let len = $value.len();
        if len > $typ::MAX.into() {
            return Err(Error::InvalidLength);
        }
        // $buf.push(len as $typ);
        $buf.extend_from_slice(&(len as $typ).to_be_bytes());
        $buf.extend_from_slice($value);
    }};
}

// There is no from_be_bytes trait so we just use a quick macro
macro_rules! take {
    ($this:ident, $typ:tt) => {{
        const SIZE: usize = std::mem::size_of::<$typ>();
        let (buf, bytes) = $this
            .split_first_chunk::<SIZE>()
            .ok_or(Error::InvalidLength)?;
        let value = $typ::from_be_bytes(*buf);
        (value, bytes)
    }};
}

macro_rules! tlv {
    ($this:ident, $typ:tt) => {{
        let (len, bytes) = take!($this, $typ);
        let (value, bytes) = bytes
            .split_at_checked(len.into())
            .ok_or(Error::InvalidLength)?;
        (value, bytes)
    }};
}

#[derive(Clone, Copy, PartialEq, Eq, Debug)]
pub enum Error {
    InvalidLength,
    InvalidEncodingVersion,
    InvalidEncodingTokenReceiverUsage,
    BorshEncodeError,
}

impl MessageV1 {
    pub fn compute_message_id(&self) -> [u8; 32] {
        let bytes = self.to_bytes().unwrap();
        keccak::hash(&bytes).to_bytes()
    }
    pub fn compute_message_id_from_bytes(bytes: &[u8]) -> [u8; 32] {
        keccak::hash(bytes).to_bytes()
    }

    pub fn from_bytes(mut bytes: &[u8]) -> std::result::Result<Self, Error> {
        if bytes.len() < MESSAGE_V1_BASE_SIZE {
            return Err(Error::InvalidLength);
        }

        let version = bytes[0];
        bytes = &bytes[1..];

        if version != MESSAGE_V1_VERSION {
            return Err(Error::InvalidEncodingVersion);
        }

        let (source_chain_selector, bytes) = take!(bytes, u64);
        let (dest_chain_selector, bytes) = take!(bytes, u64);
        let (message_number, bytes) = take!(bytes, u64);
        let (execution_gas_limit, bytes) = take!(bytes, u32);
        let (ccip_receive_gas_limit, bytes) = take!(bytes, u32);
        let (flags, bytes) = take!(bytes, u16);
        let (block_depth, bytes) = take!(bytes, u16);
        let finality = FinalityConfig { flags, block_depth };
        let (ccv_and_executor_hash, bytes) = bytes
            .split_first_chunk::<32>()
            .ok_or(Error::InvalidLength)?;

        let (on_ramp_address, bytes) = tlv!(bytes, u8);
        let (off_ramp_address, bytes) = tlv!(bytes, u8);
        let (sender, bytes) = tlv!(bytes, u8);
        let (receiver, bytes) = tlv!(bytes, u8);
        let (dest_blob, bytes) = tlv!(bytes, u16);

        // token_transfer
        let (len, mut bytes) = take!(bytes, u16);
        let mut token_transfer = None;
        if len > 0 {
            let (encoded, rest) = bytes
                .split_at_checked(len.into())
                .ok_or(Error::InvalidLength)?;
            token_transfer = Some(TokenTransferV1::from_bytes(encoded)?);
            bytes = rest;
        }

        // data
        let (data, bytes) = tlv!(bytes, u16);

        // The encoding must be canonical: trailing bytes past the final TLV would let distinct
        // byte strings decode to the same message, so reject them (as `SVMDestBlob` does).
        if !bytes.is_empty() {
            return Err(Error::InvalidLength);
        }

        Ok(Self {
            source_chain_selector,
            dest_chain_selector,
            message_number,
            execution_gas_limit: CrossChainGas::new(execution_gas_limit),
            ccip_receive_gas_limit: CrossChainGas::new(ccip_receive_gas_limit),
            finality,
            ccv_and_executor_hash: *ccv_and_executor_hash,
            on_ramp_address: on_ramp_address.to_vec(),
            off_ramp_address: off_ramp_address.to_vec(),
            sender: sender.to_vec(),
            receiver: receiver.to_vec(),
            dest_blob: dest_blob.to_vec(),
            token_transfer,
            data: data.to_vec(),
        })
    }

    pub fn to_bytes(&self) -> std::result::Result<Vec<u8>, Error> {
        // The token transfer is encoded first so the buffer below can be sized exactly. Growing it
        // instead would leave the discarded intermediate buffers occupying the program heap, which
        // is a bump allocator that never frees: the onramp send path encodes a message while
        // already holding most of its 32KB.
        let encoded_token_transfer = match &self.token_transfer {
            Some(token_transfer) => Some(token_transfer.to_bytes()?),
            None => None,
        };

        let mut buf = Vec::with_capacity(
            MESSAGE_V1_BASE_SIZE
                + self.on_ramp_address.len()
                + self.off_ramp_address.len()
                + self.sender.len()
                + self.receiver.len()
                + self.dest_blob.len()
                + encoded_token_transfer.as_ref().map_or(0, Vec::len)
                + self.data.len(),
        );
        buf.push(MESSAGE_V1_VERSION);
        buf.extend_from_slice(&self.source_chain_selector.to_be_bytes());
        buf.extend_from_slice(&self.dest_chain_selector.to_be_bytes());
        buf.extend_from_slice(&self.message_number.to_be_bytes());
        buf.extend_from_slice(&self.execution_gas_limit.convert::<u32>().to_be_bytes());
        buf.extend_from_slice(&self.ccip_receive_gas_limit.convert::<u32>().to_be_bytes());
        buf.extend_from_slice(&self.finality.flags.to_be_bytes());
        buf.extend_from_slice(&self.finality.block_depth.to_be_bytes());
        buf.extend_from_slice(&self.ccv_and_executor_hash);

        push!(buf, &self.on_ramp_address, u8);
        push!(buf, &self.off_ramp_address, u8);
        push!(buf, &self.sender, u8);
        push!(buf, &self.receiver, u8);
        push!(buf, &self.dest_blob, u16);

        if let Some(encoded) = &encoded_token_transfer {
            push!(buf, encoded, u16);
        } else {
            buf.extend_from_slice(&0u16.to_be_bytes());
        }

        push!(buf, &self.data, u16);
        Ok(buf)
    }
}

pub fn compute_ccv_and_executor_hash(ccv_addresses: &[Pubkey], executor: &Pubkey) -> [u8; 32] {
    let mut buf = Vec::with_capacity(1 + (ccv_addresses.len() + 1) * 32);
    buf.push(32); // 32 byte address length
    for addr in ccv_addresses {
        buf.extend_from_slice(&addr.to_bytes())
    }
    buf.extend_from_slice(&executor.to_bytes());
    keccak::hash(&buf).to_bytes()
}

/// CCIP 2.0 cross-chain TokenTransferV1. Chain-agnostic wire format.
#[derive(Clone, AnchorSerialize, AnchorDeserialize, PartialEq, Eq, Debug)]
pub struct TokenTransferV1 {
    // Must be 1: it's redundant with the type but exists to facilitate parsing.
    pub version: u8,
    pub amount: ProtocolAmount,
    pub source_pool_address: Vec<u8>,
    pub source_token_address: Vec<u8>,
    pub dest_token_address: Vec<u8>,
    pub token_receiver: Vec<u8>,
    pub extra_data: Vec<u8>,
}

impl TokenTransferV1 {
    pub fn from_bytes(mut bytes: &[u8]) -> std::result::Result<Self, Error> {
        if bytes.len() < TOKEN_TRANSFER_V1_BASE_SIZE {
            return Err(Error::InvalidLength);
        }

        let version = bytes[0];
        bytes = &bytes[1..];

        if version != MESSAGE_V1_VERSION {
            return Err(Error::InvalidEncodingVersion);
        }

        let (amount, bytes) = bytes
            .split_first_chunk::<32>()
            .ok_or(Error::InvalidLength)?;

        let (source_pool_address, bytes) = tlv!(bytes, u8);
        let (source_token_address, bytes) = tlv!(bytes, u8);
        let (dest_token_address, bytes) = tlv!(bytes, u8);
        let (token_receiver, bytes) = tlv!(bytes, u8);
        let (extra_data, bytes) = tlv!(bytes, u16);

        // As in `MessageV1::from_bytes`: no trailing bytes inside the length-delimited segment.
        if !bytes.is_empty() {
            return Err(Error::InvalidLength);
        }

        Ok(Self {
            version,
            amount: ProtocolAmount::from_be_bytes(*amount),
            source_pool_address: source_pool_address.to_vec(),
            source_token_address: source_token_address.to_vec(),
            dest_token_address: dest_token_address.to_vec(),
            token_receiver: token_receiver.to_vec(),
            extra_data: extra_data.to_vec(),
        })
    }

    pub fn to_bytes(&self) -> std::result::Result<Vec<u8>, Error> {
        // Sized exactly, as in `MessageV1::to_bytes`: `size_of` measures the struct (pointers and
        // all), not the encoding, so it under-sizes the buffer and forces it to grow.
        let mut buf = Vec::with_capacity(
            TOKEN_TRANSFER_V1_BASE_SIZE
                + self.source_pool_address.len()
                + self.source_token_address.len()
                + self.dest_token_address.len()
                + self.token_receiver.len()
                + self.extra_data.len(),
        );
        buf.push(self.version);
        buf.extend_from_slice(&self.amount.to_be_bytes());
        push!(buf, &self.source_pool_address, u8);
        push!(buf, &self.source_token_address, u8);
        push!(buf, &self.dest_token_address, u8);
        push!(buf, &self.token_receiver, u8);
        push!(buf, &self.extra_data, u16);
        Ok(buf)
    }
}

/// CCIP 2.0 cross-chain DestBlob when SVM is the destination. Can be borsh-encoded or
/// `abi.encodePacked`-encoded. The destBlob is equal to the executorArgs.
#[derive(Clone, AnchorSerialize, AnchorDeserialize, PartialEq, Eq, Debug)]
pub struct SVMDestBlob {
    pub use_ata: SVMTokenReceiverUsage,
    pub account_is_writable_bitmap: u64,
    pub accounts: Vec<Pubkey>,
}

impl SVMDestBlob {
    // Base size: tag(4) + useATA(1) + accountIsWritableBitmap(8) + accountsLength(1) = 14 bytes.
    const SVM_EXECUTOR_ARGS_V1_BASE_SIZE: usize = 4 + 1 + 8 + 1;
    // Same fields, but borsh length-prefixes the accounts vec with a u32 instead of a u8:
    // tag(4) + useATA(1) + accountIsWritableBitmap(8) + accountsLength(4) = 17 bytes.
    const SVM_EXECUTOR_ARGS_V1_BORSH_BASE_SIZE: usize = 4 + 1 + 8 + 4;

    const SVM_EXECUTOR_ARGS_V1_TAG: u32 = 0x1a2b3c4d;
    const SVM_EXECUTOR_ARGS_V1_TAG_BE: [u8; 4] = Self::SVM_EXECUTOR_ARGS_V1_TAG.to_be_bytes();
    const SVM_EXECUTOR_ARGS_V1_TAG_LE: [u8; 4] = Self::SVM_EXECUTOR_ARGS_V1_TAG.to_le_bytes();

    pub fn to_bytes(&self, tag: [u8; 4]) -> std::result::Result<Vec<u8>, Error> {
        match tag {
            Self::SVM_EXECUTOR_ARGS_V1_TAG_BE => self.legacy_abi_encode_packed(),
            Self::SVM_EXECUTOR_ARGS_V1_TAG_LE => self.borsh_encode(),
            _ => Err(Error::InvalidEncodingVersion),
        }
    }

    pub fn from_bytes(bytes: &[u8]) -> std::result::Result<Self, Error> {
        if bytes.len() < Self::SVM_EXECUTOR_ARGS_V1_BASE_SIZE {
            return Err(Error::InvalidLength);
        }

        // SAFETY: `bytes.len() >= SVM_EXECUTOR_ARGS_V1_BASE_SIZE` (checked above), and that base
        // size begins with the 4 tag bytes, so this slice is exactly 4 bytes and the conversion to
        // `[u8; 4]` is infallible.
        let tag: [u8; 4] = bytes[..4].try_into().unwrap();
        let remaining_bytes = &bytes[4..];

        match tag {
            Self::SVM_EXECUTOR_ARGS_V1_TAG_BE => Self::legacy_abi_decode_packed(remaining_bytes),
            Self::SVM_EXECUTOR_ARGS_V1_TAG_LE => Self::borsh_decode(remaining_bytes),
            _ => Err(Error::InvalidEncodingVersion),
        }
    }

    fn legacy_abi_encode_packed(&self) -> std::result::Result<Vec<u8>, Error> {
        let mut buf = Vec::with_capacity(
            Self::SVM_EXECUTOR_ARGS_V1_BASE_SIZE + self.accounts.len() * size_of::<Pubkey>(),
        );
        buf.extend_from_slice(&Self::SVM_EXECUTOR_ARGS_V1_TAG_BE);
        // SAFETY: `SVMTokenReceiverUsage` is a fieldless three-variant enum, so its discriminant is
        // 0..=2 and always fits in a `u8`.
        buf.push(self.use_ata as u8);
        buf.extend_from_slice(&self.account_is_writable_bitmap.to_be_bytes());

        let accounts_len: u8 = self
            .accounts
            .len()
            .try_into()
            .map_err(|_| Error::InvalidLength)?;
        buf.push(accounts_len);

        for account in &self.accounts {
            buf.extend_from_slice(account.as_ref());
        }

        Ok(buf)
    }

    fn legacy_abi_decode_packed(mut remaining_bytes: &[u8]) -> std::result::Result<Self, Error> {
        // Taking byte slices directly without checking the length, as the caller already checks the minimum length
        // of the entire input. As this is a private function and the public entrypoint checks the length, it's safe.

        let use_ata = SVMTokenReceiverUsage::try_from(remaining_bytes[0])?;
        remaining_bytes = &remaining_bytes[1..];

        // SAFETY: the public entrypoint checked `len >= SVM_EXECUTOR_ARGS_V1_BASE_SIZE`
        // (4 tag + 1 usage + 8 bitmap + 1 accounts_len), so with the tag and usage byte already
        // consumed at least 8 bytes remain and the conversion to `[u8; 8]` is infallible.
        let bitmap_bytes: [u8; 8] = remaining_bytes[..8].try_into().unwrap();
        remaining_bytes = &remaining_bytes[8..];
        let account_is_writable_bitmap = u64::from_be_bytes(bitmap_bytes);

        let accounts_len = remaining_bytes[0] as usize;
        remaining_bytes = &remaining_bytes[1..];

        if remaining_bytes.len() != accounts_len * size_of::<Pubkey>() {
            return Err(Error::InvalidLength);
        }

        let mut accounts = Vec::with_capacity(accounts_len);
        for _ in 0..accounts_len {
            let (account_bytes, rest) = remaining_bytes
                .split_first_chunk::<32>()
                .ok_or(Error::InvalidLength)?;
            accounts.push(Pubkey::new_from_array(*account_bytes));
            remaining_bytes = rest;
        }

        Ok(Self {
            use_ata,
            account_is_writable_bitmap,
            accounts,
        })
    }

    fn borsh_encode(&self) -> std::result::Result<Vec<u8>, Error> {
        // Sized exactly, like `MessageV1::to_bytes`: Using `size_of` directly would have measured the struct (pointers and all), not the encoding, under-sizing the buffer and forcing it to grow.
        let mut buf = Vec::with_capacity(
            Self::SVM_EXECUTOR_ARGS_V1_BORSH_BASE_SIZE + self.accounts.len() * size_of::<Pubkey>(),
        );
        buf.extend_from_slice(&Self::SVM_EXECUTOR_ARGS_V1_TAG_LE);
        borsh::to_writer(&mut buf, &self).map_err(|_| Error::BorshEncodeError)?;
        Ok(buf)
    }

    fn borsh_decode(mut remaining_bytes: &[u8]) -> std::result::Result<Self, Error> {
        let result =
            SVMDestBlob::deserialize(&mut remaining_bytes).map_err(|_| Error::BorshEncodeError);

        if result.is_ok() && !remaining_bytes.is_empty() {
            // enforce that all bytes were consumed
            return Err(Error::InvalidLength);
        }

        result
    }
}

#[derive(Copy, Clone, AnchorSerialize, AnchorDeserialize, PartialEq, Eq, Debug)]
pub enum SVMTokenReceiverUsage {
    DeriveAtaAndCreate,
    DeriveAtaDontCreate,
    UseAsIs,
}

impl SVMTokenReceiverUsage {
    pub fn should_create_ata(&self) -> bool {
        matches!(self, Self::DeriveAtaAndCreate)
    }

    pub fn should_derive_ata(&self) -> bool {
        !self.should_use_as_is()
    }

    pub fn should_use_as_is(&self) -> bool {
        matches!(self, Self::UseAsIs)
    }
}

impl TryFrom<u8> for SVMTokenReceiverUsage {
    type Error = Error;

    fn try_from(value: u8) -> std::result::Result<Self, Self::Error> {
        match value {
            0 => Ok(Self::DeriveAtaAndCreate),
            1 => Ok(Self::DeriveAtaDontCreate),
            2 => Ok(Self::UseAsIs),
            _ => Err(Error::InvalidEncodingTokenReceiverUsage),
        }
    }
}

impl Display for SVMTokenReceiverUsage {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        match self {
            Self::DeriveAtaAndCreate => write!(f, "DeriveAtaAndCreate"),
            Self::DeriveAtaDontCreate => write!(f, "DeriveAtaDontCreate"),
            Self::UseAsIs => write!(f, "UseAsIs"),
        }
    }
}

/// User-facing message struct for sending cross-chain messages from SVM.
#[derive(Clone, Debug, PartialEq, AnchorSerialize, AnchorDeserialize)]
pub struct SVM2AnyMessage {
    pub receiver: Vec<u8>,
    pub data: Vec<u8>,
    pub token_amounts: Vec<SVMTokenAmount>,
    pub fee_token: Pubkey, // pass zero address if native SOL
    pub extra_args: Vec<u8>,
}

#[derive(Clone, AnchorSerialize, AnchorDeserialize, Default, Debug, PartialEq, Eq)]
pub struct SVMTokenAmount {
    pub token: Pubkey,
    pub amount: u64, // u64 - amount local to solana
}

pub trait NormalizeWsol {
    /// Normalizes an address passed in as a token mint: if Pubkey::default it becomes
    /// the native mint, otherwise passes as-is.
    fn normalize_wsol(self) -> Pubkey;
}

impl NormalizeWsol for Pubkey {
    // Pre-2022 WSOL
    fn normalize_wsol(self) -> Pubkey {
        if self == Pubkey::default() {
            native_mint::ID
        } else {
            self
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    fn mock() -> MessageV1 {
        let mut amount_bytes = [0u8; 32];
        amount_bytes[31] = 42;

        MessageV1 {
            source_chain_selector: 123,
            dest_chain_selector: 456,
            message_number: 789,
            execution_gas_limit: CrossChainGas::new(400000),
            ccip_receive_gas_limit: CrossChainGas::new(0),
            finality: FinalityConfig {
                flags: 0,
                block_depth: 2000,
            },
            ccv_and_executor_hash: [0; 32],
            on_ramp_address: vec![0x11; 20],
            off_ramp_address: vec![0x22; 20],
            sender: vec![0x33; 20],
            receiver: vec![0x44; 20],
            dest_blob: b"complex destination blob".to_vec(),
            token_transfer: Some(TokenTransferV1 {
                version: 1,
                amount: ProtocolAmount::from_be_bytes(amount_bytes),
                source_pool_address: vec![0x44; 20],
                source_token_address: vec![0x55; 20],
                dest_token_address: vec![0x66; 20],
                token_receiver: vec![0x77; 20],
                extra_data: b"token extra data".to_vec(),
            }),
            data: b"message with token data".to_vec(),
        }
    }

    /// Both "no receiver" encodings — a zero-length address and the all-zero 32-byte one — must
    /// classify as token-only. Reading the empty form as a data message would make `execute_v2`
    /// reject a message that account resolution (which keys off `receiver.is_empty()`) reported as
    /// executable, stranding the tokens it carries.
    #[test]
    fn empty_and_zero_receivers_are_not_arbitrary_messages() {
        let mut msg = mock();

        msg.receiver = vec![];
        assert!(!msg.is_offramp_arbitrary_message());
        assert!(msg.is_token_transfer());

        msg.receiver = Pubkey::default().to_bytes().to_vec();
        assert!(!msg.is_offramp_arbitrary_message());
        assert!(msg.is_token_transfer());
    }

    #[test]
    fn non_zero_receiver_is_an_arbitrary_message() {
        let mut msg = mock();

        // A real 32-byte SVM receiver, and a shorter remote-chain one: both carry a receiver.
        // `mock()` has data, so neither is downgraded to a token-only transfer.
        msg.receiver = vec![0x44; 32];
        assert!(msg.is_offramp_arbitrary_message());

        msg.receiver = vec![0x44; 20];
        assert!(msg.is_offramp_arbitrary_message());
    }

    #[test]
    fn roundtrip() {
        let msg = mock();
        let encoded = msg.to_bytes().unwrap();
        let roundtrip = MessageV1::from_bytes(&encoded);
        assert_eq!(Ok(msg), roundtrip);
    }

    #[test]
    fn message_id() {
        let msg = mock();
        let id = msg.compute_message_id();
        assert_eq!(
            id.as_slice(),
            hex::decode("44d5d1d00d505307fb345b41f9a1c5dee7d944f938a9543edfef270b39f55c9a")
                .unwrap()
        )
    }

    /// `mock()` with the three fields the token-only predicate reads set explicitly.
    fn msg_for(receiver: Vec<u8>, data: &[u8], ccip_receive_gas_limit: u32) -> MessageV1 {
        MessageV1 {
            receiver,
            data: data.to_vec(),
            ccip_receive_gas_limit: CrossChainGas::new(ccip_receive_gas_limit),
            ..mock()
        }
    }

    fn zero_receiver() -> Vec<u8> {
        Pubkey::default().to_bytes().to_vec()
    }

    fn some_receiver() -> Vec<u8> {
        Pubkey::new_unique().to_bytes().to_vec()
    }

    #[test]
    fn receiver_with_data_is_arbitrary() {
        assert!(msg_for(some_receiver(), b"payload", 0).is_offramp_arbitrary_message());
    }

    #[test]
    fn receiver_with_callback_gas_is_arbitrary() {
        assert!(msg_for(some_receiver(), b"", 50_000).is_offramp_arbitrary_message());
        assert!(msg_for(some_receiver(), b"payload", 50_000).is_offramp_arbitrary_message());
    }

    #[test]
    fn receiver_without_data_or_callback_gas_is_token_only() {
        assert!(!msg_for(some_receiver(), b"", 0).is_offramp_arbitrary_message());
    }

    #[test]
    fn zero_receiver_is_never_arbitrary() {
        assert!(!msg_for(zero_receiver(), b"payload", 50_000).is_offramp_arbitrary_message());
    }

    #[test]
    fn a_message_carrying_nothing_at_all_is_not_arbitrary() {
        let msg = MessageV1 {
            token_transfer: None,
            ..msg_for(some_receiver(), b"", 0)
        };
        assert!(!msg.is_offramp_arbitrary_message());
    }

    #[test]
    fn message_with_trailing_bytes_rejected() {
        let mut encoded = mock().to_bytes().unwrap();
        encoded.push(0); // one byte past the final data TLV
        assert_eq!(MessageV1::from_bytes(&encoded), Err(Error::InvalidLength));
    }

    #[test]
    fn token_transfer_with_trailing_bytes_rejected() {
        let mut encoded = mock().token_transfer.unwrap().to_bytes().unwrap();
        encoded.push(0); // one byte past the final extra_data TLV
        assert_eq!(
            TokenTransferV1::from_bytes(&encoded),
            Err(Error::InvalidLength)
        );
    }

    /// Padding inside the message's length-delimited token transfer segment is rejected too: the
    /// outer message stays well-formed, so only the inner decoder can catch it.
    #[test]
    fn message_with_padded_token_transfer_rejected() {
        let msg = mock();
        let mut encoded = msg.to_bytes().unwrap();

        // Fixed header, then the four u8-prefixed addresses and the u16-prefixed destBlob, leaves
        // the cursor on the token transfer's own u16 length prefix.
        let offset = 69
            + (1 + msg.on_ramp_address.len())
            + (1 + msg.off_ramp_address.len())
            + (1 + msg.sender.len())
            + (1 + msg.receiver.len())
            + (2 + msg.dest_blob.len());

        let len = u16::from_be_bytes(encoded[offset..offset + 2].try_into().unwrap());
        assert_eq!(
            len as usize,
            msg.token_transfer.unwrap().to_bytes().unwrap().len(),
            "offset should land on the token transfer length prefix"
        );

        encoded.insert(offset + 2 + len as usize, 0);
        encoded[offset..offset + 2].copy_from_slice(&(len + 1).to_be_bytes());

        assert_eq!(MessageV1::from_bytes(&encoded), Err(Error::InvalidLength));
    }

    fn test_blob() -> SVMDestBlob {
        SVMDestBlob {
            use_ata: SVMTokenReceiverUsage::DeriveAtaAndCreate,
            account_is_writable_bitmap: 0b101,
            accounts: vec![
                Pubkey::new_from_array([0x11; 32]),
                Pubkey::new_from_array([0x22; 32]),
                Pubkey::new_from_array([0x33; 32]),
            ],
        }
    }

    #[test]
    fn dest_blob_borsh() {
        let serialized = test_blob()
            .borsh_encode()
            .expect("borsh encoding should succeed");

        assert_eq!(
            serialized,
            [
                &[
                    0x4d, 0x3c, 0x2b, 0x1a, // tag
                    0x00, // use_ata
                    0b101, 0, 0, 0, 0, 0, 0, 0, // account_is_writable_bitmap
                    3, 0, 0, 0, // accounts length (vec prefix, u32le)
                ][..],
                // accounts:
                &[0x11; 32],
                &[0x22; 32],
                &[0x33; 32],
            ]
            .concat()
        );
    }

    #[test]
    fn dest_blob_borsh_decode_with_extra_bytes() {
        let mut encoded = test_blob().borsh_encode().unwrap();
        encoded.push(0); // extra byte that should be ignored
        let result = SVMDestBlob::from_bytes(&encoded);
        assert!(result.is_err());
    }

    #[test]
    fn dest_blob_borsh_roundtrip() {
        let blob = test_blob();
        let encoded = blob.borsh_encode().unwrap();
        let roundtrip = SVMDestBlob::from_bytes(&encoded).unwrap();
        assert_eq!(blob, roundtrip);
    }

    #[test]
    fn dest_blob_abi() {
        let serialized = test_blob()
            .legacy_abi_encode_packed()
            .expect("abi encode-packed should succeed");

        assert_eq!(
            serialized,
            [
                &[
                    0x1a, 0x2b, 0x3c, 0x4d, // tag
                    0x00, // use_ata
                    0, 0, 0, 0, 0, 0, 0, 0b101, // account_is_writable_bitmap
                    3,     // accounts length (u8)
                ][..],
                // accounts:
                &[0x11; 32],
                &[0x22; 32],
                &[0x33; 32],
            ]
            .concat()
        )
    }

    #[test]
    fn dest_blob_abi_roundtrip() {
        let blob = test_blob();
        let encoded = blob.legacy_abi_encode_packed().unwrap();
        let roundtrip = SVMDestBlob::from_bytes(&encoded).unwrap();
        assert_eq!(blob, roundtrip);
    }

    /// The writability bitmap is a `u64`, so the widest account list either encoding ever has to
    /// carry is `SVM_EXTRA_ARGS_MAX_ACCOUNTS`. At that size every bit is meaningful, including the
    /// top one (index 63), and it must survive both round trips untruncated. Accounts repeat
    /// because a real message reaching the maximum typically does so by resending the same account.
    #[test]
    fn dest_blob_max_accounts_roundtrip() {
        let blob = SVMDestBlob {
            use_ata: SVMTokenReceiverUsage::UseAsIs,
            account_is_writable_bitmap: u64::MAX, // every account writable, up to index 63
            accounts: vec![
                Pubkey::new_from_array([0x44; 32]);
                crate::extra_args::SVM_EXTRA_ARGS_MAX_ACCOUNTS
            ],
        };

        for encoded in [
            blob.borsh_encode().unwrap(),
            blob.legacy_abi_encode_packed().unwrap(),
        ] {
            let roundtrip = SVMDestBlob::from_bytes(&encoded).unwrap();
            assert_eq!(blob, roundtrip);
            assert_eq!(
                roundtrip.accounts.len(),
                crate::extra_args::SVM_EXTRA_ARGS_MAX_ACCOUNTS
            );
            // The bit describing the last account (index 63) is the one a too-narrow bitmap type
            // or an off-by-one length would silently drop, so assert it explicitly.
            assert_ne!(roundtrip.account_is_writable_bitmap & (1 << 63), 0);
        }
    }

    /// The abi-packed encoding writes the account count into a single byte, so a list that does not
    /// fit must be rejected rather than wrapping around to a short (and differently-interpreted)
    /// blob. Such a list is already rejected earlier by the 64-account limit; this guards the
    /// encoding itself.
    #[test]
    fn dest_blob_abi_rejects_account_count_beyond_a_byte() {
        let blob = SVMDestBlob {
            use_ata: SVMTokenReceiverUsage::UseAsIs,
            account_is_writable_bitmap: 0,
            accounts: vec![Pubkey::new_from_array([0x55; 32]); 256],
        };

        assert_eq!(blob.legacy_abi_encode_packed(), Err(Error::InvalidLength));
    }
}
