use anchor_lang::prelude::*;

use crate::error::CctpPoolVerifierError;
use crate::verifier::state::*;

#[derive(AnchorSerialize, AnchorDeserialize, Debug)]
pub struct SetVerifierChainConfigCctpParams {
    pub remote_chain_selector: u64,
    pub cctp: RemoteCctpConfig,
}

pub fn set_verifier_chain_config_cctp(
    ctx: Context<SetVerifierChainConfigCctp>,
    params: SetVerifierChainConfigCctpParams,
) -> Result<()> {
    // A zero destination caller lets anyone receive the CCTP message on the destination, consuming
    // it before the CCIP verifier can. As EVM's `CCTPVerifier.setDomains`.
    require_keys_neq!(
        params.cctp.allowed_caller_receive,
        Pubkey::default(),
        CctpPoolVerifierError::CctpAllowedCallersInvalidZero
    );
    require_keys_neq!(
        params.cctp.allowed_caller_deposit,
        Pubkey::default(),
        CctpPoolVerifierError::CctpAllowedCallersInvalidZero
    );

    let remote_chain_config = &mut ctx.accounts.remote_chain_config;
    remote_chain_config.cctp = params.cctp;

    emit_cpi!(ChainConfigSetCctp {
        remote_chain_selector: params.remote_chain_selector,
        enabled: remote_chain_config.cctp.enabled,
        domain_identifier: remote_chain_config.cctp.domain_identifier,
        lookup_table: remote_chain_config.cctp.lookup_table,
        allowed_caller_receive: remote_chain_config.cctp.allowed_caller_receive,
        allowed_caller_deposit: remote_chain_config.cctp.allowed_caller_deposit,
        mint_recipient_on_dest: remote_chain_config.cctp.mint_recipient_on_dest,
        supported_tokens: remote_chain_config.cctp.supported_tokens.clone(),
    });

    Ok(())
}

#[event_cpi]
#[derive(Accounts)]
#[instruction(params: SetVerifierChainConfigCctpParams)]
pub struct SetVerifierChainConfigCctp<'info> {
    #[account(
        seeds = [verifier_seed::CONFIG],
        bump = config.bump,
        constraint = config.is_latest_version() @ CctpPoolVerifierError::InvalidVersion,
    )]
    pub config: Account<'info, Config>,

    #[account(mut, address = config.owner @ CctpPoolVerifierError::Unauthorized)]
    pub authority: Signer<'info>,

    #[account(
        mut,
        seeds = [verifier_seed::REMOTE_CHAIN, &params.remote_chain_selector.to_le_bytes()],
        bump = remote_chain_config.bump,
        constraint = remote_chain_config.is_latest_version() @ CctpPoolVerifierError::InvalidVersion,
        realloc = RemoteChainConfig::DISCRIMINATOR.len() + RemoteChainConfig::INIT_SPACE + RemoteChainConfig::dynamic_space(&remote_chain_config.allowlist_config.allowed_senders, &params.cctp.supported_tokens),
        realloc::payer = authority,
        realloc::zero = true,
    )]
    pub remote_chain_config: Account<'info, RemoteChainConfig>,

    pub system_program: Program<'info, System>,
}

#[event]
pub struct ChainConfigSetCctp {
    pub remote_chain_selector: u64,
    pub enabled: bool,          // Whether or not the domain is enabled.
    pub domain_identifier: u32, // Unique domain ID used across CCTP (https://developers.circle.com/cctp/concepts/supported-chains-and-domains#domain-identifiers)
    pub lookup_table: Option<Pubkey>,

    // The following addresses are limited to 32 bytes by Circle's CCTP message format. They are encoded into the
    // Pubkey type as the CCTP contracts expect them that way, but it's effectively a (padded) 32-byte slice.
    // See https://developers.circle.com/cctp/references/technical-guide#message-format
    pub allowed_caller_receive: Pubkey, // Address allowed to call receiveMessage on the domain.
    pub allowed_caller_deposit: Pubkey, // Address allowed to call depositForBurn on the domain.
    pub mint_recipient_on_dest: Option<Pubkey>, // Address to mint USDC to on the destination chain, for chains that proxy the funds through an intermediate account.

    pub supported_tokens: Vec<RemoteLocalPair>, // List of supported tokens for this domain.
}
