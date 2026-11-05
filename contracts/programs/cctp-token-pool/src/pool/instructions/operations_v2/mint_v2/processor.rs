//! Unlike a "standard" base-token-pool v2 pool, this does NOT perform an independent SPL mint.
//! `message_transmitter::receive` (called from the verifier's `verify`) already performs the actual mint
//! as a side effect of consuming the CCTPv2 message, so this instruction fully trusts that
//! and only validates + consumes the inbound rate limit, matching EVM's `TokenPool.releaseOrMint` V2

use anchor_lang::prelude::*;

use anchor_spl::associated_token::get_associated_token_address_with_program_id;
use anchor_spl::token_interface::{Mint, TokenAccount, TokenInterface};

use base_token_pool::{constants::*, errors::*, events::*, helpers::*, state::*, v2::*};

use ccip_common::state::{AccountSchemaVersion, PoolChainConfigOverrideType};

use crate::pool::instructions::CCTP_VERSION_2_CCV_TAG;
use crate::pool::state::*;
use crate::utils::DisplaySlice;

#[event_cpi]
#[derive(Accounts)]
#[instruction(release_or_mint: ReleaseOrMintInV2)]
pub struct TokenOfframpV2<'info> {
    // CCIP accounts ------------------------
    #[account(
        seeds = [EXTERNAL_TOKEN_POOLS_SIGNER, crate::ID.as_ref()],
        bump,
        seeds::program = offramp_program.key(),
    )]
    pub authority: Signer<'info>,

    /// CHECK offramp program: exists only to derive the allowed offramp PDA
    /// and the authority PDA.
    pub offramp_program: UncheckedAccount<'info>,

    /// CHECK PDA of the router program verifying the signer is an allowed offramp.
    /// If PDA does not exist, the router doesn't allow this offramp
    #[account(
        owner = state.config.router @ CcipTokenPoolError::InvalidPoolCaller,
        seeds = [
            ALLOWED_OFFRAMP,
            release_or_mint.remote_chain_selector.to_le_bytes().as_ref(),
            offramp_program.key().as_ref()
        ],
        bump,
        seeds::program = state.config.router,
    )]
    pub allowed_offramp: UncheckedAccount<'info>,

    // Token pool accounts ------------------
    #[account(
        seeds = [POOL_STATE_SEED, mint.key().as_ref()],
        bump,
        constraint = state.is_latest_version() @ CcipTokenPoolError::InvalidVersion,
    )]
    pub state: Box<Account<'info, State>>,

    pub token_program: Interface<'info, TokenInterface>,

    #[account(
        mut,
        address = release_or_mint.local_token,
        owner = token_program.key(),
    )]
    pub mint: InterfaceAccount<'info, Mint>,

    #[account(
        seeds = [POOL_SIGNER_SEED, mint.key().as_ref()],
        bump,
        address = state.config.pool_signer,
    )]
    /// CHECK: unchecked CPI signer
    pub pool_signer: UncheckedAccount<'info>,

    #[account(
        mut,
        associated_token::mint = mint,
        associated_token::authority = pool_signer,
        associated_token::token_program = token_program,
    )]
    pub pool_token_account: Box<InterfaceAccount<'info, TokenAccount>>,

    #[account(
        mut,
        seeds = [POOL_CHAINCONFIG_SEED, release_or_mint.remote_chain_selector.to_le_bytes().as_ref(), mint.key().as_ref()],
        bump,
        constraint = chain_config.is_latest_version() @ CcipTokenPoolError::InvalidVersion,
    )]
    pub chain_config: Box<Account<'info, ChainConfig>>,

    /// CHECK: Optional FTF override PDA. May or may not be initialized.
    /// When initialized (owner == this program), its rate limit buckets
    /// override the default ones for faster-than-finality transfers.
    #[account(
        mut,
        seeds = [POOL_OVERRIDE_CHAINCONFIG_SEED, release_or_mint.remote_chain_selector.to_le_bytes().as_ref(), mint.key().as_ref(), PoolChainConfigOverrideType::FTF.as_bytes()],
        bump,
    )]
    pub chain_config_override: UncheckedAccount<'info>,

    // User specific accounts ---------------
    #[account(
        mut,
        address = if release_or_mint.receiver_usage.should_derive_ata() {
            get_associated_token_address_with_program_id(&release_or_mint.receiver, &mint.key(), &token_program.key())
        } else {
            release_or_mint.receiver
        },
        owner = token_program.key(),
        // the SPL token programs both check for correct mint when processing the mint_to and transfer instructions,
        // so we don't need to check that here
    )]
    pub receiver_token_account: Box<InterfaceAccount<'info, TokenAccount>>,
}

pub fn mint_tokens_v2<'info>(
    ctx: Context<'info, TokenOfframpV2<'info>>,
    release_or_mint: ReleaseOrMintInV2,
) -> Result<ReleaseOrMintOutV2> {
    require_eq!(
        DisplaySlice(release_or_mint.source_pool_data.as_slice()),
        DisplaySlice(CCTP_VERSION_2_CCV_TAG.as_slice()),
        CcipTokenPoolError::InvalidVersion,
    );

    // Rate-limit bucket selection logic for override types:
    // - FTF (Faster Than Finality): uses override's inbound_rate_limit if initialized
    // When adding new override types, extend this section to check for and use their buckets.
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

    // Try to load FTF override config if the account is initialized
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

    // Persist FTF override changes if it was loaded
    if let Some(override_config) = &ftf_override {
        let mut data = ftf_account_info.try_borrow_mut_data()?;
        override_config
            .try_serialize(&mut data.as_mut())
            .map_err(|_| CcipTokenPoolError::InvalidInputs)?;
    }

    emit_cpi!(ReleasedOrMinted {
        mint: ctx.accounts.mint.key(),
        remote_chain_selector: release_or_mint.remote_chain_selector,
        sender: ctx.accounts.pool_signer.key(),
        recipient: release_or_mint.receiver,
        user_token_account: ctx.accounts.receiver_token_account.key(),
        // The CCTP verifier will mint some of the amount to the fee recipient and some to the receiver.
        // This event simply declares that the full amount has been minted, with no awareness of the split.
        // This token pool can't be aware of the split without requiring the CCTP verifier to write feeExecuted to storage every tx.
        // Additionally, the LockedOrBurned event on source includes the full amount, providing parity for offchain consumers.
        amount: parsed_amount,
    });

    Ok(ReleaseOrMintOutV2 {
        destination_amount: parsed_amount,
    })
}
