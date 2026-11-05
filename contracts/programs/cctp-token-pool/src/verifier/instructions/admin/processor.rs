use anchor_lang::prelude::*;

use anchor_spl::token_interface::{self, Mint, TokenAccount};
use ccip_common::messages::FinalityConfig;
use ccip_common::state::AccountSchemaVersion;

use crate::error::CctpPoolVerifierError;
use crate::verifier::state::{verifier_seed, Config, BPS_DIVIDER};

use super::event::*;

pub fn transfer_verifier_ownership(
    ctx: Context<UpdateVerifierConfig>,
    proposed_owner: Pubkey,
) -> Result<()> {
    let config = &mut ctx.accounts.config;

    require_keys_neq!(
        proposed_owner,
        Pubkey::default(),
        CctpPoolVerifierError::DefaultOwnerProposal
    );
    require_keys_neq!(
        proposed_owner,
        config.owner,
        CctpPoolVerifierError::RedundantOwnerProposal
    );

    emit_cpi!(OwnershipTransferRequested {
        from: config.owner,
        to: proposed_owner,
    });

    config.proposed_owner = proposed_owner;
    Ok(())
}

pub fn accept_verifier_ownership(ctx: Context<AcceptVerifierOwnership>) -> Result<()> {
    let config = &mut ctx.accounts.config;

    emit_cpi!(OwnershipTransferred {
        from: config.owner,
        to: config.proposed_owner,
    });

    // NOTE: take() resets proposed_owner to default
    config.owner = std::mem::take(&mut config.proposed_owner);
    Ok(())
}

pub fn set_verifier_allowlist_admin(
    ctx: Context<UpdateVerifierConfig>,
    allowlist_admin: Pubkey,
) -> Result<()> {
    let config = &mut ctx.accounts.config;
    let old = config.allowlist_admin;
    config.allowlist_admin = allowlist_admin;

    emit_cpi!(AllowlistAdminSet {
        old,
        new: config.allowlist_admin,
    });

    Ok(())
}

pub fn set_verifier_fee_aggregator(
    ctx: Context<UpdateVerifierConfig>,
    fee_aggregator: Pubkey,
) -> Result<()> {
    let config = &mut ctx.accounts.config;
    let old = config.fee_aggregator;
    config.fee_aggregator = fee_aggregator;

    emit_cpi!(FeeAggregatorSet {
        old,
        new: config.fee_aggregator,
    });

    Ok(())
}

pub fn set_verifier_allowed_finality_config(
    ctx: Context<UpdateVerifierConfig>,
    finality: FinalityConfig,
) -> Result<()> {
    let config = &mut ctx.accounts.config;
    let old = config.allowed_finality_config;
    config.allowed_finality_config = finality;

    emit_cpi!(AllowedFinalityConfigSet {
        old,
        new: config.allowed_finality_config,
    });

    Ok(())
}

pub fn set_verifier_fast_finality_bps(
    ctx: Context<UpdateVerifierConfig>,
    fast_finality_bps: u16,
) -> Result<()> {
    require_gte!(
        BPS_DIVIDER,
        fast_finality_bps,
        CctpPoolVerifierError::InvalidInstructionData
    );

    let config = &mut ctx.accounts.config;
    let old = config.fast_finality_bps;
    config.fast_finality_bps = fast_finality_bps;

    emit_cpi!(FastFinalityBpsSet {
        old,
        new: config.fast_finality_bps,
    });

    Ok(())
}

#[event_cpi]
#[derive(Accounts)]
pub struct AcceptVerifierOwnership<'info> {
    #[account(
        mut,
        seeds = [verifier_seed::CONFIG],
        bump = config.bump,
        constraint = config.is_latest_version() @ CctpPoolVerifierError::InvalidVersion,
    )]
    pub config: Account<'info, Config>,

    #[account(address = config.proposed_owner @ CctpPoolVerifierError::Unauthorized)]
    pub authority: Signer<'info>,
}

