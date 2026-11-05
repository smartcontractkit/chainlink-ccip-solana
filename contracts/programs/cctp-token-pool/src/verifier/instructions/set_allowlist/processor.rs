use anchor_lang::prelude::*;

use crate::error::CctpPoolVerifierError;
use crate::verifier::state::{
    verifier_seed, AccountSchemaVersion, Config, RemoteChainAllowlistConfig, RemoteChainConfig,
};

#[derive(AnchorSerialize, AnchorDeserialize, Debug)]
pub struct SetVerifierAllowlistParams {
    pub dest_chain_selector: u64,
    pub enabled: bool,
    pub allowed_senders: Vec<Pubkey>,
}

pub fn set_verifier_allowlist(
    ctx: Context<SetVerifierAllowlist>,
    params: SetVerifierAllowlistParams,
) -> Result<()> {
    ctx.accounts.remote_chain_config.allowlist_config = RemoteChainAllowlistConfig {
        allowlist_enabled: params.enabled,
        allowed_senders: params.allowed_senders.clone(),
    };

    emit_cpi!(AllowlistSet {
        enabled: params.enabled,
        allowed_senders: params.allowed_senders,
        dest_chain_selector: params.dest_chain_selector,
    });

    Ok(())
}

#[event_cpi]
#[derive(Accounts)]
#[instruction(params: SetVerifierAllowlistParams)]
pub struct SetVerifierAllowlist<'info> {
    #[account(
        seeds = [verifier_seed::CONFIG],
        bump = config.bump,
        constraint = config.is_latest_version() @ CctpPoolVerifierError::InvalidVersion,
    )]
    pub config: Account<'info, Config>,

    /// The owner or the allowlist admin, matching EVM `applyAllowlistUpdates`.
    #[account(mut, constraint = authority.key() == config.owner || authority.key() == config.allowlist_admin @ CctpPoolVerifierError::Unauthorized)]
    pub authority: Signer<'info>,

    #[account(
        mut,
        seeds = [verifier_seed::REMOTE_CHAIN, &params.dest_chain_selector.to_le_bytes()],
        constraint = remote_chain_config.is_latest_version() @ CctpPoolVerifierError::InvalidVersion,
        bump = remote_chain_config.bump,
        realloc = RemoteChainConfig::DISCRIMINATOR.len() + RemoteChainConfig::INIT_SPACE + RemoteChainConfig::dynamic_space(&params.allowed_senders, &remote_chain_config.cctp.supported_tokens),
        realloc::payer = authority,
        realloc::zero = true,
    )]
    pub remote_chain_config: Account<'info, RemoteChainConfig>,

    pub system_program: Program<'info, System>,
}

#[event]
pub struct AllowlistSet {
    pub dest_chain_selector: u64,
    pub enabled: bool,
    pub allowed_senders: Vec<Pubkey>,
}
