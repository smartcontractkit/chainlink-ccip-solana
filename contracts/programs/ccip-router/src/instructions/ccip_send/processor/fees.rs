use anchor_lang::prelude::*;
use anchor_lang::solana_program::program::invoke;
use anchor_spl::token_interface;

use ccip_common::seed::{self, vault_ata};

use crate::error::CcipRouterError;
use crate::instructions::common::CombinedReceipts;
use crate::messages::Receipt;

use super::CcipSendV2;

pub(super) fn distribute_fees<'info>(
    ctx: &mut Context<'info, CcipSendV2<'info>>,
    receipts: &CombinedReceipts,
    fee_token: Pubkey,
) -> Result<()> {
    // First, we transfer from the user ATA to the fee vault. Fees will be distributed
    // from it, and the remainder (corresponding to legacy pools) will remain in
    // the vault.
    transfer_fee_total_to_vault(ctx, receipts, fee_token)?;
    for (payable, vault_qualifier) in receipts.payable_only() {
        transfer_receipt_fee_to_issuer_ata(ctx, payable, vault_qualifier)?;
    }

    Ok(())
}

fn transfer_fee_total_to_vault<'info>(
    ctx: &mut Context<'info, CcipSendV2<'info>>,
    receipts: &CombinedReceipts,
    fee_token: Pubkey,
) -> Result<()> {
    let is_paying_with_native_sol = fee_token == Pubkey::default();
    if is_paying_with_native_sol {
        transfer_and_wrap_native_sol(
            ctx.accounts.fee_token_program.key(),
            &mut ctx.accounts.authority,
            &mut ctx.accounts.fee_vault,
            receipts.fee_token_amount_sum,
        )?;
    } else {
        let transfer = token_interface::TransferChecked {
            from: ctx
                .accounts
                .fee_token_user_associated_account
                .to_account_info(),
            to: ctx.accounts.fee_vault.to_account_info(),
            mint: ctx.accounts.fee_token_mint.to_account_info(),
            authority: ctx.accounts.fee_billing_signer.to_account_info(),
        };

        let seeds = &[seed::FEE_BILLING_SIGNER, &[ctx.bumps.fee_billing_signer]];
        let signer_seeds = &[&seeds[..]];
        let cpi_ctx = CpiContext::new_with_signer(
            ctx.accounts.fee_token_program.key(),
            transfer,
            signer_seeds,
        );
        token_interface::transfer_checked(
            cpi_ctx,
            receipts.fee_token_amount_sum,
            ctx.accounts.fee_token_mint.decimals,
        )?;
    }

    Ok(())
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

fn transfer_receipt_fee_to_issuer_ata<'info>(
    ctx: &Context<'info, CcipSendV2<'info>>,
    receipt: &Receipt,
    vault_qualifier: Option<&Pubkey>,
) -> Result<()> {
    // At this point we've erased the information of which particular issuer this is, therefore we cannot
    // search the account layout directly for the vault ATA. Instead we re-derive it here from the issuer
    // address (and its vault qualifier, if any), since fee vault ATAS all have the same seeds. Then, we
    // iterate the remaining accounts from the context and find the reference to it.
    //
    // SAFETY: It is safe because we trust that the list of receipts is correctly constructed in the layers
    // above, so there can't be any mismatch between receipt issuer and the appropriate fee as stated in
    // the receipt.
    let expected_issuer_vault_ata = vault_ata(
        receipt.issuer.key(),
        vault_qualifier.map(|q| q.as_ref()),
        &ctx.accounts.fee_token_mint.to_account_info(),
    );

    let Some(issuer_vault_ata) = ctx
        .remaining_accounts
        .iter()
        .find(|a| a.key() == expected_issuer_vault_ata)
    else {
        return Err(CcipRouterError::MissingAtaForReceiptIssuer.into());
    };

    let transfer = token_interface::TransferChecked {
        from: ctx.accounts.fee_vault.to_account_info(),
        mint: ctx.accounts.fee_token_mint.to_account_info(),
        to: issuer_vault_ata.clone(),
        authority: ctx.accounts.fee_billing_signer.to_account_info(),
    };

    let seeds = &[seed::FEE_BILLING_SIGNER, &[ctx.bumps.fee_billing_signer]];
    let signer_seeds = &[&seeds[..]];

    let cpi_ctx =
        CpiContext::new_with_signer(ctx.accounts.fee_token_program.key(), transfer, signer_seeds);
    token_interface::transfer_checked(
        cpi_ctx,
        receipt.fee_token_amount,
        ctx.accounts.fee_token_mint.decimals,
    )
}
