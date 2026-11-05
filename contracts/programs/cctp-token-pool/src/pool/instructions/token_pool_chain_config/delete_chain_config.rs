use anchor_lang::prelude::*;

use base_token_pool::constants::{POOL_CHAINCONFIG_SEED, POOL_STATE_SEED};
use base_token_pool::errors::CcipTokenPoolError;
use base_token_pool::events::RemoteChainRemoved;

use ccip_common::state::AccountSchemaVersion;

use crate::pool::state::{ChainConfig, State};

pub fn delete_chain_config(
    _ctx: Context<DeleteChainConfig>,
    remote_chain_selector: u64,
    mint: Pubkey,
) -> Result<()> {
    emit!(RemoteChainRemoved {
        chain_selector: remote_chain_selector,
        mint
    });
    Ok(())
}

#[derive(Accounts)]
#[instruction(remote_chain_selector: u64, mint: Pubkey)]
pub struct DeleteChainConfig<'info> {
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
        close = authority,
    )]
    pub chain_config: Account<'info, ChainConfig>,

    #[account(mut, address = state.config.owner)]
    pub authority: Signer<'info>,
}
