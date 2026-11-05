use anchor_lang::prelude::*;
use anchor_lang::system_program;
use base_token_pool::constants::valid_version;
use base_token_pool::errors::CcipTokenPoolError;
use ccip_common::account_management::initialize_from_authority;
use ccip_common::messages::FinalityConfig;

use crate::constants::{VERIFIER_CONFIG_SEED, VERIFIER_STORAGE_LOCATIONS_SEED};
use crate::errors::LombardTokenPoolError;
use crate::events::*;
use crate::program::LombardTokenPool;
use crate::state::{
    PoolConfig, VerifierConfig, VerifierStorageLocations, MAX_VERIFIER_CONFIG_V,
    MAX_VERIFIER_STORAGE_LOCATIONS_V,
};

/// Domain-separation tag for this verifier's `version_tag`, analogous to EVM's immutable
/// `LombardVerifier.versionTag()`.
/// Must match the string EVM's LombardVerifier is constructed with
/// (chainlink-ccv's DefaultVerifierVersionHex = keccak256("LombardVerifier 2.1.0")[:4])
pub fn lombard_verifier_version_tag() -> [u8; 4] {
    let hash = solana_keccak_hasher::hash(b"LombardVerifier 2.1.0");
    let mut tag = [0u8; 4];
    tag.copy_from_slice(&hash.to_bytes()[..4]);
    tag
}

pub fn initialize_verifier<'info>(ctx: Context<'info, InitializeVerifier<'info>>) -> Result<()> {
    let verifier_config_info: &'info AccountInfo<'info> = ctx.accounts.verifier_config.as_ref();
    let target_space = VerifierConfig::DISCRIMINATOR.len() + VerifierConfig::INIT_SPACE;
    let new_verifier_config = VerifierConfig {
        version: MAX_VERIFIER_CONFIG_V,
        bump: ctx.bumps.verifier_config,
        owner: ctx.accounts.authority.key(),
        proposed_owner: Pubkey::default(),
        storage_location_admin: ctx.accounts.authority.key(),
        proposed_storage_location_admin: Pubkey::default(),
        allowlist_admin: ctx.accounts.authority.key(),
        fee_aggregator: Pubkey::default(),
        allowed_finality_config: FinalityConfig::default(),
        version_tag: lombard_verifier_version_tag(),
        supported_tokens: vec![],
    };

    // Not yet created (owned by the system program): create it same as init
    if verifier_config_info.owner == &system_program::ID {
        let bump = ctx.bumps.verifier_config;
        let seeds: &[&[&[u8]]] = &[&[VERIFIER_CONFIG_SEED, &[bump]]];
        initialize_from_authority(
            verifier_config_info,
            &ctx.accounts.system_program.to_account_info(),
            &ctx.accounts.authority.to_account_info(),
            target_space,
            seeds,
            ctx.program_id,
        )?;

        let mut verifier_config: Account<VerifierConfig> =
            Account::try_from_unchecked(verifier_config_info)?;
        verifier_config.set_inner(new_verifier_config);
        verifier_config.exit(ctx.program_id)?;
    } else {
        // Already owned: migrate from the legacy PoolConfig PDA
        let mut migration: Migration<PoolConfig, VerifierConfig> =
            Migration::try_from(verifier_config_info)?;

        let required_lamports = Rent::get()?
            .minimum_balance(target_space)
            .saturating_sub(verifier_config_info.lamports());
        if required_lamports > 0 {
            system_program::transfer(
                CpiContext::new(
                    ctx.accounts.system_program.key(),
                    system_program::Transfer {
                        from: ctx.accounts.authority.to_account_info(),
                        to: verifier_config_info.clone(),
                    },
                ),
                required_lamports,
            )?;
        }
        verifier_config_info.resize(target_space)?;

        migration.migrate(new_verifier_config)?;
        migration.exit(ctx.program_id)?;
    }

    ctx.accounts
        .verifier_storage_locations
        .set_inner(VerifierStorageLocations {
            version: MAX_VERIFIER_STORAGE_LOCATIONS_V,
            bump: ctx.bumps.verifier_storage_locations,
            locations: vec![],
        });

    Ok(())
}

#[derive(Accounts)]
pub struct InitializeVerifier<'info> {
    /// CHECK: might not yet be created or the legacy `PoolConfig` PDA requiring migration
    #[account(mut, seeds = [VERIFIER_CONFIG_SEED], bump)]
    pub verifier_config: UncheckedAccount<'info>,

    #[account(
        init,
        seeds = [VERIFIER_STORAGE_LOCATIONS_SEED],
        bump,
        payer = authority,
        space = VerifierStorageLocations::DISCRIMINATOR.len() + VerifierStorageLocations::INIT_SPACE,
    )]
    pub verifier_storage_locations: Account<'info, VerifierStorageLocations>,

    #[account(mut)]
    pub authority: Signer<'info>,

    pub system_program: Program<'info, System>,

    // Verifier initialization only allowed by the program's upgrade authority.
    #[account(constraint = program.programdata_address()? == Some(program_data.key()))]
    pub program: Program<'info, LombardTokenPool>,

    #[account(constraint = program_data.upgrade_authority_address == Some(authority.key()) @ CcipTokenPoolError::Unauthorized)]
    pub program_data: Account<'info, ProgramData>,
}

