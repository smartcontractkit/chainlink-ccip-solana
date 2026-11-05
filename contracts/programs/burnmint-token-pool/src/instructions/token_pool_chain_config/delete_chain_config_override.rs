use anchor_lang::prelude::*;
use base_token_pool::{constants::*, errors::*, events::*};

use crate::state::*;
use crate::{ChainConfigOverride, DeleteChainConfigOverrideParams, State};

#[event_cpi]
#[derive(Accounts)]
#[instruction(params: DeleteChainConfigOverrideParams)]
pub struct DeleteChainConfigOverride<'info> {
    #[account(
        seeds = [POOL_STATE_SEED, params.mint.as_ref()],
        bump,
        constraint = valid_version(state.version, MAX_POOL_STATE_V) @ CcipTokenPoolError::InvalidVersion,
    )]
    pub state: Account<'info, State>,
    #[account(
        mut,
        seeds = [POOL_OVERRIDE_CHAINCONFIG_SEED, params.remote_chain_selector.to_le_bytes().as_ref(), params.mint.key().as_ref(), params.override_type.as_bytes()],
        bump = chain_config_override.bump,
        constraint = valid_version(chain_config_override.version, MAX_POOL_OVERRIDE_V) @ CcipTokenPoolError::InvalidVersion,
        close = authority,
    )]
    pub chain_config_override: Account<'info, ChainConfigOverride>,
    #[account(mut, address = state.config.owner)]
    pub authority: Signer<'info>,
}

pub fn delete_chain_config_override(
    ctx: Context<DeleteChainConfigOverride>,
    params: DeleteChainConfigOverrideParams,
) -> Result<()> {
    emit_cpi!(OverrideChainConfigRemoved {
        chain_selector: params.remote_chain_selector,
        override_type: params.override_type,
        mint: params.mint,
    });
    Ok(())
}
