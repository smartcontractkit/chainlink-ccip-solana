use anchor_lang::prelude::*;
use anchor_spl::token_interface::{Mint, TokenAccount, TokenInterface};
use base_token_pool::{constants::*, errors::*, events::*};

use crate::instructions::operations_v1::transfer_tokens;
use crate::instructions::token_pool_management::SetConfig;
use crate::state::*;
use crate::State;

#[derive(Accounts)]
pub struct RebalancerTokenTransfer<'info> {
    #[account(
        seeds = [POOL_STATE_SEED, mint.key().as_ref()],
        bump,
        constraint = valid_version(state.version, MAX_POOL_STATE_V) @ CcipTokenPoolError::InvalidVersion,
    )]
    pub state: Account<'info, State>,
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

    #[account(mut, token::mint = mint, token::token_program = token_program)]
    pub remote_token_account: InterfaceAccount<'info, TokenAccount>,
    // Only the rebalancer can provide or withdraw tokens liquidity
    #[account(address = state.config.rebalancer @ CcipTokenPoolError::Unauthorized)]
    pub authority: Signer<'info>,
}

// set the rebalancer address for the pool, if the rebalancer is the default address then no rebalance is allowed
pub fn set_rebalancer(ctx: Context<SetConfig>, rebalancer: Pubkey) -> Result<()> {
    let old_rebalancer = ctx.accounts.state.config.rebalancer;
    ctx.accounts.state.config.set_rebalancer(rebalancer)?;
    emit!(RebalancerSet {
        mint: ctx.accounts.mint.key(),
        old_rebalancer,
        new_rebalancer: rebalancer,
    });
    Ok(())
}

pub fn provide_liquidity(ctx: Context<RebalancerTokenTransfer>, amount: u64) -> Result<()> {
    require_gt!(amount, 0, CcipTokenPoolError::TransferZeroTokensNotAllowed);
    transfer_tokens(
        ctx.accounts.token_program.key(),
        ctx.accounts.pool_token_account.to_account_info(), // to
        ctx.accounts.remote_token_account.to_account_info(), // from
        ctx.accounts.mint.to_account_info(),
        ctx.accounts.pool_signer.to_account_info(),
        ctx.bumps.pool_signer,
        amount,
        ctx.accounts.mint.decimals,
    )?;
    emit!(LiquidityDeposited {
        provider: ctx.accounts.authority.key(),
        amount,
        mint: ctx.accounts.mint.key(),
    });
    Ok(())
}

// withdraw liquidity can be used to transfer liquidity from one pool to another by setting the `rebalancer` to the calling pool
pub fn withdraw_liquidity(ctx: Context<RebalancerTokenTransfer>, amount: u64) -> Result<()> {
    let pool_balance = ctx.accounts.pool_token_account.amount;
    let amount_to_transfer = if amount == u64::MAX {
        pool_balance
    } else {
        amount
    };
    require_gt!(
        amount_to_transfer,
        0,
        CcipTokenPoolError::TransferZeroTokensNotAllowed
    );
    require_gte!(
        pool_balance,
        amount_to_transfer,
        CcipTokenPoolError::InsufficientPoolBalance
    );
    transfer_tokens(
        ctx.accounts.token_program.key(),
        ctx.accounts.remote_token_account.to_account_info(), // to
        ctx.accounts.pool_token_account.to_account_info(),   // from
        ctx.accounts.mint.to_account_info(),
        ctx.accounts.pool_signer.to_account_info(),
        ctx.bumps.pool_signer,
        amount_to_transfer,
        ctx.accounts.mint.decimals,
    )?;
    emit!(LiquidityWithdrawn {
        provider: ctx.accounts.authority.key(),
        amount: amount_to_transfer,
        mint: ctx.accounts.mint.key(),
    });
    Ok(())
}
