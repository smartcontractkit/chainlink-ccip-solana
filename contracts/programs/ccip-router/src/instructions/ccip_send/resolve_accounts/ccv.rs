use anchor_lang::prelude::*;
use anchor_lang::solana_program::{
    instruction::{AccountMeta, Instruction},
    program::invoke,
};
use ccip_common::cpi::parse_verified_cpi_return_or;

use crate::instructions::common::OnrampRemainingAccountListLengths;
use crate::{
    context::{valid_version, MAX_CHAINSTATECCIPV2_V, MAX_CONFIG_V},
    instruction::{self as router_instruction},
    instructions::{
        ccip_send::{token_receiver, CcipSendV2Params},
        common::OnrampResolutionMetadata,
    },
    state::{Config, DestChainCcipV2},
    CcipRouterError,
};
use ccip_common::{
    messages::{
        normalized_address, MessageV1FieldsAtResolutionTime, NormalizeWsol,
        TokenTransferFieldsAtResolutionTime,
    },
    resolution::*,
    seed,
};

use committee_verifier as verifier;
use verifier::CommitteeVerifierCpiResolutionInputs;

use super::common::{parse_onramp_resolution_inputs, OnrampResolutionInputs};

pub(crate) fn begin_nested_ccv_stages(
    send_params: CcipSendV2Params,
    accounts_to_save: Vec<CcipAccountMeta>,
    ccvs: Vec<Pubkey>,
    mut account_list_lengths: OnrampRemainingAccountListLengths,
) -> Result<ResolveAccountsResponse> {
    // First CCV account list length (will be filled during CPI)
    account_list_lengths.ccvs.push(0);
    let dest_selector_seed = send_params.dest_chain_selector.to_le_bytes();
    let first_ccv = ccvs[0];
    let response = ResolveAccountsResponse {
        accounts_to_save,
        ask_again_with: ask_again_with_for_ccv(
            find(&[seed::CONFIG], crate::ID),
            find(
                &[seed::DEST_CHAIN_STATE_V2, dest_selector_seed.as_ref()],
                crate::ID,
            ),
            first_ccv,
            vec![find(&[seed::CONFIG], first_ccv)],
        ),
        lookup_tables_to_save: vec![],
        next_ix_discriminator: Some(
            router_instruction::ResolveAccountsCcvStage::DISCRIMINATOR
                .try_into()
                .unwrap(),
        ),
        // The final, merged CCV list is threaded through the metadata so each nested CCV stage
        // operates on it directly: the send params no longer reflect the merged list once
        // lane-mandated and pool-required CCVs are folded in.
        metadata: serialize_metadata(&CcipSendCcvStageMetadata {
            account_list_lengths,
            ccvs,
            ccv_index: 0,
            inner_cpi_resolution_inputs: vec![],
            ccv_signer_seen: false,
        })?,
    };
    require_response_fits(&response)?;
    Ok(response)
}

#[derive(Accounts)]
#[instruction(params: ResolveAccountsParams)]
pub struct ResolveAccountsCcvStage<'info> {
    #[account(
        seeds = [seed::CONFIG],
        bump,
        constraint = valid_version(config.version, MAX_CONFIG_V) @ CcipRouterError::InvalidVersion,
    )]
    pub config: Account<'info, Config>,

    /// Seeds are validated in the handler below (requires parsing params first).
    #[account(
        constraint = valid_version(dest_chain.version, MAX_CHAINSTATECCIPV2_V) @ CcipRouterError::InvalidVersion,
    )]
    pub dest_chain: Account<'info, DestChainCcipV2>,

    /// CHECK: This is the current CCV program whose resolver is being wrapped.
    pub ccv_program: UncheckedAccount<'info>,
}

