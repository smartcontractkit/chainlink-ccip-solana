use anchor_lang::prelude::*;

use ccip_common::seed;
use ccip_common::state::AccountSchemaVersion;

use crate::event::admin::{ConfigSet, ReferenceAddressesSet};
use crate::program::CcipOfframp;
use crate::state::{Config, ReferenceAddresses};
use crate::CcipOfframpError;

#[derive(Accounts)]
pub struct Initialize<'info> {
    #[account(
        init,
        seeds = [seed::REFERENCE_ADDRESSES],
        bump,
        payer = authority,
        space = ReferenceAddresses::DISCRIMINATOR.len() + ReferenceAddresses::INIT_SPACE,
    )]
    pub reference_addresses: Account<'info, ReferenceAddresses>,

    /// CHECK: Router address, to be persisted in reference_addresses. Just they key is used, no data is read.
    pub router: UncheckedAccount<'info>,
    /// CHECK: FeeQuoter address, to be persisted in reference_addresses. Just they key is used, no data is read.
    pub fee_quoter: UncheckedAccount<'info>,
    /// CHECK: RMNRemote address, to be persisted in reference_addresses. Just they key is used, no data is read.
    pub rmn_remote: UncheckedAccount<'info>,
    /// CHECK: ALT address, to be persisted in reference_addresses. Just they key is used, no data is read.
    pub offramp_lookup_table: UncheckedAccount<'info>,

    #[account(mut)]
    pub authority: Signer<'info>,

    pub system_program: Program<'info, System>,

    #[account(constraint = program.programdata_address()? == Some(program_data.key()))]
    pub program: Program<'info, CcipOfframp>,

    // Initialization only allowed by program upgrade authority
    #[account(constraint = program_data.upgrade_authority_address == Some(authority.key()) @ CcipOfframpError::Unauthorized)]
    pub program_data: Account<'info, ProgramData>,
}

pub fn initialize(ctx: Context<Initialize>) -> Result<()> {
    ctx.accounts
        .reference_addresses
        .set_inner(ReferenceAddresses {
            version: ReferenceAddresses::MAX_V,
            bump: ctx.bumps.reference_addresses,
            router: ctx.accounts.router.key(),
            fee_quoter: ctx.accounts.fee_quoter.key(),
            rmn_remote: ctx.accounts.rmn_remote.key(),
            offramp_lookup_table: ctx.accounts.offramp_lookup_table.key(),
        });

    emit!(ReferenceAddressesSet {
        router: ctx.accounts.router.key(),
        fee_quoter: ctx.accounts.fee_quoter.key(),
        offramp_lookup_table: ctx.accounts.offramp_lookup_table.key(),
        rmn_remote: ctx.accounts.rmn_remote.key(),
    });

    Ok(())
}

// ---

#[derive(Accounts)]
pub struct InitializeConfig<'info> {
    #[account(
        init,
        seeds = [seed::CONFIG],
        bump,
        payer = authority,
        space = Config::DISCRIMINATOR.len() + Config::INIT_SPACE,
    )]
    pub config: Account<'info, Config>,

    #[account(mut)]
    pub authority: Signer<'info>,

    pub system_program: Program<'info, System>,

    #[account(constraint = program.programdata_address()? == Some(program_data.key()))]
    pub program: Program<'info, CcipOfframp>,

    // Initialization only allowed by program upgrade authority
    #[account(constraint = program_data.upgrade_authority_address == Some(authority.key()) @ CcipOfframpError::Unauthorized)]
    pub program_data: Account<'info, ProgramData>,
}

pub fn initialize_config(ctx: Context<InitializeConfig>, svm_chain_selector: u64) -> Result<()> {
    ctx.accounts.config.set_inner(Config {
        version: Config::MAX_V,
        bump: ctx.bumps.config,
        svm_chain_selector,
        owner: ctx.accounts.authority.key(),
        proposed_owner: Pubkey::default(),
    });

    emit!(ConfigSet { svm_chain_selector });

    Ok(())
}
