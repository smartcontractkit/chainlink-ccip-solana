//! `get_pool_fee`: the pool-role fee quote, mirroring `burnmint-token-pool`'s own `get_fee`
//! processor (see reference read during implementation) - a static per-lane lookup from
//! `ChainConfigV2.token_transfer_fee_config`, with `is_enabled = false` (no bps/fee, router falls
//! back to the FeeQuoter) when no `ChainConfigV2` is configured for the lane.
//!
//! This program also plays the verifier (CCV) role - see `instructions::verifier::get_fee` for
//! its `get_verifier_fee` counterpart. The two used to share a single Anchor instruction (see git
//! history) since the router's pool-CPI and verifier-CPI conventions both hardcoded the literal
//! instruction name `get_fee`, and Anchor discriminators are name-derived; the router now calls
//! this program under two distinct instruction names, so each can be implemented directly.

use anchor_lang::prelude::*;
use base_token_pool::constants::{valid_version, POOL_CHAINCONFIG_SEED, POOL_STATE_SEED};
use base_token_pool::errors::CcipTokenPoolError;
use base_token_pool::v2::{GetFeeParams as GetPoolFeeParams, GetFeeResult as GetPoolFeeResult};

use crate::constants::{MAX_POOL_CHAIN_CONFIG_V, MAX_POOL_STATE_V, POOL_CHAINCONFIG_SEED_2_0};
use crate::state::{ChainConfig, ChainConfigV2, State};

#[derive(Accounts)]
#[instruction(get_fee_in: GetPoolFeeParams)]
pub struct GetPoolFeeContext<'info> {
    #[account(
        seeds = [POOL_STATE_SEED, state.config.mint.as_ref()],
        bump,
        constraint = valid_version(state.version, MAX_POOL_STATE_V) @ CcipTokenPoolError::InvalidVersion,
        constraint = state.config.mint == get_fee_in.local_token @ CcipTokenPoolError::InvalidToken,
    )]
    pub state: Account<'info, State>,

    #[account(
        seeds = [POOL_CHAINCONFIG_SEED, get_fee_in.remote_chain_selector.to_le_bytes().as_ref(), state.config.mint.as_ref()],
        bump,
        constraint = valid_version(chain_config.version, MAX_POOL_CHAIN_CONFIG_V) @ CcipTokenPoolError::InvalidVersion,
    )]
    pub chain_config: Account<'info, ChainConfig>,

    /// CHECK: may be uninitialized - loaded via `ChainConfigV2::try_load`. An absent lane config
    /// defaults to `is_enabled = false`; an unreadable one fails the instruction.
    #[account(
        seeds = [POOL_CHAINCONFIG_SEED_2_0, get_fee_in.remote_chain_selector.to_le_bytes().as_ref(), state.config.mint.as_ref()],
        bump,
    )]
    pub chain_config_v2: UncheckedAccount<'info>,
}

pub fn get_pool_fee(
    ctx: Context<GetPoolFeeContext>,
    get_fee_in: GetPoolFeeParams,
) -> Result<GetPoolFeeResult> {
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

        Ok(GetPoolFeeResult {
            dest_gas_limit: token_transfer_fee_config.dest_gas_overhead,
            dest_bytes_overhead: token_transfer_fee_config.dest_bytes_overhead,
            fee,
            token_fee_bps,
            is_enabled: token_transfer_fee_config.is_enabled,
        })
    } else {
        // No fee config initialized for this lane: return zeros with is_enabled=false to signal
        // the OnRamp to use the FeeQuoter fallback.
        Ok(GetPoolFeeResult {
            is_enabled: false,
            ..Default::default()
        })
    }
}
