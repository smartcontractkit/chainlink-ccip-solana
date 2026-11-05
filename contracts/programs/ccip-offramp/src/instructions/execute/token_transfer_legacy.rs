use anchor_lang::prelude::*;
use anchor_lang::solana_program::{instruction::Instruction, program::invoke_signed};
use ccip_common::cpi::get_verified_cpi_return_data;

use crate::messages::{CrossChainAmount, TOKENPOOL_RELEASE_OR_MINT_DISCRIMINATOR};
use crate::CcipOfframpError;

pub const CCIP_POOL_V1_RET_BYTES: usize = 8;

pub fn interact_with_pool(
    pool_program: Pubkey,
    signer: Pubkey,
    acc_infos: Vec<AccountInfo>,
    data: ReleaseOrMintInV1,
    seeds: &[&[u8]],
) -> Result<Vec<u8>> {
    let acc_metas: Vec<AccountMeta> = acc_infos
        .to_vec()
        .iter()
        .flat_map(|acc_info| {
            // Check signer from PDA External Execution config
            let is_signer = acc_info.key() == signer;
            acc_info.to_account_metas(Some(is_signer))
        })
        .collect();

    let ix = Instruction {
        program_id: pool_program,
        accounts: acc_metas,
        data: data.to_tx_data(),
    };

    // CPI call to pool is expected to return data using set_return_data
    // anchor does this automatically but is limited to max 256 bytes
    // https://github.com/coral-xyz/anchor/blob/0109f4a3cf4117570f627e3ae465b6247d504f69/lang/syn/src/codegen/program/handlers.rs#L113
    invoke_signed(&ix, &acc_infos, &[seeds])?;

    get_verified_cpi_return_data(pool_program, CcipOfframpError::InvalidCpiReturnProgram)
}

#[derive(Clone, AnchorSerialize, AnchorDeserialize)]
pub struct ReleaseOrMintInV1 {
    pub original_sender: Vec<u8>, //          The original sender of the tx on the source chain
    pub remote_chain_selector: u64, // ─╮ The chain ID of the source chain
    pub receiver: Pubkey, // ───────────╯ The Token Associated Account that will receive the tokens on the destination chain.
    pub amount: CrossChainAmount, // LE u256 amount - The amount of tokens to release or mint, denominated in the source token's decimals, pool expected to handle conversation to solana token specifics
    pub local_token: Pubkey,      //            The address of the Token Mint Account on SVM
    /// @dev WARNING: sourcePoolAddress should be checked prior to any processing of funds. Make sure it matches the
    /// expected pool address for the given remoteChainSelector.
    pub source_pool_address: Vec<u8>, //       The address bytes of the source pool
    pub source_pool_data: Vec<u8>, //          The data received from the source pool to process the release or mint
    /// @dev WARNING: offchainTokenData is untrusted data.
    pub offchain_token_data: Vec<u8>, //       The offchain data to process the release or mint
}

impl ReleaseOrMintInV1 {
    pub fn to_tx_data(&self) -> Vec<u8> {
        let mut data = Vec::new();
        data.extend_from_slice(&TOKENPOOL_RELEASE_OR_MINT_DISCRIMINATOR);
        borsh::to_writer(&mut data, &self).unwrap();
        data
    }
}

#[derive(Clone, AnchorSerialize, AnchorDeserialize)]
pub struct ReleaseOrMintOutV1 {
    // token amounts local to solana
    // (Correct because this is from the SVM Offramp's perspective)
    pub destination_amount: u64,
}

/// Maximum number of accounts a message's `dest_blob` may resolve to: one bit per account in the
/// `u64` writability bitmap that describes them. Raising it needs a wider bitmap, i.e. a new
/// version of the SVMExecutorArgs.
pub const MAX_MESSAGING_ACCOUNTS: usize = u64::BITS as usize;

/// Reads the writability bit for `index` out of the bitmap. Out-of-range indices read as read-only
/// instead of shifting past the bitmap width, which would panic under `overflow-checks`.
pub fn is_writable(bitmap: &u64, index: u8) -> bool {
    (index as usize) < MAX_MESSAGING_ACCOUNTS && (bitmap & (1 << index) != 0) // check valid index and that bit at index is 1
}

#[cfg(test)]
mod tests {
    use super::*;

    /// The bitmap is a `u64`, so index 63 is the last valid one: it must be readable (a naive
    /// `1 << index` on a narrower type, or an off-by-one bound, would truncate it away) and it must
    /// not be confused with any other index.
    #[test]
    fn writability_at_the_last_valid_index() {
        let only_63: u64 = 1 << 63;
        assert!(is_writable(&only_63, 63));
        for index in 0..63u8 {
            assert!(
                !is_writable(&only_63, index),
                "index {index} must not read bit 63"
            );
        }

        let all_but_63: u64 = u64::MAX >> 1;
        assert!(!is_writable(&all_but_63, 63));
    }

    #[test]
    fn every_index_of_a_full_bitmap_is_writable() {
        for index in 0..MAX_MESSAGING_ACCOUNTS as u8 {
            assert!(is_writable(&u64::MAX, index), "index {index} must be set");
        }
    }

    #[test]
    fn empty_bitmap_grants_nothing() {
        for index in 0..MAX_MESSAGING_ACCOUNTS as u8 {
            assert!(!is_writable(&0, index), "index {index} must be unset");
        }
    }

    /// Out-of-range indices are unreachable in production (both the resolution and the execution
    /// paths bound the account list by `MAX_MESSAGING_ACCOUNTS` first), but they must degrade to
    /// read-only instead of panicking on the shift.
    #[test]
    fn out_of_range_indices_are_read_only() {
        for index in [64u8, 65, 127, 128, 255] {
            assert!(!is_writable(&u64::MAX, index), "index {index} is not valid");
        }
    }
}
