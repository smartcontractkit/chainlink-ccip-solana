use anchor_lang::prelude::*;

use ccip_common::resolution::*;
use ccip_common::seed;

use super::super::resolve_accounts_start::ResolveAccountsStart;

/// First (and only) stage of account resolution for the `get_storage_locations` instruction.
pub fn start(
    ctx: Context<ResolveAccountsStart>,
    _params: ResolveAccountsParams,
) -> Result<ResolveAccountsResponse> {
    Ok(ResolveAccountsResponse {
        ask_again_with: vec![],
        accounts_to_save: vec![
            ctx.accounts.config.key().readonly(),
            find(&[seed::STORAGE_LOCATIONS], crate::ID).readonly(),
        ],
        lookup_tables_to_save: vec![],
        next_ix_discriminator: None,
        metadata: vec![],
    })
}
