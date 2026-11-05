use anchor_lang::{prelude::*, solana_program::program::invoke_signed};
use anchor_spl::{
    token_2022::spl_token_2022::{self, instruction::mint_to},
    token_interface::{Mint, TokenAccount, TokenInterface},
};
use base_token_pool::{constants::*, errors::*, events::*, helpers::*, state::*, v1::*};
use ccip_common::seed;

use crate::{error::CcipBnMTokenPoolError, state::*};
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
    #[account(
        mut,
        associated_token::mint = mint,
        associated_token::authority = release_or_mint.receiver,
        associated_token::token_program = token_program
    )]
    pub receiver_token_account: Box<InterfaceAccount<'info, TokenAccount>>,
}

pub struct MintTokenAccountsInfo<'a, 'b> {
    pub receiver_token_account: &'a AccountInfo<'b>,
    pub mint: &'a AccountInfo<'b>,
    pub pool_signer: &'a AccountInfo<'b>,
    pub pool_signer_bump: u8,
    pub multisig: Option<&'a AccountInfo<'b>>,
}

pub fn mint_tokens(
    token_program: Pubkey,
    release_or_mint: ReleaseOrMintInV1,
    parsed_amount: u64,
    accounts: MintTokenAccountsInfo,
) -> Result<()> {
    // mint to receiver
    // https://docs.rs/spl-token-2022/latest/spl_token_2022/instruction/fn.mint_to.html
    let mut ix = mint_to(
        &spl_token_2022::ID, // use spl-token-2022 to compile instruction - change program later
        &accounts.mint.key(),
        &accounts.receiver_token_account.key(),
        &accounts.multisig.unwrap_or(accounts.pool_signer).key(),
        &[accounts.pool_signer.key],
        parsed_amount,
    )?;
    ix.program_id = token_program; // set to user specified program

    let seeds = &[
        POOL_SIGNER_SEED,
        &accounts.mint.key().to_bytes(),
        &[accounts.pool_signer_bump],
    ];

    let account_infos: Vec<AccountInfo> = if let Some(multisig) = accounts.multisig {
        vec![
            accounts.receiver_token_account.clone(),
            accounts.mint.clone(),
            multisig.clone(),
            accounts.pool_signer.clone(),
        ]
    } else {
        vec![
            accounts.receiver_token_account.clone(),
            accounts.mint.clone(),
            accounts.pool_signer.clone(),
        ]
    };

    invoke_signed(&ix, &account_infos, &[&seeds[..]])?;

    emit!(Minted {
        sender: accounts.pool_signer.key(),
        recipient: release_or_mint.receiver,
        amount: parsed_amount,
        mint: accounts.mint.key(),
    });

    Ok(())
}

// ================================================================================================
// WARNING: DO NOT MODIFY THIS FUNCTION, ITS SIGNATURE, OR THE TokenOfframp ACCOUNT CONTEXT.
// Any change to the instruction discriminator, account list, ordering, or behavior will break
// backwards compatibility with existing on-chain deployments and all offchain integrations that
// depend on the stable IDL of this instruction. Treat this as a frozen interface.
// ================================================================================================
pub fn mint_tokens_v1<'info>(
    ctx: Context<'info, TokenOfframp<'info>>,
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

    // For a burnmint pool, the mint authority should never be empty (that would mean a fixed supply and prevent minting)
    // If not set, then the mint operation will fail
    require!(
        ctx.accounts.mint.mint_authority.is_some(),
        CcipBnMTokenPoolError::InvalidMultisig
    );

    let mint_authority = ctx.accounts.mint.mint_authority.unwrap();

    let multisig = if mint_authority != ctx.accounts.pool_signer.key() {
        let multisig_account = ctx
            .remaining_accounts
            .iter()
            .find(|acc| acc.key() == mint_authority)
            .ok_or(CcipBnMTokenPoolError::InvalidMultisig)?;

        Some(multisig_account)
    } else {
        None
    };

    mint_tokens(
        ctx.accounts.token_program.key(),
        release_or_mint,
        parsed_amount,
        MintTokenAccountsInfo {
            receiver_token_account: &ctx.accounts.receiver_token_account.to_account_info(),
            mint: &ctx.accounts.mint.to_account_info(),
            pool_signer: &ctx.accounts.pool_signer.to_account_info(),
            pool_signer_bump: ctx.bumps.pool_signer,
            multisig,
        },
    )?;

    Ok(ReleaseOrMintOutV1 {
        destination_amount: parsed_amount,
    })
}
