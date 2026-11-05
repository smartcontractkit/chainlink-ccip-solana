use anchor_lang::prelude::*;

use ccip_common::resolution::*;
use ccip_common::seed;

use super::super::resolve_accounts_start::ResolveAccountsStart;

/// First (and only) stage of account resolution for the `set_storage_locations` instruction.
pub fn start(
    ctx: Context<ResolveAccountsStart>,
    params: ResolveAccountsParams,
) -> Result<ResolveAccountsResponse> {
    let storage_locations_pda = find(&[seed::STORAGE_LOCATIONS], crate::ID);

    Ok(ResolveAccountsResponse {
        ask_again_with: vec![],
        accounts_to_save: vec![
            ctx.accounts.config.key().readonly(),
            params.caller.writable().signer(),
            storage_locations_pda.writable(),
            system_program::ID.readonly(),
            find(&[seed::EVENT_AUTHORITY], crate::ID).readonly(),
            crate::ID.readonly(),
        ],
        lookup_tables_to_save: vec![],
        next_ix_discriminator: None,
        metadata: vec![],
    })
}
