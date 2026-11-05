use anchor_lang::prelude::*;

use ccip_common::resolution::*;
use ccip_common::seed;

/// First (and only) stage of account resolution for the `release_or_mint_tokens_v2` instruction.
pub fn start() -> Result<NestedResolveAccountsResponse> {
    Ok(NestedResolveAccountsResponse {
        ask_again_with: vec![],
        accounts_to_save: vec![
            // These could've easily been derived by the offramp resolver, but we leave them here to
            // prove nested resolution. In the case of a real third party token pool, they won't be
            // mandatory anyway as they depend on whether the pool emits CPI events.
            find(&[seed::EVENT_AUTHORITY], crate::ID).readonly(),
            crate::ID.readonly(),
        ],
        lookup_tables_to_save: vec![],
        next_cpi_resolution_inputs: vec![],
    })
}
