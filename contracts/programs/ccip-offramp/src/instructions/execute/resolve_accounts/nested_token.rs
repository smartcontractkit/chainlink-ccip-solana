use anchor_lang::prelude::program::invoke;
use anchor_lang::prelude::*;
use solana_program::instruction::{AccountMeta, Instruction};

use base_token_pool::state::RemoteAddress;
use base_token_pool::v2::{ReleaseOrMintInV2, TokenPoolCpiResolutionInputs};

use crate::error::CcipOfframpError;
use crate::instruction as offramp_instruction;
use crate::state::Config;
use ccip_common::cpi::parse_verified_cpi_return_or;
use ccip_common::messages::{MessageV1, SVMDestBlob};
use ccip_common::state::AccountSchemaVersion;
use ccip_common::{resolution::*, seed};

use burnmint_token_pool as token_pool;

use super::super::processor::parse_pubkey;
use super::super::ResolvedExecInputs;
use super::ccv::begin_ccv_stages;
use super::common::{end_resolution, parse_exec_inputs, ExecuteNestedTokenStageMetadata};
use super::token_transfer_static::ask_again_with_for_nested_tokens;

#[derive(Accounts)]
#[instruction(params: ResolveAccountsParams)]
pub struct ResolveAccountsExecuteNestedTokenStage<'info> {
    #[account(
        seeds = [seed::CONFIG],
        bump = config.bump,
        constraint = config.version == Config::MAX_V @ CcipOfframpError::InvalidVersion,
    )]
    pub config: Account<'info, Config>,
    pub pool_program: UncheckedAccount<'info>,
    pub pool_signer: UncheckedAccount<'info>,
}

pub fn nested_token_stage<'info>(
    ctx: Context<'info, ResolveAccountsExecuteNestedTokenStage<'info>>,
    params: ResolveAccountsParams,
) -> Result<ResolveAccountsResponse> {
    let mut stage_metadata = ExecuteNestedTokenStageMetadata::try_from_slice(&params.metadata)?;
    let exec_params = parse_exec_inputs(&params, ctx.remaining_accounts)?;
    let message = MessageV1::from_bytes(&exec_params.encoded_message)
        .map_err(|_| CcipOfframpError::InvalidInstructionData)?;

    let inner_resolver_accounts = if exec_params.buffer_account.is_some() {
        ctx.remaining_accounts
            .get(1..)
            .ok_or(CcipOfframpError::InvalidInstructionData)?
    } else {
        ctx.remaining_accounts
    };

    // On the first round the stage constructs the initial resolution inputs; subsequent
    // rounds forward the inputs returned by the previous round.
    let cpi_resolution_inputs = if stage_metadata.inner_cpi_resolution_inputs.is_empty() {
        serialize_metadata(&TokenPoolCpiResolutionInputs::ReleaseOrMint(Box::new(
            release_or_mint_resolution_inputs(&message)?,
        )))?
    } else {
        stage_metadata.inner_cpi_resolution_inputs.clone()
    };

    let nested_response = invoke_token_pool_resolver(
        ctx.accounts.pool_program.to_account_info(),
        inner_resolver_accounts,
        NestedResolveAccountsParams {
            caller: ctx.accounts.pool_signer.key(),
            caller_program: crate::ID,
            cpi_resolution_inputs,
        },
    )?;

    handle_nested_token_pool_response(
        exec_params,
        message.compute_message_id(),
        ctx.accounts.config.key(),
        ctx.accounts.pool_program.key(),
        &mut stage_metadata,
        nested_response,
    )
}

