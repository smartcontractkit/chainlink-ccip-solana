use anchor_lang::prelude::*;

pub const OPTIONAL_MESSAGE_SIZE: usize = 36;

#[derive(AnchorSerialize, AnchorDeserialize, Clone, Debug)]
pub struct OutboundResponse {
    pub nonce: u64,
    pub payload_hash: [u8; 32],
}

#[derive(AnchorSerialize, AnchorDeserialize, Clone, Debug)]
pub struct InboundResponse {
    pub amount: u64,
    pub message: Option<[u8; OPTIONAL_MESSAGE_SIZE]>,
}
