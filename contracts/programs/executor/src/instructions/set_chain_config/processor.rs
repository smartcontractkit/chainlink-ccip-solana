use anchor_lang::prelude::*;

use ccip_common::seed;
use ccip_common::types::UsdCents;

use crate::error::ExecutorError;
use crate::event::ChainConfigSet;
use crate::state::{
    uninitialized, valid_version, ChainConfig, Config, ANCHOR_DISCRIMINATOR, MAX_CHAIN_CONFIG_V,
    MAX_CONFIG_V,
};

#[derive(AnchorSerialize, AnchorDeserialize, Debug)]
pub struct SetChainConfigParams {
    pub remote_chain_selector: u64,
    pub usd_cents_fee: UsdCents,
    pub enabled: bool,
}

pub fn set_chain_config(ctx: Context<SetChainConfig>, params: SetChainConfigParams) -> Result<()> {
    emit_cpi!(ChainConfigSet {
        remote_chain_selector: params.remote_chain_selector,
        usd_cents_fee: params.usd_cents_fee,
        enabled: params.enabled,
    });

    let chain_config = &mut ctx.accounts.chain_config;
    chain_config.set_inner(ChainConfig {
        version: MAX_CHAIN_CONFIG_V,
        bump: ctx.bumps.chain_config,
        chain_selector: params.remote_chain_selector,
        usd_cents_fee: params.usd_cents_fee,
        enabled: params.enabled,
    });
    Ok(())
}

#[event_cpi]
#[derive(Accounts)]
#[instruction(params: SetChainConfigParams)]
pub struct SetChainConfig<'info> {
    #[account(
        seeds = [seed::CONFIG],
        bump = config.bump,
        constraint = valid_version(config.version, MAX_CONFIG_V) @ ExecutorError::InvalidVersion,
    )]
    pub config: Account<'info, Config>,

    #[account(mut, address = config.owner @ ExecutorError::Unauthorized)]
    pub authority: Signer<'info>,

    #[account(
        init_if_needed,
        seeds = [seed::REMOTE_CHAIN, &params.remote_chain_selector.to_le_bytes()],
        bump,
        payer = authority,
        space = ANCHOR_DISCRIMINATOR + ChainConfig::INIT_SPACE,
        // A freshly initialized account has version 0; an existing one must be readable.
        constraint = uninitialized(chain_config.version)
            || valid_version(chain_config.version, MAX_CONFIG_V) @ ExecutorError::InvalidVersion,
    )]
    pub chain_config: Account<'info, ChainConfig>,

    pub system_program: Program<'info, System>,
}
