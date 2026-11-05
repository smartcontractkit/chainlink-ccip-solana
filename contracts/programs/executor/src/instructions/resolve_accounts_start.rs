use anchor_lang::prelude::*;

use ccip_common::resolution::*;
use ccip_common::seed;

use crate::error::ExecutorError;
use crate::instruction;
use crate::instructions::admin;
use crate::instructions::get_fee;
use crate::instructions::set_chain_config;
use crate::state::Config;
use crate::state::{valid_version, MAX_CONFIG_V};

#[derive(Accounts)]
pub struct ResolveAccountsStart<'info> {
    #[account(
        seeds = [seed::CONFIG],
        bump = config.bump,
        constraint = valid_version(config.version, MAX_CONFIG_V) @ ExecutorError::InvalidVersion,
    )]
    pub config: Account<'info, Config>,
}

pub fn resolve_accounts_start(
    ctx: Context<ResolveAccountsStart>,
    params: ResolveAccountsParams,
) -> Result<ResolveAccountsResponse> {
    let (ix_discriminator, _data) = validate_resolution_start(&params, false)?;

    match ix_discriminator {
        instruction::TransferOwnership::DISCRIMINATOR
        | instruction::AcceptOwnership::DISCRIMINATOR => {
            admin::resolve_accounts::start(ctx, params)
        }
        instruction::SetChainConfig::DISCRIMINATOR => {
            set_chain_config::resolve_accounts::start(ctx, params)
        }
        instruction::GetFee::DISCRIMINATOR => get_fee::resolve_accounts::start(ctx, params),
        _ => {
            msg!(
                "Unknown discriminator to resolve accounts for: {}",
                BytesWrapper(ix_discriminator)
            );
            Err(ExecutorError::InvalidInstructionData.into())
        }
    }
}
