use anchor_lang::solana_program::program_pack::Pack;
use anchor_lang::{prelude::*, solana_program::program::invoke_signed};
use anchor_spl::{
    token::spl_token::{self},
    token_2022::spl_token_2022::{self},
    token_interface::{Mint, TokenInterface},
};
use base_token_pool::{constants::*, errors::*, events::*};
use ccip_common::multisig::MultisigAccount;

use crate::program::BurnmintTokenPool;
use crate::State;
use crate::{error::CcipBnMTokenPoolError, state::*};

#[derive(Accounts)]
pub struct TransferMintAuthority<'info> {
    #[account(
        mut,
        seeds = [POOL_STATE_SEED, mint.key().as_ref()],
        bump,
        constraint = valid_version(state.version, MAX_POOL_STATE_V) @ CcipTokenPoolError::InvalidVersion,
    )]
    pub state: Account<'info, State>,
    #[account(mut, owner = token_program.key())]
    pub mint: InterfaceAccount<'info, Mint>, // underlying token that the pool wraps
    pub token_program: Interface<'info, TokenInterface>,
    #[account(
        seeds = [POOL_SIGNER_SEED, mint.key().as_ref()],
        bump,
        address = state.config.pool_signer,
    )]
    /// CHECK: unchecked CPI signer
    pub pool_signer: UncheckedAccount<'info>,
    pub authority: Signer<'info>,

    /// CHECK: The Multisig can be a Token 2022 Multisig or a SPL Token Multisig and there is no interface for it.
    #[account(owner = *mint.to_account_info().owner @ CcipBnMTokenPoolError::InvalidMultisigOwner)]
    pub new_multisig_mint_authority: UncheckedAccount<'info>, // new mint authority for the underlying token

    // Ensures that the provided program is the BurnmintTokenPool program,
    // and that its associated program data account matches the expected one.
    // This guarantees that only the program's upgrade authority can modify the global config.
    #[account(constraint = program.programdata_address()? == Some(program_data.key()))]
    pub program: Program<'info, BurnmintTokenPool>,
    // Transfer mint authority only allowed by program upgrade authority as it is a critical operation.
    #[account(constraint = program_data.upgrade_authority_address == Some(authority.key()) @ CcipTokenPoolError::Unauthorized)]
    pub program_data: Account<'info, ProgramData>,
}

#[derive(Accounts)]
pub struct TransferMintAuthorityToPdaSigner<'info> {
    #[account(
        seeds = [POOL_STATE_SEED, mint.key().as_ref()],
        bump,
        constraint = valid_version(state.version, MAX_POOL_STATE_V) @ CcipTokenPoolError::InvalidVersion,
    )]
    pub state: Account<'info, State>,
    #[account(
        mut,
        constraint = mint.mint_authority.is_some() @ CcipBnMTokenPoolError::FixedMintToken,
        owner = token_program.key()
    )]
    pub mint: InterfaceAccount<'info, Mint>, // underlying token that the pool wraps
    pub token_program: Interface<'info, TokenInterface>,
    #[account(
        seeds = [POOL_SIGNER_SEED, mint.key().as_ref()],
        bump,
        address = state.config.pool_signer,
    )]
    /// CHECK: unchecked CPI signer — this is the new mint authority
    pub pool_signer: UncheckedAccount<'info>,
    pub authority: Signer<'info>,
    /// CHECK: validated via constraint — must match the current mint authority (multisig) and not be the pool signer
    #[account(
        constraint = valid_current_mint_authority(&mint, &current_mint_authority_account, &pool_signer) @ CcipBnMTokenPoolError::InvalidMultisig,
    )]
    pub current_mint_authority_account: UncheckedAccount<'info>,

    // Ensures that the provided program is the BurnmintTokenPool program,
    // and that its associated program data account matches the expected one.
    // This guarantees that only the program's upgrade authority can invoke this method
    #[account(constraint = program.programdata_address()? == Some(program_data.key()))]
    pub program: Program<'info, BurnmintTokenPool>,
    // Transfer mint authority only allowed by program upgrade authority as it is a critical operation.
    #[account(constraint = program_data.upgrade_authority_address == Some(authority.key()) @ CcipTokenPoolError::Unauthorized)]
    pub program_data: Account<'info, ProgramData>,
}

pub fn validate_multisig_config(n: usize, m: usize, signer_count: usize) -> Result<()> {
    // Multisig must have at least two signers
    require_gte!(
        n,
        2,
        CcipBnMTokenPoolError::MultisigMustHaveAtLeastTwoSigners
    );

    // Threshold must be at least one
    require_gte!(
        m,
        1,
        CcipBnMTokenPoolError::MultisigMustHaveMoreThanOneSigner
    );

    // Threshold must not exceed the total number of signers
    require_gte!(n, m, CcipBnMTokenPoolError::InvalidMultisigThreshold);

    // Pool signer must appear at least m times to satisfy the threshold on its own
    require_gte!(
        signer_count,
        m,
        CcipBnMTokenPoolError::PoolSignerNotInMultisig
    );

    // Pool signer must not appear more than (n - m) times
    require_gte!(
        n - m,
        signer_count,
        CcipBnMTokenPoolError::InvalidMultisigThresholdTooHigh
    );

    Ok(())
}

