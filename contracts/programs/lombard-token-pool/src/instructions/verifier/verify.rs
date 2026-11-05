//! `verify`: the destination-side verifier hook. This is the instruction that actually consumes
//! the Lombard GMP message and checks the message-ID binding embedded in the bridge message.
//!
//! Unlike EVM's `IMailbox.deliverAndHandle(rawPayload, proof)` (which verifies a raw proof AND
//! consumes the message in one call), Solana's `mailbox::handle_message` only *consumes* a
//! message that's already in `MessageState::Delivered` - the cryptographic proof was already
//! verified by a separate, earlier Mailbox instruction a relayer calls before this one runs.
//! So `verifier_results` here doesn't need to carry a raw payload+proof at all: it's just
//! the 32-byte `payload_hash` identifying which already-delivered message to consume,
//! mirroring what v1's `release_or_mint_tokens` already does with `release_or_mint.source_pool_data`.

use crate::bridge::{
    program::Bridge,
    state::{RemoteBridgeConfig, RemoteTokenConfig},
    utils::gmp_messages::InboundResponse,
};
use crate::mailbox::{cpi::accounts::HandleMessage, cpi::handle_message, program::Mailbox};
use anchor_lang::prelude::*;
use anchor_spl::associated_token::get_associated_token_address_with_program_id;
use anchor_spl::token_interface::{Mint, TokenInterface};
use base_token_pool::constants::{
    valid_version, POOL_CHAINCONFIG_SEED, POOL_SIGNER_SEED, POOL_STATE_SEED,
};
use base_token_pool::errors::CcipTokenPoolError;
use base_token_pool::helpers::protocol_amount_to_svm_token_amount;
use ccip_common::messages::{MessageV1, SVMDestBlob};

use crate::constants::{VERIFIER_CONFIG_SEED, VERIFIER_REMOTE_CHAIN_SEED};
use crate::errors::LombardTokenPoolError;
use crate::instructions::verifier::message_binding::unpack_message_binding;
use crate::state::{
    ChainConfig, State, VerifierConfig, VerifierRemoteChainConfig, MAX_VERIFIER_CONFIG_V,
    MAX_VERIFIER_REMOTE_CHAIN_CONFIG_V,
};

#[derive(AnchorSerialize, AnchorDeserialize, Clone, Debug)]
pub struct VerifyParams {
    pub message_id: [u8; 32],
    pub encoded_message: Vec<u8>,
    /// 32-byte `payload_hash` of an already-`Delivered` Lombard mailbox message
    pub verifier_results: Vec<u8>,
}

