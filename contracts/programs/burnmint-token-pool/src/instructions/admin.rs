use anchor_lang::prelude::*;
use base_token_pool::{constants::*, errors::*, events::*};

use crate::program::BurnmintTokenPool;
use crate::state::*;

#[derive(Accounts)]
pub struct InitGlobalConfig<'info> {
    #[account(
        init,
        seeds = [CONFIG_SEED],
        bump,
        payer = authority,
        space = ANCHOR_DISCRIMINATOR + PoolConfig::INIT_SPACE,
    )]
    pub config: Account<'info, PoolConfig>, // Global Config PDA of the Token Pool

    #[account(mut)]
    pub authority: Signer<'info>,
    pub system_program: Program<'info, System>,

    // Ensures that the provided program is the BurnmintTokenPool program,
    // and that its associated program data account matches the expected one.
    // This guarantees that only the program's upgrade authority can modify the global config.
    #[account(constraint = program.programdata_address()? == Some(program_data.key()))]
    pub program: Program<'info, BurnmintTokenPool>,
    // Global Config updates only allowed by program upgrade authority
    #[account(constraint = program_data.upgrade_authority_address == Some(authority.key()) @ CcipTokenPoolError::Unauthorized)]
    pub program_data: Account<'info, ProgramData>,
}

#[derive(Accounts)]
pub struct UpdateGlobalConfig<'info> {
    #[account(
        mut,
        seeds = [CONFIG_SEED],
        bump,
        constraint = valid_version(config.version, MAX_POOL_CONFIG_V) @ CcipTokenPoolError::InvalidVersion,
    )]
    pub config: Account<'info, PoolConfig>, // Global Config PDA of the Token Pool

    pub authority: Signer<'info>,

    // Ensures that the provided program is the BurnmintTokenPool program,
    // and that its associated program data account matches the expected one.
    // This guarantees that only the program's upgrade authority can modify the global config.
    #[account(constraint = program.programdata_address()? == Some(program_data.key()))]
    pub program: Program<'info, BurnmintTokenPool>,
    // Global Config updates only allowed by program upgrade authority
    #[account(constraint = program_data.upgrade_authority_address == Some(authority.key()) @ CcipTokenPoolError::Unauthorized)]
    pub program_data: Account<'info, ProgramData>,
}

pub fn init_global_config(
    ctx: Context<InitGlobalConfig>,
    router_address: Pubkey,
    rmn_address: Pubkey,
) -> Result<()> {
    ctx.accounts.config.set_inner(PoolConfig {
        self_served_allowed: false,
        version: MAX_POOL_CONFIG_V,
        router: router_address,
        rmn_remote: rmn_address,
    });

    emit!(GlobalConfigUpdated {
        self_served_allowed: ctx.accounts.config.self_served_allowed,
        router: ctx.accounts.config.router,
        rmn_remote: ctx.accounts.config.rmn_remote,
    });

    Ok(())
}

pub fn update_self_served_allowed(
    ctx: Context<UpdateGlobalConfig>,
    self_served_allowed: bool,
) -> Result<()> {
    ctx.accounts.config.self_served_allowed = self_served_allowed;

    emit!(GlobalConfigUpdated {
        self_served_allowed: ctx.accounts.config.self_served_allowed,
        router: ctx.accounts.config.router,
        rmn_remote: ctx.accounts.config.rmn_remote,
    });

    Ok(())
}

pub fn update_default_router(
    ctx: Context<UpdateGlobalConfig>,
    router_address: Pubkey,
) -> Result<()> {
    ctx.accounts.config.router = router_address;

    emit!(GlobalConfigUpdated {
        self_served_allowed: ctx.accounts.config.self_served_allowed,
        router: ctx.accounts.config.router,
        rmn_remote: ctx.accounts.config.rmn_remote,
    });

    Ok(())
}

pub fn update_default_rmn(ctx: Context<UpdateGlobalConfig>, rmn_address: Pubkey) -> Result<()> {
    ctx.accounts.config.rmn_remote = rmn_address;

    emit!(GlobalConfigUpdated {
        self_served_allowed: ctx.accounts.config.self_served_allowed,
        router: ctx.accounts.config.router,
        rmn_remote: ctx.accounts.config.rmn_remote,
    });

    Ok(())
}
