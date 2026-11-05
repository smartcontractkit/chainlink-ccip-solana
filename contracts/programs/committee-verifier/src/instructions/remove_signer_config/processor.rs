use anchor_lang::prelude::*;

use ccip_common::seed;

use crate::event::SignerConfigSet;
use crate::state::valid_version;
use crate::state::{
    Config, RemoteChainConfig, RemoteChainSignerConfig, ANCHOR_DISCRIMINATOR, MAX_CONFIG_V,
};
use crate::CommitteeVerifierError;

#[derive(AnchorSerialize, AnchorDeserialize, Debug)]
pub struct RemoveSignerConfigParams {
    pub source_chain_selector: u64,
}

pub fn remove_signer_config(
    ctx: Context<RemoveSignerConfig>,
    params: RemoveSignerConfigParams,
) -> Result<()> {
    ctx.accounts.remote_chain_config.signer_config = RemoteChainSignerConfig::new_disabled();

    emit_cpi!(SignerConfigSet {
        source_chain_selector: params.source_chain_selector,
        signers: vec![],
        threshold: 0,
    });

    Ok(())
}

#[event_cpi]
#[derive(Accounts)]
#[instruction(params: RemoveSignerConfigParams)]
pub struct RemoveSignerConfig<'info> {
    #[account(
        seeds = [seed::CONFIG],
        bump = config.bump,
        constraint = valid_version(config.version, MAX_CONFIG_V) @ CommitteeVerifierError::InvalidVersion,
    )]
    pub config: Account<'info, Config>,

    #[account(mut, address = config.owner @ CommitteeVerifierError::Unauthorized)]
    pub authority: Signer<'info>,

    #[account(
        mut,
        seeds = [seed::REMOTE_CHAIN, &params.source_chain_selector.to_le_bytes()],
        constraint = valid_version(remote_chain_config.version, MAX_CONFIG_V) @ CommitteeVerifierError::InvalidVersion,
        bump = remote_chain_config.bump,
        realloc = ANCHOR_DISCRIMINATOR + RemoteChainConfig::INIT_SPACE + RemoteChainConfig::dynamic_space(&remote_chain_config.allowlist_config.allowed_senders, &[]),
        realloc::payer = authority,
        realloc::zero = false,
    )]
    pub remote_chain_config: Account<'info, RemoteChainConfig>,

    pub system_program: Program<'info, System>,
}