#[derive(Accounts)]
pub struct Verify<'info> {
    /// Writable: forwarded as the real mailbox's `HandleMessage.handler`
    /// (`Signer` + `#[account(mut)]`).
    #[account(
        mut,
        seeds = [VERIFIER_CONFIG_SEED],
        bump = verifier_config.bump,
        constraint = valid_version(verifier_config.version, MAX_VERIFIER_CONFIG_V) @ CcipTokenPoolError::InvalidVersion,
    )]
    pub verifier_config: Account<'info, VerifierConfig>,

    /// CHECK: PDA-address-and-version validated manually in the handler once
    /// `source_chain_selector` is known from the decoded message; `owner` is checked here since
    /// it doesn't depend on that.
    #[account(owner = crate::ID)]
    pub remote_chain_config: UncheckedAccount<'info>,

    /// The offramp-derived `ccv_signer` PDA the offramp signs this CPI with - proves whoever
    /// signed is under the control of `offramp_program`.
    #[account(
        seeds = [ccip_common::seed::CCV_SIGNER, crate::ID.as_ref()],
        bump,
        seeds::program = offramp_program.key(),
    )]
    pub authority: Signer<'info>,

    /// CHECK: exists only to derive the `authority` and `allowed_offramp` PDAs.
    pub offramp_program: UncheckedAccount<'info>,

    /// CHECK: PDA of the router program proving `offramp_program` is a currently-registered
    /// offramp for this message's source chain - if the PDA doesn't exist, the router doesn't
    /// allow it. Exact address is verified manually in the handler.
    #[account(owner = state.config.router @ CcipTokenPoolError::Unauthorized)]
    pub allowed_offramp: UncheckedAccount<'info>,

    /// This program's own pool `State` for the mint being verified.
    #[account(seeds = [POOL_STATE_SEED, state.config.mint.as_ref()], bump)]
    pub state: Box<Account<'info, State>>,

    pub token_program: Interface<'info, TokenInterface>,
    #[account(mut)]
    pub mint: InterfaceAccount<'info, Mint>,

    /// CHECK: this program's own pool_signer PDA.
    ///
    /// Forwarded as the bridge's `GMPReceive.handler`, is `Signer` + `#[account(mut)]` (pays `message_handled`'s rent
    /// via `init`), so this must be writable.
    #[account(mut, seeds = [POOL_SIGNER_SEED, mint.key().as_ref()], bump)]
    pub pool_signer: UncheckedAccount<'info>,

    /// CHECK: verified by the bridge program
    #[account(mut)]
    pub receiver_token_account: UncheckedAccount<'info>,
    /// CHECK: verified by the bridge program
    pub mint_authority: UncheckedAccount<'info>,
    /// CHECK: verified by the bridge program
    pub token_authority: UncheckedAccount<'info>,
    pub bridge: Program<'info, Bridge>,
    /// CHECK: verified by the bridge program
    pub bridge_config: UncheckedAccount<'info>,
    pub mailbox: Program<'info, Mailbox>,
    /// CHECK: verified by the mailbox program
    pub mailbox_config: UncheckedAccount<'info>,
    /// CHECK: verified by the bridge program
    pub local_token_config: UncheckedAccount<'info>,
    pub system_program: Program<'info, System>,
    pub remote_bridge_config: Box<Account<'info, RemoteBridgeConfig>>,
    /// bridge's `GMPReceive.remote_token_config` is `#[account(mut, ...)]` - `gmp_receive`
    /// consumes `inbound_rate_limit` on it, so it must be writable or the nested CPI fails.
    #[account(mut)]
    pub remote_token_config: Box<Account<'info, RemoteTokenConfig>>,
    /// CHECK: PDA address is verified manually in the handler since the remote chain selector is only
    /// known once the message is decoded.
    pub chain_config: Box<Account<'info, ChainConfig>>,
    /// CHECK: verified by the mailbox program
    pub inbound_message_path: UncheckedAccount<'info>,
    /// CHECK: verified by the mailbox program
    #[account(mut)]
    pub message_info: UncheckedAccount<'info>,
    /// CHECK: verified by the mailbox program
    #[account(mut)]
    pub message_handled: UncheckedAccount<'info>,
}

