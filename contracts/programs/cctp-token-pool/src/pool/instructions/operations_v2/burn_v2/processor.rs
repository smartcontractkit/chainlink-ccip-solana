//! Unlike a "standard" base-token-pool v2 pool, this does NOT perform an independent SPL burn.
//! `token_messenger_minter::deposit_for_burn_with_hook` (called from the verifier's `start_verification_at_source`)
//! is what actually burns the tokens, as a side effect of dispatching the CCTPv2 message.
//!
//! This instruction only validates, computes the fee, consumes the outbound rate limit,
//! and reports what the verifier is expected to move.

use anchor_lang::prelude::*;
use anchor_spl::token_interface::{Mint, TokenAccount, TokenInterface};

use base_token_pool::{constants::*, errors::*, events::*, v2::*};

use ccip_common::state::{AccountSchemaVersion, PoolChainConfigOverrideType};

use crate::pool::{instructions::CCTP_VERSION_2_CCV_TAG, state::*};

#[event_cpi]
#[derive(Accounts)]
#[instruction(lock_or_burn: LockOrBurnInV2)]
pub struct TokenOnrampV2<'info> {
    // CCIP accounts ------------------------
    #[account(address = state.config.router_onramp_authority @ CcipTokenPoolError::InvalidPoolCaller)]
    pub authority: Signer<'info>,

    // Token pool accounts ------------------
    #[account(
        seeds = [POOL_STATE_SEED, mint.key().as_ref()],
        bump,
        constraint = state.is_latest_version() @ CcipTokenPoolError::InvalidVersion,
    )]
    pub state: Box<Account<'info, State>>,

    pub token_program: Interface<'info, TokenInterface>,

    #[account(
        mut,
        address = lock_or_burn.local_token,
        owner = token_program.key(),
    )]
    pub mint: InterfaceAccount<'info, Mint>,

    #[account(
        seeds = [POOL_SIGNER_SEED, mint.key().as_ref()],
        bump,
        address = state.config.pool_signer,
    )]
    /// CHECK: unchecked CPI signer
    pub pool_signer: UncheckedAccount<'info>,

    #[account(
        mut,
        associated_token::mint = mint,
        associated_token::authority = pool_signer,
        associated_token::token_program = token_program,
    )]
    pub pool_token_account: InterfaceAccount<'info, TokenAccount>,

    #[account(
        mut,
        seeds = [POOL_CHAINCONFIG_SEED, lock_or_burn.remote_chain_selector.to_le_bytes().as_ref(), mint.key().as_ref()],
        bump,
        constraint = chain_config.is_latest_version() @ CcipTokenPoolError::InvalidVersion,
    )]
    pub chain_config: Account<'info, ChainConfig>,

    /// CHECK: V2 chain config PDA holding the allowed finality policy for this lane.
    /// May or may not be initialized
    #[account(
        seeds = [POOL_CHAINCONFIG_SEED_2_0, lock_or_burn.remote_chain_selector.to_le_bytes().as_ref(), mint.key().as_ref()],
        bump,
    )]
    pub chain_config_v2: UncheckedAccount<'info>,

    /// CHECK: Optional FTF override PDA. May or may not be initialized.
    /// When initialized (owner == this program), its rate limit buckets
    /// override the default ones for faster-than-finality transfers.
    #[account(
        mut,
        seeds = [POOL_OVERRIDE_CHAINCONFIG_SEED, lock_or_burn.remote_chain_selector.to_le_bytes().as_ref(), mint.key().as_ref(), PoolChainConfigOverrideType::FTF.as_bytes()],
        bump,
    )]
    pub chain_config_override: UncheckedAccount<'info>,
}

pub fn burn_tokens_v2(
    ctx: Context<TokenOnrampV2>,
    lock_or_burn: LockOrBurnInV2,
) -> Result<LockOrBurnOutV2> {
    // Rate-limit bucket selection logic for override types:
    // - FTF (Faster Than Finality): uses override's outbound_rate_limit if initialized
    // When adding new override types, extend this section to check for and use their buckets.
    let ftf_account_info = &ctx.accounts.chain_config_override;
    let mut ftf_override = ChainConfigOverride::try_load(ftf_account_info)?;

    let ftf_outbound = ftf_override
        .as_mut()
        .map(|o| &mut o.base.outbound_rate_limit);

    // Possibly uninitialized: We'll map its fields to defaults when appropriate.
    let config_v2 = ChainConfigV2::try_load(&ctx.accounts.chain_config_v2)?;

    // An absent config defaults to WAIT_FOR_FINALITY, permitting only full-finality transfers.
    let allowed_finality_config = config_v2
        .as_ref()
        .map(|c| c.allowed_finality_config)
        .unwrap_or_default();

    // The pool keeps its BPS fee in its own token (accruing in `pool_token_account`, which
    // already holds the full transferred amount) and only "burns" the remainder, mirroring EVM's
    // `TokenPool.lockOrBurn`. This step doesn't actually burn the tokens, but computes the amount
    // that will be burned by the verifier (when the router CPI's into it), and sets that in the return value.
    let fee_amount = token_transfer_bps_fee(
        config_v2
            .as_ref()
            .map(|config| &config.token_transfer_fee_config),
        lock_or_burn.amount,
        lock_or_burn.requested_finality_config,
    );
    let dest_token_amount = lock_or_burn
        .amount
        .checked_sub(fee_amount)
        .ok_or(CcipTokenPoolError::InvalidInputs)?;

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

    // Persist FTF override changes if it was loaded, such as updated RL buckets
    if let Some(override_config) = &ftf_override {
        let mut data = ftf_account_info.try_borrow_mut_data()?;
        override_config
            .try_serialize(&mut data.as_mut())
            .map_err(|_| CcipTokenPoolError::InvalidInputs)?;
    }

    // The actual burn of tokens is done by the verifier module on a separate CPI into this program
    emit_cpi!(LockedOrBurned {
        mint: ctx.accounts.mint.key(),
        remote_chain_selector: lock_or_burn.remote_chain_selector,
        sender: ctx.accounts.authority.key(),
        amount: dest_token_amount,
    });

    Ok(LockOrBurnOutV2 {
        dest_token_address: ctx.accounts.chain_config.base.remote.token_address.clone(),
        dest_pool_data: CCTP_VERSION_2_CCV_TAG.to_vec(),
        dest_token_amount,
    })
}
