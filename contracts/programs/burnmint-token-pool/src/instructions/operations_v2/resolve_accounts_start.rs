use anchor_lang::prelude::*;

use base_token_pool::v2::TokenPoolCpiResolutionInputs;
use base_token_pool::{constants::*, errors::*};
use ccip_common::resolution::*;

use crate::instructions::operations_v2::{burn_v2, get_required_ccvs, mint_v2};
use crate::state::*;
use crate::State;

#[derive(Accounts)]
pub struct NestedResolveAccountsStart<'info> {
    #[account(
        seeds = [POOL_STATE_SEED, state.config.mint.as_ref()],
        bump,
        constraint = valid_version(state.version, MAX_POOL_STATE_V) @ CcipTokenPoolError::InvalidVersion,
    )]
    pub state: Account<'info, State>,
}

pub fn nested_resolve_accounts_start(
    ctx: Context<NestedResolveAccountsStart>,
    params: NestedResolveAccountsParams,
) -> Result<NestedResolveAccountsResponse> {
    let inputs = TokenPoolCpiResolutionInputs::try_from_slice(&params.cpi_resolution_inputs)?;
    match inputs {
        TokenPoolCpiResolutionInputs::LockOrBurn(lock_or_burn_in_v2) => {
            burn_v2::resolve_accounts::start(ctx, &lock_or_burn_in_v2)
        }
        TokenPoolCpiResolutionInputs::ReleaseOrMint(release_or_mint_in_v2) => {
            mint_v2::resolve_accounts::start(ctx, &release_or_mint_in_v2)
        }
        TokenPoolCpiResolutionInputs::GetRequiredCcv(get_required_ccvs_in) => {
            get_required_ccvs::resolve_accounts::start(ctx, &get_required_ccvs_in)
        }
    }
}
