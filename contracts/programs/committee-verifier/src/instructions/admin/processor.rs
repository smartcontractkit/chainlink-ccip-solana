use anchor_lang::prelude::*;
use anchor_spl::token_interface;
use ccip_common::messages::FinalityConfig;
use ccip_common::seed;

use crate::error::CommitteeVerifierError;
use crate::event::{
    AllowedFinalityConfigSet, AllowlistAdminSet, DefaultCodeVersionSet, FeeAggregatorSet,
    OwnershipTransferRequested, OwnershipTransferred, StorageLocationAdminTransferRequested,
    StorageLocationAdminTransferred,
};
use crate::state::{
    valid_version, CodeVersion, Config, StorageLocations, MAX_CONFIG_V, MAX_STORAGE_LOCATIONS_V,
};

use super::context::{InitializeConfig, UpdateConfig, WithdrawFeeTokens};

pub fn initialize(ctx: Context<InitializeConfig>) -> Result<()> {
    ctx.accounts.config.set_inner(Config {
        version: MAX_CONFIG_V,
        default_code_version: CodeVersion::V1,
        owner: ctx.accounts.authority.key(),
        proposed_owner: Pubkey::default(),
        storage_location_admin: ctx.accounts.authority.key(),
        allowlist_admin: ctx.accounts.authority.key(),
        proposed_storage_location_admin: Pubkey::default(),
        bump: ctx.bumps.config,
        fee_aggregator: Pubkey::default(),
        allowed_finality_config: Default::default(),
    });

    ctx.accounts.storage_locations.set_inner(StorageLocations {
        version: MAX_STORAGE_LOCATIONS_V,
        bump: ctx.bumps.storage_locations,
        locations: vec![],
    });

    Ok(())
}

pub fn transfer_ownership(ctx: Context<UpdateConfig>, proposed_owner: Pubkey) -> Result<()> {
    let config = &mut ctx.accounts.config;

    require_keys_neq!(
        proposed_owner,
        Pubkey::default(),
        CommitteeVerifierError::DefaultOwnerProposal
    );
    require_keys_neq!(
        proposed_owner,
        config.owner,
        CommitteeVerifierError::RedundantOwnerProposal
    );

    emit_cpi!(OwnershipTransferRequested {
        from: config.owner,
        to: proposed_owner,
    });

    config.proposed_owner = proposed_owner;
    Ok(())
}

pub fn accept_ownership(ctx: Context<AcceptOwnership>) -> Result<()> {
    let config = &mut ctx.accounts.config;

    emit_cpi!(OwnershipTransferred {
        from: config.owner,
        to: config.proposed_owner,
    });

    // NOTE: take() resets proposed_owner to default
    config.owner = std::mem::take(&mut config.proposed_owner);
    Ok(())
}

pub fn set_default_code_version(
    ctx: Context<UpdateConfig>,
    code_version: CodeVersion,
) -> Result<()> {
    require_neq!(
        code_version,
        CodeVersion::Default,
        CommitteeVerifierError::InvalidCodeVersion
    );

    let config = &mut ctx.accounts.config;
    let old = config.default_code_version;
    config.default_code_version = code_version;

    emit_cpi!(DefaultCodeVersionSet {
        old,
        new: config.default_code_version,
    });

    Ok(())
}

pub fn set_allowlist_admin(ctx: Context<UpdateConfig>, allowlist_admin: Pubkey) -> Result<()> {
    let config = &mut ctx.accounts.config;
    let old = config.allowlist_admin;
    config.allowlist_admin = allowlist_admin;

    emit_cpi!(AllowlistAdminSet {
        old,
        new: config.allowlist_admin,
    });

    Ok(())
}

pub fn set_fee_aggregator(ctx: Context<UpdateConfig>, fee_aggregator: Pubkey) -> Result<()> {
    let config = &mut ctx.accounts.config;
    let old = config.fee_aggregator;
    config.fee_aggregator = fee_aggregator;

    emit_cpi!(FeeAggregatorSet {
        old,
        new: config.fee_aggregator,
    });

    Ok(())
}

