use anchor_lang::prelude::*;
use anchor_lang::solana_program::program::invoke;
use anchor_lang::solana_program::system_instruction;
use anchor_spl::token_interface;
use ccip_common::seed;

use crate::messages::SVM2AnyMessage;
use crate::{CcipRouterError, SVMTokenAmount};

/// Invokes Fee Quoter to:
/// - validate the message,
/// - calculate the fee in both the billing token and LINK, and
/// - assert that it doesn't exceed a maximum
///
/// # Returns
///  GetFeeResult with fee quoter's response
#[allow(clippy::too_many_arguments)]
pub(super) fn get_fee_cpi<'info>(
    fee_quoter_program: Pubkey,
    fee_quoter_config: AccountInfo<'info>,
    fee_quoter_dest_chain: AccountInfo<'info>,
    billing_token_config: AccountInfo<'info>,
    link_token_config: AccountInfo<'info>,
    dest_chain_selector: u64,
    message: &SVM2AnyMessage,
    get_fee_remaining_accounts: Vec<AccountInfo<'info>>,
) -> Result<fee_quoter::messages::GetFeeResult> {
    let cpi_accounts = fee_quoter::cpi::accounts::GetFee {
        config: fee_quoter_config,
        dest_chain: fee_quoter_dest_chain,
        billing_token_config,
        link_token_config,
    };

    let cpi_ctx = CpiContext::new(fee_quoter_program, cpi_accounts)
        .with_remaining_accounts(get_fee_remaining_accounts);

    let fee = fee_quoter::cpi::get_fee(
        cpi_ctx,
        dest_chain_selector,
        // Construct inline to avoid a named local overlapping with cpi_ctx in the frame (BPF stack limit)
        fee_quoter::messages::SVM2AnyMessage {
            receiver: message.receiver.clone(),
            data: message.data.clone(),
            token_amounts: message
                .token_amounts
                .iter()
                .map(|x| fee_quoter::messages::SVMTokenAmount {
                    token: x.token,
                    amount: x.amount,
                })
                .collect(),
            fee_token: message.fee_token,
            extra_args: message.extra_args.clone(),
        },
    )?
    .get();

    Ok(fee)
}

pub fn transfer_and_wrap_native_sol<'info>(
    token_program: Pubkey,
    from: &mut Signer<'info>,
    to: &mut InterfaceAccount<'info, token_interface::TokenAccount>,
    amount: u64,
) -> Result<()> {
    require!(
        // guarantee that if caller is a PDA with data it won't get garbage-collected
        from.data_is_empty() || from.get_lamports() > amount,
        CcipRouterError::InsufficientLamports
    );

    invoke(
        &system_instruction::transfer(&from.key(), &to.key(), amount),
        &[from.to_account_info(), to.to_account_info()],
    )?;

    // `SyncNative` only takes the writable wrapped-native token account: it has no authority
    // account, so no signer seeds are needed.
    let account = to.to_account_info();
    let sync: anchor_spl::token_2022::SyncNative = anchor_spl::token_2022::SyncNative { account };
    let cpi_ctx = CpiContext::new(token_program, sync);

    token_interface::sync_native(cpi_ctx)
}

pub fn transfer_fee<'info>(
    fee: &SVMTokenAmount,
    token_program: Pubkey,
    transfer: token_interface::TransferChecked<'info>,
    decimals: u8,
    signer_bump: u8,
) -> Result<()> {
    require_keys_eq!(
        fee.token,
        transfer.mint.key(),
        CcipRouterError::FeeTokenMismatch
    );

    do_billing_transfer(token_program, transfer, fee.amount, decimals, signer_bump)
}

pub fn do_billing_transfer<'info>(
    token_program: Pubkey,
    transfer: token_interface::TransferChecked<'info>,
    amount: u64,
    decimals: u8,
    signer_bump: u8,
) -> Result<()> {
    let seeds = &[seed::FEE_BILLING_SIGNER, &[signer_bump]];
    let signer_seeds = &[&seeds[..]];
    let cpi_ctx = CpiContext::new_with_signer(token_program, transfer, signer_seeds);
    token_interface::transfer_checked(cpi_ctx, amount, decimals)
}
