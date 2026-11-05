use anchor_lang::prelude::*;
use anchor_spl::token_interface::{self, Mint, TokenAccount, TokenInterface};
use base_token_pool::{constants::*, errors::*};
use ccip_common::seed;

use crate::state::*;
use crate::State;

/// Withdraws the fees this pool accrued while serving `mint`, in a single fee token.
///
/// The router pays every token-pool receipt fee into the ATA of the `[FEE_VAULT, mint]` PDA for
/// the fee token the sender chose (see `ccip_common::seed::vault_ata`). The mint is part of the
/// seeds, so a program serving several tokens accrues each token's fees in its own vault; and only
/// this program can sign for that PDA, so the fees can only leave through this instruction.
#[derive(Accounts)]
pub struct WithdrawFeeTokens<'info> {
    #[account(
        seeds = [POOL_STATE_SEED, mint.key().as_ref()],
        bump,
        constraint = valid_version(state.version, MAX_POOL_STATE_V) @ CcipTokenPoolError::InvalidVersion,
    )]
    pub state: Account<'info, State>,

    /// The token this pool wraps. Unrelated to `fee_token_mint`, but part of the fee vault seeds.
    pub mint: InterfaceAccount<'info, Mint>,

    /// CHECK: PDA that owns the fee vault ATA, and therefore the CPI signer for the withdrawal.
    #[account(
        seeds = [seed::FEE_VAULT, mint.key().as_ref()],
        bump,
    )]
    pub fee_vault: UncheckedAccount<'info>,

    #[account(
        mut,
        associated_token::mint = fee_token_mint,
        associated_token::authority = fee_vault,
        associated_token::token_program = fee_token_program,
    )]
    pub fee_vault_token_account: InterfaceAccount<'info, TokenAccount>,

    /// Where the fees go. Freely chosen by the pool owner, unlike the executor and the CCVs, which
    /// pin the destination to a `fee_aggregator` held in their config.
    #[account(mut, token::mint = fee_token_mint, token::token_program = fee_token_program)]
    pub recipient: InterfaceAccount<'info, TokenAccount>,

    /// The token the fees were billed in, which is whatever the sender paid with, so a pool
    /// accrues one vault ATA per fee token it has ever been paid in.
    #[account(owner = fee_token_program.key())]
    pub fee_token_mint: InterfaceAccount<'info, Mint>,

    pub fee_token_program: Interface<'info, TokenInterface>,

    #[account(address = state.config.owner @ CcipTokenPoolError::Unauthorized)]
    pub authority: Signer<'info>,
}

pub fn withdraw_fee_tokens(
    ctx: Context<WithdrawFeeTokens>,
    withdraw_amount: Option<u64>,
) -> Result<()> {
    let stored_amount = ctx.accounts.fee_vault_token_account.amount;
    require_gt!(stored_amount, 0, CcipTokenPoolError::NoFeesToWithdraw);

    let withdraw_amount = withdraw_amount.unwrap_or(stored_amount);

    require_gt!(
        withdraw_amount,
        0,
        CcipTokenPoolError::InvalidFeeWithdrawAmount
    );
    require_gte!(
        stored_amount,
        withdraw_amount,
        CcipTokenPoolError::InsufficientFeesToWithdraw
    );

    let transfer = token_interface::TransferChecked {
        from: ctx.accounts.fee_vault_token_account.to_account_info(),
        to: ctx.accounts.recipient.to_account_info(),
        mint: ctx.accounts.fee_token_mint.to_account_info(),
        authority: ctx.accounts.fee_vault.to_account_info(),
    };

    let mint = ctx.accounts.mint.key();
    let seeds = &[seed::FEE_VAULT, mint.as_ref(), &[ctx.bumps.fee_vault]];
    let signer_seeds = &[&seeds[..]];
    let cpi_ctx =
        CpiContext::new_with_signer(ctx.accounts.fee_token_program.key(), transfer, signer_seeds);
    token_interface::transfer_checked(
        cpi_ctx,
        withdraw_amount,
        ctx.accounts.fee_token_mint.decimals,
    )
}
