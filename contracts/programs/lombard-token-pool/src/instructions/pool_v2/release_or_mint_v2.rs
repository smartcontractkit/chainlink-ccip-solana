//! Unlike a "standard" base-token-pool v2 pool, this does NOT perform an independent SPL mint.
//! `bridge::gmp_receive` (called from the verifier's `verify`) already performs the actual mint
//! as a side effect of consuming the Lombard GMP message, so this instruction fully trusts that
//! and only validates + consumes the inbound rate limit, matching EVM's `TokenPool.releaseOrMint` V2

use anchor_lang::prelude::*;
use anchor_spl::associated_token::get_associated_token_address_with_program_id;
use base_token_pool::constants::{
    valid_version, ALLOWED_OFFRAMP, EXTERNAL_TOKEN_POOLS_SIGNER, POOL_CHAINCONFIG_SEED,
    POOL_OVERRIDE_CHAINCONFIG_SEED, POOL_SIGNER_SEED, POOL_STATE_SEED,
};
use base_token_pool::errors::CcipTokenPoolError;
use base_token_pool::events::ReleasedOrMinted;
use base_token_pool::helpers::{
    protocol_amount_to_svm_token_amount, validate_source_pool_decimals,
};
use base_token_pool::state::BaseChain;
use base_token_pool::v2::{validate_release_or_mint_v2, ReleaseOrMintInV2, ReleaseOrMintOutV2};
use ccip_common::state::PoolChainConfigOverrideType;

use crate::constants::{MAX_POOL_CHAIN_CONFIG_V, MAX_POOL_STATE_V};
use crate::state::{ChainConfig, ChainConfigOverride, State};

/// Account order must match exactly what `ccip-offramp`'s `token_transfer.rs`
/// `interact_with_pool_v2` always builds for ANY v2 pool's `release_or_mint` CPI.
#[event_cpi]
#[derive(Accounts)]
#[instruction(release_or_mint: ReleaseOrMintInV2)]
pub struct TokenOfframpV2<'info> {
    #[account(
        seeds = [EXTERNAL_TOKEN_POOLS_SIGNER, crate::ID.as_ref()],
        bump,
        seeds::program = offramp_program.key(),
    )]
    pub authority: Signer<'info>,

    /// CHECK: exists only to derive the allowed-offramp PDA and the authority PDA.
    pub offramp_program: UncheckedAccount<'info>,

    /// CHECK: PDA of the router program verifying the signer is an allowed offramp. If absent,
    /// the router doesn't allow this offramp for this remote chain.
    #[account(
        owner = state.config.router @ CcipTokenPoolError::InvalidPoolCaller,
        seeds = [
            ALLOWED_OFFRAMP,
            &release_or_mint.remote_chain_selector.to_le_bytes(),
            offramp_program.key().as_ref()
        ],
        bump,
        seeds::program = state.config.router,
    )]
    pub allowed_offramp: UncheckedAccount<'info>,

    #[account(
        seeds = [POOL_STATE_SEED, mint.key().as_ref()],
        bump,
        constraint = valid_version(state.version, MAX_POOL_STATE_V) @ CcipTokenPoolError::InvalidVersion,
    )]
    pub state: Box<Account<'info, State>>,

    #[account(address = state.config.token_program)]
    pub token_program: UncheckedAccount<'info>,

    #[account(
        mut,
        address = release_or_mint.local_token,
        owner = token_program.key(),
    )]
    pub mint: UncheckedAccount<'info>,

    #[account(
        seeds = [POOL_SIGNER_SEED, mint.key().as_ref()],
        bump,
        address = state.config.pool_signer,
    )]
    pub pool_signer: UncheckedAccount<'info>,

    #[account(address = state.config.pool_token_account)]
    pub pool_token_account: UncheckedAccount<'info>,

    #[account(
        mut,
        seeds = [POOL_CHAINCONFIG_SEED, &release_or_mint.remote_chain_selector.to_le_bytes(), state.config.mint.as_ref()],
        bump,
        constraint = valid_version(chain_config.version, MAX_POOL_CHAIN_CONFIG_V) @ CcipTokenPoolError::InvalidVersion,
    )]
    pub chain_config: Box<Account<'info, ChainConfig>>,

    /// CHECK: Optional FTF override PDA. May or may not be initialized; when initialized (owner
    /// == this program), its inbound rate limit bucket overrides the default one for
    /// faster-than-finality transfers.
    #[account(
        mut,
        seeds = [POOL_OVERRIDE_CHAINCONFIG_SEED, &release_or_mint.remote_chain_selector.to_le_bytes(), state.config.mint.as_ref(), PoolChainConfigOverrideType::FTF.as_bytes()],
        bump,
    )]
    pub chain_config_override: UncheckedAccount<'info>,

    /// CHECK: the account the offramp resolved/created as the message's real token destination
    #[account(
        address = if release_or_mint.receiver_usage.should_derive_ata() {
            get_associated_token_address_with_program_id(&release_or_mint.receiver, &mint.key(), &token_program.key())
        } else {
            release_or_mint.receiver
        },
    )]
    pub receiver_token_account: UncheckedAccount<'info>,
}

pub fn release_or_mint_tokens_v2(
    ctx: Context<TokenOfframpV2>,
    release_or_mint: ReleaseOrMintInV2,
) -> Result<ReleaseOrMintOutV2> {
    validate_source_pool_decimals(
        &release_or_mint.source_pool_data,
        ctx.accounts.chain_config.base.remote.decimals,
    )?;

    let parsed_amount = protocol_amount_to_svm_token_amount(
        release_or_mint.amount,
        ctx.accounts.chain_config.base.remote.decimals,
        ctx.accounts.state.config.decimals,
    )?;

    let BaseChain {
        remote,
        inbound_rate_limit,
        ..
    } = &mut ctx.accounts.chain_config.base;

    let ftf_account_info = &ctx.accounts.chain_config_override;
    let mut ftf_override = ChainConfigOverride::try_load(ftf_account_info)?;
    let ftf_inbound = ftf_override
        .as_mut()
        .map(|o| &mut o.base.inbound_rate_limit);

    validate_release_or_mint_v2(
        &release_or_mint,
        parsed_amount,
        ctx.accounts.state.config.mint,
        &remote.pool_addresses,
        inbound_rate_limit,
        ftf_inbound,
    )?;

    if let Some(override_config) = &ftf_override {
        let mut data = ftf_account_info.try_borrow_mut_data()?;
        override_config
            .try_serialize(&mut data.as_mut())
            .map_err(|_| CcipTokenPoolError::InvalidInputs)?;
    }

    emit_cpi!(ReleasedOrMinted {
        mint: ctx.accounts.state.config.mint,
        remote_chain_selector: release_or_mint.remote_chain_selector,
        sender: ctx.accounts.pool_signer.key(),
        recipient: release_or_mint.receiver,
        user_token_account: ctx.accounts.receiver_token_account.key(),
        amount: parsed_amount,
    });

    Ok(ReleaseOrMintOutV2 {
        destination_amount: parsed_amount,
    })
}
