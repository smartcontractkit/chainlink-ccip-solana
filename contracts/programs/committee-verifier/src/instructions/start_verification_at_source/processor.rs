use anchor_lang::prelude::*;

use ccip_common::messages::{MessageV1, SVMTokenAmount};
use ccip_common::seed;

use crate::instructions::VERSION_TAG;
use crate::state::{valid_version, Config, RemoteChainConfig, MAX_CONFIG_V};
use crate::CommitteeVerifierError;

#[derive(AnchorSerialize, AnchorDeserialize, Debug)]
pub struct StartVerificationAtSourceParams {
    pub message: MessageV1,
    pub message_id: [u8; 32],
    pub fee_token: SVMTokenAmount,
    pub verifier_args: Vec<u8>,
}

/// Called on the sending side as a verifier hook during message sending.
pub fn start_verification_at_source(
    ctx: Context<StartVerificationAtSource>,
    params: StartVerificationAtSourceParams,
) -> Result<Vec<u8>> {
    let remote_chain_config = &ctx.accounts.remote_chain_config;

    if remote_chain_config.allowlist_config.allowlist_enabled {
        let sender = Pubkey::try_from(params.message.sender.as_slice())
            .map_err(|_| CommitteeVerifierError::InvalidInstructionData)?;

        require!(
            remote_chain_config
                .allowlist_config
                .allowed_senders
                .contains(&sender),
            CommitteeVerifierError::SenderNotAllowed
        );
    }

    Ok(VERSION_TAG.to_vec())
}

/// The only account list the router resolves for this verifier; [`GetFee`] must present the same
/// one, since the router reuses this resolution for its `get_verifier_fee` CPI. See [`GetFee`] for why the
/// two are duplicated rather than macro-generated, and
/// `account_lists_match_start_verification_at_source` in `instructions/mod.rs` for the guard that
/// enforces it.
#[derive(Accounts)]
#[instruction(params: StartVerificationAtSourceParams)]
pub struct StartVerificationAtSource<'info> {
    #[account(
        seeds = [seed::CONFIG],
        bump = config.bump,
        constraint = valid_version(config.version, MAX_CONFIG_V) @ CommitteeVerifierError::InvalidVersion,
    )]
    pub config: Account<'info, Config>,

    #[account(
        address = remote_chain_config.admin_config.router_signer @ CommitteeVerifierError::Unauthorized,
    )]
    pub authority: Signer<'info>,

    #[account(
        seeds = [seed::REMOTE_CHAIN, &params.message.dest_chain_selector.to_le_bytes()],
        bump = remote_chain_config.bump,
        constraint = valid_version(remote_chain_config.version, MAX_CONFIG_V) @ CommitteeVerifierError::InvalidVersion,
    )]
    pub remote_chain_config: Account<'info, RemoteChainConfig>,
}
