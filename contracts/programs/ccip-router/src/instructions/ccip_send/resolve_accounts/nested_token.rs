use anchor_lang::prelude::*;
use anchor_lang::solana_program::{
    instruction::{AccountMeta, Instruction},
    program::invoke,
};
use base_token_pool::v2::{LockOrBurnInV2, TokenPoolCpiResolutionInputs};
use ccip_common::cpi::parse_verified_cpi_return_or;
use ccip_common::messages::NormalizeWsol;

use crate::instructions::ccip_send::token_receiver;
use crate::state::DestChainCcipV2;
use crate::{
    context::{valid_version, MAX_CONFIG_V},
    instruction::{self as router_instruction},
    instructions::ccip_send::CcipSendV2Params,
    state::Config,
    CcipRouterError,
};
use ccip_common::{resolution::*, seed};

// We use one of the reference implementation token pools as an interface, to construct ix
// data for resolution, similarly to the committee verifier as a CCV interface.
use burnmint_token_pool as token_pool;

use super::common::{
    parse_onramp_resolution_inputs, pool_ccv_resolution_accounts, CcipSendTokenStagesMetadata,
    OnrampResolutionInputs,
};
use super::executor::begin_executor_stage;
use super::token_transfer_static::ask_again_with_for_nested_tokens;

#[derive(Accounts)]
#[instruction(params: ResolveAccountsParams)]
pub struct ResolveAccountsNestedTokenStage<'info> {
    #[account(
        seeds = [seed::CONFIG],
        bump,
        constraint = valid_version(config.version, MAX_CONFIG_V) @ CcipRouterError::InvalidVersion,
    )]
    pub config: Account<'info, Config>,
    pub pool_program: UncheckedAccount<'info>,
    pub pool_signer: UncheckedAccount<'info>,
    pub dest_chain: Account<'info, DestChainCcipV2>,
}

pub fn nested_token_stage<'info>(
    ctx: Context<'info, ResolveAccountsNestedTokenStage<'info>>,
    params: ResolveAccountsParams,
) -> Result<ResolveAccountsResponse> {
    let mut stage_metadata = CcipSendTokenStagesMetadata::try_from_slice(&params.metadata)
        .map_err(|_| CcipRouterError::InvalidInstructionData)?;
    let OnrampResolutionInputs {
        params: send_params,
        sender,
    } = parse_onramp_resolution_inputs(&params)?;
    let dest_chain_config_v2 = &ctx.accounts.dest_chain.config;
    let extra_args = dest_chain_config_v2.parse_extra_args_with_defaults(&send_params.message)?;
    let token_transfer = send_params
        .message
        .token_amounts
        .first()
        .cloned()
        .ok_or(CcipRouterError::InvalidInstructionData)?;

    let token_receiver =
        token_receiver(&extra_args, &send_params.message, &ctx.accounts.dest_chain)?;

    // On the first round the stage constructs the initial resolution inputs; subsequent
    // rounds forward the inputs returned by the previous round.
    let cpi_resolution_inputs = if stage_metadata.inner_cpi_resolution_inputs.is_empty() {
        serialize_metadata(&TokenPoolCpiResolutionInputs::LockOrBurn(Box::new(
            LockOrBurnInV2 {
                receiver: token_receiver,
                remote_chain_selector: send_params.dest_chain_selector,
                original_sender: sender,
                amount: token_transfer.amount,
                local_token: token_transfer.token,
                requested_finality_config: extra_args.finality,
                token_args: extra_args.token_args,
            },
        )))?
    } else {
        stage_metadata.inner_cpi_resolution_inputs.clone()
    };

    let nested_response = invoke_token_pool_resolver(
        ctx.accounts.pool_program.to_account_info(),
        ctx.remaining_accounts,
        NestedResolveAccountsParams {
            caller: ctx.accounts.pool_signer.key(),
            caller_program: crate::ID,
            cpi_resolution_inputs,
        },
    )?;

    handle_nested_token_pool_response(
        send_params,
        ctx.accounts.pool_program.key(),
        &mut stage_metadata,
        nested_response,
    )
}

fn handle_nested_token_pool_response(
    send_params: CcipSendV2Params,
    pool_program: Pubkey,
    metadata: &mut CcipSendTokenStagesMetadata,
    nested_response: NestedResolveAccountsResponse,
) -> Result<ResolveAccountsResponse> {
    // The pool decides how much this response carries, so the assembled result is size-checked
    // before it goes out; see `require_response_fits`.
    let response =
        build_nested_token_pool_response(send_params, pool_program, metadata, nested_response)?;
    require_response_fits(&response)?;
    Ok(response)
}

