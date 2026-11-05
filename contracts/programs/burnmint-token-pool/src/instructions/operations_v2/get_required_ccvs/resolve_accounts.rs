use anchor_lang::prelude::*;
use base_token_pool::v2::pool_query_accounts;
use ccip_common::ccvs::GetRequiredCCVsIn;
use ccip_common::resolution::*;

use super::super::resolve_accounts_start::NestedResolveAccountsStart;

/// First (and only) stage of account resolution for the `get_required_ccvs` instruction.
pub fn start(
    ctx: Context<NestedResolveAccountsStart>,
    params: &GetRequiredCCVsIn,
) -> Result<NestedResolveAccountsResponse> {
    Ok(NestedResolveAccountsResponse {
        ask_again_with: vec![],
        accounts_to_save: pool_query_accounts(
            crate::ID,
            ctx.accounts.state.key(),
            &ctx.accounts.state.config.mint,
            params.remote_chain_selector,
        ),
        lookup_tables_to_save: vec![],
        next_cpi_resolution_inputs: vec![],
    })
}
