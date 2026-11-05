use anchor_lang::prelude::*;
use anchor_spl::token_interface::{Mint, TokenAccount, TokenInterface};
use base_token_pool::{constants::*, errors::*, events::*, v2::*};
use ccip_common::state::PoolChainConfigOverrideType;

use crate::state::*;
use crate::{ChainConfig, ChainConfigOverride, ChainConfigV2, State};

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
        constraint = valid_version(state.version, MAX_POOL_STATE_V) @ CcipTokenPoolError::InvalidVersion,
    )]
    pub state: Box<Account<'info, State>>,
    pub token_program: Interface<'info, TokenInterface>,
    #[account(mut, owner = token_program.key())]
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
        associated_token::token_program = token_program
    )]
    pub pool_token_account: InterfaceAccount<'info, TokenAccount>,

    #[account(
        mut,
        seeds = [POOL_CHAINCONFIG_SEED, lock_or_burn.remote_chain_selector.to_le_bytes().as_ref(), mint.key().as_ref()],
        bump,
    )]
    pub chain_config: Account<'info, ChainConfig>,

    /// CHECK: V2 chain config PDA holding the allowed finality policy for this lane.
    /// May or may not be initialized; read via `ChainConfigV2::try_load`, defaulting to
    /// `WAIT_FOR_FINALITY` (full-finality only) when absent. The router supplies this account for
    /// every v2 pool transfer, and its position (between `chain_config` and `chain_config_override`)
    /// must mirror the router's `transfer_token_v2` CPI account order and the resolver's
    /// `accounts_to_save` order.
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

    /// CHECK: The ATA, for the pool's own mint, of the `[FEE_VAULT, mint]` PDA the router pays
    /// token-pool receipt fees into. In-token BPS fees accrue here until the owner withdraws them
    /// with `withdraw_fee_tokens`; keeping them here rather than in `pool_token_account` is what
    /// makes fees distinguishable from the locked liquidity. The vault PDA is derived inline rather
    /// than passed in: it holds no data and never signs here, and every account this instruction
    /// takes costs 32 bytes of a `ccip_send` transaction that is already close to the size limit.
    #[account(mut, address = fee_vault_token_account_address(&crate::ID, &mint.key(), &token_program.key()))]
    pub fee_vault_token_account: UncheckedAccount<'info>,
}

pub fn lock_tokens_v2(
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

    // The pool keeps its BPS fee in its own token and only locks the remainder, mirroring
    // EVM's `TokenPool.lockOrBurn`. The fee is moved out of `pool_token_account` and into the
    // pool's fee vault below, so what stays behind is exactly the locked liquidity.
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

    // Persist FTF override changes if it was loaded
    if let Some(override_config) = &ftf_override {
        let mut data = ftf_account_info.try_borrow_mut_data()?;
        override_config
            .try_serialize(&mut data.as_mut())
            .map_err(|_| CcipTokenPoolError::InvalidInputs)?;
    }

    // The router transferred the full amount into
    // `pool_token_account`, so the fee is moved out to the fee vault.
    collect_bps_fee(
        ctx.accounts.token_program.key(),
        ctx.accounts.pool_token_account.to_account_info(),
        ctx.accounts.fee_vault_token_account.to_account_info(),
        ctx.accounts.mint.to_account_info(),
        ctx.accounts.pool_signer.to_account_info(),
        ctx.bumps.pool_signer,
        fee_amount,
        ctx.accounts.mint.decimals,
    )?;

    emit_cpi!(LockedOrBurned {
        mint: ctx.accounts.mint.key(),
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