pub fn verify(ctx: Context<Verify>, params: VerifyParams) -> Result<()> {
    let message = MessageV1::from_bytes(&params.encoded_message)
        .map_err(|_| LombardTokenPoolError::InvalidPayload)?;

    let (expected_remote_chain_config, _) = Pubkey::find_program_address(
        &[
            VERIFIER_REMOTE_CHAIN_SEED,
            &message.source_chain_selector.to_le_bytes(),
        ],
        &crate::ID,
    );
    require_keys_eq!(
        ctx.accounts.remote_chain_config.key(),
        expected_remote_chain_config,
        LombardTokenPoolError::PathNotConfigured
    );
    let remote_chain_config = VerifierRemoteChainConfig::try_deserialize(
        &mut &ctx.accounts.remote_chain_config.try_borrow_data()?[..],
    )?;
    require!(
        valid_version(
            remote_chain_config.version,
            MAX_VERIFIER_REMOTE_CHAIN_CONFIG_V
        ),
        CcipTokenPoolError::InvalidVersion
    );
    let path = remote_chain_config.path;
    require!(path.is_set(), LombardTokenPoolError::PathNotConfigured);

    // Verify this was called by an allowed offramp to protect from it being invoked directly
    let (expected_allowed_offramp, _) = Pubkey::find_program_address(
        &[
            ccip_common::seed::ALLOWED_OFFRAMP,
            &message.source_chain_selector.to_le_bytes(),
            ctx.accounts.offramp_program.key().as_ref(),
        ],
        &ctx.accounts.state.config.router,
    );
    require_keys_eq!(
        ctx.accounts.allowed_offramp.key(),
        expected_allowed_offramp,
        CcipTokenPoolError::Unauthorized
    );

    // remote_bridge_config/remote_token_config aren't pinned via a struct-level `seeds =`
    // constraint (path.l_chain_id, needed for both, is only known once the message is decoded
    // above), so - mirroring remote_chain_config's own manual address check - re-derive and
    // verify each PDA explicitly rather than trusting a data-field match alone (an account with
    // owner == bridge::ID and a coincidentally-matching field wouldn't necessarily be the
    // canonical account for this chain_id/mint).
    let (expected_remote_bridge_config, _) = Pubkey::find_program_address(
        &[b"remote_bridge_config", &path.l_chain_id],
        &crate::bridge::ID,
    );
    require_keys_eq!(
        ctx.accounts.remote_bridge_config.key(),
        expected_remote_bridge_config,
        LombardTokenPoolError::PathNotConfigured
    );
    let (expected_remote_token_config, _) = Pubkey::find_program_address(
        &[
            b"remote_token_config",
            ctx.accounts.mint.key().as_ref(),
            &path.l_chain_id,
        ],
        &crate::bridge::ID,
    );
    require_keys_eq!(
        ctx.accounts.remote_token_config.key(),
        expected_remote_token_config,
        LombardTokenPoolError::PathNotConfigured
    );
    // Mirrors EVM's `_validateEnvelope`: the GMP envelope sender must be the remote chain's
    // canonical bridge/asset-router address, not just any message on the same mailbox.
    require!(
        ctx.accounts.remote_bridge_config.bridge == path.remote_bridge_sender,
        LombardTokenPoolError::PathNotConfigured
    );

    let token_transfer = message
        .token_transfer
        .as_ref()
        .ok_or(LombardTokenPoolError::InvalidPath)?;
    let dest_token = Pubkey::try_from(token_transfer.dest_token_address.as_slice())
        .map_err(|_| LombardTokenPoolError::InvalidPath)?;
    require_keys_eq!(
        dest_token,
        ctx.accounts.mint.key(),
        LombardTokenPoolError::InvalidPath
    );
    require_keys_eq!(
        ctx.accounts.mint.key(),
        ctx.accounts.state.config.mint,
        LombardTokenPoolError::InvalidPath
    );

    // Mirrors EVM's `_validatePayload`: the bridged recipient must be the CCIP message's own
    // token receiver, not just whatever account the executor happened to pass in. Without this,
    // an unrelated (but validly proven) Lombard payload bound to this message's ID via
    // `optional_message` could be consumed in its place, minting to the wrong account while the
    // real transfer for this message is stranded.
    let receiver = Pubkey::try_from(token_transfer.token_receiver.as_slice())
        .map_err(|_| LombardTokenPoolError::MalformedReceiver)?;
    let receiver_usage = SVMDestBlob::from_bytes(&message.dest_blob)
        .map_err(|_| LombardTokenPoolError::InvalidResolutionInputs)?
        .use_ata;
    let expected_receiver_token_account = if receiver_usage.should_derive_ata() {
        get_associated_token_address_with_program_id(
            &receiver,
            &ctx.accounts.mint.key(),
            &ctx.accounts.token_program.key(),
        )
    } else {
        receiver
    };
    require_keys_eq!(
        ctx.accounts.receiver_token_account.key(),
        expected_receiver_token_account,
        LombardTokenPoolError::InvalidReceiver
    );

    // chain_config isn't pinned via a struct-level `seeds =` constraint for the same reason as
    // remote_bridge_config/remote_token_config above: the remote chain selector is only known
    // once the message is decoded.
    let (expected_chain_config, _) = Pubkey::find_program_address(
        &[
            POOL_CHAINCONFIG_SEED,
            &message.source_chain_selector.to_le_bytes(),
            ctx.accounts.mint.key().as_ref(),
        ],
        &crate::ID,
    );
    require_keys_eq!(
        ctx.accounts.chain_config.key(),
        expected_chain_config,
        LombardTokenPoolError::PathNotConfigured
    );

    require!(
        params.verifier_results.len() == 32,
        LombardTokenPoolError::InvalidPayloadLength
    );
    let mut payload_hash = [0u8; 32];
    payload_hash.copy_from_slice(&params.verifier_results);

    let response = mailbox_receive_message(&ctx, &payload_hash)?;

    // Mirrors EVM's `_validatePayload` amount check: the amount the bridge actually minted must
    // equal the CCIP message's own token_transfer amount, converted to local mint decimals. v1
    // (release_or_mint_tokens.rs) enforces this same equality; it was dropped when v2 was ported.
    let expected_amount = protocol_amount_to_svm_token_amount(
        token_transfer.amount,
        ctx.accounts.chain_config.base.remote.decimals,
        ctx.accounts.mint.decimals,
    )?;
    require!(
        response.amount == expected_amount,
        LombardTokenPoolError::AmountMismatch
    );

    let bridged = response
        .message
        .ok_or(LombardTokenPoolError::InvalidPayload)?;
    let (version_tag, returned_message_id) = unpack_message_binding(&bridged);
    require!(
        version_tag == ctx.accounts.verifier_config.version_tag,
        LombardTokenPoolError::InvalidPayload
    );
    require!(
        returned_message_id == params.message_id,
        LombardTokenPoolError::InvalidPayload
    );

    Ok(())
}