pub fn set_allowed_finality_config(
    ctx: Context<UpdateConfig>,
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

#[event_cpi]
#[derive(Accounts)]
pub struct AcceptOwnership<'info> {
    #[account(
        mut,
        seeds = [seed::CONFIG],
        bump = config.bump,
        constraint = valid_version(config.version, MAX_CONFIG_V) @ CommitteeVerifierError::InvalidVersion,
    )]
    pub config: Account<'info, Config>,

    #[account(address = config.proposed_owner @ CommitteeVerifierError::Unauthorized)]
    pub authority: Signer<'info>,
}

pub fn transfer_storage_location_admin(
    ctx: Context<UpdateStorageLocationAdmin>,
    proposed_storage_location_admin: Pubkey,
) -> Result<()> {
    let config = &mut ctx.accounts.config;

    require_keys_neq!(
        proposed_storage_location_admin,
        Pubkey::default(),
        CommitteeVerifierError::DefaultStorageLocationAdminProposal
    );
    require_keys_neq!(
        proposed_storage_location_admin,
        config.storage_location_admin,
        CommitteeVerifierError::RedundantStorageLocationAdminProposal
    );

    emit_cpi!(StorageLocationAdminTransferRequested {
        from: config.storage_location_admin,
        to: proposed_storage_location_admin,
    });

    config.proposed_storage_location_admin = proposed_storage_location_admin;
    Ok(())
}

pub fn accept_storage_location_admin(ctx: Context<AcceptStorageLocationAdmin>) -> Result<()> {
    let config = &mut ctx.accounts.config;

    emit_cpi!(StorageLocationAdminTransferred {
        from: config.storage_location_admin,
        to: config.proposed_storage_location_admin,
    });

    config.storage_location_admin = std::mem::take(&mut config.proposed_storage_location_admin);
    Ok(())
}

#[event_cpi]
#[derive(Accounts)]
pub struct UpdateStorageLocationAdmin<'info> {
    #[account(
        mut,
        seeds = [seed::CONFIG],
        bump = config.bump,
        constraint = valid_version(config.version, MAX_CONFIG_V) @ CommitteeVerifierError::InvalidVersion,
    )]
    pub config: Account<'info, Config>,

    #[account(address = config.storage_location_admin @ CommitteeVerifierError::Unauthorized)]
    pub authority: Signer<'info>,
}

#[event_cpi]
#[derive(Accounts)]
pub struct AcceptStorageLocationAdmin<'info> {
    #[account(
        mut,
        seeds = [seed::CONFIG],
        bump = config.bump,
        constraint = valid_version(config.version, MAX_CONFIG_V) @ CommitteeVerifierError::InvalidVersion,
    )]
    pub config: Account<'info, Config>,

    #[account(address = config.proposed_storage_location_admin @ CommitteeVerifierError::Unauthorized)]
    pub authority: Signer<'info>,
}

#[event_cpi]
#[derive(Accounts)]
pub struct MigrateConfigWithFinality<'info> {
    /// CHECK: holds the pre-migration config schema, which today's `Account<Config>` can no
    /// longer deserialize. The handler checks the `Config` discriminator, the version and the
    /// owner-based authority before overwriting the account in place.
    #[account(
        mut,
        seeds = [seed::CONFIG],
        bump,
        owner = crate::ID, // Initialized
    )]
    pub config: UncheckedAccount<'info>,

    /// Pays the rent top-up when the new layout is larger, hence `mut`. Checked against the
    /// pre-migration config's owner in the handler.
    // PERMISSIONLESS: see above.
    #[account(mut)]
    pub authority: Signer<'info>,
    pub system_program: Program<'info, System>,
}

/// Sweeps every fee this verifier accrued in `fee_token_mint` into the configured fee aggregator's
/// ATA.
///
/// Permissionless: the destination is pinned to `config.fee_aggregator`, so a caller can only
/// trigger the sweep, never redirect it.
pub fn withdraw_fee_tokens(ctx: Context<WithdrawFeeTokens>) -> Result<()> {
    require_keys_neq!(
        ctx.accounts.config.fee_aggregator,
        Pubkey::default(),
        CommitteeVerifierError::FeeAggregatorNotSet
    );

    let amount = ctx.accounts.fee_vault_token_account.amount;
    require_gt!(amount, 0, CommitteeVerifierError::InsufficientFunds);

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
