use anchor_lang::prelude::*;
use base_token_pool::v1::*;

use crate::instructions::Empty;

/// Called by the router via CPI during `DeriveAccountsCcipSend` to discover
/// any extra accounts needed for the onramp token transfer.
/// Lockrelease pools have no additional account requirements beyond the
/// standard token lookup table, so this returns an empty set.
pub fn derive_accounts_lock_or_burn_tokens<'info>(
    _ctx: Context<'info, Empty>,
    stage: String,
    _lock_or_burn: LockOrBurnInV1,
) -> Result<DeriveAccountsResponse> {
    Ok(DeriveAccountsResponse {
        current_stage: stage,
        ..Default::default()
    })
}

/// Called by the offramp via CPI during `DeriveAccountsExecute` to discover
/// any extra accounts needed for the offramp token transfer.
/// Lockrelease pools have no additional account requirements beyond the
/// standard token lookup table, so this returns an empty set.
pub fn derive_accounts_release_or_mint_tokens<'info>(
    _ctx: Context<'info, Empty>,
    stage: String,
    _release_or_mint: ReleaseOrMintInV1,
) -> Result<DeriveAccountsResponse> {
    Ok(DeriveAccountsResponse {
        current_stage: stage,
        ..Default::default()
    })
}
