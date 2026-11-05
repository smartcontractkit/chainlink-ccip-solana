use std::vec;

use anchor_lang::prelude::*;

use ccip_common::resolution::*;
use ccip_common::seed;

use crate::context::{valid_version, MAX_CONFIG_V};
use crate::instruction as router_instruction;
use crate::instructions::ccip_send;
use crate::instructions::get_fee;
use crate::state::*;
use crate::CcipRouterError;

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
        bump,
        constraint = valid_version(config.version, MAX_CONFIG_V) @ CcipRouterError::InvalidVersion,
    )]
    pub config: Account<'info, Config>,
}

pub fn resolve_accounts_start(
    ctx: Context<ResolveAccountsStart>,
    params: ResolveAccountsParams,
) -> Result<ResolveAccountsResponse> {
    let (ix_discriminator, _data) = validate_resolution_start(&params, true)?;

    match ix_discriminator {
        router_instruction::GetFeeV2::DISCRIMINATOR => {
            get_fee::resolve_accounts::start(ctx, params)
        }
        router_instruction::CcipSendV2::DISCRIMINATOR => {
            ccip_send::resolve_accounts::start(ctx, params)
        }
        _ => {
            msg!(
                "Unknown discriminator to resolve accounts for: {}",
                BytesWrapper(ix_discriminator)
            );
            Err(CcipRouterError::InvalidInstructionData.into())
        }
    }
}
