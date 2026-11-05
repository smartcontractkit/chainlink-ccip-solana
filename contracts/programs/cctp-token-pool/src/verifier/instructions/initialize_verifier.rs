use anchor_lang::prelude::*;
use ccip_common::account_management;

use crate::error::CctpPoolVerifierError;
use crate::program::CctpTokenPool;
use crate::verifier::state::*;

pub fn initialize_verifier<'info>(ctx: Context<'info, InitializeVerifier<'info>>) -> Result<()> {
    // Initialize or migrate Config PDA
    {
        let account_info = ctx.accounts.config.as_ref();

        let target_space = Config::DISCRIMINATOR.len() + Config::INIT_SPACE;
        let new_config = Config {
            version: Config::MAX_V,
            owner: ctx.accounts.authority.key(),
            proposed_owner: Pubkey::default(),
            storage_location_admin: ctx.accounts.authority.key(),
            allowlist_admin: ctx.accounts.authority.key(),
            proposed_storage_location_admin: Pubkey::default(),
            bump: ctx.bumps.config,
            fee_aggregator: Pubkey::default(),
            allowed_finality_config: Default::default(),
            fast_finality_bps: 1, // 1bps, 0.01% https://developers.circle.com/cctp/concepts/fees#fee-tables
        };

        if account_info.owner == &system_program::ID {
            // Not yet created (owned by the system program): create it same as init
            let bump = ctx.bumps.config;
            let seeds = &[verifier_seed::CONFIG, &[bump]];
            account_management::initialize_from_authority(
                account_info,
                &ctx.accounts.system_program.to_account_info(),
                &ctx.accounts.authority.to_account_info(),
                target_space,
                &[seeds],
                ctx.program_id,
            )?;
            let mut config_account: Account<Config> = Account::try_from_unchecked(account_info)?;
            config_account.set_inner(new_config);
            config_account.exit(ctx.program_id)?;
        } else {
            // Already owned: migrate from the legacy PoolConfig PDA
            let mut migration: Migration<PoolConfig, Config> = Migration::try_from(account_info)?;

            account_management::resize(
                account_info,
                target_space,
                &ctx.accounts.authority.to_account_info(),
            )?;

            migration.migrate(new_config)?;
            migration.exit(ctx.program_id)?;
        }
    }

    // Initialize the StorageLocations PDA
    ctx.accounts.storage_locations.set_inner(StorageLocations {
        version: StorageLocations::MAX_V,
        bump: ctx.bumps.storage_locations,
        locations: vec![],
    });

    Ok(())
}

#[derive(Accounts)]
pub struct InitializeVerifier<'info> {
    #[account(
        mut,
        seeds = [verifier_seed::CONFIG],
        bump,
    )]
    pub config: UncheckedAccount<'info>,

    #[account(
        init,
        seeds = [verifier_seed::STORAGE_LOCATIONS],
        bump,
        payer = authority,
        space = StorageLocations::DISCRIMINATOR.len() + StorageLocations::INIT_SPACE,
    )]
    pub storage_locations: Account<'info, StorageLocations>,

    #[account(mut)]
    pub authority: Signer<'info>,

    pub system_program: Program<'info, System>,

    #[account(constraint = program.programdata_address()? == Some(program_data.key()))]
    pub program: Program<'info, CctpTokenPool>,

    #[account(constraint = program_data.upgrade_authority_address == Some(authority.key()) @ CctpPoolVerifierError::Unauthorized)]
    pub program_data: Account<'info, ProgramData>,
}

// Helper type just for the migration. This is intentionally not `pub` and hidden from the
// IDL and the rest of the state. This represents what used to be the old PoolConfig PDA,
// which now gets migrated in the verifier's Config
#[account]
struct PoolConfig {
    pub version: u8,
}
