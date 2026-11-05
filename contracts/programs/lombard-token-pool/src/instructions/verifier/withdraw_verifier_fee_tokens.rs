use anchor_lang::prelude::*;

use anchor_spl::token_interface::{self, Mint, TokenAccount};
use base_token_pool::constants::valid_version;
use base_token_pool::errors::CcipTokenPoolError;
use ccip_common::seed;

use crate::constants::VERIFIER_CONFIG_SEED;
use crate::errors::LombardTokenPoolError;
use crate::state::{VerifierConfig, MAX_VERIFIER_CONFIG_V};

/// Sweeps every fee this verifier accrued in `fee_token_mint` into the configured fee aggregator's
/// ATA.
///
/// Permissionless: the destination is pinned to `config.fee_aggregator`, so a caller can only
/// trigger the sweep, never redirect it.
pub fn withdraw_verifier_fee_tokens(ctx: Context<WithdrawVerifierFeeTokens>) -> Result<()> {
    require_keys_neq!(
        ctx.accounts.config.fee_aggregator,
        Pubkey::default(),
        LombardTokenPoolError::FeeAggregatorNotSet
    );

    let amount = ctx.accounts.fee_vault_token_account.amount;
    require_gt!(amount, 0, LombardTokenPoolError::InsufficientFunds);

    let transfer = token_interface::TransferChecked {
        from: ctx.accounts.fee_vault_token_account.to_account_info(),
        to: ctx.accounts.fee_aggregator_token_account.to_account_info(),
        mint: ctx.accounts.fee_token_mint.to_account_info(),
        authority: ctx.accounts.fee_vault.to_account_info(),
    };

    let seeds = &[seed::FEE_VAULT, &[ctx.bumps.fee_vault]];
    let signer_seeds = &[&seeds[..]];
    let cpi_ctx =
        CpiContext::new_with_signer(*ctx.accounts.token_program.key, transfer, signer_seeds);
    token_interface::transfer_checked(cpi_ctx, amount, ctx.accounts.fee_token_mint.decimals)
}

#[derive(Accounts)]
pub struct WithdrawVerifierFeeTokens<'info> {
    #[account(
        seeds = [VERIFIER_CONFIG_SEED],
        bump = config.bump,
        constraint = valid_version(config.version, MAX_VERIFIER_CONFIG_V) @ CcipTokenPoolError::InvalidVersion,
    )]
    pub config: Account<'info, VerifierConfig>,

    /// The account the router pays this verifier's fees into: the ATA of `fee_vault` for the fee
    /// token. There is one per fee token, since the sender picks which token to bill in.
    #[account(
        mut,
        associated_token::mint = fee_token_mint,
        associated_token::authority = fee_vault,
        associated_token::token_program = token_program,
    )]
    pub fee_vault_token_account: InterfaceAccount<'info, TokenAccount>,

    /// CHECK: PDA that owns the fee vault ATA, and therefore the CPI signer for the withdrawal.
    /// The router derives the destination of every CCV receipt fee from these exact seeds (see
    /// `ccip_common::seed::vault_ata`), so this is the only authority that can move the accrued
    /// fees out.
    #[account(
        seeds = [seed::FEE_VAULT],
        bump,
    )]
    pub fee_vault: UncheckedAccount<'info>,

    #[account(
        mut,
        associated_token::mint = fee_token_mint,
        associated_token::token_program = token_program,
        associated_token::authority = config.fee_aggregator,
    )]
    pub fee_aggregator_token_account: InterfaceAccount<'info, TokenAccount>,

    #[account(owner = token_program.key())]
    pub fee_token_mint: InterfaceAccount<'info, Mint>,

    /// CHECK: CPI to token program
    pub token_program: Program<'info>,
}
