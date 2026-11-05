use anchor_discriminator::anchor_discriminator;
use anchor_lang::prelude::Pubkey;

// bytes4(keccak256("CCTPVerifier 2.1.0"))
// although this is 2.0.0, the EVM counterpart uses 2.1.0 and we need it to match for cross-chain verification
// https://github.com/smartcontractkit/chainlink-ccip/blob/7520b255725e639d2a71c23ef6235cb0d08d2e5c/chains/evm/deployment/v2_0_0/verifier_tags/tags.go#L5
pub const VERSION_TAG: [u8; 4] = [0x91, 0xb3, 0x33, 0x8e];

pub const TOKEN_MESSENGER_MINTER_V2: Pubkey =
    anchor_lang::pubkey!("CCTPV2vPZJS2u2BBsUoscuikbYjnpFmbFsvVuJdgUMQe");
pub const MESSAGE_TRANSMITTER_V2: Pubkey =
    anchor_lang::pubkey!("CCTPV2Sm4AdWt5296sk4P66VBZ7bEhcARwFaaS9YPbeC");

pub const CCTP_FAST_FINALITY_THRESHOLD: u32 = 1000;
pub const CCTP_STANDARD_FINALITY_THRESHOLD: u32 = 2000;

pub const DEPOSIT_FOR_BURN_WITH_HOOK_DISCRIMINATOR: [u8; 8] =
    anchor_discriminator!("deposit_for_burn_with_hook");
pub const RECEIVE_MESSAGE_DISCRIMINATOR: [u8; 8] = anchor_discriminator!("receive_message");
