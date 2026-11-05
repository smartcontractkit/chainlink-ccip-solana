use anchor_lang::prelude::*;
use base_token_pool::{constants::*, errors::*, state::*};

use crate::state::*;
use crate::{ChainConfig, State};

#[derive(Accounts)]
#[instruction(remote_chain_selector: u64, mint: Pubkey)]
pub struct InitializeChainConfig<'info> {
    #[account(
        seeds = [POOL_STATE_SEED, mint.key().as_ref()],
        bump,
        constraint = valid_version(state.version, MAX_POOL_STATE_V) @ CcipTokenPoolError::InvalidVersion,
    )]
    pub state: Account<'info, State>,
    #[account(
        init,
        seeds = [POOL_CHAINCONFIG_SEED, remote_chain_selector.to_le_bytes().as_ref(), mint.key().as_ref()],
        bump,
        payer = authority,
        space = ANCHOR_DISCRIMINATOR + ChainConfig::INIT_SPACE
    )]
    pub chain_config: Account<'info, ChainConfig>,
    #[account(mut, address = state.config.owner)]
    pub authority: Signer<'info>,
    pub system_program: Program<'info, System>,
}

// initialize remote config (with no remote pools as it must be zero sized)
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

    require!(
        !cfg.token_address.is_zero(),
        CcipTokenPoolError::InvalidToken
    );

    ctx.accounts
        .chain_config
        .base
        .set(remote_chain_selector, mint, cfg)
}
