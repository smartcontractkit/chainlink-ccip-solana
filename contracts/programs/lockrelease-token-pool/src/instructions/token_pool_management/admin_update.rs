use anchor_lang::prelude::*;
use anchor_spl::token_interface::Mint;
use base_token_pool::{constants::*, errors::*};

use crate::program::LockreleaseTokenPool;
use crate::state::*;

#[derive(Accounts)]
pub struct AdminUpdateTokenPool<'info> {
    #[account(
        mut,
        seeds = [POOL_STATE_SEED, mint.key().as_ref()],
        bump,
        constraint = valid_version(state.version, MAX_POOL_STATE_V) @ CcipTokenPoolError::InvalidVersion,
    )]
    pub state: Account<'info, State>, // config PDA for token pool
    pub mint: InterfaceAccount<'info, Mint>, // underlying token that the pool wraps

    #[account(mut)]
    pub authority: Signer<'info>,

    #[account(constraint = program.programdata_address()? == Some(program_data.key()))]
    pub program: Program<'info, LockreleaseTokenPool>,

    // The upgrade authority of the token pool program can update certain values of their token pools only (router and rmn addresses for testing)
    #[account(constraint = allowed_admin_modify_token_pool(&program_data, &authority, &state) @ CcipTokenPoolError::Unauthorized)]
    pub program_data: Account<'info, ProgramData>,
}

/// Checks that the authority and the token pool owner are the upgrade authority
pub fn allowed_admin_modify_token_pool(
    program_data: &Account<ProgramData>,
    authority: &Signer,
    state: &Account<State>,
) -> bool {
    program_data.upgrade_authority_address == Some(authority.key()) && // Only the upgrade authority of the token pool program can modify certain values of a given token pool
    state.config.owner == authority.key() // if only if the token pool is owned by the upgrade authority
}

// set_router changes the expected signers for mint/release + burn/lock method calls
// this is used to update the router address
pub fn set_router(ctx: Context<AdminUpdateTokenPool>, new_router: Pubkey) -> Result<()> {
    ctx.accounts
        .state
        .config
        .set_router(new_router, ctx.program_id)
}

pub fn set_rmn(ctx: Context<AdminUpdateTokenPool>, rmn_address: Pubkey) -> Result<()> {
    ctx.accounts.state.config.set_rmn(rmn_address)
}

#[derive(Accounts)]
#[instruction(mint: Pubkey)]
pub struct SetRateLimitAdmin<'info> {
    #[account(
        mut,
        seeds = [POOL_STATE_SEED, mint.key().as_ref()],
        bump,
        constraint = valid_version(state.version, MAX_POOL_STATE_V) @ CcipTokenPoolError::InvalidVersion,
    )]
    pub state: Account<'info, State>,
    #[account(mut, address = state.config.owner @ CcipTokenPoolError::Unauthorized)]
    pub authority: Signer<'info>,
}

// set rate limit admin
pub fn set_rate_limit_admin(
    ctx: Context<SetRateLimitAdmin>,
    _mint: Pubkey,
    new_rate_limit_admin: Pubkey,
) -> Result<()> {
    ctx.accounts
        .state
        .config
        .set_rate_limit_admin(new_rate_limit_admin)
}
