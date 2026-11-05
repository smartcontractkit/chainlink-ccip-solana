use anchor_lang::prelude::*;
use base_token_pool::constants::valid_version;
use base_token_pool::errors::CcipTokenPoolError;

use crate::constants::{VERIFIER_CONFIG_SEED, VERIFIER_REMOTE_CHAIN_SEED};
use crate::events::VerifierRemoteAdaptersSet;
use crate::state::{
    RemoteAdapter, VerifierConfig, VerifierRemoteChainConfig, MAX_VERIFIER_CONFIG_V,
    MAX_VERIFIER_REMOTE_CHAIN_CONFIG_V,
};

/// Sets remote adapter overrides for one remote chain.
/// Only needed when the remote adapter differs from the token's own address (e.g. non-upgradeable
/// wrapped-asset adapters like BTC.b).
pub fn set_verifier_remote_adapters(
    ctx: Context<SetVerifierRemoteAdapters>,
    remote_chain_selector: u64,
    remote_adapters: Vec<RemoteAdapter>,
) -> Result<()> {
    let config = &mut ctx.accounts.remote_chain_config;

    for adapter in &remote_adapters {
        if let Some(existing) = config
            .remote_adapters
            .iter_mut()
            .find(|a| a.token == adapter.token)
        {
            existing.remote_adapter = adapter.remote_adapter;
        } else {
            config.remote_adapters.push(*adapter);
        }
    }

    emit!(VerifierRemoteAdaptersSet {
        remote_chain_selector,
        remote_adapters,
    });

    Ok(())
}

#[derive(Accounts)]
#[instruction(remote_chain_selector: u64, remote_adapters: Vec<RemoteAdapter>)]
pub struct SetVerifierRemoteAdapters<'info> {
    #[account(
        seeds = [VERIFIER_CONFIG_SEED],
        bump = verifier_config.bump,
        constraint = valid_version(verifier_config.version, MAX_VERIFIER_CONFIG_V) @ CcipTokenPoolError::InvalidVersion,
    )]
    pub verifier_config: Account<'info, VerifierConfig>,

    #[account(mut, address = verifier_config.owner @ CcipTokenPoolError::Unauthorized)]
    pub authority: Signer<'info>,

    #[account(
        mut,
        seeds = [VERIFIER_REMOTE_CHAIN_SEED, &remote_chain_selector.to_le_bytes()],
        bump = remote_chain_config.bump,
        constraint = valid_version(remote_chain_config.version, MAX_VERIFIER_REMOTE_CHAIN_CONFIG_V) @ CcipTokenPoolError::InvalidVersion,
        realloc = VerifierRemoteChainConfig::DISCRIMINATOR.len() + VerifierRemoteChainConfig::INIT_SPACE + VerifierRemoteChainConfig::dynamic_space(&remote_chain_config.allowlist_config.allowed_senders, &remote_chain_config.remote_adapters) + remote_adapters.len() * RemoteAdapter::INIT_SPACE,
        realloc::payer = authority,
        realloc::zero = false,
    )]
    pub remote_chain_config: Account<'info, VerifierRemoteChainConfig>,

    pub system_program: Program<'info, System>,
}
