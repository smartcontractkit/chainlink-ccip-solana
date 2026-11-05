use anchor_lang::prelude::*;

use base_token_pool::v2::{fee_vault_token_account_address, LockOrBurnInV2};
use ccip_common::resolution::*;
use ccip_common::seed;

use super::super::resolve_accounts_start::NestedResolveAccountsStart;

/// First (and only) stage of account resolution for the `lock_or_burn_tokens_v2` instruction. Note
/// this is incomplete: It only returns the additional accounts that are NOT resolved by the onramp
/// resolver, as the `lock_or_burn_tokens_v2` is only authorized to be invoked as a CPI by the onramp
/// anyway.
pub fn start(
    ctx: Context<NestedResolveAccountsStart>,
    _lock_or_burn: &LockOrBurnInV2,
) -> Result<NestedResolveAccountsResponse> {
    Ok(NestedResolveAccountsResponse {
        ask_again_with: vec![],
        accounts_to_save: vec![
            // The ATA of the pool's fee vault for the pool's own mint: the in-token BPS fee is
            // moved there so it can only be withdrawn through `withdraw_fee_tokens`. The vault PDA
            // itself isn't resolved, as the instruction re-derives it to check this address and
            // would otherwise cost 32 more bytes in an already tight transaction. Always resolved,
            // as the resolver doesn't read the lane's fee config; the instruction only touches the
            // ATA when the lane actually charges a fee.
            fee_vault_token_account_address(
                &crate::ID,
                &ctx.accounts.state.config.mint,
                &ctx.accounts.state.config.token_program,
            )
            .writable(),
            // These could've easily been derived by the onramp resolver, but we leave them here to
            // prove nested resolution. In the case of a real third party token pool, they won't be
            // mandatory anyway as they depend on whether the pool emits CPI events.
            find(&[seed::EVENT_AUTHORITY], crate::ID).readonly(),
            crate::ID.readonly(),
        ],
        lookup_tables_to_save: vec![],
        next_cpi_resolution_inputs: vec![],
    })
}
