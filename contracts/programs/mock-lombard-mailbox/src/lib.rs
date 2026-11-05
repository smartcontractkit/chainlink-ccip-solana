//! Minimal mock of the real Lombard `mailbox` program's on-chain interface,

use anchor_lang::prelude::*;
use anchor_lang::solana_program::instruction::{AccountMeta, Instruction};
use anchor_lang::solana_program::program::{get_return_data, invoke};

mod errors;
mod state;

pub use state::{Config, DeliveredMessage};

use errors::MockLombardMailboxError;

declare_id!("Lomsq4ZNkZQGysC1pQc2NqNiAbXQm6C8nLmKcUoESEA");

pub const MAILBOX_CONFIG_SEED: &[u8] = b"mailbox_config";
pub const MESSAGE_SEED: &[u8] = b"message";

#[program]
pub mod mock_lombard_mailbox {
    use super::*;

    /// Single condensed test-setup call.
    pub fn test_setup_config(
        ctx: Context<TestSetupConfig>,
        global_nonce: u64,
        treasury: Pubkey,
    ) -> Result<()> {
        let config = &mut ctx.accounts.config;
        config.admin = Pubkey::default();
        config.pending_admin = Pubkey::default();
        config.treasury = treasury;
        config.paused = false;
        config.consortium = Pubkey::default();
        config.global_nonce = global_nonce;
        config.default_max_payload_size = 0;
        config.fee_per_byte = 0;
        Ok(())
    }

    /// Test-only:. Records a `DeliveredMessage` that `handle_message` will later consume.
    pub fn mark_delivered(
        ctx: Context<MarkDelivered>,
        _payload_hash: [u8; 32],
        recipient: Pubkey,
        amount: u64,
        message: Option<[u8; 36]>,
        destination_caller: Option<[u8; 32]>,
    ) -> Result<()> {
        let message_info = &mut ctx.accounts.message_info;
        message_info.status = 0; // delivered, not yet handled
        message_info.recipient = recipient;
        message_info.amount = amount;
        message_info.message = message;
        message_info.destination_caller = destination_caller;
        Ok(())
    }

    pub fn handle_message<'info>(
        ctx: Context<'info, HandleMessage<'info>>,
        payload_hash: [u8; 32],
    ) -> Result<Option<Vec<u8>>> {
        let message_info = &mut ctx.accounts.message_info;
        if let Some(destination_caller) = message_info.destination_caller {
            require_keys_eq!(
                Pubkey::new_from_array(destination_caller),
                ctx.accounts.handler.key(),
                MockLombardMailboxError::InvalidDestinationCaller
            );
        }
        require!(
            message_info.status == 0,
            MockLombardMailboxError::AlreadyHandled
        );
        message_info.status = 1; // handled

        require!(
            ctx.remaining_accounts.len() >= 8,
            MockLombardMailboxError::MissingRemainingAccounts
        );
        let inner_accounts = vec![
            ctx.remaining_accounts[4].clone(), // recipient_token_account
            ctx.remaining_accounts[5].clone(), // mint
            ctx.remaining_accounts[6].clone(), // mint_authority (unused by mock_gmp_receive)
            ctx.remaining_accounts[7].clone(), // token_authority
            ctx.remaining_accounts[3].clone(), // token_program
        ];
        let inner_metas: Vec<AccountMeta> = inner_accounts
            .iter()
            .map(|info| AccountMeta {
                pubkey: *info.key,
                is_signer: info.is_signer,
                is_writable: info.is_writable,
            })
            .collect();

        #[derive(AnchorSerialize)]
        struct MockGmpReceiveArgs {
            payload_hash: [u8; 32],
            recipient: Pubkey,
            amount: u64,
            message: Option<[u8; 36]>,
        }

        let mut data = anchor_discriminator::anchor_discriminator!("mock_gmp_receive").to_vec();
        AnchorSerialize::serialize(
            &MockGmpReceiveArgs {
                payload_hash,
                recipient: message_info.recipient,
                amount: message_info.amount,
                message: message_info.message,
            },
            &mut data,
        )
        .map_err(|_| anchor_lang::error::ErrorCode::InstructionDidNotSerialize)?;

        let instruction = Instruction {
            program_id: ctx.accounts.recipient_program.key(),
            accounts: inner_metas,
            data,
        };
        invoke(&instruction, &inner_accounts)?;

        let result = get_return_data();
        Ok(result.map(|(_, data)| data))
    }
}