// This method transfers the mint authority of the mint, so it does a CPI to the Token Program.
// It is only defined in the burn and mint program as the mint authority is only used for minting tokens.
pub fn transfer_mint_authority_to_multisig<'info>(
    ctx: Context<'info, TransferMintAuthority<'info>>,
) -> Result<()> {
    let mint = &ctx.accounts.mint;

    let old_mint_authority = mint
        .mint_authority
        .ok_or(CcipBnMTokenPoolError::FixedMintToken)?;

    let new_mint_authority = ctx.accounts.new_multisig_mint_authority.key();

    require_keys_neq!(
        old_mint_authority,
        new_mint_authority,
        CcipBnMTokenPoolError::MintAuthorityAlreadySet
    );

    // The new Mint Authority must be a multisig account that contains the pool signer as one of its signers.
    let token_program_id = &ctx.accounts.state.config.token_program.key();

    let multisig_data = &mut &ctx.accounts.new_multisig_mint_authority.data.borrow()[..];
    // then check that the multisig account is a valid multisig account
    let multisig_account: MultisigAccount = if token_program_id == &spl_token_2022::ID {
        spl_token_2022::state::Multisig::unpack_from_slice(multisig_data)
            .map_err(|_| CcipBnMTokenPoolError::InvalidToken2022Multisig)?
            .into()
    } else if token_program_id == &spl_token::ID {
        spl_token::state::Multisig::unpack_from_slice(multisig_data)
            .map_err(|_| CcipBnMTokenPoolError::InvalidSPLTokenMultisig)?
            .into()
    } else {
        // Reject any unsupported token programs
        return Err(CcipBnMTokenPoolError::UnsupportedTokenProgram.into());
    };

    // `Multisig::unpack_from_slice` reads `n` as a raw byte and never checks it against
    // MAX_SIGNERS, so a malformed account can claim more signers than the array can hold.
    require!(
        multisig_account.is_len_valid(),
        CcipBnMTokenPoolError::InvalidMultisig
    );

    let n = multisig_account.len();
    let m = multisig_account.m as usize;

    let signer_count = multisig_account
        .signers()
        .iter()
        .filter(|s| *s == &ctx.accounts.pool_signer.key())
        .count();

    validate_multisig_config(n, m, signer_count)?;

    // Transfer the mint authority to the new mint authority using the corresponding Token Program. It can be token 22 or token SPL
    let ix = spl_token_2022::instruction::set_authority(
        token_program_id,
        &ctx.accounts.mint.key(),
        Some(&new_mint_authority),
        spl_token_2022::instruction::AuthorityType::MintTokens,
        &old_mint_authority,
        &[ctx.accounts.pool_signer.key],
    )?;

    let seeds = &[
        POOL_SIGNER_SEED,
        &ctx.accounts.mint.key().to_bytes(),
        &[ctx.bumps.pool_signer],
    ];

    let account_infos: Vec<AccountInfo> = if old_mint_authority != ctx.accounts.pool_signer.key() {
        // If the old mint authority is not the pool signer, we need to include the multisig account in the instruction
        let multisig_signer_account = ctx
            .remaining_accounts
            .iter()
            .find(|acc| acc.key() == old_mint_authority)
            .ok_or(CcipBnMTokenPoolError::InvalidMultisig)?;
        vec![
            ctx.accounts.mint.to_account_info(),
            multisig_signer_account.to_account_info(),
            ctx.accounts.pool_signer.to_account_info(),
        ]
    } else {
        vec![
            ctx.accounts.mint.to_account_info(),
            ctx.accounts.pool_signer.to_account_info(),
        ]
    };

    invoke_signed(&ix, &account_infos, &[&seeds[..]])?;

    emit!(MintAuthorityTransferred {
        mint: ctx.accounts.mint.key(),
        old_mint_authority,
        new_mint_authority,
    });

    Ok(())
}

/// Checks that the current mint authority account matches the mint's authority and is not the pool signer.
pub fn valid_current_mint_authority(
    mint: &InterfaceAccount<Mint>,
    current_mint_authority_account: &UncheckedAccount,
    pool_signer: &UncheckedAccount,
) -> bool {
    mint.mint_authority == Some(current_mint_authority_account.key()).into()
        && mint.mint_authority != Some(pool_signer.key()).into()
}

// This method transfers the mint authority back to the pool signer PDA.
// Used during migration cleanup to revert from a multisig mint authority back to the pool signer.
pub fn transfer_mint_authority_to_pda_signer<'info>(
    ctx: Context<'info, TransferMintAuthorityToPdaSigner<'info>>,
) -> Result<()> {
    let old_mint_authority = ctx.accounts.current_mint_authority_account.key();
    let new_mint_authority = ctx.accounts.pool_signer.key();

    let token_program_id = &ctx.accounts.state.config.token_program.key();

    // Transfer the mint authority to the new mint authority using the corresponding Token Program. It can be token 22 or token SPL
    let ix = spl_token_2022::instruction::set_authority(
        token_program_id,
        &ctx.accounts.mint.key(),
        Some(&new_mint_authority),
        spl_token_2022::instruction::AuthorityType::MintTokens,
        &old_mint_authority,
        &[ctx.accounts.pool_signer.key],
    )?;

    let seeds = &[
        POOL_SIGNER_SEED,
        &ctx.accounts.mint.key().to_bytes(),
        &[ctx.bumps.pool_signer],
    ];

    let account_infos = vec![
        ctx.accounts.mint.to_account_info(),
        ctx.accounts
            .current_mint_authority_account
            .to_account_info(),
        ctx.accounts.pool_signer.to_account_info(),
    ];

    invoke_signed(&ix, &account_infos, &[&seeds[..]])?;

    emit!(MintAuthorityTransferred {
        mint: ctx.accounts.mint.key(),
        old_mint_authority,
        new_mint_authority,
    });

    Ok(())
}
