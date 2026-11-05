use anchor_lang::prelude::*;

use anchor_spl::token_interface::Mint;
use base_token_pool::constants::POOL_STATE_SEED;
use base_token_pool::errors::CcipTokenPoolError;
use base_token_pool::events::{OwnershipTransferRequested, OwnershipTransferred};

use ccip_common::state::AccountSchemaVersion;

use crate::pool::state::State;

pub fn transfer_ownership(ctx: Context<TransferOwnership>, proposed_owner: Pubkey) -> Result<()> {
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

#[derive(Accounts)]
pub struct TransferOwnership<'info> {
    #[account(
        mut,
        seeds = [
            POOL_STATE_SEED,
            mint.key().as_ref()
        ],
        bump,
        constraint = state.is_latest_version() @ CcipTokenPoolError::InvalidVersion,
    )]
    pub state: Account<'info, State>,

    pub mint: InterfaceAccount<'info, Mint>, // underlying token that the pool wraps

    #[account(constraint = state.config.owner == authority.key() @ CcipTokenPoolError::Unauthorized)]
    pub authority: Signer<'info>,
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

#[derive(Accounts)]
pub struct AcceptOwnership<'info> {
    #[account(
        mut,
        seeds = [
            POOL_STATE_SEED,
            mint.key().as_ref()
        ],
        bump,
        constraint = state.is_latest_version() @ CcipTokenPoolError::InvalidVersion,
    )]
    pub state: Account<'info, State>,

    pub mint: InterfaceAccount<'info, Mint>, // underlying token that the pool wraps

    #[account(address = state.config.proposed_owner @ CcipTokenPoolError::Unauthorized)]
    pub authority: Signer<'info>,
}
