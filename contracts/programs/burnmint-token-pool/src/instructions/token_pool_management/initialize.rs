use anchor_lang::prelude::*;
use anchor_spl::token_interface::Mint;
use base_token_pool::{constants::*, errors::*, state::*};
use ccip_common::multisig::{is_1_of_n_multisig_signer, is_mint_authority, is_multisig};

use crate::program::BurnmintTokenPool;
use crate::state::*;
use crate::State;

#[derive(Accounts)]
pub struct InitializeTokenPool<'info> {
    #[account(
        init,
        seeds = [POOL_STATE_SEED, mint.key().as_ref()],
        bump,
        payer = authority,
        space = ANCHOR_DISCRIMINATOR + State::INIT_SPACE,
    )]
    pub state: Account<'info, State>, // config PDA for token pool
    pub mint: InterfaceAccount<'info, Mint>, // underlying token that the pool wraps

    #[account(mut)]
    pub authority: Signer<'info>,
    pub system_program: Program<'info, System>,

    #[account(constraint = program.programdata_address()? == Some(program_data.key()))]
    pub program: Program<'info, BurnmintTokenPool>,

    // The upgrade authority or the mint authority (when self-serve is enabled) can initialize
    #[account(constraint = allowed_to_initialize_token_pool(&program_data, &authority, &config, &mint) @ CcipTokenPoolError::Unauthorized)]
    pub program_data: Account<'info, ProgramData>,

    #[account(
        seeds = [CONFIG_SEED],
        bump,
        constraint = valid_version(config.version, MAX_POOL_CONFIG_V) @ CcipTokenPoolError::InvalidVersion,
    )]
    pub config: Account<'info, PoolConfig>,
}

/// Checks if the given authority is allowed to initialize the token pool.
fn allowed_to_initialize_token_pool(
    program_data: &Account<ProgramData>,
    authority: &Signer,
    config: &Account<PoolConfig>,
    mint: &InterfaceAccount<Mint>,
) -> bool {
    program_data.upgrade_authority_address == Some(authority.key())
        || (config.self_served_allowed
            && is_mint_authority(mint, &authority.key())
            && !is_multisig(&authority.to_account_info()))
}

#[derive(Accounts)]
pub struct InitializeTokenPoolByMultisig<'info> {
    #[account(
        init,
        seeds = [POOL_STATE_SEED, mint.key().as_ref()],
        bump,
        payer = authority,
        space = ANCHOR_DISCRIMINATOR + State::INIT_SPACE,
    )]
    pub state: Account<'info, State>,
    pub mint: InterfaceAccount<'info, Mint>,

    #[account(mut)]
    pub authority: Signer<'info>,
    pub system_program: Program<'info, System>,

    #[account(constraint = program.programdata_address()? == Some(program_data.key()))]
    pub program: Program<'info, BurnmintTokenPool>,

    // Self-serve must be enabled and signer must be a valid multisig signer of the mint authority
    #[account(constraint = allowed_to_initialize_by_multisig(&config, &mint, &authority, &mint_authority_multisig) @ CcipTokenPoolError::Unauthorized)]
    pub program_data: Account<'info, ProgramData>,

    #[account(
        seeds = [CONFIG_SEED],
        bump,
        constraint = valid_version(config.version, MAX_POOL_CONFIG_V) @ CcipTokenPoolError::InvalidVersion,
    )]
    pub config: Account<'info, PoolConfig>,

    /// CHECK: Multisig account validated inside is_1_of_n_multisig_signer.
    /// Only 1-of-N SPL Token multisigs are supported here.
    pub mint_authority_multisig: UncheckedAccount<'info>,
}

fn allowed_to_initialize_by_multisig(
    config: &Account<PoolConfig>,
    mint: &InterfaceAccount<Mint>,
    authority: &Signer,
    mint_authority_multisig: &UncheckedAccount,
) -> bool {
    config.self_served_allowed
        && is_1_of_n_multisig_signer(
            mint,
            &authority.key(),
            &mint_authority_multisig.to_account_info(),
        )
}

pub fn initialize(ctx: Context<InitializeTokenPool>) -> Result<()> {
    init_state(
        &mut ctx.accounts.state,
        &ctx.accounts.mint,
        ctx.program_id,
        &ctx.accounts.authority,
        &ctx.accounts.config,
    )
}

pub fn initialize_by_multisig(ctx: Context<InitializeTokenPoolByMultisig>) -> Result<()> {
    init_state(
        &mut ctx.accounts.state,
        &ctx.accounts.mint,
        ctx.program_id,
        &ctx.accounts.authority,
        &ctx.accounts.config,
    )
}

fn init_state<'info>(
    state: &mut Account<'info, State>,
    mint: &InterfaceAccount<'info, Mint>,
    program_id: &Pubkey,
    authority: &Signer<'info>,
    config: &Account<'info, PoolConfig>,
) -> Result<()> {
    state.set_inner(State {
        version: MAX_POOL_STATE_V,
        config: BaseConfig::init(
            mint,
            program_id.key(),
            authority.key(),
            config.router,
            config.rmn_remote,
        ),
    });

    Ok(())
}
