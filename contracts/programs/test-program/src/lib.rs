/**
 * This program is meant to only be used in integration tests on localnet.
 */
use anchor_lang::prelude::*;

use ccip_common::messages::FinalityConfig;
use ccip_common::types::CrossChainGas;
use ccip_router::{event::events::on_ramp::CCIPMessageSentV2, messages::Receipt};

declare_id!("9yDw4zo6CxspgkUD5mGcs8LQiKQCZhkZCbaMhKEUHboU");

#[cfg(all(feature = "custom-entrypoint", not(feature = "cpi")))]
anchor_lang::solana_program::entrypoint!(custom_entrypoint);

#[cfg(all(feature = "custom-entrypoint", not(feature = "cpi")))]
pub fn custom_entrypoint<'info>(
    program_id: &'info Pubkey,
    accounts: &'info [AccountInfo<'info>],
    data: &'info [u8],
) -> anchor_lang::solana_program::entrypoint::ProgramResult {
    if program_id == &ID {
        entry(program_id, accounts, data)
    } else {
        test_program::fallback(program_id, accounts, data).map_err(|error| {
            error.log();
            error.into()
        })
    }
}

#[program]
pub mod test_program {
    use ccip_common::v2::ProtocolAmount;

    use super::*;

    pub fn emit_ccip_message_sent_v2(ctx: Context<CcipSendV2>) -> Result<()> {
        msg!("Called `emit_cpi` {:?}. Emitting CPI log...", ctx);
        let dest_chain_selector = 123;

        emit_cpi!(CCIPMessageSentV2 {
            dest_chain_selector,
            sender: Pubkey::new_from_array([1u8; 32]),
            message_id: [2u8; 32],
            fee_token: Pubkey::new_from_array([3u8; 32]),
            token_amount_before_token_pool_fees: ProtocolAmount::from(0u8),
            encoded_message: ccip_common::messages::MessageV1 {
                source_chain_selector: 1,
                dest_chain_selector,
                message_number: 1,
                execution_gas_limit: CrossChainGas::new(2),
                ccip_receive_gas_limit: CrossChainGas::new(3),
                finality: FinalityConfig {
                    flags: 0,
                    block_depth: 0,
                },
                ccv_and_executor_hash: [4u8; 32],
                on_ramp_address: vec![5u8; 32],
                off_ramp_address: vec![6u8; 32],
                sender: vec![7u8; 32],
                receiver: vec![8u8; 32],
                dest_blob: vec![],
                token_transfer: None,
                data: vec![1, 2, 3, 4],
            }
            .to_bytes()
            .unwrap(),
            // Two receipts (executor + network fee) — DefaultMessageSentEventParser
            // rejects single-receipt events as malformed.
            receipts: vec![
                Receipt {
                    issuer: ccip_router::ID,
                    dest_gas_limit: CrossChainGas::new(1),
                    dest_bytes_overhead: 1,
                    fee_token_amount: 0,
                    extra_args: vec![],
                },
                Receipt {
                    issuer: ccip_router::ID,
                    dest_gas_limit: CrossChainGas::new(1),
                    dest_bytes_overhead: 1,
                    fee_token_amount: 0,
                    extra_args: vec![],
                },
            ],
            verifier_blobs: vec![vec![4, 5, 6]]
        });

        Ok(())
    }

    pub fn fallback<'info>(
        program_id: &'info Pubkey,
        accounts: &'info [AccountInfo<'info>],
        data: &'info [u8],
    ) -> Result<()> {
        let prefix_len = data.len().min(8);
        msg!(
            "Mock CPI: program={}, accounts={}, data_len={}, data_prefix={:?}",
            program_id,
            accounts.len(),
            data.len(),
            &data[..prefix_len]
        );
        Ok(())
    }
}

#[event_cpi]
#[derive(Accounts, Debug)]
pub struct CcipSendV2<'info> {
    // PERMISSIONLESS: test program
    pub authority: Signer<'info>,
}

#[cfg(all(test, feature = "custom-entrypoint", not(feature = "cpi")))]
mod tests {
    use super::*;

    #[test]
    fn unknown_instruction_uses_fallback() {
        let account_key = Pubkey::new_unique();
        let account_owner = Pubkey::new_unique();
        let mut lamports = 0;
        let mut account_data = [];
        let account = AccountInfo::new(
            &account_key,
            false,
            true,
            &mut lamports,
            &mut account_data,
            &account_owner,
            false,
        );

        custom_entrypoint(&ID, &[account], &[1, 2, 3]).unwrap();
    }

    #[test]
    fn alternate_program_id_bypasses_anchor_id_check() {
        custom_entrypoint(&Pubkey::new_unique(), &[], &[1, 2, 3]).unwrap();
    }
}