pub fn ccv_stage<'info>(
    ctx: Context<'info, ResolveAccountsCcvStage<'info>>,
    params: ResolveAccountsParams,
) -> Result<ResolveAccountsResponse> {
    let mut stage_metadata = CcipSendCcvStageMetadata::try_from_slice(&params.metadata)
        .map_err(|_| CcipRouterError::InvalidInstructionData)?;
    let OnrampResolutionInputs {
        params: send_params,
        sender,
    } = parse_onramp_resolution_inputs(&params)?;

    // The final CCV list was computed once in the construct-final-ccv-list stage and carried in the
    // metadata. We clone it out so `stage_metadata` can be borrowed mutably further down.
    let ccvs = stage_metadata.ccvs.clone();
    let ccv_index = stage_metadata.ccv_index as usize;

    require_gt!(
        ccvs.len(),
        ccv_index,
        CcipRouterError::InvalidInstructionData
    );
    // SAFETY: ccv_index is validated < ccvs.len() by require_gt! above; +1 cannot overflow usize
    require_eq!(
        stage_metadata.account_list_lengths.ccvs.len(),
        ccv_index + 1,
        CcipRouterError::InvalidAccountListLengths
    );

    let dest_selector_seed = send_params.dest_chain_selector.to_le_bytes();
    require_keys_eq!(
        ctx.accounts.dest_chain.key(),
        find(
            &[seed::DEST_CHAIN_STATE_V2, dest_selector_seed.as_ref()],
            crate::ID,
        ),
        CcipRouterError::InvalidAccountListLengths
    );

    let ccv = ccvs[ccv_index];
    require_keys_eq!(
        ctx.accounts.ccv_program.key(),
        ccv,
        CcipRouterError::InvalidAccountListLengths
    );

    let ccv_signer = find(&[seed::CCV_SIGNER, ccv.as_ref()], crate::ID);

    // On the first round the stage constructs the initial resolution inputs; subsequent
    // rounds forward the inputs returned by the previous round.
    let cpi_resolution_inputs = if stage_metadata.inner_cpi_resolution_inputs.is_empty() {
        let (fields, verifier_args) = start_verification_resolution_inputs(
            &ctx.accounts.config,
            &ctx.accounts.dest_chain,
            &send_params,
            sender,
            ccv,
            ccv_index,
        )?;
        serialize_metadata(
            &CommitteeVerifierCpiResolutionInputs::StartVerificationAtSource {
                fields,
                fee_token: send_params.message.fee_token.normalize_wsol(),
                verifier_args,
            },
        )?
    } else {
        stage_metadata.inner_cpi_resolution_inputs.clone()
    };

    let nested_response = invoke_ccv_resolver(
        ccv,
        ctx.accounts.ccv_program.to_account_info(),
        ctx.remaining_accounts,
        NestedResolveAccountsParams {
            caller: ccv_signer,
            caller_program: crate::ID,
            cpi_resolution_inputs,
        },
    )?;

    handle_nested_ccv_response(
        ctx.accounts.config.key(),
        ctx.accounts.dest_chain.key(),
        &mut stage_metadata,
        &ccvs,
        ccv,
        ccv_signer,
        nested_response,
    )
}

/// Builds the inputs the CCV at `ccv_index` gets for its `start_verification_at_source`
/// resolution: the resolution-time view of the message plus that CCV's own args.
fn start_verification_resolution_inputs(
    config: &Config,
    dest_chain: &DestChainCcipV2,
    send_params: &CcipSendV2Params,
    sender: Pubkey,
    ccv: Pubkey,
    ccv_index: usize,
) -> Result<(MessageV1FieldsAtResolutionTime, Vec<u8>)> {
    let extra_args = dest_chain
        .config
        .parse_extra_args_with_defaults(&send_params.message)?;

    let verifier_args = match extra_args.ccvs.get(ccv_index) {
        Some(parsed_ccv) => {
            require_keys_eq!(*parsed_ccv, ccv, CcipRouterError::InvalidInstructionData);
            extra_args
                .ccv_args
                .get(ccv_index)
                .cloned()
                .unwrap_or_default()
        }
        None => vec![],
    };

    let receiver = normalized_address(
        &send_params.message.receiver,
        dest_chain.config.address_bytes_length as usize,
        CcipRouterError::InvalidReceiverAddress,
    )?;

    let token_transfer = send_params
        .message
        .token_amounts
        .first()
        .map(|tt| {
            Ok::<_, Error>(TokenTransferFieldsAtResolutionTime {
                source_token_address: tt.token,
                amount: tt.amount,
                token_receiver: token_receiver(&extra_args, &send_params.message, dest_chain)?,
            })
        })
        .transpose()?;

    Ok((
        MessageV1FieldsAtResolutionTime {
            source_chain_selector: config.svm_chain_selector,
            dest_chain_selector: send_params.dest_chain_selector,
            ccip_receive_gas_limit: extra_args.gas_limit,
            finality: extra_args.finality,
            on_ramp_address: crate::ID.to_bytes().to_vec(),
            off_ramp_address: dest_chain.config.offramp.clone(),
            sender: sender.to_bytes().to_vec(),
            receiver,
            dest_blob: extra_args.executor_args,
            data: send_params.message.data.clone(),
            token_transfer,
        },
        verifier_args,
    ))
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
        .map_err(|_| CcipRouterError::InvalidInstructionData)?;

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

    parse_verified_cpi_return_or(ccv, CcipRouterError::InvalidCCVAccountResolutionResponse)
}

