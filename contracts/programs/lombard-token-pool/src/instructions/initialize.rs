use anchor_lang::prelude::*;

use anchor_spl::token_interface::Mint;

use base_token_pool::constants::{valid_version, POOL_CHAINCONFIG_SEED, POOL_STATE_SEED};
use base_token_pool::errors::CcipTokenPoolError;
use base_token_pool::state::{BaseConfig, RemoteConfig};

use crate::{
    constants::*,
    program::LombardTokenPool,
    state::{ChainConfig, LombardChain, PoolConfig, State},
};

#[derive(Accounts)]
pub struct InitGlobalConfig<'info> {
    #[account(
        init,
        seeds = [POOL_CONFIG_SEED],
        bump,
        payer = authority,
        space = PoolConfig::DISCRIMINATOR.len() + PoolConfig::INIT_SPACE,
    )]
    pub config: Account<'info, PoolConfig>, // Global Config PDA of the Token Pool

    #[account(mut)]
    pub authority: Signer<'info>,
    pub system_program: Program<'info, System>,

    // Ensures that the provided program is the LombardTokenPool program,
    // and that its associated program data account matches the expected one.
    // This guarantees that only the program's upgrade authority can modify the global config.
    #[account(constraint = program.programdata_address()? == Some(program_data.key()))]
    pub program: Program<'info, LombardTokenPool>,
    // Global Config updates only allowed by program upgrade authority
    #[account(constraint = program_data.upgrade_authority_address == Some(authority.key()) @ CcipTokenPoolError::Unauthorized)]
    pub program_data: Account<'info, ProgramData>,
}

pub fn init_global_config(ctx: Context<InitGlobalConfig>) -> Result<()> {
    ctx.accounts.config.set_inner(PoolConfig {
        version: MAX_POOL_CONFIG_V,
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

    // Token pool initialization only allowed by program upgrade authority. Initializing token pools managed
    // by the CLL deployment of this program is limited to CLL. Users must deploy their own instance of this program.
    #[account(constraint = program.programdata_address()? == Some(program_data.key()))]
    pub program: Program<'info, LombardTokenPool>,

    #[account(constraint = program_data.upgrade_authority_address == Some(authority.key()) @ CcipTokenPoolError::Unauthorized)]
    pub program_data: Account<'info, ProgramData>,

    #[account(
        seeds = [POOL_CONFIG_SEED],
        bump,
        constraint = valid_version(config.version, MAX_POOL_CONFIG_V) @ CcipTokenPoolError::InvalidVersion,
    )]
    pub config: Account<'info, PoolConfig>, // Global Config PDA of the Token Pool

    pub system_program: Program<'info, System>,
}

pub fn initialize(
    ctx: Context<InitializeTokenPool>,
    router: Pubkey,
    rmn_remote: Pubkey,
    bridge: Pubkey,
) -> Result<()> {
    ctx.accounts.state.set_inner(State {
        version: MAX_POOL_STATE_V,
        config: BaseConfig::init(
            &ctx.accounts.mint,
            ctx.program_id.key(),
            ctx.accounts.authority.key(),
            router,
            rmn_remote,
        ),
        bridge,
        alt: None,
    });

    Ok(())
}

#[derive(Accounts)]
#[instruction(remote_chain_selector: u64, mint: Pubkey)]
pub struct InitializeChainConfig<'info> {
    #[account(
        seeds = [
            POOL_STATE_SEED,
            mint.key().as_ref()
        ],
        bump,
        constraint = valid_version(state.version, MAX_POOL_STATE_V) @ CcipTokenPoolError::InvalidVersion,
    )]
    pub state: Account<'info, State>,

    #[account(
        init,
        seeds = [
            POOL_CHAINCONFIG_SEED,
            &remote_chain_selector.to_le_bytes(),
            mint.key().as_ref(),
        ],
        bump,
        payer = authority,
        space = ChainConfig::DISCRIMINATOR.len() + ChainConfig::INIT_SPACE,
    )]
    pub chain_config: Account<'info, ChainConfig>,

    #[account(mut, address = state.config.owner)]
    pub authority: Signer<'info>,

    pub system_program: Program<'info, System>,
}

pub fn init_chain_remote_config(
    ctx: Context<InitializeChainConfig>,
    remote_chain_selector: u64,
    mint: Pubkey,
    cfg: RemoteConfig,
    dest_chain_id: [u8; 32],
    dest_caller: [u8; 32],
) -> Result<()> {
    require!(
        cfg.pool_addresses.is_empty(),
        CcipTokenPoolError::NonemptyPoolAddressesInit
    );
    require!(
        !cfg.token_address.is_zero(),
        CcipTokenPoolError::InvalidToken
    );
    let bridge = LombardChain {
        destination_chain_id: dest_chain_id,
        destination_caller: dest_caller,
    };
    bridge.validate()?;

    ctx.accounts.chain_config.version = 1;

    ctx.accounts.chain_config.bridge = bridge;

    ctx.accounts
        .chain_config
        .base
        .set(remote_chain_selector, mint, cfg)
}