/// Builds the params the pool's `release_or_mint_tokens_v2` will receive, so its resolver can
/// resolve on them.
///
/// Unlike the onramp side, nothing here has to be left out: the offramp is executing an
/// already-built message, so the whole transfer is readable off it, and the receiver usage off its
/// dest blob. This mirrors `transfer_token_ccipv2`, which builds the same params at execution time.
fn release_or_mint_resolution_inputs(message: &MessageV1) -> Result<ReleaseOrMintInV2> {
    let token_transfer = message
        .token_transfer
        .as_ref()
        .ok_or(CcipOfframpError::InvalidInstructionData)?;
    let dest_blob = SVMDestBlob::from_bytes(&message.dest_blob)
        .map_err(|_| CcipOfframpError::InvalidDestBlob)?;

    Ok(ReleaseOrMintInV2 {
        original_sender: RemoteAddress {
            address: message.sender.clone(),
        },
        remote_chain_selector: message.source_chain_selector,
        receiver: parse_pubkey(&token_transfer.token_receiver)?,
        amount: token_transfer.amount,
        local_token: parse_pubkey(&token_transfer.dest_token_address)?,
        source_pool_address: RemoteAddress {
            address: token_transfer.source_pool_address.clone(),
        },
        source_pool_data: token_transfer.extra_data.clone(),
        requested_finality_config: message.finality,
        receiver_usage: dest_blob.use_ata,
    })
}

fn handle_nested_token_pool_response(
    exec_params: ResolvedExecInputs,
    message_id: [u8; 32],
    offramp_config: Pubkey,
    pool_program: Pubkey,
    metadata: &mut ExecuteNestedTokenStageMetadata,
    nested_response: NestedResolveAccountsResponse,
) -> Result<ResolveAccountsResponse> {
    // The pool decides how much this response carries, so the assembled result is size-checked
    // before it goes out; see `require_response_fits`.
    let response = build_nested_token_pool_response(
        exec_params,
        message_id,
        offramp_config,
        pool_program,
        metadata,
        nested_response,
    )?;
    require_response_fits(&response)?;
    Ok(response)
}

fn build_nested_token_pool_response(
    exec_params: ResolvedExecInputs,
    message_id: [u8; 32],
    offramp_config: Pubkey,
    pool_program: Pubkey,
    metadata: &mut ExecuteNestedTokenStageMetadata,
    nested_response: NestedResolveAccountsResponse,
) -> Result<ResolveAccountsResponse> {
    let current_length = &mut metadata.account_list_lengths.token_transfer;
    let mut accounts_to_save = vec![];

    let (mut token_accounts, _) =
        normalize_nested_accounts(nested_response.accounts_to_save, Default::default())?;

    *current_length = checked_add_u8(*current_length, token_accounts.len())?;
    accounts_to_save.append(&mut token_accounts);

    if !nested_response.next_cpi_resolution_inputs.is_empty() {
        metadata.inner_cpi_resolution_inputs = nested_response.next_cpi_resolution_inputs;

        Ok(ResolveAccountsResponse {
            ask_again_with: ask_again_with_for_nested_tokens(
                offramp_config,
                pool_program,
                find(
                    &[seed::EXTERNAL_TOKEN_POOLS_SIGNER, pool_program.as_ref()],
                    crate::ID,
                ),
                exec_params.buffer_account,
                nested_response.ask_again_with,
            ),
            accounts_to_save,
            lookup_tables_to_save: nested_response.lookup_tables_to_save,
            next_ix_discriminator: Some(
                offramp_instruction::ResolveAccountsExecuteNestedTokenStage::DISCRIMINATOR
                    .try_into()
                    .unwrap(),
            ),
            metadata: serialize_metadata(metadata)?,
        })
    } else {
        // Nested pool resolution is complete. NOTE: even though this path is only reached for v2
        // pools (which expose get_required_ccvs), no dedicated segment is appended — the accounts the
        // OffRamp needs to CPI `get_required_ccvs` are part of the mint-validated `token_transfer`
        // accounts and are extracted from there at execution time (`get_pool_required_ccvs_accounts`).
        if exec_params.ccvs.is_empty() {
            end_resolution(
                accounts_to_save,
                metadata.account_list_lengths.clone(),
                nested_response.lookup_tables_to_save,
                message_id,
            )
        } else {
            begin_ccv_stages(
                exec_params,
                accounts_to_save,
                metadata.account_list_lengths.clone(),
                nested_response.lookup_tables_to_save,
            )
        }
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
        .map_err(|_| CcipOfframpError::InvalidInstructionData)?;
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
        CcipOfframpError::InvalidTokenPoolAccountDerivationResponse,
    )
}