#[derive(Clone, Debug, PartialEq, Eq, AnchorSerialize, AnchorDeserialize)]
struct CcipSendCcvStageMetadata {
    account_list_lengths: OnrampRemainingAccountListLengths,
    // The final, merged CCV list (user/default + lane-mandated + pool-required), computed once in
    // the construct-final-ccv-list stage and carried across the nested CCV stages.
    //
    // The parallel `ccv_args` are deliberately NOT carried: resolution never reads them (each CCV's
    // resolver is driven by its own `cpi_resolution_inputs`), they are only needed at execution
    // time, and `ccip_send_v2` reads them straight from the message's extra args. They are
    // caller-supplied and unbounded, so threading them here would spend the response's 1kB budget
    // on bytes no stage uses — see `MAX_METADATA_PER_RESPONSE`.
    ccvs: Vec<Pubkey>,
    ccv_index: u32,
    /// Inputs for the next round of nested CCV resolution. Populated by the caller for the first
    /// round, populated from the previous round's `next_cpi_resolution_inputs` afterwards.
    inner_cpi_resolution_inputs: Vec<u8>,
    ccv_signer_seen: bool,
}

fn handle_nested_ccv_response(
    router_config: Pubkey,
    dest_chain_state: Pubkey,
    stage_metadata: &mut CcipSendCcvStageMetadata,
    ccvs: &[Pubkey],
    ccv: Pubkey,
    ccv_signer: Pubkey,
    nested_response: NestedResolveAccountsResponse,
) -> Result<ResolveAccountsResponse> {
    // The CCV decides how many accounts and how much `inner_cpi_resolution_inputs` this response
    // carries, and the merged CCV list rides in the metadata alongside them, so the assembled
    // result is size-checked before it goes out.
    let response = build_nested_ccv_response(
        router_config,
        dest_chain_state,
        stage_metadata,
        ccvs,
        ccv,
        ccv_signer,
        nested_response,
    )?;
    require_response_fits(&response)?;
    Ok(response)
}

