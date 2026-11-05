use anchor_discriminator::anchor_discriminator;
use anchor_lang::prelude::*;

use crate::CcipOfframpError;

pub use ccip_common::messages::{SVMTokenAmount, SVMTokenReceiverUsage};

pub const TOKENPOOL_RELEASE_OR_MINT_DISCRIMINATOR: [u8; 8] =
    anchor_discriminator!("release_or_mint_tokens");
pub const CCIP_RECEIVE_DISCRIMINATOR: [u8; 8] = anchor_discriminator!("ccip_receive");
pub use ccip_common::ccvs::CCIP_RECEIVE_V2 as CCIP_RECEIVE_V2_DISCRIMINATOR;

/// Message delivered to a receiver program via the `ccip_receive` CPI. The
/// layout must match the v1 receiver interface's `Any2SVMMessage` byte-for-byte.
#[derive(Clone, AnchorSerialize, AnchorDeserialize, Debug)]
pub struct Any2SVMMessage {
    pub message_id: [u8; 32],
    pub source_chain_selector: u64,
    pub sender: Vec<u8>,
    pub data: Vec<u8>,
    // Must stay a `Vec` to match the receiver's `ccip_receive` interface byte-for-byte, even
    // though a v2 message carries at most one token transfer (so it holds 0 or 1 element).
    pub token_amounts: Vec<SVMTokenAmount>,
}

impl Any2SVMMessage {
    /// Build the `ccip_receive` instruction data: the Anchor discriminator followed by the
    /// Borsh-serialized message. The message is serialized directly into the discriminator-prefixed
    /// buffer (a single allocation) rather than via `borsh::to_vec` + copy.
    pub fn build_receiver_discriminator_and_data(&self) -> Result<Vec<u8>> {
        // Capacity is a hint: discriminator + message_id + source_chain_selector + the two Vec
        // length prefixes + one token amount, plus the variable-length fields. A miss only costs a
        // realloc, not correctness, so this does not couple us to the exact Borsh layout.
        let mut data = Vec::with_capacity(
            CCIP_RECEIVE_DISCRIMINATOR.len() + 96 + self.sender.len() + self.data.len(),
        );
        data.extend_from_slice(&CCIP_RECEIVE_DISCRIMINATOR);
        self.serialize(&mut data)
            .map_err(|_| CcipOfframpError::InvalidMessage)?;

        Ok(data)
    }
}

/// The single (optional) token transfer carried by a v2 [`Any2SVMMessageV2`].
#[derive(Clone, AnchorSerialize, AnchorDeserialize, Debug)]
pub struct TokenTransfer {
    pub token_amount: SVMTokenAmount,
    /// The account the OffRamp credited, interpreted per `token_receiver_usage`: with `UseAsIs` it
    /// is the token account directly; otherwise it is the wallet the ATA is derived from (with
    /// mint = `token_amount.token`).
    pub token_receiver: Pubkey,
    pub token_receiver_usage: SVMTokenReceiverUsage,
}

/// Message delivered to a v2 receiver via the `ccip_receive_v2` CPI. The layout must match the v2
/// receiver interface's `Any2SVMMessage` byte-for-byte — in particular `token_transfer` is an
/// `Option` (a v2 message carries at most one token transfer), NOT the v1 `Vec`, so this is not
/// Borsh-compatible with [`Any2SVMMessage`]. Unlike v1's `token_amounts` (bare `SVMTokenAmount`s),
/// each v2 transfer also carries the `token_receiver` + usage (see [`TokenTransfer`]).
#[derive(Clone, AnchorSerialize, AnchorDeserialize, Debug)]
pub struct Any2SVMMessageV2 {
    pub message_id: [u8; 32],
    pub source_chain_selector: u64,
    pub sender: Vec<u8>,
    pub data: Vec<u8>,
    pub token_transfer: Option<TokenTransfer>,
}

impl Any2SVMMessageV2 {
    /// Build the `ccip_receive_v2` instruction data: the v2 discriminator followed by the
    /// Borsh-serialized message.
    pub fn build_receiver_discriminator_and_data(&self) -> Result<Vec<u8>> {
        let mut data = Vec::with_capacity(
            CCIP_RECEIVE_V2_DISCRIMINATOR.len()
            + 49 // fixed size for fixed fields and vec/option prefixes
            + self.sender.len()
            + self.data.len()
            + if self.token_transfer.is_none() { 0 } else { 73 /*TokenTransfer size*/ },
        );
        data.extend_from_slice(&CCIP_RECEIVE_V2_DISCRIMINATOR);
        self.serialize(&mut data)
            .map_err(|_| CcipOfframpError::InvalidMessage)?;

        Ok(data)
    }
}

#[derive(Clone, AnchorSerialize, AnchorDeserialize, Debug)]
pub struct ExecutionInputsV2 {
    pub encoded_message: Vec<u8>,
    pub ccvs: Vec<Pubkey>,
    pub verifier_results: Vec<Vec<u8>>,
}

