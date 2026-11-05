use anchor_lang::prelude::*;

use anchor_spl::token_interface::Mint;
use base_token_pool::constants::POOL_STATE_SEED;
use base_token_pool::errors::CcipTokenPoolError;

use ccip_common::state::AccountSchemaVersion;

use crate::pool::state::State;

pub fn configure_allow_list(
    ctx: Context<AddToAllowList>,
    add: Vec<Pubkey>,
    enabled: bool,
) -> Result<()> {
    ctx.accounts.state.config.list_enabled = enabled;
    let list = &mut ctx.accounts.state.config.allow_list;
    for key in add {
        require!(
            !list.contains(&key),
            CcipTokenPoolError::AllowlistKeyAlreadyExisted
        );
        list.push(key);
    }

    Ok(())
}

pub fn remove_from_allow_list(
    ctx: Context<RemoveFromAllowlist>,
    remove: Vec<Pubkey>,
) -> Result<()> {
    let list = &mut ctx.accounts.state.config.allow_list;
    // Cache initial length
    let initial_list_len = list.len();
    // Collect all keys to remove into a HashSet for O(1) lookups
    let input_len = remove.len();
    let keys_to_remove: std::collections::HashSet<Pubkey> = remove.into_iter().collect();

    // Ensure that there were no duplicates (which would affect how the realloc math is done)
    require_eq!(
        input_len,
        keys_to_remove.len(),
        CcipTokenPoolError::DuplicatedKeys
    );

    // Perform a single pass through the list
    list.retain(|k| !keys_to_remove.contains(k));

    // We don't store repeated keys, so the keys_to_remove should match the removed keys
    require_eq!(
        initial_list_len,
        list.len().checked_add(keys_to_remove.len()).unwrap(),
        CcipTokenPoolError::AllowlistKeyDidNotExist
    );

    Ok(())
}

#[derive(Accounts)]
#[instruction(add: Vec<Pubkey>)]
pub struct AddToAllowList<'info> {
    #[account(
        mut,
        seeds = [
            POOL_STATE_SEED,
            mint.key().as_ref()
        ],
        bump,
        realloc = State::DISCRIMINATOR.len() + State::INIT_SPACE + 32 * (state.config.allow_list.len() + add.len()),
        realloc::payer = authority,
        realloc::zero = false,
        constraint = state.is_latest_version() @ CcipTokenPoolError::InvalidVersion,
    )]
    pub state: Account<'info, State>,

    pub mint: InterfaceAccount<'info, Mint>, // underlying token that the pool wraps

    #[account(mut, address = state.config.owner @ CcipTokenPoolError::Unauthorized)]
    pub authority: Signer<'info>,

    pub system_program: Program<'info, System>,
}

#[derive(Accounts)]
#[instruction(remove: Vec<Pubkey>)]
pub struct RemoveFromAllowlist<'info> {
    #[account(
        mut,
        seeds = [
            POOL_STATE_SEED,
            mint.key().as_ref()
        ],
        bump,
        constraint = state.is_latest_version() @ CcipTokenPoolError::InvalidVersion,
        realloc = State::DISCRIMINATOR.len() + State::INIT_SPACE + std::mem::size_of::<Pubkey>() * (state.config.allow_list.len().saturating_sub(remove.len())),
        realloc::payer = authority,
        realloc::zero = false,
    )]
    pub state: Account<'info, State>,

    pub mint: InterfaceAccount<'info, Mint>, // underlying token that the pool wraps

    #[account(mut, address = state.config.owner @ CcipTokenPoolError::Unauthorized)]
    pub authority: Signer<'info>,

    pub system_program: Program<'info, System>,
}
