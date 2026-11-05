//! Unlike a "standard" base-token-pool v2 pool, this does NOT perform an independent SPL burn.
//! `bridge::deposit` (called from the verifier's `start_verification_at_source`) is what actually
//! burns the tokens, as a side effect of dispatching the Lombard GMP message.
//!
//! This instruction only validates, computes the fee, consumes the outbound rate limit,
//! and reports what the verifier is expected to move.

use anchor_lang::prelude::*;
use base_token_pool::constants::{
    valid_version, POOL_CHAINCONFIG_SEED, POOL_OVERRIDE_CHAINCONFIG_SEED, POOL_SIGNER_SEED,
    POOL_STATE_SEED,
};
use base_token_pool::errors::CcipTokenPoolError;
use base_token_pool::events::LockedOrBurned;
use base_token_pool::v2::{
    token_transfer_bps_fee, validate_lock_or_burn_v2, LockOrBurnInV2, LockOrBurnOutV2,
};
use ccip_common::state::PoolChainConfigOverrideType;

use crate::constants::{MAX_POOL_CHAIN_CONFIG_V, MAX_POOL_STATE_V, POOL_CHAINCONFIG_SEED_2_0};
use crate::state::{ChainConfig, ChainConfigOverride, ChainConfigV2, State};

/// Account order must match exactly what `ccip-router`'s `ccip_send/processor` always builds for
/// ANY v2 pool's `lock_or_burn` CPI.
#[event_cpi]
#[derive(Accounts)]
#[instruction(lock_or_burn: LockOrBurnInV2)]
pub struct TokenOnrampV2<'info> {
    #[account(address = state.config.router_onramp_authority @ CcipTokenPoolError::InvalidPoolCaller)]
    pub authority: Signer<'info>,

    #[account(
        seeds = [POOL_STATE_SEED, mint.key().as_ref()],
        bump,
        constraint = valid_version(state.version, MAX_POOL_STATE_V) @ CcipTokenPoolError::InvalidVersion,
    )]
    pub state: Box<Account<'info, State>>,

    #[account(address = state.config.token_program)]
    pub token_program: UncheckedAccount<'info>,

    #[account(
        mut,
        address = lock_or_burn.local_token,
        owner = token_program.key(),
    )]
    pub mint: UncheckedAccount<'info>,

    #[account(
        seeds = [POOL_SIGNER_SEED, mint.key().as_ref()],
        bump,
        address = state.config.pool_signer,
    )]
    pub pool_signer: UncheckedAccount<'info>,

    #[account(address = state.config.pool_token_account)]
    pub pool_token_account: UncheckedAccount<'info>,

    #[account(
        mut,
        seeds = [POOL_CHAINCONFIG_SEED, &lock_or_burn.remote_chain_selector.to_le_bytes(), state.config.mint.as_ref()],
        bump,
        constraint = valid_version(chain_config.version, MAX_POOL_CHAIN_CONFIG_V) @ CcipTokenPoolError::InvalidVersion,
    )]
    pub chain_config: Box<Account<'info, ChainConfig>>,

    /// CHECK: may not be initialized - loaded via `ChainConfigV2::try_load`. An absent lane config
    /// defaults to WAIT_FOR_FINALITY (full-finality-only) and a disabled fee config; an unreadable
    /// one fails the instruction.
    #[account(
        seeds = [POOL_CHAINCONFIG_SEED_2_0, &lock_or_burn.remote_chain_selector.to_le_bytes(), state.config.mint.as_ref()],
        bump,
    )]
    pub chain_config_v2: UncheckedAccount<'info>,

    /// CHECK: Optional FTF override PDA. May or may not be initialized; when initialized (owner
    /// == this program), its outbound rate limit bucket overrides the default one for
    /// faster-than-finality transfers.
    #[account(
        mut,
        seeds = [POOL_OVERRIDE_CHAINCONFIG_SEED, &lock_or_burn.remote_chain_selector.to_le_bytes(), state.config.mint.as_ref(), PoolChainConfigOverrideType::FTF.as_bytes()],
        bump,
    )]
    pub chain_config_override: UncheckedAccount<'info>,
}

pub fn lock_or_burn_tokens_v2(
    ctx: Context<TokenOnrampV2>,
    lock_or_burn: LockOrBurnInV2,
) -> Result<LockOrBurnOutV2> {
    let config_v2 = ChainConfigV2::try_load(&ctx.accounts.chain_config_v2)?;

    let allowed_finality_config = config_v2
        .as_ref()
        .map(|c| c.allowed_finality_config)
        .unwrap_or_default();

    let fee_amount = token_transfer_bps_fee(
        config_v2.as_ref().map(|c| &c.token_transfer_fee_config),
        lock_or_burn.amount,
        lock_or_burn.requested_finality_config,
    );
    let dest_token_amount = lock_or_burn
        .amount
        .checked_sub(fee_amount)
        .ok_or(CcipTokenPoolError::InvalidInputs)?;

    let ftf_account_info = &ctx.accounts.chain_config_override;
    let mut ftf_override = ChainConfigOverride::try_load(ftf_account_info)?;
    let ftf_outbound = ftf_override
        .as_mut()
        .map(|o| &mut o.base.outbound_rate_limit);

    validate_lock_or_burn_v2(
        &lock_or_burn,
        fee_amount,
        ctx.accounts.state.config.mint,
        allowed_finality_config,
        &mut ctx.accounts.chain_config.base.outbound_rate_limit,
        ftf_outbound,
        ctx.accounts.state.config.list_enabled,
        &ctx.accounts.state.config.allow_list,
    )?;

    if let Some(override_config) = &ftf_override {
        let mut data = ftf_account_info.try_borrow_mut_data()?;
        override_config
            .try_serialize(&mut data.as_mut())
            .map_err(|_| CcipTokenPoolError::InvalidInputs)?;
    }

    emit_cpi!(LockedOrBurned {
        mint: ctx.accounts.state.config.mint,
        remote_chain_selector: lock_or_burn.remote_chain_selector,
        sender: ctx.accounts.authority.key(),
        amount: dest_token_amount,
    });

    Ok(LockOrBurnOutV2 {
        dest_token_address: ctx.accounts.chain_config.base.remote.token_address.clone(),
        dest_pool_data: {
            let mut abi_encoded_decimals = vec![0u8; 32];
            abi_encoded_decimals[31] = ctx.accounts.state.config.decimals;
            abi_encoded_decimals
        },
        dest_token_amount,
    })
}