fn build_nested_ccv_response(
    router_config: Pubkey,
    dest_chain_state: Pubkey,
    stage_metadata: &mut CcipSendCcvStageMetadata,
    ccvs: &[Pubkey],
    ccv: Pubkey,
    ccv_signer: Pubkey,
    nested_response: NestedResolveAccountsResponse,
) -> Result<ResolveAccountsResponse> {
    let ccv_index = stage_metadata.ccv_index as usize;
    require_gt!(
        ccvs.len(),
        ccv_index,
        CcipRouterError::InvalidInstructionData
    );

    let current_length = stage_metadata
        .account_list_lengths
        .ccvs
        .get_mut(ccv_index) // SAFETY: ccv_index validated by require_gt!; .get_mut() returns Option with safe .ok_or()
        .ok_or(CcipRouterError::InvalidAccountListLengths)?;

    let mut accounts_to_save = vec![];
    // We always start with the CCV pubkey to enable CPIs, even if the
    // inner resolver hasn't returned it as an account to save.
    if *current_length == 0 {
        accounts_to_save.push(ccv.readonly());
        *current_length = checked_add_u8(*current_length, 1)?;
    }

    // Normalization removes the signer bit and flags if we've seen the signer (
    // this must be tracked across stages as it might not be at the start)
    let (mut ccv_accounts, ccv_signer_seen) =
        normalize_nested_accounts(nested_response.accounts_to_save, ccv_signer)?;

    stage_metadata.ccv_signer_seen |= ccv_signer_seen;
    *current_length = checked_add_u8(*current_length, ccv_accounts.len())?;
    accounts_to_save.append(&mut ccv_accounts);

    if !nested_response.next_cpi_resolution_inputs.is_empty() {
        stage_metadata.inner_cpi_resolution_inputs = nested_response.next_cpi_resolution_inputs;

        return Ok(ResolveAccountsResponse {
            ask_again_with: ask_again_with_for_ccv(
                router_config,
                dest_chain_state,
                ccv,
                nested_response.ask_again_with,
            ),
            accounts_to_save,
            lookup_tables_to_save: nested_response.lookup_tables_to_save,
            next_ix_discriminator: Some(
                router_instruction::ResolveAccountsCcvStage::DISCRIMINATOR
                    .try_into()
                    .unwrap(),
            ),
            metadata: serialize_metadata(stage_metadata)?,
        });
    }

    // We're done with a CCV at this point, so we must have seen the
    // signer account at some point during its inner stages.
    require!(
        stage_metadata.ccv_signer_seen,
        CcipRouterError::InvalidCCVAccountResolutionResponse
    );

    // SAFETY: ccv_index < ccvs.len() ensured by require_gt!, so ccv_index + 1 cannot overflow
    if ccv_index + 1 < ccvs.len() {
        let next_ccv = ccvs[ccv_index + 1];
        stage_metadata.ccv_index += 1;
        stage_metadata.account_list_lengths.ccvs.push(0);
        stage_metadata.inner_cpi_resolution_inputs = vec![];
        stage_metadata.ccv_signer_seen = false;

        Ok(ResolveAccountsResponse {
            ask_again_with: ask_again_with_for_ccv(
                router_config,
                dest_chain_state,
                next_ccv,
                vec![find(&[seed::CONFIG], next_ccv)],
            ),
            accounts_to_save,
            lookup_tables_to_save: nested_response.lookup_tables_to_save,
            next_ix_discriminator: Some(
                router_instruction::ResolveAccountsCcvStage::DISCRIMINATOR
                    .try_into()
                    .unwrap(),
            ),
            metadata: serialize_metadata(stage_metadata)?,
        })
    } else {
        Ok(ResolveAccountsResponse {
            ask_again_with: vec![],
            accounts_to_save,
            lookup_tables_to_save: nested_response.lookup_tables_to_save,
            next_ix_discriminator: None,
            metadata: serialize_metadata(&OnrampResolutionMetadata {
                account_list_lengths: stage_metadata.account_list_lengths.clone(),
            })?,
        })
    }
}

fn ask_again_with_for_ccv(
    router_config: Pubkey,
    dest_chain_state: Pubkey,
    ccv: Pubkey,
    inner_accounts: Vec<Pubkey>,
) -> Vec<Pubkey> {
    // SAFETY: all values are small (3 + accounts.len()), bounded by Solana transaction limits
    let mut ask_again_with = Vec::with_capacity(3 + inner_accounts.len());
    ask_again_with.push(router_config);
    ask_again_with.push(dest_chain_state);
    ask_again_with.push(ccv);
    ask_again_with.extend(inner_accounts);
    ask_again_with
}

#[cfg(test)]
mod tests {
    use super::*;
    use ccip_common::{extra_args::MAX_CCVS_PER_MESSAGE, messages::SVM2AnyMessage};

    fn send_params() -> CcipSendV2Params {
        CcipSendV2Params {
            dest_chain_selector: 42,
            message: SVM2AnyMessage {
                receiver: vec![],
                data: vec![],
                token_amounts: vec![],
                fee_token: Pubkey::default(),
                extra_args: vec![],
            },
            resolution_metadata: vec![],
        }
    }

