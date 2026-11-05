use anchor_lang::prelude::*;

use ccip_common::resolution::*;
use ccip_common::seed;

use super::super::resolve_accounts_start::ResolveAccountsStart;

/// Single-stage account resolution shared by all admin instructions
/// (transfer_ownership, accept_ownership, set_default_code_version).
/// All require a writable config PDA and the caller as a signer.
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
        lookup_tables_to_save: vec![],
        next_ix_discriminator: None,
        metadata: vec![],
    })
}
