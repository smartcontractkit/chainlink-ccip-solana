use anchor_lang::prelude::*;
use ccip_common::messages::NormalizeWsol;

use crate::instruction::{self as router_instruction};
use crate::instructions::common::{OnrampRemainingAccountListLengths, OnrampResolutionMetadata};
use ccip_common::{resolution::*, seed};

use super::super::super::resolve_accounts_start::ResolveAccountsStart;
use super::common::parse_onramp_resolution_inputs;

pub fn start(
    ctx: Context<ResolveAccountsStart>,
    params: ResolveAccountsParams,
) -> Result<ResolveAccountsResponse> {
    let send_params = parse_onramp_resolution_inputs(&params)?.params;

    let fee_quoter = ctx.accounts.config.fee_quoter;
    let dest_selector_seed = send_params.dest_chain_selector.to_le_bytes();

    let billing_seed = send_params.message.fee_token.normalize_wsol();
    let link_seed = ctx.accounts.config.link_token_mint.key();

    // If there's a transfer, we add the registry to the response, and the zero pubkey if not.
    // This is not bundled inside the token accounts as it's used to determine the schema between
    // 1.6 and 2.0 token transfer accounts.
    let token_admin_registry =
        if let Some(token_transfer) = send_params.message.token_amounts.first() {
            seed::token_admin_registry(&token_transfer.token, crate::ID)
        } else {
            Pubkey::default()
        };

    let accounts_to_save = vec![
        // config
        ctx.accounts.config.key().readonly(),
        // dest_chain_state
        find(
            &[seed::DEST_CHAIN_STATE_V2, dest_selector_seed.as_ref()],
            crate::ID,
        )
        .writable(),
        token_admin_registry.readonly(),
        // authority (caller)
        params.caller.writable().signer(),
        // system_program
        anchor_lang::system_program::ID.readonly(),
        // fee quoter
        fee_quoter.readonly(),
        // fee quoter CPI accounts
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
    ];

    let ask_again_with = vec![
        find(&[seed::CONFIG], crate::ID),
        send_params.message.fee_token.normalize_wsol(),
        find(&[seed::FEE_BILLING_SIGNER], crate::ID),
    ];

    let account_list_lengths = OnrampRemainingAccountListLengths::default();

    Ok(ResolveAccountsResponse {
        ask_again_with,
        accounts_to_save,
        lookup_tables_to_save: vec![],
        next_ix_discriminator: Some(
            router_instruction::ResolveAccountsFeeTokenBillingStage::DISCRIMINATOR
                .try_into()
                .unwrap(),
        ),
        metadata: serialize_metadata(&OnrampResolutionMetadata {
            account_list_lengths,
        })?,
    })
}
