use anchor_lang::prelude::*;
use ccip_common::seed;
use ccip_common::state::AccountSchemaVersion;

use crate::event::admin::{OwnershipTransferRequested, OwnershipTransferred};
use crate::state::Config;
use crate::CcipOfframpError;

#[event_cpi]
#[derive(Accounts)]
pub struct TransferOwnership<'info> {
    #[account(
        mut,
        seeds = [seed::CONFIG],
        bump = config.bump,
        constraint = config.version == Config::MAX_V @ CcipOfframpError::InvalidVersion,
    )]
    pub config: Account<'info, Config>,

    #[account(address = config.owner @ CcipOfframpError::Unauthorized)]
    pub authority: Signer<'info>,
}

pub fn transfer_ownership(ctx: Context<TransferOwnership>, proposed_owner: Pubkey) -> Result<()> {
    let config = &mut ctx.accounts.config;
    require_keys_neq!(
        proposed_owner,
        Pubkey::default(),
        CcipOfframpError::DefaultOwnerProposal
    );

    require_keys_neq!(
        proposed_owner,
        config.owner,
        CcipOfframpError::RedundantOwnerProposal
    );
    emit_cpi!(OwnershipTransferRequested {
        from: config.owner,
        to: proposed_owner,
    });
    config.proposed_owner = proposed_owner;
    Ok(())
}

// ----

#[event_cpi]
#[derive(Accounts)]
pub struct AcceptOwnership<'info> {
    #[account(
        mut,
        seeds = [seed::CONFIG],
        bump = config.bump,
        constraint = config.version == Config::MAX_V @ CcipOfframpError::InvalidVersion,
    )]
    pub config: Account<'info, Config>,

    #[account(address = config.proposed_owner @ CcipOfframpError::Unauthorized)]
    pub authority: Signer<'info>,
}

pub fn accept_ownership(ctx: Context<AcceptOwnership>) -> Result<()> {
    let config = &mut ctx.accounts.config;
    emit_cpi!(OwnershipTransferred {
        from: config.owner,
        to: config.proposed_owner,
    });
    // NOTE: take() resets proposed_owner to default
    config.owner = std::mem::take(&mut config.proposed_owner);
    Ok(())
}
