use anchor_lang::prelude::program::invoke;
use anchor_lang::prelude::*;
use solana_keccak_hasher as keccak;
use solana_program::instruction::{AccountMeta, Instruction};

use crate::error::CcipOfframpError;
use crate::instruction as offramp_instruction;
use crate::messages::OfframpAccountListLengths;
use crate::state::Config;
use ccip_common::cpi::parse_verified_cpi_return_or;
use ccip_common::messages::MessageV1;
use ccip_common::state::AccountSchemaVersion;
use ccip_common::{resolution::*, seed};

use committee_verifier as verifier;
use verifier::CommitteeVerifierCpiResolutionInputs;

use super::super::ResolvedExecInputs;
use super::common::{end_resolution, parse_exec_inputs};

// Extracted as it can be started from more than one place.
pub(crate) fn begin_ccv_stages(
    exec_params: ResolvedExecInputs,
    accounts_to_save: Vec<CcipAccountMeta>,
    mut account_list_lengths: OfframpAccountListLengths,
    lookup_tables_to_save: Vec<Pubkey>,
) -> Result<ResolveAccountsResponse> {
    // start with 0 accounts for the first CCV; the CCV resolver will add the CCV itself and its accounts, and update this length accordingly
    account_list_lengths.ccvs.push(0);

    Ok(ResolveAccountsResponse {
        accounts_to_save,
        ask_again_with: ask_again_with_for_ccv(
            find(&[seed::CONFIG], crate::ID),
            exec_params.ccvs[0],
            exec_params.buffer_account,
            // Mandatory stage one account for inner resolution.
            vec![find(&[seed::CONFIG], exec_params.ccvs[0])],
        ),
        lookup_tables_to_save,
        next_ix_discriminator: Some(
            offramp_instruction::ResolveAccountsExecuteCcvStage::DISCRIMINATOR
                .try_into()
                .unwrap(),
        ),
        metadata: serialize_metadata(&ExecuteCcvStageMetadata {
            account_list_lengths,
            ccv_index: 0,
            inner_cpi_resolution_inputs: vec![],
        })?,
    })
}

/// Builds the `ask_again_with` list for the next `ccv_stage` round.
///
/// Layout of the resulting `remaining_accounts` (matched by [`ResolveAccountsExecuteCcvStageContext`]
/// + the convention used by [`parse_exec_inputs`]):
///   [offramp_config, ccv_program, buffer_account?, ...inner_resolver_accounts]
///
/// `offramp_config` and `ccv_program` are consumed by the context; the buffer (if buffered
/// execution) sits at `remaining_accounts[0]` so [`parse_exec_inputs`] can find it; the inner
/// resolver accounts follow and are forwarded to the CCV via [`invoke_ccv_resolver`].
fn ask_again_with_for_ccv(
    offramp_config: Pubkey,
    ccv: Pubkey,
    buffer_account: Option<Pubkey>,
    inner_accounts: Vec<Pubkey>,
) -> Vec<Pubkey> {
    // SAFETY: all values are small (2 + bool + accounts.len()), bounded by Solana transaction limits
    let mut ask_again_with =
        Vec::with_capacity(2 + buffer_account.is_some() as usize + inner_accounts.len());
    ask_again_with.push(offramp_config);
    ask_again_with.push(ccv);
    if let Some(buffer) = buffer_account {
        ask_again_with.push(buffer);
    }
    ask_again_with.extend(inner_accounts);
    ask_again_with
}

/// In-progress metadata threaded through successive `ccv_stage` calls.
#[derive(Clone, Debug, PartialEq, Eq, AnchorSerialize, AnchorDeserialize)]
struct ExecuteCcvStageMetadata {
    account_list_lengths: OfframpAccountListLengths,
    ccv_index: u8,
    /// Inputs for the next round of nested CCV resolution. Populated by the caller
    /// the first round for each CCV; populated from the previous round's
    /// `next_cpi_resolution_inputs` afterwards.
    inner_cpi_resolution_inputs: Vec<u8>,
}

#[derive(Accounts)]
pub struct ResolveAccountsExecuteCcvStageContext<'info> {
    #[account(
        seeds = [seed::CONFIG],
        bump = config.bump,
        constraint = config.version == Config::MAX_V @ CcipOfframpError::InvalidVersion,
    )]
    pub config: Account<'info, Config>,

    /// CHECK: This is the current CCV program whose resolver is being wrapped.
    pub ccv_program: UncheckedAccount<'info>,
}

