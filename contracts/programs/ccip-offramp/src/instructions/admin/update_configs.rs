use anchor_lang::prelude::*;
use ccip_common::seed;
use ccip_common::state::AccountSchemaVersion;

use crate::error::CcipOfframpError;
use crate::event::admin::{ConfigSet, ReferenceAddressesSet};
use crate::state::{Config, ReferenceAddresses};

#[event_cpi]
#[derive(Accounts)]
pub struct UpdateReferenceAddresses<'info> {
    #[account(
        seeds = [seed::CONFIG],
        bump = config.bump,
        constraint = config.is_latest_version() @ CcipOfframpError::InvalidVersion,
    )]
    pub config: Account<'info, Config>,

    #[account(
        mut,
        seeds = [seed::REFERENCE_ADDRESSES],
        bump = reference_addresses.bump,
        constraint = reference_addresses.is_latest_version() @ CcipOfframpError::InvalidVersion,
    )]
    pub reference_addresses: Account<'info, ReferenceAddresses>,

    #[account(address = config.owner @ CcipOfframpError::Unauthorized)]
    pub authority: Signer<'info>,
}

pub fn update_reference_addresses(
    ctx: Context<UpdateReferenceAddresses>,
    router: Pubkey,
    fee_quoter: Pubkey,
    offramp_lookup_table: Pubkey,
    rmn_remote: Pubkey,
) -> Result<()> {
    let bump = ctx.accounts.reference_addresses.bump;

    ctx.accounts
        .reference_addresses
        .set_inner(ReferenceAddresses {
            version: ReferenceAddresses::MAX_V,
            bump,
            router,
            fee_quoter,
            offramp_lookup_table,
            rmn_remote,
        });

    emit_cpi!(ReferenceAddressesSet {
        router,
        fee_quoter,
        offramp_lookup_table,
        rmn_remote
    });

    Ok(())
}

// ---

#[event_cpi]
#[derive(Accounts)]
pub struct UpdateConfig<'info> {
    #[account(
        mut,
        seeds = [seed::CONFIG],
        bump = config.bump,
        constraint = config.is_latest_version() @ CcipOfframpError::InvalidVersion,
    )]
    pub config: Account<'info, Config>,

    #[account(address = config.owner @ CcipOfframpError::Unauthorized)]
    pub authority: Signer<'info>,
}

pub fn update_svm_chain_selector(
    ctx: Context<UpdateConfig>,
    new_chain_selector: u64,
) -> Result<()> {
    ctx.accounts.config.svm_chain_selector = new_chain_selector;

    emit_cpi!(ConfigSet {
        svm_chain_selector: new_chain_selector,
    });

    Ok(())
}