fn mailbox_receive_message(
    ctx: &Context<Verify>,
    payload_hash: &[u8; 32],
) -> Result<InboundResponse> {
    // The mailbox handler must be a chain-level identity, not the per-mint State PDA: the EVM
    // verifier's path.allowedCaller is one value per chain, while the State PDA differs per mint.
    let verifier_config_signer_seed = &[VERIFIER_CONFIG_SEED, &[ctx.accounts.verifier_config.bump]];
    let pool_signer_seed = &[
        POOL_SIGNER_SEED,
        &ctx.accounts.mint.key().to_bytes(),
        &[ctx.bumps.pool_signer],
    ];
    let signer_seeds: &[&[&[u8]]] = &[verifier_config_signer_seed, pool_signer_seed];

    let pool_signer = AccountInfo {
        is_signer: true,
        ..ctx.accounts.pool_signer.to_account_info()
    };

    let remaining_accounts: Vec<AccountInfo<'_>> = vec![
        pool_signer,
        ctx.accounts.bridge_config.to_account_info(),
        ctx.accounts.message_handled.to_account_info(),
        ctx.accounts.token_program.to_account_info(),
        ctx.accounts.receiver_token_account.to_account_info(),
        ctx.accounts.mint.to_account_info(),
        ctx.accounts.mint_authority.to_account_info(),
        ctx.accounts.token_authority.to_account_info(),
        ctx.accounts.remote_bridge_config.to_account_info(),
        ctx.accounts.local_token_config.to_account_info(),
        ctx.accounts.remote_token_config.to_account_info(),
        ctx.accounts.inbound_message_path.to_account_info(),
        ctx.accounts.system_program.to_account_info(),
    ];

    let cpi_context = CpiContext::new_with_signer(
        ctx.accounts.mailbox.key(),
        HandleMessage {
            handler: ctx.accounts.verifier_config.to_account_info(),
            config: ctx.accounts.mailbox_config.to_account_info(),
            message_info: ctx.accounts.message_info.to_account_info(),
            recipient_program: ctx.accounts.bridge.to_account_info(),
        },
        signer_seeds,
    )
    .with_remaining_accounts(remaining_accounts);

    let result = handle_message(cpi_context, *payload_hash)?;
    let result_data = result
        .get()
        .ok_or(LombardTokenPoolError::FailedGmpMessageHandle)?;

    InboundResponse::try_from_slice(&result_data)
        .map_err(|_| LombardTokenPoolError::FailedGmpMessageHandle.into())
}
