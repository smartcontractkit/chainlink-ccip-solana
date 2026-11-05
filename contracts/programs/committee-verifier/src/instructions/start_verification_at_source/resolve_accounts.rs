use anchor_lang::prelude::*;

use ccip_common::messages::MessageV1FieldsAtResolutionTime;
use ccip_common::resolution::*;
use ccip_common::seed;

use super::super::resolve_accounts_start::NestedResolveAccountsStart;

/// First (and only) stage of account resolution for the `start_verification_at_source`
/// instruction.
///
/// This verifier only keys off the dest chain selector, but the full resolution-time view of the
/// message is passed so that other implementations of this interface can resolve on any
/// of it.
pub fn start(
    ctx: Context<NestedResolveAccountsStart>,
    caller: Pubkey,
    fields: &MessageV1FieldsAtResolutionTime,
    _fee_token: Pubkey,
    _verifier_args: &[u8],
) -> Result<NestedResolveAccountsResponse> {
    let remote_chain_pda = find(
        &[
            seed::REMOTE_CHAIN,
            &fields.dest_chain_selector.to_le_bytes(),
        ],
        crate::ID,
    );

    Ok(NestedResolveAccountsResponse {
        ask_again_with: vec![],
        accounts_to_save: vec![
            ctx.accounts.config.key().readonly(),
            caller.signer(),
            remote_chain_pda.readonly(),
        ],
        lookup_tables_to_save: vec![],
        next_cpi_resolution_inputs: vec![],
    })
}
