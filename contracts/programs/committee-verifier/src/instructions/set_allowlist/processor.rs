use anchor_lang::prelude::*;

use ccip_common::seed;

use crate::event::AllowlistSet;
use crate::state::valid_version;
use crate::state::{
    Config, RemoteChainAllowlistConfig, RemoteChainConfig, ANCHOR_DISCRIMINATOR, MAX_CONFIG_V,
};
use crate::CommitteeVerifierError;

#[derive(AnchorSerialize, AnchorDeserialize, Debug)]
pub struct SetAllowlistParams {
    pub dest_chain_selector: u64,
    pub enabled: bool,
    pub allowed_senders: Vec<Pubkey>,
}

pub fn set_allowlist(ctx: Context<SetAllowlist>, params: SetAllowlistParams) -> Result<()> {
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
#[instruction(params: SetAllowlistParams)]
pub struct SetAllowlist<'info> {
    #[account(
        seeds = [seed::CONFIG],
        bump = config.bump,
        constraint = valid_version(config.version, MAX_CONFIG_V) @ CommitteeVerifierError::InvalidVersion,
    )]
    pub config: Account<'info, Config>,

    /// The owner or the allowlist admin, matching EVM `applyAllowlistUpdates`.
    #[account(mut, constraint = authority.key() == config.owner || authority.key() == config.allowlist_admin @ CommitteeVerifierError::Unauthorized)]
    pub authority: Signer<'info>,

    #[account(
        mut,
        seeds = [seed::REMOTE_CHAIN, &params.dest_chain_selector.to_le_bytes()],
        constraint = valid_version(remote_chain_config.version, MAX_CONFIG_V) @ CommitteeVerifierError::InvalidVersion,
        bump = remote_chain_config.bump,
        realloc = ANCHOR_DISCRIMINATOR + RemoteChainConfig::INIT_SPACE + RemoteChainConfig::dynamic_space(&params.allowed_senders, remote_chain_config.signer_config.signers()),
        realloc::payer = authority,
        realloc::zero = false,
    )]
    pub remote_chain_config: Account<'info, RemoteChainConfig>,

    pub system_program: Program<'info, System>,
}