fn build_nested_token_pool_response(
    send_params: CcipSendV2Params,
    pool_program: Pubkey,
    metadata: &mut CcipSendTokenStagesMetadata,
    nested_response: NestedResolveAccountsResponse,
) -> Result<ResolveAccountsResponse> {
    let mut accounts_to_save = vec![];

    // Normalization removes the signer bit
    let (mut token_accounts, _) =
        normalize_nested_accounts(nested_response.accounts_to_save, Default::default())?;

    metadata.account_list_lengths.token_transfer = checked_add_u8(
        metadata.account_list_lengths.token_transfer,
        token_accounts.len(),
    )?;
    accounts_to_save.append(&mut token_accounts);

    if !nested_response.next_cpi_resolution_inputs.is_empty() {
        metadata.inner_cpi_resolution_inputs = nested_response.next_cpi_resolution_inputs;

        Ok(ResolveAccountsResponse {
            ask_again_with: ask_again_with_for_nested_tokens(
                find(&[seed::CONFIG], crate::ID),
                pool_program,
                send_params.dest_chain_selector,
                nested_response.ask_again_with,
            ),
            accounts_to_save,
            lookup_tables_to_save: nested_response.lookup_tables_to_save,
            next_ix_discriminator: Some(
                router_instruction::ResolveAccountsNestedTokenStage::DISCRIMINATOR
                    .try_into()
                    .unwrap(),
            ),
            metadata: serialize_metadata(metadata)?,
        })
    } else {
        // We're finished with token transfer, so we move on to CCVs. This path is only reached for
        // v2 pools that implement account resolution, so the pool always exposes get_required_ccvs.
        let token_mint = send_params
            .message
            .token_amounts
            .first()
            .ok_or(CcipRouterError::InvalidInstructionData)?
            .token;
        begin_executor_stage(
            send_params.dest_chain_selector,
            pool_ccv_resolution_accounts(pool_program, token_mint, send_params.dest_chain_selector),
            accounts_to_save,
            metadata.account_list_lengths.clone(),
            nested_response.lookup_tables_to_save,
            send_params.message.fee_token.normalize_wsol(),
        )
    }
}

fn invoke_token_pool_resolver<'info>(
    pool_program: AccountInfo<'info>,
    pool_program_resolver_accounts: &[AccountInfo<'info>],
    nested_params: NestedResolveAccountsParams,
) -> Result<NestedResolveAccountsResponse> {
    let mut data = Vec::from(token_pool::instruction::NestedResolveAccountsStart::DISCRIMINATOR);
    nested_params
        .serialize(&mut data)
        .map_err(|_| CcipRouterError::InvalidInstructionData)?;
    let ix = Instruction {
        program_id: pool_program.key(),
        accounts: pool_program_resolver_accounts
            .iter()
            .map(|account| AccountMeta::new_readonly(account.key(), false))
            .collect(),
        data,
    };
    let pool_program_address = pool_program.key();
    let account_infos = std::iter::once(pool_program)
        .chain(pool_program_resolver_accounts.iter().cloned())
        .collect::<Vec<_>>();
    invoke(&ix, &account_infos)?;

    parse_verified_cpi_return_or(
        pool_program_address,
        CcipRouterError::InvalidTokenPoolAccountDerivationResponse,
    )
}

#[cfg(test)]
mod tests {
    use super::*;
    use ccip_common::messages::{SVM2AnyMessage, SVMTokenAmount};

    fn send_params() -> CcipSendV2Params {
        CcipSendV2Params {
            dest_chain_selector: 42,
            message: SVM2AnyMessage {
                receiver: vec![],
                data: vec![],
                token_amounts: vec![SVMTokenAmount {
                    token: Pubkey::new_unique(),
                    amount: 1,
                }],
                fee_token: Pubkey::default(),
                extra_args: vec![],
            },
            resolution_metadata: vec![],
        }
    }

    #[test]
    fn final_pool_round_forwards_lookup_tables() {
        let lookup_table = Pubkey::new_unique();
        let response = build_nested_token_pool_response(
            send_params(),
            Pubkey::new_unique(),
            &mut CcipSendTokenStagesMetadata::default(),
            NestedResolveAccountsResponse {
                accounts_to_save: vec![Pubkey::new_unique().readonly()],
                lookup_tables_to_save: vec![lookup_table],
                ..Default::default()
            },
        )
        .unwrap();

        assert_eq!(
            response.next_ix_discriminator,
            Some(
                router_instruction::ResolveAccountsExecutorStage::DISCRIMINATOR
                    .try_into()
                    .unwrap()
            )
        );
        assert_eq!(response.lookup_tables_to_save, vec![lookup_table]);
    }
}