#[derive(Accounts)]
pub struct TestSetupConfig<'info> {
    #[account(mut)]
    pub payer: Signer<'info>,

    #[account(
        init,
        payer = payer,
        space = Config::SPACE,
        seeds = [MAILBOX_CONFIG_SEED],
        bump,
    )]
    pub config: Account<'info, Config>,

    pub system_program: Program<'info, System>,
}

#[derive(Accounts)]
#[instruction(payload_hash: [u8; 32])]
pub struct MarkDelivered<'info> {
    #[account(mut)]
    pub payer: Signer<'info>,

    #[account(
        init,
        payer = payer,
        space = DeliveredMessage::SPACE,
        seeds = [MESSAGE_SEED, payload_hash.as_ref()],
        bump,
    )]
    pub message_info: Account<'info, DeliveredMessage>,

    pub system_program: Program<'info, System>,
}

#[derive(Accounts)]
#[instruction(payload_hash: [u8; 32])]
pub struct HandleMessage<'info> {
    /// Checked against `message_info.destination_caller`, like the real mailbox program.
    #[account(mut)]
    pub handler: Signer<'info>,
    /// CHECK: unused - no paused check, no consortium, present only for account-order parity.
    pub config: UncheckedAccount<'info>,
    #[account(
        mut,
        seeds = [MESSAGE_SEED, payload_hash.as_ref()],
        bump,
    )]
    pub message_info: Account<'info, DeliveredMessage>,
    /// CHECK: the program CPI'd into with `mock_gmp_receive`; validated by the CPI itself.
    pub recipient_program: UncheckedAccount<'info>,
}

#[cfg(test)]
mod tests {
    use super::*;

    /// `handle_message` derives the inner CPI's instruction discriminator from the literal
    /// instruction name via `anchor_discriminator!`, matching what Anchor's own `#[program]`
    /// macro computes for a `mock_gmp_receive` instruction on the receiving (mock-lombard-bridge)
    /// side - see `discriminator_bytes` in `crates/anchor_discriminator/src/lib.rs` for the
    /// `sha256("global:<name>")[..8]` formula this must agree with.
    #[test]
    fn mock_gmp_receive_discriminator_matches_expected_sighash() {
        assert_eq!(
            anchor_discriminator::anchor_discriminator!("mock_gmp_receive"),
            [0x8c, 0x2d, 0x8d, 0x8c, 0x5b, 0x08, 0xd4, 0x2f]
        );
    }

    /// `DeliveredMessage::SPACE` is computed by hand rather than via `#[derive(InitSpace)]`, so
    /// pin it against the actual Borsh-serialized size of both the `None` and `Some(..)` shapes
    /// of `message` (the account is sized once at `init` time and must fit either case -
    /// `Some(..)`, the larger of the two, exactly, and `None` with room to spare).
    #[test]
    fn delivered_message_space_fits_serialized_size() {
        let with_none = DeliveredMessage {
            status: 1,
            recipient: Pubkey::new_unique(),
            amount: u64::MAX,
            message: None,
            destination_caller: None,
        };
        let with_some = DeliveredMessage {
            message: Some([0xab; 36]),
            destination_caller: Some([0xcd; 32]),
            ..with_none
        };
        assert!(borsh::to_vec(&with_none).unwrap().len() + 8 <= DeliveredMessage::SPACE);
        assert_eq!(
            borsh::to_vec(&with_some).unwrap().len() + 8,
            DeliveredMessage::SPACE
        );
    }

    /// Same idea for `Config::SPACE` - no `Option`/variable-size fields, so there's only one
    /// shape to check.
    #[test]
    fn config_space_fits_serialized_size() {
        let config = Config {
            admin: Pubkey::new_unique(),
            pending_admin: Pubkey::new_unique(),
            treasury: Pubkey::new_unique(),
            paused: true,
            consortium: Pubkey::new_unique(),
            global_nonce: u64::MAX,
            default_max_payload_size: u32::MAX,
            fee_per_byte: u64::MAX,
        };
        assert_eq!(borsh::to_vec(&config).unwrap().len() + 8, Config::SPACE);
    }
}
