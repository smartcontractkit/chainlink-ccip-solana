use anchor_lang::prelude::*;

use ccip_common::resolution::*;
use ccip_common::seed;

use super::super::resolve_accounts_start::NestedResolveAccountsStart;

/// First (and only) stage of account resolution for the `verify` instruction.
pub fn start(
    ctx: Context<NestedResolveAccountsStart>,
    source_chain_selector: u64,
) -> Result<NestedResolveAccountsResponse> {
    let remote_chain_pda = find(
        &[seed::REMOTE_CHAIN, &source_chain_selector.to_le_bytes()],
        crate::ID,
    );

    Ok(NestedResolveAccountsResponse {
        ask_again_with: vec![],
        accounts_to_save: vec![
            ctx.accounts.config.key().readonly(),
            remote_chain_pda.readonly(),
        ],
        lookup_tables_to_save: vec![],
        next_cpi_resolution_inputs: vec![],
    })
}
