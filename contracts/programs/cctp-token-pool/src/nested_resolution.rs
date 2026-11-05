//! Combined `nested_resolve_accounts_start` dispatcher.
//!
//! This program serves two CPI-resolution protocols under the one Anchor instruction name
//! `nested_resolve_accounts_start` (mandated by the router/offramp, see plan §2.1):
//! - the pool-v2 protocol (`base_token_pool::v2::TokenPoolCpiResolutionInputs`), used by the
//!   router/onramp/offramp when resolving accounts for this program's *pool* role.
//! - the verifier/CCV protocol (`CctpCcvCpiResolutionInputs`), used when resolving
//!   accounts for this program's *verifier* role. Only its `Verify` variant must stay
//!   Borsh-shape-identical to `committee_verifier::CommitteeVerifierCpiResolutionInputs::Verify`
//!   (see that variant's doc below) - round-1 input shared by every CCV. Later rounds are
//!   this program's own private continuation and carry no such constraint.
//!
//! Disambiguation is NOT done via `NestedResolveAccountsParams.caller` (that field's value is
//! only used later, to strip the signer bit off whichever resolved account matches it - see
//! `ccip_common::resolution::normalize_nested_accounts`). Instead, the single account this
//! instruction takes is decoded by Anchor discriminator: it is always either this program's own
//! per-mint `State` (pool-flavor calls) or its global `VerifierConfig` (verifier-flavor calls),
//! and those two account types have distinct, collision-free 8-byte discriminators. This is more
//! robust than branching on `caller`'s value, since it doesn't require recomputing/trusting any
//! router- or offramp-derived PDA, and makes testing easier by not requiring a particular signer

use anchor_lang::prelude::*;
use ccip_common::resolution::{NestedResolveAccountsParams, NestedResolveAccountsResponse};

use crate::error::CctpPoolVerifierError;
use crate::pool::instructions::operations_v2::resolve_accounts_start as pool_v2_resolution;
use crate::pool::state::State as PoolState;
use crate::verifier::instructions::resolve_accounts_start as verifier_resolution;
use crate::verifier::state::Config as VerifierConfig;

#[derive(Accounts)]
pub struct NestedResolveAccountsStart<'info> {
    /// CHECK: manually discriminated in the handler as either this program's pool `State` or its
    /// `VerifierConfig`, per the module doc above. Whichever account resolution is being asked
    /// about (pool or verifier role) determines which one the caller passes in.
    pub pool_state_or_verifier_config: UncheckedAccount<'info>,
}

pub fn start(
    ctx: Context<NestedResolveAccountsStart>,
    params: NestedResolveAccountsParams,
) -> Result<NestedResolveAccountsResponse> {
    let account_info = ctx.accounts.pool_state_or_verifier_config.to_account_info();
    let account_key = account_info.key();

    require_keys_eq!(
        *account_info.owner,
        crate::ID,
        CctpPoolVerifierError::InvalidResolutionAccountOwner
    );

    {
        let data = account_info.try_borrow_data()?;

        if let Ok(pool_state) = PoolState::try_deserialize(&mut &data[..]) {
            drop(data);
            return pool_v2_resolution::nested_resolve_accounts_start(
                account_key,
                pool_state,
                params,
            );
        }
    }

    let data = account_info.try_borrow_data()?;

    let verifier_config = VerifierConfig::try_deserialize(&mut &data[..]).map_err(|e| {
        msg!(
            "Failed to deserialize pool state or verifier config: {:?}",
            e
        );
        CctpPoolVerifierError::InvalidResolutionInputs
    })?;

    drop(data);
    verifier_resolution::nested_resolve_accounts_start(
        account_key,
        verifier_config,
        ctx.remaining_accounts,
        params,
    )
}
