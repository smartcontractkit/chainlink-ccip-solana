use anchor_lang::{prelude::*, solana_program::program::invoke_signed};
use anchor_spl::{
    associated_token::get_associated_token_address_with_program_id,
    token_2022::spl_token_2022::{self, instruction::burn},
    token_interface::{Mint, TokenAccount, TokenInterface},
};
use base_token_pool::{constants::*, errors::*, events::*, v1::*};
use ccip_common::seed;

use crate::state::*;
use crate::{ChainConfig, State};

#[derive(Accounts)]
#[instruction(lock_or_burn: LockOrBurnInV1)]
pub struct TokenOnramp<'info> {
    // CCIP accounts ------------------------
    #[account(address = state.config.router_onramp_authority @ CcipTokenPoolError::InvalidPoolCaller)]
    pub authority: Signer<'info>,

    // Token pool accounts ------------------
    // consistent set + token pool program
    #[account(
        seeds = [POOL_STATE_SEED, mint.key().as_ref()],
        bump,
        constraint = valid_version(state.version, MAX_POOL_STATE_V) @ CcipTokenPoolError::InvalidVersion,
    )]
    pub state: Account<'info, State>,
    #[account(address = *mint.to_account_info().owner)]
    pub token_program: Interface<'info, TokenInterface>,
    #[account(mut)]
    pub mint: InterfaceAccount<'info, Mint>,
    #[account(
        seeds = [POOL_SIGNER_SEED, mint.key().as_ref()],
        bump,
        address = state.config.pool_signer,
    )]
    /// CHECK: unchecked CPI signer
    pub pool_signer: UncheckedAccount<'info>,
    #[account(mut, address = get_associated_token_address_with_program_id(&pool_signer.key(), &mint.key(), &token_program.key()))]
    pub pool_token_account: InterfaceAccount<'info, TokenAccount>,
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

    #[account(
        mut,
        seeds = [POOL_CHAINCONFIG_SEED, lock_or_burn.remote_chain_selector.to_le_bytes().as_ref(), mint.key().as_ref()],
        bump,
    )]
    pub chain_config: Account<'info, ChainConfig>,
}

pub fn burn_tokens<'a>(
    token_program: Pubkey,
    pool_token_account: AccountInfo<'a>,
    mint: AccountInfo<'a>,
    pool_signer: AccountInfo<'a>,
    pool_signer_bump: u8,
    sender: Pubkey,
    lock_or_burn: LockOrBurnInV1,
) -> Result<()> {
    // receiver -> token pool (occurs outside pool)
    // burn tokens held in pool
    // https://docs.rs/spl-token-2022/latest/spl_token_2022/instruction/fn.burn.html
    let mut ix = burn(
        &spl_token_2022::ID, // use spl-token-2022 to compile instruction - change program later
        &pool_token_account.key(),
        &mint.key(),
        &pool_signer.key(),
        &[],
        lock_or_burn.amount,
    )?;
    ix.program_id = token_program; // set to user specified program

    let seeds = &[
        POOL_SIGNER_SEED,
        &mint.key().to_bytes(),
        &[pool_signer_bump],
    ];
    invoke_signed(
        &ix,
        &[pool_token_account, mint.clone(), pool_signer],
        &[&seeds[..]],
    )?;

    emit!(Burned {
        sender,
        amount: lock_or_burn.amount,
        mint: mint.key(),
    });

    Ok(())
}

// ================================================================================================
// WARNING: DO NOT MODIFY THIS FUNCTION, ITS SIGNATURE, OR THE TokenOnramp ACCOUNT CONTEXT.
// Any change to the instruction discriminator, account list, ordering, or behavior will break
// backwards compatibility with existing on-chain deployments and all offchain integrations that
// depend on the stable IDL of this instruction. Treat this as a frozen interface.
// ================================================================================================
pub fn burn_tokens_v1(
    ctx: Context<TokenOnramp>,
    lock_or_burn: LockOrBurnInV1,
) -> Result<LockOrBurnOutV1> {
    validate_lock_or_burn(
        &lock_or_burn,
        ctx.accounts.state.config.mint,
        &mut ctx.accounts.chain_config.base.outbound_rate_limit,
        ctx.accounts.state.config.list_enabled,
        &ctx.accounts.state.config.allow_list,
    )?;

    burn_tokens(
        ctx.accounts.token_program.key(),
        ctx.accounts.pool_token_account.to_account_info(),
        ctx.accounts.mint.to_account_info(),
        ctx.accounts.pool_signer.to_account_info(),
        ctx.bumps.pool_signer,
        ctx.accounts.authority.key(),
        lock_or_burn,
    )?;

    Ok(LockOrBurnOutV1 {
        dest_token_address: ctx.accounts.chain_config.base.remote.token_address.clone(),
        dest_pool_data: {
            let mut abi_encoded_decimals = vec![0u8; 32];
            abi_encoded_decimals[31] = ctx.accounts.state.config.decimals;
            abi_encoded_decimals
        },
    })
}
