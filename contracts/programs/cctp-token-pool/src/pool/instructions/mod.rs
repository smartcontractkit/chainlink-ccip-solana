use anchor_lang::prelude::Pubkey;

pub(super) const TOKEN_MESSENGER_MINTER_V1: Pubkey =
    anchor_lang::pubkey!("CCTPiPYPc6AsJuwueEnWgSgucamXDZwBd53dQ11YiKX3");
pub(super) const MESSAGE_TRANSMITTER_V1: Pubkey =
    anchor_lang::pubkey!("CCTPmbSD7gX1bxKPAmg77w8oFzNFpaQiQUWD43TKaecd");

// bytes4(keccak256("CCTP_V2_CCV"))
pub const CCTP_VERSION_2_CCV_TAG: [u8; 4] = [0x30, 0x47, 0x58, 0x7c];

pub mod observe;
pub mod operations_v1;
pub mod operations_v2;
pub mod token_pool_chain_config;
pub mod token_pool_management;
