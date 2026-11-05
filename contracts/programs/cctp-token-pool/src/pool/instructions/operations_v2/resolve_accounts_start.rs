use anchor_lang::prelude::*;

use base_token_pool::v2::TokenPoolCpiResolutionInputs;
use ccip_common::resolution::*;

use super::{burn_v2, get_required_ccvs, mint_v2};
use crate::pool::state::*;

pub fn nested_resolve_accounts_start(
    state_key: Pubkey,
    state: State,
    params: NestedResolveAccountsParams,
) -> Result<NestedResolveAccountsResponse> {
    let inputs = TokenPoolCpiResolutionInputs::try_from_slice(&params.cpi_resolution_inputs)?;
    match inputs {
        TokenPoolCpiResolutionInputs::LockOrBurn(lock_or_burn_in_v2) => {
            burn_v2::resolve_accounts::start(state, &lock_or_burn_in_v2)
        }
        TokenPoolCpiResolutionInputs::ReleaseOrMint(_) => mint_v2::resolve_accounts::start(),
        TokenPoolCpiResolutionInputs::GetRequiredCcv(get_required_ccvs_in) => {
            get_required_ccvs::resolve_accounts::start(state_key, &state, &get_required_ccvs_in)
        }
    }
}
