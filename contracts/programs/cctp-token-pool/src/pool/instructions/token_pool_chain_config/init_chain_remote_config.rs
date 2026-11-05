use anchor_lang::prelude::*;

use base_token_pool::constants::{POOL_CHAINCONFIG_SEED, POOL_STATE_SEED};
use base_token_pool::errors::CcipTokenPoolError;
use base_token_pool::state::RemoteConfig;

use ccip_common::state::AccountSchemaVersion;

use crate::pool::state::{ChainConfig, State};

pub fn init_chain_remote_config(
    ctx: Context<InitializeChainConfig>,
    remote_chain_selector: u64,
    mint: Pubkey,
    cfg: RemoteConfig,
) -> Result<()> {
    require!(
        cfg.pool_addresses.is_empty(),
        CcipTokenPoolError::NonemptyPoolAddressesInit
    );

    // INV-PCFG-1: the remote token address must be set — a zero/empty address would make the
    // remote config unusable and is never valid.
    require!(
        !cfg.token_address.is_zero(),
        CcipTokenPoolError::ZeroTokenAddress
    );

    ctx.accounts.chain_config.version = 1;

    ctx.accounts
        .chain_config
        .base
        .set(remote_chain_selector, mint, cfg)
}

#[derive(Accounts)]
#[instruction(remote_chain_selector: u64, mint: Pubkey)]
pub struct InitializeChainConfig<'info> {
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
        init,
        seeds = [
            POOL_CHAINCONFIG_SEED,
            &remote_chain_selector.to_le_bytes(),
            mint.key().as_ref(),
        ],
        bump,
        payer = authority,
        space = ChainConfig::DISCRIMINATOR.len() + ChainConfig::INIT_SPACE,
    )]
    pub chain_config: Account<'info, ChainConfig>,

    #[account(mut, address = state.config.owner)]
    pub authority: Signer<'info>,

    pub system_program: Program<'info, System>,
}