#[derive(Accounts)]
pub struct UpdateVerifierConfig<'info> {
    #[account(
        mut,
        seeds = [VERIFIER_CONFIG_SEED],
        bump = verifier_config.bump,
        constraint = valid_version(verifier_config.version, MAX_VERIFIER_CONFIG_V) @ CcipTokenPoolError::InvalidVersion,
    )]
    pub verifier_config: Account<'info, VerifierConfig>,

    #[account(address = verifier_config.owner @ CcipTokenPoolError::Unauthorized)]
    pub authority: Signer<'info>,
}

pub fn transfer_verifier_ownership(
    ctx: Context<UpdateVerifierConfig>,
    proposed_owner: Pubkey,
) -> Result<()> {
    let config = &mut ctx.accounts.verifier_config;

    require_keys_neq!(
        proposed_owner,
        Pubkey::default(),
        LombardTokenPoolError::DefaultAdminProposal
    );
    require_keys_neq!(
        proposed_owner,
        config.owner,
        LombardTokenPoolError::RedundantAdminProposal
    );

    emit!(VerifierOwnershipTransferRequested {
        from: config.owner,
        to: proposed_owner,
    });

    config.proposed_owner = proposed_owner;
    Ok(())
}

#[derive(Accounts)]
pub struct AcceptVerifierOwnership<'info> {
    #[account(
        mut,
        seeds = [VERIFIER_CONFIG_SEED],
        bump = verifier_config.bump,
        constraint = valid_version(verifier_config.version, MAX_VERIFIER_CONFIG_V) @ CcipTokenPoolError::InvalidVersion,
    )]
    pub verifier_config: Account<'info, VerifierConfig>,

    #[account(address = verifier_config.proposed_owner @ CcipTokenPoolError::Unauthorized)]
    pub authority: Signer<'info>,
}

pub fn accept_verifier_ownership(ctx: Context<AcceptVerifierOwnership>) -> Result<()> {
    let config = &mut ctx.accounts.verifier_config;

    emit!(VerifierOwnershipTransferred {
        from: config.owner,
        to: config.proposed_owner,
    });

    // NOTE: take() resets proposed_owner to default
    config.owner = std::mem::take(&mut config.proposed_owner);
    Ok(())
}

pub fn transfer_verifier_storage_location_admin(
    ctx: Context<UpdateVerifierConfig>,
    proposed_storage_location_admin: Pubkey,
) -> Result<()> {
    let config = &mut ctx.accounts.verifier_config;

    require_keys_neq!(
        proposed_storage_location_admin,
        Pubkey::default(),
        LombardTokenPoolError::DefaultAdminProposal
    );
    require_keys_neq!(
        proposed_storage_location_admin,
        config.storage_location_admin,
        LombardTokenPoolError::RedundantAdminProposal
    );

    emit!(VerifierStorageLocationAdminTransferRequested {
        from: config.storage_location_admin,
        to: proposed_storage_location_admin,
    });

    config.proposed_storage_location_admin = proposed_storage_location_admin;
    Ok(())
}

#[derive(Accounts)]
pub struct AcceptVerifierStorageLocationAdmin<'info> {
    #[account(
        mut,
        seeds = [VERIFIER_CONFIG_SEED],
        bump = verifier_config.bump,
        constraint = valid_version(verifier_config.version, MAX_VERIFIER_CONFIG_V) @ CcipTokenPoolError::InvalidVersion,
    )]
    pub verifier_config: Account<'info, VerifierConfig>,

    #[account(address = verifier_config.proposed_storage_location_admin @ CcipTokenPoolError::Unauthorized)]
    pub authority: Signer<'info>,
}

pub fn accept_verifier_storage_location_admin(
    ctx: Context<AcceptVerifierStorageLocationAdmin>,
) -> Result<()> {
    let config = &mut ctx.accounts.verifier_config;

    emit!(VerifierStorageLocationAdminTransferred {
        from: config.storage_location_admin,
        to: config.proposed_storage_location_admin,
    });

    config.storage_location_admin = std::mem::take(&mut config.proposed_storage_location_admin);
    Ok(())
}

pub fn set_verifier_allowlist_admin(
    ctx: Context<UpdateVerifierConfig>,
    allowlist_admin: Pubkey,
) -> Result<()> {
    let config = &mut ctx.accounts.verifier_config;
    let old = config.allowlist_admin;
    config.allowlist_admin = allowlist_admin;

    emit!(VerifierAllowlistAdminSet {
        old,
        new: config.allowlist_admin,
    });

    Ok(())
}

pub fn set_verifier_fee_aggregator(
    ctx: Context<UpdateVerifierConfig>,
    fee_aggregator: Pubkey,
) -> Result<()> {
    let config = &mut ctx.accounts.verifier_config;
    let old = config.fee_aggregator;
    config.fee_aggregator = fee_aggregator;

    emit!(VerifierFeeAggregatorSet {
        old,
        new: config.fee_aggregator,
    });

    Ok(())
}

pub fn set_verifier_allowed_finality_config(
    ctx: Context<UpdateVerifierConfig>,
    finality: FinalityConfig,
) -> Result<()> {
    let config = &mut ctx.accounts.verifier_config;
    let old = config.allowed_finality_config;
    config.allowed_finality_config = finality;

    emit!(VerifierAllowedFinalityConfigSet {
        old,
        new: config.allowed_finality_config,
    });

    Ok(())
}
