use anchor_lang::prelude::*;

use base_token_pool::constants::{POOL_CHAINCONFIG_SEED, POOL_STATE_SEED};
use base_token_pool::errors::CcipTokenPoolError;
use base_token_pool::state::RemoteAddress;

use ccip_common::state::AccountSchemaVersion;

use crate::pool::state::{ChainConfig, State};

pub fn append_remote_pool_addresses(
    ctx: Context<AppendRemotePoolAddresses>,
    remote_chain_selector: u64,
    mint: Pubkey,
    addresses: Vec<RemoteAddress>,
) -> Result<()> {
    ctx.accounts.chain_config.base.append_remote_pool_addresses(
        remote_chain_selector,
        mint,
        addresses,
    )
}

#[derive(Accounts)]
#[instruction(remote_chain_selector: u64, mint: Pubkey, addresses: Vec<RemoteAddress>)]
pub struct AppendRemotePoolAddresses<'info> {
    #[account(
        seeds = [
            POOL_STATE_SEED,
            mint.key().as_ref()
        ],
        bump,
        constraint = state.is_latest_version() @ CcipTokenPoolError::InvalidVersion,
    )]
    pub state: Account<'info, State>,

    #[account(
        mut,
        seeds = [
            POOL_CHAINCONFIG_SEED,
            &remote_chain_selector.to_le_bytes(),
            mint.key().as_ref(),
        ],
        bump,
        constraint = chain_config.is_latest_version() @ CcipTokenPoolError::InvalidVersion,
        realloc = ChainConfig::DISCRIMINATOR.len() + ChainConfig::INIT_SPACE
            + chain_config.base.remote.pool_addresses.iter().map(RemoteAddress::space).sum::<usize>()
            + addresses.iter().map(RemoteAddress::space).sum::<usize>(),
        realloc::payer = authority,
        realloc::zero = false
    )]
    pub chain_config: Account<'info, ChainConfig>,

    #[account(mut, address = state.config.owner)]
    pub authority: Signer<'info>,

    pub system_program: Program<'info, System>,
}