pub fn transfer_verifier_storage_location_admin(
    ctx: Context<UpdateVerifierStorageLocationAdmin>,
    proposed_storage_location_admin: Pubkey,
) -> Result<()> {
    let config = &mut ctx.accounts.config;

    require_keys_neq!(
        proposed_storage_location_admin,
        Pubkey::default(),
        CctpPoolVerifierError::DefaultStorageLocationAdminProposal
    );
    require_keys_neq!(
        proposed_storage_location_admin,
        config.storage_location_admin,
        CctpPoolVerifierError::RedundantStorageLocationAdminProposal
    );

    emit_cpi!(StorageLocationAdminTransferRequested {
        from: config.storage_location_admin,
        to: proposed_storage_location_admin,
    });

    config.proposed_storage_location_admin = proposed_storage_location_admin;
    Ok(())
}

pub fn accept_verifier_storage_location_admin(
    ctx: Context<AcceptVerifierStorageLocationAdmin>,
) -> Result<()> {
    let config = &mut ctx.accounts.config;

    emit_cpi!(StorageLocationAdminTransferred {
        from: config.storage_location_admin,
        to: config.proposed_storage_location_admin,
    });

    config.storage_location_admin = std::mem::take(&mut config.proposed_storage_location_admin);
    Ok(())
}

/// Sweeps every fee this verifier accrued in `fee_token_mint` into the configured fee aggregator's
/// ATA.
///
/// Permissionless: the destination is pinned to `config.fee_aggregator`, so a caller can only
/// trigger the sweep, never redirect it.
pub fn withdraw_verifier_fee_tokens(ctx: Context<WithdrawVerifierFeeTokens>) -> Result<()> {
    require_keys_neq!(
        ctx.accounts.config.fee_aggregator,
        Pubkey::default(),
        CctpPoolVerifierError::FeeAggregatorNotSet
    );

    let amount = ctx.accounts.fee_vault_token_account.amount;
    require_gt!(amount, 0, CctpPoolVerifierError::InsufficientFunds);

    let transfer = token_interface::TransferChecked {
        from: ctx.accounts.fee_vault_token_account.to_account_info(),
        to: ctx.accounts.fee_aggregator_token_account.to_account_info(),
        mint: ctx.accounts.fee_token_mint.to_account_info(),
        authority: ctx.accounts.fee_vault.to_account_info(),
    };

    let seeds = &[verifier_seed::FEE_VAULT, &[ctx.bumps.fee_vault]];
    let signer_seeds = &[&seeds[..]];
    let cpi_ctx =
        CpiContext::new_with_signer(*ctx.accounts.token_program.key, transfer, signer_seeds);
    token_interface::transfer_checked(cpi_ctx, amount, ctx.accounts.fee_token_mint.decimals)
}

#[event_cpi]
#[derive(Accounts)]
pub struct UpdateVerifierConfig<'info> {
    #[account(
        mut,
        seeds = [verifier_seed::CONFIG],
        bump = config.bump,
        constraint = config.is_latest_version() @ CctpPoolVerifierError::InvalidVersion,
    )]
    pub config: Account<'info, Config>,

    #[account(address = config.owner @ CctpPoolVerifierError::Unauthorized)]
    pub authority: Signer<'info>,
}

#[event_cpi]
#[derive(Accounts)]
pub struct UpdateVerifierStorageLocationAdmin<'info> {
    #[account(
        mut,
        seeds = [verifier_seed::CONFIG],
        bump = config.bump,
        constraint = config.is_latest_version() @ CctpPoolVerifierError::InvalidVersion,
    )]
    pub config: Account<'info, Config>,

    #[account(address = config.storage_location_admin @ CctpPoolVerifierError::Unauthorized)]
    pub authority: Signer<'info>,
}

#[event_cpi]
#[derive(Accounts)]
pub struct AcceptVerifierStorageLocationAdmin<'info> {
    #[account(
        mut,
        seeds = [verifier_seed::CONFIG],
        bump = config.bump,
        constraint = config.is_latest_version() @ CctpPoolVerifierError::InvalidVersion,
    )]
    pub config: Account<'info, Config>,

    #[account(address = config.proposed_storage_location_admin @ CctpPoolVerifierError::Unauthorized)]
    pub authority: Signer<'info>,
}

#[derive(Accounts)]
pub struct WithdrawVerifierFeeTokens<'info> {
    #[account(
        seeds = [verifier_seed::CONFIG],
        bump = config.bump,
        constraint = config.is_latest_version() @ CctpPoolVerifierError::InvalidVersion,
    )]
    pub config: Account<'info, Config>,

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
        seeds = [verifier_seed::FEE_VAULT],
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
