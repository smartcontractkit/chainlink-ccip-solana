use anchor_lang::prelude::*;
use anchor_spl::token_interface::Mint;
use base_token_pool::{constants::*, errors::*, events::*};

use crate::state::*;
use crate::State;

#[derive(Accounts)]
pub struct SetConfig<'info> {
    #[account(
        mut,
        seeds = [POOL_STATE_SEED, mint.key().as_ref()],
        bump,
        constraint = valid_version(state.version, MAX_POOL_STATE_V) @ CcipTokenPoolError::InvalidVersion,
    )]
    pub state: Account<'info, State>,
    pub mint: InterfaceAccount<'info, Mint>, // underlying token that the pool wraps
    #[account(address = state.config.owner @ CcipTokenPoolError::Unauthorized)]
    pub authority: Signer<'info>,
}

#[derive(Accounts)]
pub struct AcceptOwnership<'info> {
    #[account(
        mut,
        seeds = [POOL_STATE_SEED, mint.key().as_ref()],
        bump,
        constraint = valid_version(state.version, MAX_POOL_STATE_V) @ CcipTokenPoolError::InvalidVersion,
    )]
    pub state: Account<'info, State>,
    pub mint: InterfaceAccount<'info, Mint>, // underlying token that the pool wraps
    #[account(address = state.config.proposed_owner @ CcipTokenPoolError::Unauthorized)]
    pub authority: Signer<'info>,
}

pub fn transfer_ownership(ctx: Context<SetConfig>, proposed_owner: Pubkey) -> Result<()> {
    let mint = ctx.accounts.mint.key();
    let owner = ctx.accounts.state.config.owner;
    ctx.accounts
        .state
        .config
        .transfer_ownership(proposed_owner)?;
    emit!(OwnershipTransferRequested {
        mint,
        from: owner,
        to: proposed_owner,
    });
    Ok(())
}

pub fn accept_ownership(ctx: Context<AcceptOwnership>) -> Result<()> {
    let mint = ctx.accounts.mint.key();
    let old_owner = ctx.accounts.state.config.owner;
    ctx.accounts.state.config.accept_ownership()?;
    emit!(OwnershipTransferred {
        mint,
        from: old_owner,
        to: ctx.accounts.state.config.owner,
    });
    Ok(())
}
