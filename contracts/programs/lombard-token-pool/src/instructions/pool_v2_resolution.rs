//! Nested (CPI) account resolution for the pool-v2 operations

use anchor_lang::prelude::*;
use base_token_pool::v2::{pool_query_accounts, LockOrBurnInV2};
use ccip_common::resolution::*;
use ccip_common::seed;

use crate::state::State;

pub fn lock_or_burn_start(
    _lock_or_burn_in: &LockOrBurnInV2,
) -> Result<NestedResolveAccountsResponse> {
    Ok(NestedResolveAccountsResponse {
        ask_again_with: vec![],
        accounts_to_save: vec![
            find(&[seed::EVENT_AUTHORITY], crate::ID).readonly(),
            crate::ID.readonly(),
        ],
        lookup_tables_to_save: vec![],
        next_cpi_resolution_inputs: vec![],
    })
}

pub fn release_or_mint_start(_state_key: Pubkey) -> Result<NestedResolveAccountsResponse> {
    Ok(NestedResolveAccountsResponse {
        ask_again_with: vec![],
        accounts_to_save: vec![
            find(&[seed::EVENT_AUTHORITY], crate::ID).readonly(),
            crate::ID.readonly(),
        ],
        lookup_tables_to_save: vec![],
        next_cpi_resolution_inputs: vec![],
    })
}

pub fn get_required_ccvs_start(
    state_key: Pubkey,
    state: &State,
    remote_chain_selector: u64,
) -> Result<NestedResolveAccountsResponse> {
    query_start(state_key, state, remote_chain_selector)
}

fn query_start(
    state_key: Pubkey,
    state: &State,
    remote_chain_selector: u64,
) -> Result<NestedResolveAccountsResponse> {
    Ok(NestedResolveAccountsResponse {
        ask_again_with: vec![],
        accounts_to_save: pool_query_accounts(
            crate::ID,
            state_key,
            &state.config.mint,
            remote_chain_selector,
        ),
        lookup_tables_to_save: vec![],
        next_cpi_resolution_inputs: vec![],
    })
}