    fn begin_stages_with(ccv_count: usize) -> Result<ResolveAccountsResponse> {
        let ccvs: Vec<Pubkey> = (0..ccv_count).map(|_| Pubkey::new_unique()).collect();
        let accounts_to_save = ccvs
            .iter()
            .map(|_| Pubkey::new_unique().writable())
            .collect();
        begin_nested_ccv_stages(
            send_params(),
            accounts_to_save,
            ccvs,
            OnrampRemainingAccountListLengths {
                token_transfer: 0,
                executor: 0,
                ccv_vault_atas: ccv_count as u8,
                ccvs: vec![],
            },
        )
    }

    /// The first round embeds the whole merged CCV list in its metadata on top of one vault ATA
    /// per CCV, so it costs ~66 bytes per CCV and runs out of return-data budget one CCV past
    /// [`MAX_CCVS_PER_MESSAGE`]. A list the merge should already have rejected must fail here with
    /// the bounded pagination error rather than an opaque serialization failure when the runtime
    /// writes the return data.
    #[test]
    fn first_round_rejects_a_ccv_list_that_overflows_the_response() {
        let err = begin_stages_with(MAX_CCVS_PER_MESSAGE + 1).unwrap_err();
        assert_eq!(
            err,
            ccip_common::CommonCcipError::ResolutionPageCapacityExhausted.into()
        );
    }

    /// Pins the effective first-round ceiling, which is what [`MAX_CCVS_PER_MESSAGE`] is set to: a
    /// response is 174 bytes of fixed overhead plus 66 per CCV, so a full list fits in the 1kB cap
    /// and one more does not. Raising the limit without widening the response would admit messages
    /// that pass the merge and then fail to resolve.
    #[test]
    fn first_round_fits_the_ccv_limit() {
        for ccv_count in 1..=MAX_CCVS_PER_MESSAGE {
            let response = begin_stages_with(ccv_count).unwrap();
            assert!(
                response.encoded_size() <= MAX_RETURN_DATA_BYTES,
                "{ccv_count} CCVs encode to {} bytes",
                response.encoded_size()
            );
        }
        assert!(begin_stages_with(MAX_CCVS_PER_MESSAGE + 1).is_err());
    }

    fn stage_metadata(ccv_count: usize) -> (CcipSendCcvStageMetadata, Vec<Pubkey>, Pubkey) {
        let ccvs = (0..ccv_count)
            .map(|_| Pubkey::new_unique())
            .collect::<Vec<_>>();
        let ccv_signer = Pubkey::new_unique();
        (
            CcipSendCcvStageMetadata {
                account_list_lengths: OnrampRemainingAccountListLengths {
                    token_transfer: 0,
                    executor: 0,
                    ccv_vault_atas: ccv_count as u8,
                    ccvs: vec![0],
                },
                ccv_index: 0,
                inner_cpi_resolution_inputs: vec![],
                ccv_signer_seen: false,
                ccvs: ccvs.clone(),
            },
            ccvs,
            ccv_signer,
        )
    }

    /// A CCV that returns a large `next_cpi_resolution_inputs` would push the wrapping response
    /// past the return-data cap, where it fails as an opaque serialization error. The stage must
    /// reject it instead. This is also why the caller's `ccv_args` are not threaded through the
    /// metadata: resolution never reads them, and they compete for the very same budget.
    #[test]
    fn a_ccv_that_overspends_the_response_budget_is_rejected() {
        let (mut metadata, ccvs, ccv_signer) = stage_metadata(1);
        let result = handle_nested_ccv_response(
            Pubkey::new_unique(),
            Pubkey::new_unique(),
            &mut metadata,
            &ccvs,
            ccvs[0],
            ccv_signer,
            NestedResolveAccountsResponse {
                ask_again_with: vec![Pubkey::new_unique()],
                accounts_to_save: vec![ccv_signer.signer()],
                lookup_tables_to_save: vec![],
                next_cpi_resolution_inputs: vec![0u8; MAX_RETURN_DATA_BYTES],
            },
        );

        assert!(result.is_err());
    }