#[derive(Clone, AnchorSerialize, AnchorDeserialize, Debug, PartialEq, Eq)]
pub struct OfframpAccountListLengths {
    // Ordering here matters! It has to be consistent with the account resolution, and the Borsh
    // layout below is the wire format carried across resolution rounds — reordering these fields
    // silently repoints every segment, so `borsh_layout_ordering_is_stable` locks it down.
    pub buffer: u8,
    pub messaging: u8,
    pub token_transfer: u8,
    // Accounts for the receiver's `get_ccvs_and_finality_config` CPI (CCIP v2). Unlike the token
    // pool's `get_required_ccvs` (see note below), the receiver's per-source-chain config PDA is
    // NOT part of any other validated account set, so it gets its own dedicated segment. Laid out
    // as `[receiver_program, source_chain_ccv_config]`; 0 for a v1 receiver or a token-only transfer.
    pub receiver_ccv: u8,
    // The router-owned `ReceiverRegistry` PDA. The OffRamp reads it to select the v1 vs v2 receiver flow; an
    // absent (System-owned) account means v1.
    pub receiver_registry: u8,
    pub ccvs: Vec<u8>,
    // DESIGN NOTE (non-obvious): there is intentionally NO separate segment for the token pool's
    // `get_required_ccvs` CPI. The 4 accounts that CPI needs
    // (`[pool_program, pool_config, pool_chain_config, chain_config_v2]`) are all already resolved AND
    // mint-validated as part of the `token_transfer` set: `validate_and_parse_offramp_token_accounts_v2`
    // (via `OfframpTokenAccountsValidationContext`) PDA-checks `chain_config_v2` as
    // `POOL_CHAINCONFIG_SEED_2_0` under the canonical (LUT-derived, mint-bound) pool program. They are
    // taken from there at execution time via
    // `CcipV2OfframpOnrampCommonTokenAccounts::get_pool_required_ccvs_accounts`, so a permissionless
    // executor cannot substitute a pool that under-reports CCVs. (`release_or_mint` simply does not
    // pass `chain_config_v2` to its own CPI — pool's `TokenOfframpV2` has no such account — but
    // the OffRamp still validates it here because `get_required_ccvs` consumes it.) A dedicated
    // segment would just duplicate an already-validated account, so it is omitted.
}

impl OfframpAccountListLengths {
    pub fn total(&self) -> usize {
        self.buffer as usize
            + self.messaging as usize
            + self.token_transfer as usize
            + self.receiver_ccv as usize
            + self.receiver_registry as usize
            + self.ccvs.iter().map(|&len| len as usize).sum::<usize>()
    }

    pub fn validate_with(&self, accounts: &[AccountInfo]) -> Result<()> {
        require_eq!(
            self.total(),
            accounts.len(),
            CcipOfframpError::InvalidAccountListLengths
        );
        require_gte!(1, self.buffer, CcipOfframpError::InvalidAccountListLengths);
        require_gte!(
            1,
            self.receiver_registry,
            CcipOfframpError::InvalidAccountListLengths
        );
        Ok(())
    }
}

pub use ccip_common::v2::CrossChainAmount;

#[cfg(feature = "idl-build")]
pub use ccip_common::v2::ProtocolAmount;

#[cfg(test)]
mod tests {
    use super::*;

    /// The Borsh layout is the wire format the resolution metadata carries from one round to the
    /// next, and every segment is located by its position in it. Reordering the struct fields would
    /// keep everything compiling while silently repointing every segment, so pin the order here.
    #[test]
    fn borsh_layout_ordering_is_stable() {
        let lengths = OfframpAccountListLengths {
            buffer: 1,
            messaging: 2,
            token_transfer: 3,
            receiver_ccv: 4,
            receiver_registry: 5,
            ccvs: vec![6, 7],
        };

        // Five single-byte fields in declaration order, then `ccvs` as a Borsh vector (u32
        // little-endian length prefix followed by its bytes).
        let mut encoded = Vec::new();
        lengths.serialize(&mut encoded).unwrap();
        assert_eq!(encoded, vec![1, 2, 3, 4, 5, 2, 0, 0, 0, 6, 7]);

        assert_eq!(
            OfframpAccountListLengths::try_from_slice(&encoded).unwrap(),
            lengths
        );
    }

    /// The fixed prefix is five bytes plus the vector's length prefix; anything shorter is not a
    /// layout, and the per-CCV tail is allowed to be empty (a message with no CCVs).
    #[test]
    fn deserialization_requires_the_fixed_prefix() {
        assert!(OfframpAccountListLengths::try_from_slice(&[1, 2, 3, 4]).is_err());

        let no_ccvs =
            OfframpAccountListLengths::try_from_slice(&[1, 2, 3, 4, 5, 0, 0, 0, 0]).unwrap();
        assert!(no_ccvs.ccvs.is_empty());
    }
}
