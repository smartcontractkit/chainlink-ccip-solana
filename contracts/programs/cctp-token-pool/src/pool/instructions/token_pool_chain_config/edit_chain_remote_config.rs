use anchor_lang::prelude::*;

use ccip_common::state::AccountSchemaVersion;

use base_token_pool::constants::{POOL_CHAINCONFIG_SEED, POOL_STATE_SEED};
use base_token_pool::errors::CcipTokenPoolError;
use base_token_pool::state::{RemoteAddress, RemoteConfig};

use crate::error::CctpPoolVerifierError;
use crate::pool::state::{CctpChain, ChainConfig, State};

// edit remote config's CCIP values
pub fn edit_chain_remote_config(
    ctx: Context<EditChainConfigDynamicSize>,
    remote_chain_selector: u64,
    mint: Pubkey,
    cfg: RemoteConfig,
) -> Result<()> {
    // INV-PCFG-1: keep the remote token address non-zero on edits too, so a valid config
    // cannot later be updated to an unusable zero/empty address.
    require!(
        !cfg.token_address.is_zero(),
        CcipTokenPoolError::ZeroTokenAddress
    );

    ctx.accounts
        .chain_config
        .base
        .set(remote_chain_selector, mint, cfg)
}

#[derive(Accounts)]
#[instruction(remote_chain_selector: u64, mint: Pubkey, cfg: RemoteConfig)]
pub struct EditChainConfigDynamicSize<'info> {
    #[account(
        seeds = [
            POOL_STATE_SEED,
            mint.key().as_ref()
        ],
        bump,
        constraint = state.is_latest_version() @ CcipTokenPoolError::InvalidVersion,
    )]
    pub state: Account<'info, State>,

    #[account(
        mut,
        seeds = [
            POOL_CHAINCONFIG_SEED,
            &remote_chain_selector.to_le_bytes(),
            mint.key().as_ref(),
        ],
        bump,
        constraint = chain_config.is_latest_version() @ CcipTokenPoolError::InvalidVersion,
        realloc = ChainConfig::DISCRIMINATOR.len() + ChainConfig::INIT_SPACE + cfg.pool_addresses.iter().map(RemoteAddress::space).sum::<usize>(),
        realloc::payer = authority,
        realloc::zero = false
    )]
    pub chain_config: Account<'info, ChainConfig>,

    #[account(mut, address = state.config.owner)]
    pub authority: Signer<'info>,

    pub system_program: Program<'info, System>,
}

// --- CCTP-specific config edits ---

pub fn edit_chain_remote_config_cctp(
    ctx: Context<EditChainConfig>,
    _remote_chain_selector: u64,
    _mint: Pubkey,
    cfg: CctpChain,
) -> Result<()> {
    // A zero destination caller lets anyone receive the CCTP message on the destination, consuming
    // it before the remote pool can.
    require_keys_neq!(
        cfg.destination_caller,
        Pubkey::default(),
        CctpPoolVerifierError::CctpAllowedCallersInvalidZero
    );
    ctx.accounts.chain_config.cctp = cfg;
    emit!(RemoteChainCctpConfigChanged {
        config: ctx.accounts.chain_config.cctp
    });
    Ok(())
}

#[derive(Accounts)]
#[instruction(remote_chain_selector: u64, mint: Pubkey)]
pub struct EditChainConfig<'info> {
    #[account(
        seeds = [
            POOL_STATE_SEED,
            mint.key().as_ref()
        ],
        bump,
        constraint = state.is_latest_version() @ CcipTokenPoolError::InvalidVersion,
    )]
    pub state: Account<'info, State>,

    #[account(
        mut,
        seeds = [
            POOL_CHAINCONFIG_SEED,
            &remote_chain_selector.to_le_bytes(),
            mint.key().as_ref(),
        ],
        bump,
        constraint = chain_config.is_latest_version() @ CcipTokenPoolError::InvalidVersion,
    )]
    pub chain_config: Account<'info, ChainConfig>,

    #[account(mut, constraint = authority.key() == state.config.owner @ CcipTokenPoolError::Unauthorized)]
    pub authority: Signer<'info>,
}

#[event]
pub struct RemoteChainCctpConfigChanged {
    pub config: CctpChain,
}
