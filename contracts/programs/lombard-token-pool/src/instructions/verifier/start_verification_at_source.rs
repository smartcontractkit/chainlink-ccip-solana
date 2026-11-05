//! `start_verification_at_source`: the source-side verifier hook, called by the router after
//! `lock_or_burn_tokens_v2` and after the CCIP `message_id` has been computed.
//! This is the instruction that actually performs the Lombard GMP dispatch.

use crate::bridge::{
    cpi::{accounts::Deposit, deposit},
    program::Bridge,
    state::{RemoteBridgeConfig, RemoteTokenConfig},
};
use anchor_lang::prelude::*;
use anchor_spl::token_interface::{Mint, TokenAccount, TokenInterface};
use base_token_pool::constants::{valid_version, POOL_SIGNER_SEED};
use base_token_pool::errors::CcipTokenPoolError;
use ccip_common::messages::{MessageV1, SVMTokenAmount};
use ccip_common::resolution::find;

use crate::constants::{VERIFIER_CONFIG_SEED, VERIFIER_REMOTE_CHAIN_SEED};
use crate::errors::LombardTokenPoolError;
use crate::instructions::verifier::message_binding::pack_message_binding;
use crate::state::{
    VerifierConfig, VerifierRemoteChainConfig, MAX_VERIFIER_CONFIG_V,
    MAX_VERIFIER_REMOTE_CHAIN_CONFIG_V,
};

#[derive(AnchorSerialize, AnchorDeserialize, Clone, Debug)]
pub struct StartVerificationAtSourceParams {
    pub message: MessageV1,
    pub message_id: [u8; 32],
    pub fee_token: SVMTokenAmount,
    pub verifier_args: Vec<u8>,
}

#[derive(Accounts)]
#[instruction(params: StartVerificationAtSourceParams)]
pub struct StartVerificationAtSource<'info> {
    #[account(
        seeds = [VERIFIER_CONFIG_SEED],
        bump = verifier_config.bump,
        constraint = valid_version(verifier_config.version, MAX_VERIFIER_CONFIG_V) @ CcipTokenPoolError::InvalidVersion,
    )]
    pub verifier_config: Box<Account<'info, VerifierConfig>>,

    /// The router-derived `ccv_signer` PDA the router signs this CPI with, matching
    /// `committee-verifier`'s own `authority: Signer` requirement.
    #[account(
        address = remote_chain_config.admin_config.router_signer @ CcipTokenPoolError::Unauthorized,
    )]
    pub authority: Signer<'info>,

    #[account(
        seeds = [VERIFIER_REMOTE_CHAIN_SEED, &params.message.dest_chain_selector.to_le_bytes()],
        bump = remote_chain_config.bump,
        constraint = valid_version(remote_chain_config.version, MAX_VERIFIER_REMOTE_CHAIN_CONFIG_V) @ CcipTokenPoolError::InvalidVersion,
    )]
    pub remote_chain_config: Box<Account<'info, VerifierRemoteChainConfig>>,

    pub token_program: Interface<'info, TokenInterface>,

    /// CHECK against `params.message.token_transfer.source_token_address` in the handler - not
    /// tied to it via a static seed/address constraint because that field lives inside a Borsh
    /// `Vec<u8>` nested in the instruction args.
    #[account(mut, owner = token_program.key())]
    pub mint: InterfaceAccount<'info, Mint>,

    /// CHECK: this program's own pool_signer PDA - identical seeds to the pool's own v1/v2
    /// lock_or_burn contexts, since verifier and pool are embedded in the same program.
    ///
    /// Used as both `fee_payer` and `sender` in the `bridge::deposit` CPI,
    /// both are `Signer` + `#[account(mut)]` on the bridge program.
    #[account(mut, seeds = [POOL_SIGNER_SEED, mint.key().as_ref()], bump)]
    pub pool_signer: UncheckedAccount<'info>,

    /// The custody account `lock_or_burn_tokens_v2` left the (post-fee) tokens in; this is what
    /// `bridge::deposit` actually burns from.
    #[account(
        mut,
        associated_token::mint = mint,
        associated_token::authority = pool_signer,
        associated_token::token_program = token_program
    )]
    pub pool_token_account: Box<InterfaceAccount<'info, TokenAccount>>,

    pub bridge: Program<'info, Bridge>,
    /// CHECK: verified by the bridge program
    pub bridge_config: UncheckedAccount<'info>,
    /// CHECK: verified by the bridge program
    pub bridge_sender_config: UncheckedAccount<'info>,
    #[account(
        seeds = [b"remote_bridge_config", remote_chain_config.path.l_chain_id.as_ref()],
        bump = remote_bridge_config.bump,
        seeds::program = crate::bridge::ID,
    )]
    pub remote_bridge_config: Account<'info, RemoteBridgeConfig>,
    /// CHECK: verified by the bridge program
    pub local_token_config: UncheckedAccount<'info>,
    #[account(
        seeds = [b"remote_token_config", mint.key().as_ref(), remote_chain_config.path.l_chain_id.as_ref()],
        bump = remote_token_config.bump,
        seeds::program = crate::bridge::ID,
    )]
    pub remote_token_config: Account<'info, RemoteTokenConfig>,
    /// CHECK: verified by the bridge program
    pub mailbox: UncheckedAccount<'info>,
    /// CHECK: verified by the mailbox program
    #[account(mut)]
    pub mailbox_config: UncheckedAccount<'info>,
    /// CHECK: verified by the mailbox program
    pub outbound_message_path: UncheckedAccount<'info>,
    /// CHECK: verified by the mailbox program
    #[account(mut)]
    pub outbound_message: UncheckedAccount<'info>,
    /// CHECK: verified by the mailbox program
    pub mailbox_sender_config: UncheckedAccount<'info>,
    /// CHECK: verified by the mailbox program
    #[account(mut)]
    pub treasury: Option<UncheckedAccount<'info>>,
    pub system_program: Program<'info, System>,
}

