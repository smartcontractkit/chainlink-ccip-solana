use anchor_lang::prelude::*;
use base_token_pool::{constants::*, errors::*, v2::*};

use ccip_common::state::AccountSchemaVersion;

use crate::pool::state::*;

#[derive(Accounts)]
#[instruction(get_fee_in: GetFeeParams)]
pub struct GetFeeContext<'info> {
    #[account(
        seeds = [POOL_STATE_SEED, state.config.mint.as_ref()],
        bump,
        constraint = state.is_latest_version() @ CcipTokenPoolError::InvalidVersion,
        constraint = state.config.mint == get_fee_in.local_token @ CcipTokenPoolError::InvalidToken,
    )]
    pub state: Account<'info, State>,

    #[account(
        seeds = [POOL_CHAINCONFIG_SEED, get_fee_in.remote_chain_selector.to_le_bytes().as_ref(), state.config.mint.as_ref()],
        bump,
        constraint = chain_config.is_latest_version() @ CcipTokenPoolError::InvalidVersion,
    )]
    pub chain_config: Account<'info, ChainConfig>,

    /// CHECK: V2 chain config PDA holding the allowed finality policy and token transfer config for this lane.
    /// May or may not be initialized
    #[account(
        seeds = [POOL_CHAINCONFIG_SEED_2_0, get_fee_in.remote_chain_selector.to_le_bytes().as_ref(), state.config.mint.as_ref()],
        bump,
    )]
    pub chain_config_v2: UncheckedAccount<'info>,
}

pub fn get_fee(ctx: Context<GetFeeContext>, get_fee_in: GetFeeParams) -> Result<GetFeeResult> {
    let maybe_chain_config = ChainConfigV2::try_load(&ctx.accounts.chain_config_v2)?;

    let allowed_finality_config = maybe_chain_config
        .as_ref()
        .map(|c| c.allowed_finality_config)
        .unwrap_or_default();

    let finality_allowed = allowed_finality_config
        .ensure_requested_finality_allowed(get_fee_in.requested_finality_config);
    require!(
        finality_allowed,
        CcipTokenPoolError::InvalidRequestedFinality
    );

    let is_fast = !get_fee_in.requested_finality_config.is_wait_for_finality();

    if let Some(ChainConfigV2 {
        token_transfer_fee_config,
        ..
    }) = maybe_chain_config
    {
        let (fee, token_fee_bps) = if is_fast {
            (
                token_transfer_fee_config.fast_finality_fee,
                token_transfer_fee_config.fast_finality_bps_fee,
            )
        } else {
            (
                token_transfer_fee_config.finality_fee,
                token_transfer_fee_config.finality_bps_fee,
            )
        };

        Ok(GetFeeResult {
            dest_gas_limit: token_transfer_fee_config.dest_gas_overhead,
            dest_bytes_overhead: token_transfer_fee_config.dest_bytes_overhead,
            fee,
            token_fee_bps,
            is_enabled: token_transfer_fee_config.is_enabled,
        })
    } else {
        // No fee config initialized for this lane: return zeros with is_enabled=false to signal
        // the OnRamp to use the FeeQuoter fallback.
        Ok(GetFeeResult {
            is_enabled: false,
            ..Default::default()
        })
    }
}
