use anchor_lang::prelude::*;

use ccip_common::seed;

use crate::event::SignerConfigSet;
use crate::state::valid_version;
use crate::state::{
    Config, RemoteChainConfig, RemoteChainSignerConfig, ANCHOR_DISCRIMINATOR, MAX_CONFIG_V,
};
use crate::CommitteeVerifierError;

const ZERO_ADDRESS: crate::state::Signer = [0u8; 20];

#[derive(AnchorSerialize, AnchorDeserialize, Debug)]
pub struct SetSignerConfigParams {
    pub source_chain_selector: u64,
    pub threshold: u8,
    pub signers: Vec<crate::state::Signer>,
}

pub fn set_signer_config(
    ctx: Context<SetSignerConfig>,
    mut params: SetSignerConfigParams,
) -> Result<()> {
    for signer in &params.signers {
        require!(
            *signer != ZERO_ADDRESS,
            CommitteeVerifierError::ZeroAddressSigner
        );
    }

    params.signers.sort();
    for pair in params.signers.windows(2) {
        // require! instead of require_neq!: Signer does not implement Display
        require!(pair[0] != pair[1], CommitteeVerifierError::DuplicateSigner);
    }

    ctx.accounts.remote_chain_config.signer_config =
        RemoteChainSignerConfig::new(params.threshold, params.signers.clone())?;

    emit_cpi!(SignerConfigSet {
        source_chain_selector: params.source_chain_selector,
        signers: params.signers,
        threshold: params.threshold,
    });

    Ok(())
}

#[event_cpi]
#[derive(Accounts)]
#[instruction(params: SetSignerConfigParams)]
pub struct SetSignerConfig<'info> {
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
        realloc = ANCHOR_DISCRIMINATOR + RemoteChainConfig::INIT_SPACE + RemoteChainConfig::dynamic_space(&remote_chain_config.allowlist_config.allowed_senders, &params.signers),
        realloc::payer = authority,
        realloc::zero = false,
    )]
    pub remote_chain_config: Account<'info, RemoteChainConfig>,

    pub system_program: Program<'info, System>,
}
