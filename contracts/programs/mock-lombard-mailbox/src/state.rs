use anchor_lang::prelude::*;

#[account]
pub struct Config {
    pub admin: Pubkey,
    pub pending_admin: Pubkey,
    pub treasury: Pubkey,
    pub paused: bool,
    pub consortium: Pubkey,
    pub global_nonce: u64,
    pub default_max_payload_size: u32,
    pub fee_per_byte: u64,
}

impl Config {
    pub const SPACE: usize = 8 + 32 + 32 + 32 + 1 + 32 + 8 + 4 + 8;
}

#[account]
pub struct DeliveredMessage {
    /// 0 = delivered (by `mark_delivered`, not yet handled), 1 = handled (by `handle_message`).
    /// Mock-internal encoding, not derived from the real mailbox.
    pub status: u8,
    pub recipient: Pubkey,
    pub amount: u64,
    pub message: Option<[u8; 36]>,
    /// Mirrors the real mailbox's `MessageV1.destination_caller`: when set, only this handler may
    /// consume the message.
    pub destination_caller: Option<[u8; 32]>,
}

impl DeliveredMessage {
    pub const SPACE: usize = 8 + 1 + 32 + 8 + (1 + 36) + (1 + 32);
}
