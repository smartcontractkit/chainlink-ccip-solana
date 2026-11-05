use anchor_lang::prelude::*;
use anchor_spl::token::spl_token::native_mint;

use crate::{
    instruction::GetFeeV2,
    instructions::{
        ccip_send::{begin_executor_stage, begin_token_transfer_stages},
        common::OnrampRemainingAccountListLengths,
        get_fee::GetFeeParams,
    },
    CcipRouterError,
};

use super::super::resolve_accounts_start::ResolveAccountsStart;
use ccip_common::{
    messages::NormalizeWsol,
    resolution::{find, ResolveAccountsParams, ResolveAccountsResponse, ToMeta},
    seed,
};

/// IMPORTANT: This initial stage is the only one that's different between resolution for `get_fee`
/// and for `ccip_send_v2`. All subsequent stages and nested resolution is reused
/// *directly* from the `ccip_send_v2` account resolution code for simplicity. This means,
/// among other things, that the ResolutionMetadata is shared with `ccip_send_v2`, which
/// the `get_fee` handler can use to determine the account layout.
pub fn start(
    ctx: Context<ResolveAccountsStart>,
    params: ResolveAccountsParams,
) -> Result<ResolveAccountsResponse> {
    let get_fee_params = parse_get_fee_inputs(&params)?;

    let fee_quoter = ctx.accounts.config.fee_quoter;
    let dest_selector_seed = get_fee_params.dest_chain_selector.to_le_bytes();

    let billing_seed = if get_fee_params.message.fee_token == Pubkey::default() {
        native_mint::ID // pre-2022 WSOL
    } else {
        get_fee_params.message.fee_token
    };

    // If there's a transfer, we add the registry to the response, and the zero pubkey if not.
    // This is not bundled inside the token accounts as it's used to determine the schema between
    // 1.6 and 2.0 token transfer accounts.
    let token_admin_registry =
        if let Some(token_transfer) = get_fee_params.message.token_amounts.first() {
            seed::token_admin_registry(&token_transfer.token, crate::ID)
        } else {
            Pubkey::default()
        };

    let link_seed = ctx.accounts.config.link_token_mint.key();
    let rmn_remote = ctx.accounts.config.rmn_remote;
    let accounts_to_save = vec![
        ctx.accounts.config.key().readonly(),
        // dest_chain_state
        find(
            &[seed::DEST_CHAIN_STATE_V2, dest_selector_seed.as_ref()],
            crate::ID,
        )
        .readonly(),
        token_admin_registry.readonly(),
        fee_quoter.readonly(),
        // Fee quoter CPI accounts
        find(&[seed::CONFIG], fee_quoter).readonly(),
        find(
            &[seed::DEST_CHAIN_V2, dest_selector_seed.as_ref()],
            fee_quoter,
        )
        .readonly(),
        // Legacy dest chain state (1.6) for gas pricing
        find(&[seed::DEST_CHAIN, dest_selector_seed.as_ref()], fee_quoter).readonly(),
        find(
            &[seed::FEE_BILLING_TOKEN_CONFIG, billing_seed.as_ref()],
            fee_quoter,
        )
        .readonly(),
        find(
            &[seed::FEE_BILLING_TOKEN_CONFIG, link_seed.as_ref()],
            fee_quoter,
        )
        .readonly(),
        // RMN Remote
        rmn_remote.readonly(),
        find(&[seed::CURSES], rmn_remote).readonly(),
        find(&[seed::CONFIG], rmn_remote).readonly(),
        // CPI event accounts
        find(&[seed::EVENT_AUTHORITY], crate::ID).readonly(), // CPI event authority
        crate::ID.readonly(),                                 // the router itself for CPI event
    ];

    let account_list_lengths = OnrampRemainingAccountListLengths::default();
    if let Some(token_transfer) = get_fee_params.message.token_amounts.first() {
        begin_token_transfer_stages(
            ctx.accounts.config.key(),
            token_transfer,
            accounts_to_save,
            account_list_lengths,
        )
    } else {
        // No token transfer, so no pool to query for required CCVs.
        begin_executor_stage(
            get_fee_params.dest_chain_selector,
            vec![],
            accounts_to_save,
            account_list_lengths,
            vec![],
            get_fee_params.message.fee_token.normalize_wsol(),
        )
    }
}

fn parse_get_fee_inputs(params: &ResolveAccountsParams) -> Result<GetFeeParams> {
    let (_, data) = params
        .ix_data
        .split_at_checked(GetFeeV2::DISCRIMINATOR.len())
        .ok_or(CcipRouterError::InvalidInstructionData)?;

    let params =
        GetFeeParams::try_from_slice(data).map_err(|_| CcipRouterError::InvalidInstructionData)?;

    Ok(params)
}