pub fn ccv_stage<'info>(
    ctx: Context<'info, ResolveAccountsExecuteCcvStageContext<'info>>,
    params: ResolveAccountsParams,
) -> Result<ResolveAccountsResponse> {
    let mut stage_metadata = ExecuteCcvStageMetadata::try_from_slice(&params.metadata)?;

    let exec_params = parse_exec_inputs(&params, ctx.remaining_accounts)?;
    let ccvs = &exec_params.ccvs;
    let verifier_results = &exec_params.verifier_results;
    let ccv_index = stage_metadata.ccv_index as usize;

    require_gt!(
        ccvs.len(),
        ccv_index,
        CcipOfframpError::InvalidInstructionData
    );
    require_gt!(
        verifier_results.len(),
        ccv_index,
        CcipOfframpError::InvalidInstructionData
    );
    require_eq!(
        stage_metadata.account_list_lengths.ccvs.len(),
        // we're always processing the last CCV on the list, and the last index is `len-1`.
        // This can't overflow as the max number of CCVs is bounded by a few things, like the max amount of accounts
        // in any tx (currently 64, may be increased to 128) - https://solana.com/docs/core/transactions#limits
        // SAFETY: `ccv_index` counts resolved CCVs, bounded by the per-tx account limit well below u8::MAX.
        ccv_index + 1,
        CcipOfframpError::InvalidAccountListLengths
    );

    let ccv = ccvs[ccv_index];

    require_keys_eq!(
        ctx.accounts.ccv_program.key(),
        ccv,
        CcipOfframpError::InvalidInstructionData
    );

    let message_id = keccak::hash(&exec_params.encoded_message).to_bytes();
    let message = MessageV1::from_bytes(&exec_params.encoded_message)
        .map_err(|_| CcipOfframpError::InvalidInstructionData)?;
    let buffer_account = exec_params.buffer_account;

    // The buffer (if present) sits at remaining_accounts[0] by convention; the inner
    // CCV resolver must not see it, so peel it off before forwarding.
    let inner_resolver_accounts = if buffer_account.is_some() {
        ctx.remaining_accounts
            .get(1..)
            .ok_or(CcipOfframpError::InvalidInstructionData)?
    } else {
        ctx.remaining_accounts
    };

    // On the first round the stage constructs the initial resolution inputs; subsequent
    // rounds forward the inputs returned by the previous round.
    let cpi_resolution_inputs = if stage_metadata.inner_cpi_resolution_inputs.is_empty() {
        serialize_metadata(&CommitteeVerifierCpiResolutionInputs::Verify {
            source_chain_selector: message.source_chain_selector,
            encoded_message: exec_params.encoded_message.clone(),
            verifier_results: verifier_results[ccv_index].clone(),
        })?
    } else {
        stage_metadata.inner_cpi_resolution_inputs.clone()
    };

    let ccv_signer = find(&[seed::CCV_SIGNER, ccv.as_ref()], crate::ID);

    let nested_response = invoke_ccv_resolver(
        ccv,
        ctx.accounts.ccv_program.to_account_info(),
        inner_resolver_accounts,
        NestedResolveAccountsParams {
            caller: ccv_signer,
            caller_program: crate::ID,
            cpi_resolution_inputs,
        },
    )?;

    handle_nested_ccv_response(
        ctx.accounts.config.key(),
        buffer_account,
        &mut stage_metadata,
        ccvs,
        ccv,
        ccv_signer,
        nested_response,
        message_id,
    )
}

#[allow(clippy::too_many_arguments)]
fn handle_nested_ccv_response(
    offramp_config: Pubkey,
    buffer_account: Option<Pubkey>,
    stage_metadata: &mut ExecuteCcvStageMetadata,
    ccvs: &[Pubkey],
    ccv: Pubkey,
    ccv_signer: Pubkey,
    nested_response: NestedResolveAccountsResponse,
    message_id: [u8; 32],
) -> Result<ResolveAccountsResponse> {
    // The CCV decides how many accounts and how much `inner_cpi_resolution_inputs` this response
    // carries, so the assembled result is size-checked before it goes out.
    let response = build_nested_ccv_response(
        offramp_config,
        buffer_account,
        stage_metadata,
        ccvs,
        ccv,
        ccv_signer,
        nested_response,
        message_id,
    )?;
    require_response_fits(&response)?;
    Ok(response)
}

