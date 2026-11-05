use anchor_lang::prelude::*;

use crate::{
    context::{valid_version, MAX_CONFIG_V},
    instruction::{self as router_instruction},
    instructions::ccip_send::resolve_accounts::common::parse_onramp_resolution_inputs,
    state::Config,
    CcipRouterError,
};
use ccip_common::{
    messages::NormalizeWsol, resolution::*, seed, v1::load_token_admin_registry_checked,
};

#[derive(Accounts)]
pub struct ResolveAccountsCcipSendTokenTransferRetrieveLutStage<'info> {
    #[account(
        seeds = [seed::CONFIG],
        bump,
        constraint = valid_version(config.version, MAX_CONFIG_V) @ CcipRouterError::InvalidVersion,
    )]
    pub config: Account<'info, Config>,
    pub admin_registry: UncheckedAccount<'info>,
}

pub fn retrieve_token_lut_stage<'info>(
    ctx: Context<'info, ResolveAccountsCcipSendTokenTransferRetrieveLutStage<'info>>,
    params: ResolveAccountsParams,
) -> Result<ResolveAccountsResponse> {
    let inputs = parse_onramp_resolution_inputs(&params)?;
    let send_params = &inputs.params;
    let registry = load_token_admin_registry_checked(
        &ctx.accounts.admin_registry,
        send_params
            .message
            .token_amounts
            .first()
            .ok_or(CcipRouterError::InvalidInstructionData)?
            .token,
    )?;
    let ask_again_with = vec![
        ctx.accounts.config.key(),
        ctx.accounts.admin_registry.key(),
        registry.lookup_table,
        send_params.message.fee_token.normalize_wsol(),
    ];

    Ok(ResolveAccountsResponse {
        ask_again_with,
        accounts_to_save: vec![],
        lookup_tables_to_save: vec![registry.lookup_table],
        next_ix_discriminator: Some(
            router_instruction::ResolveAccountsTokenTransferStaticAccountsStage::DISCRIMINATOR
                .try_into()
                .unwrap(),
        ),
        metadata: params.metadata,
    })
}
