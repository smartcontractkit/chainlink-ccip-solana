use anchor_lang::prelude::*;

use crate::pool::instructions::{MESSAGE_TRANSMITTER_V1, TOKEN_MESSENGER_MINTER_V1};

// We restrict to the first version. New pool may be required for subsequent versions.
pub(super) const SUPPORTED_CCTP_MESSAGE_VERSION: u32 = 0;

#[derive(AnchorSerialize, AnchorDeserialize, Clone)]
pub struct CctpMessage {
    pub data: Vec<u8>,
}

impl CctpMessage {
    const MAX_NONCES: u64 = 6400;

    // Indices of each field in the message
    const VERSION: (usize, usize) = (0, 4); // u32 (4 bytes)
    const SOURCE_DOMAIN: (usize, usize) = (4, 8); // u32 (4 bytes)
    const DESTINATION_DOMAIN: (usize, usize) = (8, 12); // u32 (4 bytes)
    const NONCE: (usize, usize) = (12, 20); // u64 (8 bytes)
    const _SENDER: (usize, usize) = (20, 52); // Pubkey (32 bytes)
    const _RECIPIENT: (usize, usize) = (52, 84); // Pubkey (32 bytes)
    const _DESTINATION_CALLER: (usize, usize) = (84, 116); // Pubkey (32 bytes)
    const MESSAGE_BODY_START: usize = 116;

    // From CCIP's point of view, the CCTP message must be at least long enough to contain the metadata fields,
    // so that CCIP's own checks can be performed without panicking. Whenever CCTP is invoked, CCTP will perform
    // further checks on the message.
    pub const MINIMUM_LENGTH: usize = Self::MESSAGE_BODY_START;

    pub fn version(&self) -> u32 {
        u32::from_be_bytes(
            self.data[Self::VERSION.0..Self::VERSION.1]
                .try_into()
                .unwrap(),
        )
    }

    pub fn source_domain(&self) -> u32 {
        u32::from_be_bytes(
            self.data[Self::SOURCE_DOMAIN.0..Self::SOURCE_DOMAIN.1]
                .try_into()
                .unwrap(),
        )
    }

    pub fn destination_domain(&self) -> u32 {
        u32::from_be_bytes(
            // SAFETY: DESTINATION_DOMAIN slice is constant (8..12), always 4 bytes; guarded by MINIMUM_LENGTH
            self.data[Self::DESTINATION_DOMAIN.0..Self::DESTINATION_DOMAIN.1]
                .try_into()
                .unwrap(),
        )
    }

    pub fn nonce(&self) -> u64 {
        u64::from_be_bytes(
            self.data[Self::NONCE.0..Self::NONCE.1].try_into().expect(
                "NONCE slice is constant (12..20), always 8 bytes; guarded by MINIMUM_LENGTH",
            ),
        )
    }

    pub fn _sender(&self) -> Pubkey {
        Pubkey::new_from_array(
            self.data[Self::_SENDER.0..Self::_SENDER.1]
                .try_into()
                .unwrap(),
        )
    }

    pub fn _recipient(&self) -> Pubkey {
        Pubkey::new_from_array(
            self.data[Self::_RECIPIENT.0..Self::_RECIPIENT.1]
                .try_into()
                .unwrap(),
        )
    }

    pub fn _destination_caller(&self) -> Pubkey {
        Pubkey::new_from_array(
            self.data[Self::_DESTINATION_CALLER.0..Self::_DESTINATION_CALLER.1]
                .try_into()
                .unwrap(),
        )
    }

    pub fn first_nonce(&self) -> u64 {
        self.nonce()
            .checked_sub(1)
            .and_then(|n| n.checked_div(Self::MAX_NONCES))
            .and_then(|n| n.checked_mul(Self::MAX_NONCES))
            .and_then(|n| n.checked_add(1))
            .unwrap()
    }
}

#[inline]
pub fn get_message_transmitter_pda(seeds: &[&[u8]]) -> Pubkey {
    Pubkey::find_program_address(seeds, &MESSAGE_TRANSMITTER_V1).0
}
#[inline]
pub fn get_token_messenger_minter_pda(seeds: &[&[u8]]) -> Pubkey {
    Pubkey::find_program_address(seeds, &TOKEN_MESSENGER_MINTER_V1).0
}