    #[test]
    fn first_ccv_inner_resolver_needs_another_stage() {
        let (mut metadata, ccvs, ccv_signer) = stage_metadata(1);
        let nested_next_inputs = vec![1, 2, 3];
        let response = handle_nested_ccv_response(
            Pubkey::new_unique(),
            Pubkey::new_unique(),
            &mut metadata,
            &ccvs,
            ccvs[0],
            ccv_signer,
            NestedResolveAccountsResponse {
                ask_again_with: vec![Pubkey::new_unique()],
                accounts_to_save: vec![ccv_signer.signer()],
                lookup_tables_to_save: vec![],
                next_cpi_resolution_inputs: nested_next_inputs.clone(),
            },
        )
        .unwrap();

        assert_eq!(response.accounts_to_save.len(), 2);
        assert_eq!(response.accounts_to_save[0].pubkey, ccvs[0]);
        assert!(!response.accounts_to_save[1].is_signer);
        assert_eq!(
            response.next_ix_discriminator.unwrap(),
            router_instruction::ResolveAccountsCcvStage::DISCRIMINATOR
        );
        assert_eq!(metadata.inner_cpi_resolution_inputs, nested_next_inputs);
        assert_eq!(
            metadata.account_list_lengths,
            OnrampRemainingAccountListLengths {
                token_transfer: 0,
                executor: 0,
                ccv_vault_atas: 1,
                ccvs: vec![2],
            }
        );
        assert_eq!(response.ask_again_with.len(), 4);
    }

    #[test]
    fn first_ccv_completes_and_advances_to_next_ccv() {
        let (mut metadata, ccvs, ccv_signer) = stage_metadata(2);
        let response = handle_nested_ccv_response(
            Pubkey::new_unique(),
            Pubkey::new_unique(),
            &mut metadata,
            &ccvs,
            ccvs[0],
            ccv_signer,
            NestedResolveAccountsResponse {
                ask_again_with: vec![],
                accounts_to_save: vec![Pubkey::new_unique().readonly(), ccv_signer.signer()],
                lookup_tables_to_save: vec![],
                next_cpi_resolution_inputs: vec![],
            },
        )
        .unwrap();

        assert_eq!(
            response.next_ix_discriminator.unwrap(),
            router_instruction::ResolveAccountsCcvStage::DISCRIMINATOR
        );
        assert_eq!(metadata.ccv_index, 1);
        assert_eq!(
            metadata.account_list_lengths,
            OnrampRemainingAccountListLengths {
                token_transfer: 0,
                executor: 0,
                ccv_vault_atas: 2,
                ccvs: vec![3, 0],
            }
        );
        assert!(!metadata.ccv_signer_seen);
        assert!(metadata.inner_cpi_resolution_inputs.is_empty());
        assert_eq!(response.ask_again_with[2], ccvs[1]);
    }

    #[test]
    fn last_ccv_completes_with_final_metadata() {
        let (mut metadata, ccvs, ccv_signer) = stage_metadata(1);
        let response = handle_nested_ccv_response(
            Pubkey::new_unique(),
            Pubkey::new_unique(),
            &mut metadata,
            &ccvs,
            ccvs[0],
            ccv_signer,
            NestedResolveAccountsResponse {
                ask_again_with: vec![],
                accounts_to_save: vec![ccv_signer.signer()],
                lookup_tables_to_save: vec![],
                next_cpi_resolution_inputs: vec![],
            },
        )
        .unwrap();

        assert!(response.next_ix_discriminator.is_none());
        let final_metadata = OnrampResolutionMetadata::try_from_slice(&response.metadata).unwrap();
        assert_eq!(
            final_metadata.account_list_lengths,
            OnrampRemainingAccountListLengths {
                token_transfer: 0,
                executor: 0,
                ccv_vault_atas: 1,
                ccvs: vec![2],
            }
        );
    }

    #[test]
    fn non_ccv_signer_is_rejected() {
        let (mut metadata, ccvs, ccv_signer) = stage_metadata(1);
        let err = handle_nested_ccv_response(
            Pubkey::new_unique(),
            Pubkey::new_unique(),
            &mut metadata,
            &ccvs,
            ccvs[0],
            ccv_signer,
            NestedResolveAccountsResponse {
                ask_again_with: vec![],
                accounts_to_save: vec![Pubkey::new_unique().signer()],
                lookup_tables_to_save: vec![],
                next_cpi_resolution_inputs: vec![],
            },
        )
        .unwrap_err();

        assert_eq!(
            err,
            ccip_common::CommonCcipError::InvalidNestedResolutionSigner.into()
        );
    }
}
