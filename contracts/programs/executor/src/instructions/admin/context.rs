use anchor_lang::prelude::*;
use ccip_common::seed;

use crate::error::ExecutorError;
use crate::program::Executor;
use crate::state::{valid_version, Config, ANCHOR_DISCRIMINATOR, MAX_CONFIG_V};

/// Deliberately not `#[event_cpi]`. That macro appends its own unconstrained
/// `program: UncheckedAccount`, which collides with the `Program<'info, Executor>` this context
/// needs for the upgrade-authority check, and there is no way to add the constraint back onto the
/// generated field. Declaring `event_authority` by hand is equivalent: `emit_cpi!` only reads
/// `ctx.accounts.event_authority`, and the self-CPI's callee just has to be present in the
/// transaction's account list — which `program` already does.
#[derive(Accounts)]
pub struct InitializeConfig<'info> {
    #[account(
        init,
        seeds = [seed::CONFIG],
        bump,
        payer = authority,
        space = ANCHOR_DISCRIMINATOR + Config::INIT_SPACE,
    )]
    pub config: Account<'info, Config>,

    #[account(mut)]
    pub authority: Signer<'info>,

    pub system_program: Program<'info, System>,

    /// CHECK: Only the event authority can invoke self-CPI
    #[account(address = crate::EVENT_AUTHORITY_AND_BUMP.0)]
    pub event_authority: UncheckedAccount<'info>,

    #[account(constraint = program.programdata_address()? == Some(program_data.key()))]
    pub program: Program<'info, Executor>,

    #[account(constraint = program_data.upgrade_authority_address == Some(authority.key()) @ ExecutorError::Unauthorized)]
    pub program_data: Account<'info, ProgramData>,
}

#[event_cpi]
#[derive(Accounts)]
pub struct UpdateConfig<'info> {
    #[account(
        mut,
        seeds = [seed::CONFIG],
        bump = config.bump,
        constraint = valid_version(config.version, MAX_CONFIG_V) @ ExecutorError::InvalidVersion,
    )]
    pub config: Account<'info, Config>,

    #[account(address = config.owner @ ExecutorError::Unauthorized)]
    pub authority: Signer<'info>,
}

#[event_cpi]
#[derive(Accounts)]
#[instruction(allowed_ccvs: Vec<Pubkey>)]
pub struct UpdateCCVs<'info> {
    #[account(
        mut,
        seeds = [seed::CONFIG],
        bump = config.bump,
        constraint = valid_version(config.version, MAX_CONFIG_V) @ ExecutorError::InvalidVersion,
        realloc = ANCHOR_DISCRIMINATOR + Config::dynamic_len(allowed_ccvs.len()),
        realloc::payer = authority,
        realloc::zero = false,
    )]
    pub config: Account<'info, Config>,

    #[account(mut, address = config.owner @ ExecutorError::Unauthorized)]
    pub authority: Signer<'info>,

    pub system_program: Program<'info, System>,
}
