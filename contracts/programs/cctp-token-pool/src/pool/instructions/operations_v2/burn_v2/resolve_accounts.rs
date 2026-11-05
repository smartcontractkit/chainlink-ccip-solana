use anchor_lang::prelude::*;

use base_token_pool::constants::POOL_CHAINCONFIG_SEED_2_0;
use base_token_pool::v2::LockOrBurnInV2;
use ccip_common::resolution::*;
use ccip_common::seed;

use crate::pool::state::State;

/// First (and only) stage of account resolution for the `lock_or_burn_tokens_v2` instruction. Note
/// this is incomplete: It only returns the additional accounts that are NOT resolved by the onramp
/// resolver, as the `lock_or_burn_tokens_v2` is only authorized to be invoked as a CPI by the onramp
/// anyway.
pub fn start(state: State, lock_or_burn: &LockOrBurnInV2) -> Result<NestedResolveAccountsResponse> {
    let remote_chain_selector = lock_or_burn.remote_chain_selector.to_le_bytes();

    Ok(NestedResolveAccountsResponse {
        ask_again_with: vec![],
        accounts_to_save: vec![
            // These could've easily been derived by the onramp resolver, but we leave them here to
            // prove nested resolution. In the case of a real third party token pool, they won't be
            // mandatory anyway as they depend on whether the pool emits CPI events.
            find(&[seed::EVENT_AUTHORITY], crate::ID).readonly(),
            crate::ID.readonly(),
            find(
                &[
                    POOL_CHAINCONFIG_SEED_2_0,
                    remote_chain_selector.as_ref(),
                    state.config.mint.as_ref(),
                ],
                crate::ID,
            )
            .readonly(),
        ],
        lookup_tables_to_save: vec![],
        next_cpi_resolution_inputs: vec![],
    })
}
