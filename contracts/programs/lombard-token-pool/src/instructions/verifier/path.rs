use anchor_lang::prelude::*;
use base_token_pool::constants::valid_version;
use base_token_pool::errors::CcipTokenPoolError;

use crate::constants::{VERIFIER_CONFIG_SEED, VERIFIER_REMOTE_CHAIN_SEED};
use crate::errors::LombardTokenPoolError;
use crate::events::{VerifierPathRemoved, VerifierPathSet};
use crate::state::{
    LombardVerifierPath, VerifierConfig, VerifierRemoteChainConfig, MAX_VERIFIER_CONFIG_V,
    MAX_VERIFIER_REMOTE_CHAIN_CONFIG_V,
};

/// Sets the Lombard GMP path for a remote chain.
/// Requires `init_verifier_chain_config` to have run first for this remote chain.
pub fn set_verifier_path(
    ctx: Context<SetVerifierPath>,
    remote_chain_selector: u64,
    l_chain_id: [u8; 32],
    allowed_caller: [u8; 32],
    remote_bridge_sender: [u8; 32],
) -> Result<()> {
    require!(
        l_chain_id != [0u8; 32] && allowed_caller != [0u8; 32] && remote_bridge_sender != [0u8; 32],
        LombardTokenPoolError::InvalidPath
    );

    let path = LombardVerifierPath {
        allowed_caller,
        l_chain_id,
        remote_bridge_sender,
    };
    ctx.accounts.remote_chain_config.path = path;

    emit!(VerifierPathSet {
        remote_chain_selector,
        path,
    });

    Ok(())
}

pub fn remove_verifier_path(
    ctx: Context<SetVerifierPath>,
    remote_chain_selector: u64,
) -> Result<()> {
    let old_path = ctx.accounts.remote_chain_config.path;
    require!(old_path.is_set(), LombardTokenPoolError::PathNotConfigured);

    ctx.accounts.remote_chain_config.path = LombardVerifierPath::default();

    emit!(VerifierPathRemoved {
        remote_chain_selector,
        path: old_path,
    });

    Ok(())
}

#[derive(Accounts)]
#[instruction(remote_chain_selector: u64)]
pub struct SetVerifierPath<'info> {
    #[account(
        seeds = [VERIFIER_CONFIG_SEED],
        bump = verifier_config.bump,
        constraint = valid_version(verifier_config.version, MAX_VERIFIER_CONFIG_V) @ CcipTokenPoolError::InvalidVersion,
    )]
    pub verifier_config: Account<'info, VerifierConfig>,

    #[account(address = verifier_config.owner @ CcipTokenPoolError::Unauthorized)]
    pub authority: Signer<'info>,

    #[account(
        mut,
        seeds = [VERIFIER_REMOTE_CHAIN_SEED, &remote_chain_selector.to_le_bytes()],
        bump = remote_chain_config.bump,
        constraint = valid_version(remote_chain_config.version, MAX_VERIFIER_REMOTE_CHAIN_CONFIG_V) @ CcipTokenPoolError::InvalidVersion,
    )]
    pub remote_chain_config: Account<'info, VerifierRemoteChainConfig>,
}
