use crate::context::valid_version;
use crate::context::MAX_CONFIG_V;
use crate::error::CcipRouterError;
use crate::instruction::{self as router_instruction};
use crate::instructions::ccip_send::begin_executor_stage;
use crate::instructions::ccip_send::resolve_accounts::common::parse_onramp_resolution_inputs;
use crate::instructions::ccip_send::resolve_accounts::common::CcipSendTokenStaticStageMetadata;
use crate::instructions::common::OnrampRemainingAccountListLengths;
use crate::instructions::common::OnrampResolutionMetadata;
use crate::state::Config;
use anchor_lang::prelude::*;
use anchor_spl::associated_token::get_associated_token_address_with_program_id;
use anchor_spl::token::spl_token;
use ccip_common::messages::NormalizeWsol;
use ccip_common::resolution::*;
use ccip_common::seed;
use fee_quoter::messages::SVMTokenAmount;

#[derive(Accounts)]
pub struct ResolveAccountsFeeTokenBillingStage<'info> {
    #[account(
        seeds = [seed::CONFIG],
        bump,
        constraint = valid_version(config.version, MAX_CONFIG_V) @ CcipRouterError::InvalidVersion,
    )]
    pub config: Account<'info, Config>,

    pub fee_token_mint: UncheckedAccount<'info>,
    pub billing_signer: UncheckedAccount<'info>,
}

pub fn fee_token_billing_stage(
    ctx: Context<ResolveAccountsFeeTokenBillingStage>,
    params: ResolveAccountsParams,
) -> Result<ResolveAccountsResponse> {
    let send_params = parse_onramp_resolution_inputs(&params)?.params;
    let metadata = deserialize_metadata::<OnrampResolutionMetadata>(&params.metadata)?;
    let is_fee_in_sol = send_params.message.fee_token == Pubkey::default();
    let rmn_remote = ctx.accounts.config.rmn_remote;

    let (fee_token_user_ata, fee_token_program) = if is_fee_in_sol {
        (Pubkey::default().readonly(), spl_token::ID.readonly())
    } else {
        let fee_token_program = ctx.accounts.fee_token_mint.owner;
        (
            get_associated_token_address_with_program_id(
                &params.caller.key(),
                &send_params.message.fee_token,
                &fee_token_program.key(),
            )
            .writable(),
            fee_token_program.readonly(),
        )
    };

    let fee_vault = get_associated_token_address_with_program_id(
        &ctx.accounts.billing_signer.key(),
        &ctx.accounts.fee_token_mint.key(),
        &fee_token_program.pubkey,
    );

    let accounts_to_save = vec![
        fee_token_program.readonly(),
        ctx.accounts.fee_token_mint.key().readonly(),
        fee_token_user_ata,
        fee_vault.writable(),
        find(&[seed::FEE_BILLING_SIGNER], crate::ID).readonly(),
        // RMN Remote
        rmn_remote.readonly(),
        find(&[seed::CURSES], rmn_remote).readonly(),
        find(&[seed::CONFIG], rmn_remote).readonly(),
        // CPI event accounts
        find(&[seed::EVENT_AUTHORITY], crate::ID).readonly(),
        crate::ID.readonly(),
    ];

    if let Some(token_transfer) = send_params.message.token_amounts.first() {
        begin_token_transfer_stages(
            ctx.accounts.config.key(),
            token_transfer,
            accounts_to_save,
            metadata.account_list_lengths,
        )
    } else {
        // No token transfer, so no pool to query for required CCVs.
        begin_executor_stage(
            send_params.dest_chain_selector,
            vec![],
            accounts_to_save,
            metadata.account_list_lengths,
            vec![],
            send_params.message.fee_token.normalize_wsol(),
        )
    }
}

/// Public as it's reused by `get_fee`
pub(crate) fn begin_token_transfer_stages(
    config: Pubkey,
    token_transfer: &SVMTokenAmount,
    accounts_to_save: Vec<CcipAccountMeta>,
    account_list_lengths: OnrampRemainingAccountListLengths,
) -> Result<ResolveAccountsResponse> {
    let ask_again_with = vec![
        config,
        seed::token_admin_registry(&token_transfer.token, crate::ID),
    ];

    Ok(ResolveAccountsResponse {
        ask_again_with,
        accounts_to_save,
        lookup_tables_to_save: vec![],
        next_ix_discriminator: Some(
            router_instruction::ResolveAccountsTokenTransferRetrieveLutStage::DISCRIMINATOR
                .try_into()
                .unwrap(),
        ),
        metadata: serialize_metadata(&CcipSendTokenStaticStageMetadata {
            account_list_lengths,
            ..Default::default()
        })?,
    })
}
