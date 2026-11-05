use anchor_lang::prelude::*;
use base_token_pool::{constants::*, errors::*, events::*};

use crate::state::*;
use crate::{ChainConfigOverride, InitChainConfigOverrideParams, State};

#[event_cpi]
#[derive(Accounts)]
#[instruction(params: InitChainConfigOverrideParams)]
pub struct InitializeChainConfigOverride<'info> {
    #[account(
        seeds = [POOL_STATE_SEED, params.mint.as_ref()],
        bump,
        constraint = valid_version(state.version, MAX_POOL_STATE_V) @ CcipTokenPoolError::InvalidVersion,
    )]
    pub state: Account<'info, State>,
    #[account(
        init,
        seeds = [POOL_OVERRIDE_CHAINCONFIG_SEED, params.remote_chain_selector.to_le_bytes().as_ref(), params.mint.key().as_ref(), params.override_type.as_bytes()],
        bump,
        payer = authority,
        space = ANCHOR_DISCRIMINATOR + ChainConfigOverride::INIT_SPACE
    )]
    pub chain_config_override: Account<'info, ChainConfigOverride>,
    #[account(mut, address = state.config.owner)]
    pub authority: Signer<'info>,
    pub system_program: Program<'info, System>,
}

pub fn init_chain_config_override(
    ctx: Context<InitializeChainConfigOverride>,
    params: InitChainConfigOverrideParams,
) -> Result<()> {
    ctx.accounts.chain_config_override.bump = ctx.bumps.chain_config_override;
    ctx.accounts.chain_config_override.version = MAX_POOL_OVERRIDE_V;
    let override_config = &mut ctx.accounts.chain_config_override.base;
    override_config.remote_chain_selector = params.remote_chain_selector;
    override_config.mint = params.mint;
    override_config.override_type = params.override_type;
    override_config.set_rate_limit(params.inbound.clone(), params.outbound.clone())?;

    emit_cpi!(OverrideChainConfigConfigured {
        chain_selector: params.remote_chain_selector,
        override_type: params.override_type,
        mint: params.mint,
        outbound_rate_limit: params.outbound,
        inbound_rate_limit: params.inbound,
    });

    Ok(())
}