/// Returns the raw `payload_hash` as `verifierData`, mirroring EVM's `forwardToVerifier`
/// returning `bytes.concat(payloadHash)`.
pub fn start_verification_at_source(
    ctx: Context<StartVerificationAtSource>,
    params: StartVerificationAtSourceParams,
) -> Result<Vec<u8>> {
    let token_transfer = params
        .message
        .token_transfer
        .as_ref()
        .ok_or(LombardTokenPoolError::InvalidPath)?;

    let allowlist_config = &ctx.accounts.remote_chain_config.allowlist_config;

    if allowlist_config.allowlist_enabled {
        let sender = Pubkey::try_from(params.message.sender.as_slice())
            .map_err(|_| LombardTokenPoolError::SenderNotAllowed)?;

        require!(
            allowlist_config.allowed_senders.contains(&sender),
            LombardTokenPoolError::SenderNotAllowed
        );
    }

    let source_token = Pubkey::try_from(token_transfer.source_token_address.as_slice())
        .map_err(|_| LombardTokenPoolError::InvalidPath)?;
    require_keys_eq!(
        source_token,
        ctx.accounts.mint.key(),
        LombardTokenPoolError::InvalidPath
    );

    // Mirrors EVM's `_callDepositOnBridge`: only tokens the verifier has explicitly been
    // configured to support may be forwarded.
    require!(
        ctx.accounts
            .verifier_config
            .supported_tokens
            .iter()
            .any(|t| t.local_token == ctx.accounts.mint.key()),
        LombardTokenPoolError::UnsupportedToken
    );

    let source_pool_pubkey = Pubkey::try_from(token_transfer.source_pool_address.as_slice())
        .map_err(|_| LombardTokenPoolError::InvalidPath)?;
    let pool_config_pda = find(
        &[
            base_token_pool::constants::POOL_STATE_SEED,
            source_token.as_ref(),
        ],
        crate::ID,
    );
    require_keys_eq!(
        source_pool_pubkey,
        pool_config_pda,
        LombardTokenPoolError::UnexpectedSourcePool
    );

    let path = ctx.accounts.remote_chain_config.path;
    require!(path.is_set(), LombardTokenPoolError::PathNotConfigured);

    // Mirrors EVM's `LombardVerifier._callDepositOnBridge`: the bridge's own configured
    // destination token for this mint must match what the CCIP message declares, unless a
    // remote adapter override is configured for this token, confirming the mismatch is a known,
    // admin-approved substitution rather than a misconfiguration.
    require_gte!(
        32,
        token_transfer.dest_token_address.len(),
        LombardTokenPoolError::InvalidPath
    );
    let mut expected_dest_token = [0u8; 32];
    let dest_token_offset = 32 - token_transfer.dest_token_address.len();
    expected_dest_token[dest_token_offset..].copy_from_slice(&token_transfer.dest_token_address);
    if ctx.accounts.remote_token_config.token != expected_dest_token {
        let remote_adapter = ctx
            .accounts
            .remote_chain_config
            .remote_adapters
            .iter()
            .find(|adapter| adapter.token == ctx.accounts.mint.key())
            .map(|adapter| adapter.remote_adapter);
        require!(
            remote_adapter == Some(ctx.accounts.remote_token_config.token),
            LombardTokenPoolError::RemoteTokenOrAdapterMismatch
        );
    }

    // The Lombard bridge assumes addresses fit in 32 bytes and therefore only supports up to 32 byte addresses.
    require_gte!(
        32,
        token_transfer.token_receiver.len(),
        LombardTokenPoolError::MalformedReceiver
    );
    let mut recipient = [0u8; 32];
    let offset = 32 - token_transfer.token_receiver.len();
    recipient[offset..].copy_from_slice(&token_transfer.token_receiver);

    require_eq!(
        params.message.sender.len(),
        32,
        LombardTokenPoolError::InvalidPath
    );
    let mut sender = [0u8; 32];
    sender.copy_from_slice(&params.message.sender);

    let amount: u64 = token_transfer
        .amount
        .try_into()
        .map_err(|_| LombardTokenPoolError::InvalidPayload)?;

    let optional_message =
        pack_message_binding(ctx.accounts.verifier_config.version_tag, params.message_id);

    let pool_signer_seeds: &[&[&[u8]]] = &[&[
        POOL_SIGNER_SEED,
        &ctx.accounts.mint.key().to_bytes(),
        &[ctx.bumps.pool_signer],
    ]];

    let cpi_context = CpiContext::new_with_signer(
        ctx.accounts.bridge.key(),
        Deposit {
            fee_payer: ctx.accounts.pool_signer.to_account_info(),
            sender: ctx.accounts.pool_signer.to_account_info(),
            sender_token_account: ctx.accounts.pool_token_account.to_account_info(),
            token_program: ctx.accounts.token_program.to_account_info(),
            config: ctx.accounts.bridge_config.to_account_info(),
            mint: ctx.accounts.mint.to_account_info(),
            mailbox: ctx.accounts.mailbox.to_account_info(),
            mailbox_config: ctx.accounts.mailbox_config.to_account_info(),
            outbound_message_path: ctx.accounts.outbound_message_path.to_account_info(),
            outbound_message: ctx.accounts.outbound_message.to_account_info(),
            mailbox_sender_config: ctx.accounts.mailbox_sender_config.to_account_info(),
            sender_config: ctx.accounts.bridge_sender_config.to_account_info(),
            remote_bridge_config: ctx.accounts.remote_bridge_config.to_account_info(),
            local_token_config: ctx.accounts.local_token_config.to_account_info(),
            remote_token_config: ctx.accounts.remote_token_config.to_account_info(),
            treasury: ctx.accounts.treasury.as_ref().map(|t| t.to_account_info()),
            system_program: ctx.accounts.system_program.to_account_info(),
        },
        pool_signer_seeds,
    );

    let result = deposit(
        cpi_context,
        sender,
        recipient,
        Some(path.allowed_caller),
        amount,
        Some(optional_message),
    )?;

    Ok(result.get().payload_hash.to_vec())
}