#[allow(clippy::too_many_arguments)]
fn build_nested_ccv_response(
    offramp_config: Pubkey,
    buffer_account: Option<Pubkey>,
    stage_metadata: &mut ExecuteCcvStageMetadata,
    ccvs: &[Pubkey],
    ccv: Pubkey,
    ccv_signer: Pubkey,
    nested_response: NestedResolveAccountsResponse,
    message_id: [u8; 32],
) -> Result<ResolveAccountsResponse> {
    let ccv_index = stage_metadata.ccv_index as usize;
    require_gt!(
        ccvs.len(),
        ccv_index,
        CcipOfframpError::InvalidInstructionData
    );

    let current_length = stage_metadata
        .account_list_lengths
        .ccvs
        .get_mut(ccv_index) // SAFETY: ccv_index validated by require_gt!; .get_mut() returns Option with safe .ok_or()
        .ok_or(CcipOfframpError::InvalidAccountListLengths)?;

    let mut accounts_to_save = vec![];
    // We always start with the CCV pubkey to enable CPIs, even if the
    // inner resolver hasn't returned it as an account to save.
    if *current_length == 0 {
        accounts_to_save.push(ccv.readonly());
        *current_length = checked_add_u8(*current_length, 1)?;
    }

    // Only allow ccv_signer as a signer
    let (mut ccv_accounts, _) =
        normalize_nested_accounts(nested_response.accounts_to_save, ccv_signer)?;

    *current_length = checked_add_u8(*current_length, ccv_accounts.len())?;
    accounts_to_save.append(&mut ccv_accounts);

    if !nested_response.next_cpi_resolution_inputs.is_empty() {
        stage_metadata.inner_cpi_resolution_inputs = nested_response.next_cpi_resolution_inputs;

        return Ok(ResolveAccountsResponse {
            ask_again_with: ask_again_with_for_ccv(
                offramp_config,
                ccv,
                buffer_account,
                nested_response.ask_again_with,
            ),
            accounts_to_save,
            lookup_tables_to_save: nested_response.lookup_tables_to_save,
            next_ix_discriminator: Some(
                offramp_instruction::ResolveAccountsExecuteCcvStage::DISCRIMINATOR
                    .try_into()
                    .unwrap(),
            ),
            metadata: serialize_metadata(stage_metadata)?,
        });
    }

    // SAFETY: ccv_index is a u8 widened to usize, so ccv_index + 1 cannot overflow here
    if ccv_index + 1 < ccvs.len() {
        let next_ccv = ccvs[ccv_index + 1];
        stage_metadata.ccv_index = checked_add_u8(stage_metadata.ccv_index, 1)?;
        stage_metadata.account_list_lengths.ccvs.push(0);
        stage_metadata.inner_cpi_resolution_inputs = vec![];

        Ok(ResolveAccountsResponse {
            ask_again_with: ask_again_with_for_ccv(
                offramp_config,
                next_ccv,
                buffer_account,
                vec![find(&[seed::CONFIG], next_ccv)],
            ),
            accounts_to_save,
            lookup_tables_to_save: nested_response.lookup_tables_to_save,
            next_ix_discriminator: Some(
                offramp_instruction::ResolveAccountsExecuteCcvStage::DISCRIMINATOR
                    .try_into()
                    .unwrap(),
            ),
            metadata: serialize_metadata(stage_metadata)?,
        })
    } else {
        end_resolution(
            accounts_to_save,
            stage_metadata.account_list_lengths.clone(),
            nested_response.lookup_tables_to_save,
            message_id,
        )
    }
}

fn invoke_ccv_resolver<'info>(
    ccv: Pubkey,
    ccv_program: AccountInfo<'info>,
    ccv_resolver_accounts: &[AccountInfo<'info>],
    nested_params: NestedResolveAccountsParams,
) -> Result<NestedResolveAccountsResponse> {
    let mut data = Vec::from(verifier::instruction::NestedResolveAccountsStart::DISCRIMINATOR);
    nested_params
        .serialize(&mut data)
        .map_err(|_| CcipOfframpError::InvalidInstructionData)?;

    let ix = Instruction {
        program_id: ccv,
        accounts: ccv_resolver_accounts
            .iter()
            .map(|account| AccountMeta::new_readonly(account.key(), false))
            .collect(),
        data,
    };

    let account_infos = std::iter::once(ccv_program)
        .chain(ccv_resolver_accounts.iter().cloned())
        .collect::<Vec<_>>();
    invoke(&ix, &account_infos)?;

    parse_verified_cpi_return_or(ccv, CcipOfframpError::InvalidCCVAccountResolutionResponse)
}
