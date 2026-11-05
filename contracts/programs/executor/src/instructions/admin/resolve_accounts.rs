use anchor_lang::prelude::*;

use ccip_common::resolution::*;
use ccip_common::seed;

use super::super::resolve_accounts_start::ResolveAccountsStart;

pub fn start(
    ctx: Context<ResolveAccountsStart>,
    params: ResolveAccountsParams,
) -> Result<ResolveAccountsResponse> {
    Ok(ResolveAccountsResponse {
        ask_again_with: vec![],
        accounts_to_save: vec![
            ctx.accounts.config.key().writable(),
            params.caller.writable().signer(),
            find(&[seed::EVENT_AUTHORITY], crate::ID).readonly(),
            crate::ID.readonly(),
        ],
        metadata: vec![],
        lookup_tables_to_save: vec![],
        next_ix_discriminator: None,
    })
}
