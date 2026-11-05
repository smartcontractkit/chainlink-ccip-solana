use anchor_lang::prelude::*;

use anchor_spl::token_interface::Mint;
use base_token_pool::constants::{CONFIG_SEED, POOL_STATE_SEED};
use base_token_pool::state::BaseConfig;

use ccip_common::state::AccountSchemaVersion;

use crate::pool::state::{FundingConfig, State};
use crate::program::CctpTokenPool;

pub fn initialize(
    ctx: Context<InitializeTokenPool>,
    router: Pubkey,
    rmn_remote: Pubkey,
) -> Result<()> {
    ctx.accounts.state.set_inner(State {
        version: State::MAX_V,
        config: BaseConfig::init(
            &ctx.accounts.mint,
            ctx.program_id.key(),
            ctx.accounts.authority.key(),
            router,
            rmn_remote,
        ),
        funding: FundingConfig {
            manager: Pubkey::default(),
            reclaim_destination: Pubkey::default(),
            minimum_signer_funds: 0,
        },
    });

    Ok(())
}

#[derive(Accounts)]
pub struct InitializeTokenPool<'info> {
    #[account(
        init,
        seeds = [
            POOL_STATE_SEED,
            mint.key().as_ref(),
        ],
        bump,
        payer = authority,
        space = State::DISCRIMINATOR.len() + State::INIT_SPACE,
    )]
    pub state: Account<'info, State>,

    pub mint: InterfaceAccount<'info, Mint>,

    #[account(mut)]
    pub authority: Signer<'info>,

    pub system_program: Program<'info, System>,

    // Token pool initialization only allowed by program upgrade authority. Initializing token pools managed
    // by the CLL deployment of this program is limited to CLL. Users must deploy their own instance of this program.
    #[account(constraint = program.programdata_address()? == Some(program_data.key()))]
    pub program: Program<'info, CctpTokenPool>,

    #[account(constraint = program_data.upgrade_authority_address == Some(authority.key()))]
    pub program_data: Account<'info, ProgramData>,

    /// CHECK: just a placeholder if we ever want to customize behaviour, not used currently
    #[account(
        seeds = [CONFIG_SEED],
        bump,
    )]
    pub config: UncheckedAccount<'info>, // Global Config PDA of the Token Pool, not really used for now so unchecked is fine
}
