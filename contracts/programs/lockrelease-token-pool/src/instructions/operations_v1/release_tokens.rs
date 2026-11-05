use anchor_lang::prelude::*;
use anchor_lang::solana_program::program::invoke_signed;
use anchor_spl::{
    associated_token::get_associated_token_address_with_program_id,
    token_2022::spl_token_2022::{self, instruction::transfer_checked},
    token_interface::{Mint, TokenAccount, TokenInterface},
};
use base_token_pool::{constants::*, errors::*, events::*, helpers::*, state::*, v1::*};
use ccip_common::seed;

use crate::state::*;
use crate::{ChainConfig, State};

#[derive(Accounts)]
#[instruction(release_or_mint: ReleaseOrMintInV1)]
pub struct TokenOfframp<'info> {
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
        owner = state.config.router @ CcipTokenPoolError::InvalidPoolCaller, // this guarantees that it was initialized
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
    // consistent set + token pool program
    #[account(
        seeds = [POOL_STATE_SEED, mint.key().as_ref()],
        bump,
        constraint = valid_version(state.version, MAX_POOL_STATE_V) @ CcipTokenPoolError::InvalidVersion,
    )]
    pub state: Account<'info, State>,

    pub token_program: Interface<'info, TokenInterface>,

    #[account(
        mut,
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
        dup,
        associated_token::mint = mint,
        associated_token::authority = pool_signer,
        associated_token::token_program = token_program,
    )]
    pub pool_token_account: Box<InterfaceAccount<'info, TokenAccount>>,

    #[account(
        mut,
        seeds = [POOL_CHAINCONFIG_SEED, release_or_mint.remote_chain_selector.to_le_bytes().as_ref(), mint.key().as_ref()],
        bump,
    )]
    pub chain_config: Account<'info, ChainConfig>,

    ////////////////////
    // RMN Remote CPI //
    ////////////////////
    /// CHECK: This is the account for the RMN Remote program
    #[account(
        address = state.config.rmn_remote @ CcipTokenPoolError::InvalidRMNRemoteAddress,
    )]
    pub rmn_remote: UncheckedAccount<'info>,

    /// CHECK: This account is just used in the CPI to the RMN Remote program
    #[account(
        seeds = [seed::CURSES],
        bump,
        seeds::program = state.config.rmn_remote,
    )]
    pub rmn_remote_curses: UncheckedAccount<'info>,

    /// CHECK: This account is just used in the CPI to the RMN Remote program
    #[account(
        seeds = [seed::CONFIG],
        bump,
        seeds::program = state.config.rmn_remote,
    )]
    pub rmn_remote_config: UncheckedAccount<'info>,

    // User specific accounts ---------------
    #[account(mut, address = get_associated_token_address_with_program_id(&release_or_mint.receiver, &mint.key(), &token_program.key()))]
    pub receiver_token_account: Box<InterfaceAccount<'info, TokenAccount>>,
}

pub fn release_tokens_v1(
    ctx: Context<TokenOfframp>,
    release_or_mint: ReleaseOrMintInV1,
) -> Result<ReleaseOrMintOutV1> {
    let parsed_amount = cross_chain_amount_to_svm_token_amount(
        CrossChainAmount::from_le_bytes(release_or_mint.amount),
        ctx.accounts.chain_config.base.remote.decimals,
        ctx.accounts.state.config.decimals,
    )?;

    let BaseChain {
        remote,
        inbound_rate_limit,
        ..
    } = &mut ctx.accounts.chain_config.base;

    validate_release_or_mint(
        &release_or_mint,
        parsed_amount,
        ctx.accounts.state.config.mint,
        &remote.pool_addresses,
        inbound_rate_limit,
    )?;

    release_tokens(
        ctx.accounts.token_program.key(),
        ctx.accounts.receiver_token_account.to_account_info(),
        ctx.accounts.pool_token_account.to_account_info(),
        ctx.accounts.mint.to_account_info(),
        ctx.accounts.pool_signer.to_account_info(),
        ctx.bumps.pool_signer,
        release_or_mint,
        parsed_amount,
        ctx.accounts.mint.decimals,
    )?;

    Ok(ReleaseOrMintOutV1 {
        destination_amount: parsed_amount,
    })
}

#[allow(clippy::too_many_arguments)]
pub fn release_tokens<'a>(
    token_program: Pubkey,
    receiver_token_account: AccountInfo<'a>,
    pool_token_account: AccountInfo<'a>,
    mint: AccountInfo<'a>,
    pool_signer: AccountInfo<'a>,
    pool_signer_bump: u8,
    release_or_mint: ReleaseOrMintInV1,
    parsed_amount: u64,
    decimals: u8,
) -> Result<()> {
    // transfer from pool -> receiver
    transfer_tokens(
        token_program,
        receiver_token_account,
        pool_token_account,
        mint.clone(),
        pool_signer.clone(),
        pool_signer_bump,
        parsed_amount,
        decimals,
    )?;

    emit!(Released {
        sender: pool_signer.key(),
        recipient: release_or_mint.receiver,
        amount: parsed_amount,
        mint: mint.key(),
    });

    Ok(())
}

#[allow(clippy::too_many_arguments)]
pub fn transfer_tokens<'a>(
    token_program: Pubkey,
    to: AccountInfo<'a>,
    from: AccountInfo<'a>,
    mint: AccountInfo<'a>,
    pool_signer: AccountInfo<'a>,
    pool_signer_bump: u8,
    parsed_amount: u64,
    decimals: u8,
) -> Result<()> {
    // https://docs.rs/spl-token-2022/latest/spl_token_2022/instruction/fn.transfer.html
    let mut ix = transfer_checked(
        &spl_token_2022::ID, // use spl-token-2022 to compile instruction - change program later
        &from.key(),
        &mint.key(),
        &to.key(),
        &pool_signer.key(),
        &[],
        parsed_amount,
        decimals,
    )?;
    ix.program_id = token_program; // set to user specified program

    let seeds = &[
        POOL_SIGNER_SEED,
        &mint.key().to_bytes(),
        &[pool_signer_bump],
    ];
    invoke_signed(&ix, &[from, mint, to, pool_signer.clone()], &[&seeds[..]]).map_err(|e| e.into())
}
