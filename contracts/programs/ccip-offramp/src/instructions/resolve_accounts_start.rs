use std::vec;

use anchor_lang::prelude::*;

use ccip_common::resolution::*;
use ccip_common::seed;
use ccip_common::state::AccountSchemaVersion;

use crate::error::CcipOfframpError;
use crate::instruction;
use crate::instructions::{execute, get_ccvs_for_msg};
use crate::state::*;

/**
 * -- General API for account resolution --
 * Contains the single entrypoint to start account resolution of any instruction in the program that implements it.
 * It has the context for it and routes to the correct resolution logic based on the instruction discriminator,
 * which is implemented in each instruction's `resolve_accounts` submodule.
 * All other I/O types (ix data & response) are defined in ccip-common and are shared across contracts.
 */

#[derive(Accounts)]
pub struct ResolveAccountsStart<'info> {
    #[account(
        seeds = [seed::CONFIG],
        bump = config.bump,
        constraint = config.version == Config::MAX_V @ CcipOfframpError::InvalidVersion,
    )]
    pub config: Account<'info, Config>,
}

pub fn resolve_accounts_start(
    ctx: Context<ResolveAccountsStart>,
    params: ResolveAccountsParams,
) -> Result<ResolveAccountsResponse> {
    let (ix_discriminator, _data) = validate_resolution_start(&params, true)?;

    match ix_discriminator {
        instruction::ExecuteV2::DISCRIMINATOR => execute::resolve_accounts::start(ctx, params),
        instruction::GetCcvsForMsg::DISCRIMINATOR => {
            get_ccvs_for_msg::resolve_accounts::start(ctx, params)
        }
        _ => {
            msg!(
                "Unknown discriminator to resolve accounts for: {}",
                BytesWrapper(ix_discriminator)
            );
            Err(CcipOfframpError::InvalidInstructionData.into())
        }
    }
}
