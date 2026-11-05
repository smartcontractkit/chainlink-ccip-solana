use anchor_lang::prelude::*;

use base_token_pool::constants::{POOL_CHAINCONFIG_SEED, POOL_STATE_SEED};
use base_token_pool::errors::CcipTokenPoolError;
use base_token_pool::rate_limiter::RateLimitConfig;

use ccip_common::state::AccountSchemaVersion;

use crate::pool::state::{ChainConfig, State};

pub fn set_chain_rate_limit(
    ctx: Context<SetChainRateLimit>,
    remote_chain_selector: u64,
    mint: Pubkey,
    inbound: RateLimitConfig,
    outbound: RateLimitConfig,
) -> Result<()> {
    ctx.accounts.chain_config.base.set_chain_rate_limit(
        remote_chain_selector,
        mint,
        inbound,
        outbound,
    )
}

#[derive(Accounts)]
#[instruction(remote_chain_selector: u64, mint: Pubkey)]
pub struct SetChainRateLimit<'info> {
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
    )]
    pub chain_config: Account<'info, ChainConfig>,

    #[account(mut, constraint = authority.key() == state.config.owner || authority.key() == state.config.rate_limit_admin @ CcipTokenPoolError::Unauthorized)]
    pub authority: Signer<'info>,
}
