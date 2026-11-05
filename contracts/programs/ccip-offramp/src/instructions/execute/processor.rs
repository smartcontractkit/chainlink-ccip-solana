use anchor_lang::prelude::sysvar::SysvarId as _;
use anchor_lang::prelude::*;
use anchor_lang::solana_program::{
    instruction::{AccountMeta, Instruction},
    program::{invoke, invoke_signed},
};
use anchor_lang::InstructionData;
use anchor_spl::associated_token::spl_associated_token_account;
use committee_verifier as verifier;

use ccip_common::messages::{MessageV1, SVMDestBlob};
use ccip_common::seed;
use ccip_common::state::AccountSchemaVersion;

use super::token_transfer_legacy::{is_writable, MAX_MESSAGING_ACCOUNTS};

use crate::buffering::AccountInfoExt;
use crate::event::{BufferClosed, ExecutionStateChangedV2};
use crate::instructions::ccv_resolution::{self, ReceiverRequiredCcvs};
use crate::instructions::execute::rmn::verify_uncursed_cpi;
use crate::instructions::execute::token_transfer;
use crate::instructions::get_ccvs_for_msg::GetCcvsForMsgResponse;
use crate::messages::{
    Any2SVMMessage, Any2SVMMessageV2, ExecutionInputsV2, OfframpAccountListLengths, TokenTransfer,
};
use crate::program::CcipOfframp;
use crate::state::*;
use crate::CcipOfframpError;

#[derive(AnchorSerialize, AnchorDeserialize, Debug)]
pub struct ExecuteParams {
    /// When present, it is the execution inputs directly. When missing, there _must_ be a buffer account with the
    /// exec_inputs bytes. Account resolution for buffered execution must receive that same buffer account as an
    /// additional remaining account on `resolve_accounts_start`, so the resolver can read the inputs.
    pub exec_inputs: Option<ExecutionInputsV2>,
    /// Opaque account-resolution metadata produced by account resolution. The wire format is the
    /// borsh-encoded [`ExecuteResolutionMetadata`] struct, which carries both the per-section
    /// account-list lengths and the message_id used to derive the `message_exec_state` PDA at constraint time.
    pub resolution_metadata: Vec<u8>,
}

#[derive(Clone, AnchorSerialize, AnchorDeserialize, Debug)]
pub(super) struct ExecuteResolutionMetadata {
    pub account_list_lengths: OfframpAccountListLengths,
    pub message_id: [u8; 32],
}

/// Result of `resolve_exec_inputs`: the raw execution-inputs bytes plus the buffer
/// pubkey (if buffered mode was used).
pub(crate) struct ResolvedExecInputs {
    pub encoded_message: Vec<u8>,
    pub ccvs: Vec<Pubkey>,
    pub verifier_results: Vec<Vec<u8>>,
    pub buffer_account: Option<Pubkey>,
    pub buffer_id: Option<[u8; 32]>,
}

/// Resolves the execution inputs from either the instruction data (direct mode) or
/// a buffer account (buffered mode). Shared between the on-chain executor and the
/// account-resolution stages so they can't drift on what counts as a valid input.
///
/// When `exec_params.exec_inputs.is_none()`, `buffer_info` MUST be `Some` — otherwise
/// `buffer_info` is ignored.
pub(super) fn resolve_exec_inputs(
    exec_params: &ExecuteParams,
    buffer_info: Option<&AccountInfo>,
    authority: Pubkey,
) -> Result<ResolvedExecInputs> {
    let (inputs, buffer_account, buffer_id) = if let Some(inputs) = &exec_params.exec_inputs {
        require!(buffer_info.is_none(), CcipOfframpError::RedundantBuffer);
        (inputs.clone(), None, None)
    } else {
        let buffer = buffer_info.ok_or(CcipOfframpError::InvalidInstructionData)?;
        let (deserialized, _len, buffer_id) = buffer.deserialize_as_buffer(authority)?;
        (deserialized, Some(*buffer.key), Some(buffer_id))
    };

    require_eq!(
        inputs.ccvs.len(),
        inputs.verifier_results.len(),
        CcipOfframpError::InvalidInstructionData
    );

    Ok(ResolvedExecInputs {
        encoded_message: inputs.encoded_message,
        ccvs: inputs.ccvs,
        verifier_results: inputs.verifier_results,
        buffer_account,
        buffer_id,
    })
}

/// Decides whether the message's receiver uses the CCIP v2 interface, by reading the router-owned
/// `ReceiverRegistry` PDA resolved into the `receiver_registry` layout segment. This
/// is the OffRamp's single authoritative source for the v1/v2 receiver flow, replacing the old
/// executor-supplied hint.
fn receiver_is_v2_from_layout(
    registry_account: Option<&AccountInfo>,
    receiver_program: Pubkey,
    router: Pubkey,
) -> Result<bool> {
    let Some(registry) = registry_account else {
        return Err(CcipOfframpError::ReceiverRegistryAccountMissing.into());
    };
    ccip_common::router_accounts::receiver_is_v2_from_account(registry, receiver_program, router)
}

/// CCIPv2 message execution.
/// CCV verification and legacy token transfers.
pub fn execute_v2<'info>(
    ctx: Context<'info, ExecuteV2<'info>>,
    params: ExecuteParams,
) -> Result<()> {
    let resolution_metadata =
        ExecuteResolutionMetadata::try_from_slice(&params.resolution_metadata).map_err(|e| {
            msg!(
                "Failed to deserialize resolution metadata: {:?}. Metadata: {:?}",
                e,
                params.resolution_metadata
            );
            CcipOfframpError::InvalidMetadata
        })?;

    let layout = ExecuteRemainingAccountsLayout::new(
        ctx.remaining_accounts,
        &resolution_metadata.account_list_lengths,
    )?;
    let exec_inputs =
        resolve_exec_inputs(&params, layout.buffer_account, ctx.accounts.authority.key())?;

    let message_id = MessageV1::compute_message_id_from_bytes(&exec_inputs.encoded_message);
    let mut message = MessageV1::from_bytes(&exec_inputs.encoded_message)
        .map_err(|_| CcipOfframpError::InvalidInstructionData)?;

    require!(
        resolution_metadata.message_id == message_id,
        CcipOfframpError::InvalidInstructionData
    );

    verify_uncursed_cpi(
        &ctx.accounts.rmn_remote,
        &ctx.accounts.rmn_remote_curses,
        &ctx.accounts.rmn_remote_config,
        message.source_chain_selector,
    )?;

    // Initialize state on first execution (version == 0 means just created by init_if_needed)
    if ctx.accounts.message_exec_state.version == 0 {
        ctx.accounts.message_exec_state.set_inner(MessageState {
            version: MessageState::MAX_V,
            bump: ctx.bumps.message_exec_state,
            execution_state: MessageExecutionState::InProgress, // this gets overwritten later
            message_id,
        });
    }

    let state = &ctx.accounts.message_exec_state.execution_state;
    if *state == MessageExecutionState::Success {
        msg!("Message already executed successfully, skipping execution");
        close_buffer(&ctx, exec_inputs.buffer_id, layout)?;
        return Ok(());
    }

    let input_ccvs = &exec_inputs.ccvs;

    // Message/lane validation. Mirrors the checks EVM's `OffRamp.execute` runs before it writes
    // `s_executionStates[messageId] = IN_PROGRESS`, so the InProgress emission below marks exactly
    // the same point in the lifecycle.

    // Binds the `source_chain` account to the message. It also transitively binds
    // `allowed_offramp`, whose seed is the same selector. Must precede every read of
    // `source_chain`.
    require_eq!(
        message.source_chain_selector,
        ctx.accounts.source_chain.chain_selector,
        CcipOfframpError::InvalidMessageSourceChainSelector
    );

    let source_chain_cfg = &ctx.accounts.source_chain.config;

    // Specified source chain config must actually match the source chain specified by the message
    require_eq!(
        message.source_chain_selector,
        ctx.accounts.source_chain.chain_selector,
        CcipOfframpError::InvalidInstructionData,
    );

    require!(
        source_chain_cfg.is_enabled,
        CcipOfframpError::UnsupportedSourceChainSelector
    );

    // Any of the configured OnRamps may have sent this message. Several are allowed at once so the
    // source chain can upgrade its OnRamp without downtime: messages already inflight from the old
    // OnRamp stay executable (including manually) while the new one starts sending. Execution state
    // cannot collide across OnRamps because the `message_exec_state` PDA is seeded by the message id,
    // which commits to `on_ramp_address`.
    require!(
        source_chain_cfg.allows_on_ramp(message.on_ramp_address.as_slice()),
        CcipOfframpError::InvalidOnrampAddress
    );

    require!(
        // require_eq expects slices to implement Display
        message.off_ramp_address.as_slice() == crate::ID.as_ref(),
        CcipOfframpError::InvalidOfframpAddress
    );

    require_eq!(
        message.dest_chain_selector,
        ctx.accounts.config.svm_chain_selector,
        CcipOfframpError::InvalidMessageDestChainSelector
    );

    require_eq!(
        exec_inputs.verifier_results.len(),
        input_ccvs.len(),
        CcipOfframpError::InvalidInputsNumberOfCCVs
    );
    require_eq!(
        layout.ccv_accounts_per_ccv.len(),
        input_ccvs.len(),
        CcipOfframpError::InvalidInputsNumberOfCCVs
    );

    // Marks the start of the execution attempt, at the point where EVM transitions the stored
    // state to IN_PROGRESS — i.e. after the validation above and before CCV verification, the
    // token transfer and the receiver CPI.
    //
    // EVM runs that trio inside `executeSingleMessage`, wrapped in a try/catch, so a failure there
    // still lands an `ExecutionStateChanged(FAILURE, err)`. Solana has no try/catch: any of those
    // failing aborts the transaction and rolls back state. Emitting here is what preserves the
    // signal, because a failed transaction is still committed to the ledger with the logs it
    // produced before aborting. Offchain therefore reads (InProgress observed, tx.Meta.Err) as the
    // equivalent of EVM's terminal FAILURE event — see pkg/destinationreader/execution_attempt_poller.go.
    //
    // Anything emitted after the CCV loop would be rolled back by a verification failure, leaving
    // no trace of the attempt at all.
    emit_cpi!(ExecutionStateChangedV2 {
        source_chain_selector: message.source_chain_selector,
        message_id,
        state: MessageExecutionState::InProgress,
        message_number: message.message_number,
        return_data: vec![],
    });

    // Everything below corresponds to EVM's `executeSingleMessage`: resolve the CCV requirements,
    // verify, release-or-mint, then call the receiver.

    // The dest_blob has two consumers during execution: the token-transfer path (which reads the
    // token-receiver ATA usage) and the receiver CPI (which reads the account list + writability
    // bitmap). Parse it once here and share it with both — every parse allocates its accounts vec
    // from the fixed 32KB heap, which is never reclaimed. The guard is exactly the union of the two
    // consumers' own guards, so a message that reaches neither still never parses the blob.
    let dest_blob = if message.is_token_transfer() || message.is_offramp_arbitrary_message() {
        Some(SVMDestBlob::from_bytes(&message.dest_blob).map_err(|e| {
            msg!("Error decoding destBlob: {:#?}", e);
            CcipOfframpError::InvalidDestBlob
        })?)
    } else {
        None
    };

    let mut token_transfer_context = token_transfer::TokenTransferContext::new(
        &message,
        &ctx,
        &ctx.accounts.token_admin_registry,
        &layout,
        dest_blob.as_ref(),
    )?;

    let router = ctx.accounts.reference_addresses.router;
    let mut receiver_is_v2 = false;
    let mut receiver_required_ccvs: Option<ReceiverRequiredCcvs> = None;
    // Resolve the receiver's CCV/finality contribution (shared with the `get_ccvs_for_msg` view so
    // the authoritative and predicted sets can't diverge). Only arbitrary (data) messages consult
    // the receiver; for v2 receivers the receiver program is bound to `message.receiver`.
    // `is_offramp_arbitrary_message()` guarantees a 32-byte, non-default receiver, so `parse_pubkey` cannot
    // fail on that path (the value is unused for token-only transfers).
    if message.is_offramp_arbitrary_message() {
        let expected_receiver = parse_pubkey(&message.receiver)?;
        receiver_is_v2 = receiver_is_v2_from_layout(
            layout.receiver_registry_account,
            expected_receiver,
            router,
        )?;
        receiver_required_ccvs = ccv_resolution::resolve_receiver_required_ccvs(
            message.is_offramp_arbitrary_message(),
            receiver_is_v2,
            layout.receiver_ccv_accounts,
            expected_receiver,
            message.source_chain_selector,
            message.sender.clone(),
            message.finality,
        )?;
    }

    let ccv_requirements = ccv_resolution::get_ccvs_for_message(
        &source_chain_cfg.default_ccvs,
        &source_chain_cfg.lane_mandated_ccvs,
        token_transfer_context.take_pool_ccv_source(),
        receiver_required_ccvs,
    )?;
    let ccv_indexes_to_query = ensure_ccv_quorum_is_reached(input_ccvs, &ccv_requirements)?;

    // We create the ATA early and track the balance of the receiver prior to verification because a
    // verifier may be responsible for releasing or minting the token.
    let token_transfer_init_bal = match token_transfer_context {
        token_transfer::TokenTransferContext::V1(ref accounts, _) => {
            // the option is only Some(value) when token_receiver_usage.should_create_ata() is true
            if let Some(user_receiver_wallet) = accounts.user_receiver_wallet {
                create_idempotent_ata(
                    user_receiver_wallet,
                    &ctx.accounts.authority.to_account_info(),
                    accounts.common.mint,
                    accounts.common.token_program,
                    accounts.common.user_token_account,
                    &ctx.accounts.system_program.to_account_info(),
                )?;
            }
            Some(token_transfer::get_balance(
                accounts.common.user_token_account,
            )?)
        }
        token_transfer::TokenTransferContext::V2(ref accounts, _, _) => {
            // the option is only Some(value) when token_receiver_usage.should_create_ata() is true
            if let Some(user_receiver_wallet) = accounts.user_receiver_wallet {
                create_idempotent_ata(
                    user_receiver_wallet,
                    &ctx.accounts.authority.to_account_info(),
                    accounts.common.mint,
                    accounts.common.token_program,
                    accounts.common.user_token_account,
                    &ctx.accounts.system_program.to_account_info(),
                )?;
            }
            Some(token_transfer::get_balance(
                accounts.common.user_token_account,
            )?)
        }
        token_transfer::TokenTransferContext::None => None,
    };

    for ccv_index in ccv_indexes_to_query {
        let ccv = input_ccvs[ccv_index];
        let accounts = layout
            .ccv_accounts_per_ccv
            .get(ccv_index)
            .ok_or(CcipOfframpError::InvalidInputsNumberOfCCVs)?;

        // The input_ccv at this index (from the signed protocol message) is verified to match the
        // accounts at the same index inside `call_verify`, which requires `ccv_accounts[0] == ccv`
        // before invoking. So a mismatch between the signed CCV and the supplied account group is
        // rejected there.
        let results = exec_inputs
            .verifier_results
            .get(ccv_index)
            .ok_or(CcipOfframpError::InvalidInputsNumberOfCCVs)?;

        // Pretty large clones, but need to happen somewhere because of CPI. If this becomes a CU
        // problem, we could serialize the shared part of the VerifyParams once and only prepend the
        // CCV-specific part on each iteration.
        call_verify(
            ccv,
            accounts,
            exec_inputs.encoded_message.clone(),
            message_id,
            results.to_vec(),
        )?;
    }

    // If we have a token transfer, perform it. If we don't, this is a NOOP. The delivered token
    // transfer (if any) is forwarded to the receiver in the `Any2SVMMessage`. It carries the local
    // token amount plus the `token_receiver` + usage the OffRamp credited.
    let delivered_token_transfer =
        token_transfer_context.execute(&ctx, &message, token_transfer_init_bal)?;

    // If the message carries a receiver (i.e. it's not a pure token transfer), CPI into it. If the
    // receiver reverts, so does the whole execution.

    if message.is_offramp_arbitrary_message() {
        let dest_blob = dest_blob.as_ref().expect("Guard checked above");
        call_receiver(
            &ctx,
            &layout,
            &mut message,
            message_id,
            delivered_token_transfer,
            receiver_is_v2,
            dest_blob,
        )?;
    }

    // EVM's single terminal `ExecutionStateChanged`. Only the Success case is reachable here: the
    // Failure counterpart would require surviving a failed execution, which Solana rolls back.
    // `return_data` is empty for the same reason — it carries EVM's caught revert data, and on
    // success EVM passes empty bytes too.
    let next_state = MessageExecutionState::Success;
    let state = &mut ctx.accounts.message_exec_state.execution_state;
    *state = next_state;
    emit_cpi!(ExecutionStateChangedV2 {
        source_chain_selector: message.source_chain_selector,
        message_id,
        state: next_state,
        message_number: message.message_number,
        return_data: vec![],
    });

    close_buffer(&ctx, exec_inputs.buffer_id, layout)?;

    Ok(())
}

// Close the buffer account if it was used. Invoke this at the end (similar to what anchor's `close` would do)
// so that the lamport math on the in-memory structs is persisted onchain right after this is done, and therefore
// avoiding errors on runtime checks performed during CPIs where the in-memory state differs from the onchain one.
fn close_buffer<'info>(
    ctx: &Context<'info, ExecuteV2<'info>>,
    buffer_id: Option<[u8; 32]>,
    layout: ExecuteRemainingAccountsLayout<'_, 'info>,
) -> Result<()> {
    let Some(buffer_info) = layout.buffer_account else {
        return Ok(()); // if there's no buffer, there's nothing to close
    };

    // since there is a buffer account, there must be a buffer id (otherwise it is a dev error)
    let buffer_id = buffer_id.expect("Buffer account exists, so buffer_id must be Some");

    require!(
        buffer_info.is_writable,
        CcipOfframpError::InvalidInputsMissingWritable
    );
    ccip_common::account_management::close(buffer_info, &ctx.accounts.authority.to_account_info())?;
    emit_cpi!(BufferClosed {
        buffer_id,
        authority: ctx.accounts.authority.key(),
    });

    Ok(())
}

/// CPI into the message's receiver program via its `ccip_receive` (v1) or `ccip_receive_v2` (v2)
/// entrypoint, selected by `receiver_is_v2`.
///
/// The messaging segment is laid out as `[receiver_program, external_execution_signer,
/// ..receiver_accounts]` (see `resolve_accounts::second`). Both interface versions require the CPI
/// account list to begin with `[external_execution_signer (signer), offramp, allowed_offramp]`
/// followed by the receiver-specific accounts (which, for a v2 receiver, include its
/// `#[event_cpi]` `event_authority` + `program` accounts, carried in the dest_blob). The offramp
/// signs the CPI with the `external_execution_signer` PDA, which proves to the receiver that the
/// call originated from this offramp. Only the instruction payload differs between versions: v1
/// serializes `Any2SVMMessage` (`token_amounts: Vec`), v2 serializes `Any2SVMMessageV2`
/// (`token_amount: Option`) under the `ccip_receive_v2` discriminator.
// Takes `message` by `&mut` so it can move `sender`/`data` out (via `std::mem::take`) instead of
// cloning them — those two fields are not read again after this call.
fn call_receiver<'info>(
    ctx: &Context<'info, ExecuteV2<'info>>,
    layout: &ExecuteRemainingAccountsLayout<'_, 'info>,
    message: &mut MessageV1,
    message_id: [u8; 32],
    delivered_token_transfer: Option<TokenTransfer>,
    receiver_is_v2: bool,
    dest_blob: &SVMDestBlob,
) -> Result<()> {
    let messaging = layout.messaging_accounts;
    // `is_offramp_arbitrary_message()` guarantees a receiver, so resolution added at least the receiver
    // program and the external_execution_signer PDA.
    require_gte!(
        messaging.len(),
        2,
        CcipOfframpError::InvalidAccountListLengths
    );
    let receiver_program = &messaging[0];
    let external_execution_signer = &messaging[1];
    let receiver_accounts = &messaging[2..];

    // Re-derive the external_execution_signer PDA (and its bump) rather than trusting the resolved
    // account: only this offramp can sign for it, so it is what authenticates the CPI to the receiver.
    let (expected_signer, signer_bump) = Pubkey::find_program_address(
        &[
            seed::EXTERNAL_EXECUTION_CONFIG,
            receiver_program.key.as_ref(),
        ],
        &crate::ID,
    );
    require_keys_eq!(
        external_execution_signer.key(),
        expected_signer,
        CcipOfframpError::InvalidExternalExecutionSigner
    );

    // SECURITY (INV-SOL-RECV-2): the receiver CPI target program and its receiver-specific account
    // list are authoritative parts of the SIGNED message — `message.receiver` and the `dest_blob`
    // parsed from `message.dest_blob` by the caller both live inside the CCV-verified
    // `encoded_message`. The `messaging` slice, by contrast, is executor-supplied (sliced out of
    // `remaining_accounts` using executor-supplied `AccountListLengths`).
    let expected_receiver = parse_pubkey(&message.receiver)?;
    require_keys_eq!(
        receiver_program.key(),
        expected_receiver,
        CcipOfframpError::InvalidReceiverAddress
    );

    // The writability bitmap is a u64, so at most 64 accounts can be addressed; keep the same bound
    // resolution enforces so `is_writable`'s index stays valid and can't be silently truncated.
    require_gte!(
        MAX_MESSAGING_ACCOUNTS,
        dest_blob.accounts.len(),
        CcipOfframpError::TooManyMessagingAccounts
    );
    require_eq!(
        receiver_accounts.len(),
        dest_blob.accounts.len(),
        CcipOfframpError::InvalidReceiverAccounts
    );
    for (i, expected_key) in dest_blob.accounts.iter().enumerate() {
        let acc = &receiver_accounts[i];
        require_keys_eq!(
            acc.key(),
            *expected_key,
            CcipOfframpError::InvalidReceiverAccounts
        );
        // Feasibility: an account the source marked writable must be passed writable at the tx
        // level, else the writable CPI meta built below can't be honored. A source-readonly account
        // passed tx-writable (e.g. because it's writable in another section of the same tx) is
        // fine — the CPI meta forces it readonly for the receiver.
        //
        // SAFETY: `i < dest_blob.accounts.len() <= MAX_MESSAGING_ACCOUNTS` (64) per the
        // `require_gte!` above, so the index cast cannot truncate.
        require!(
            !is_writable(&dest_blob.account_is_writable_bitmap, i as u8) || acc.is_writable,
            CcipOfframpError::InvalidReceiverAccounts
        );
    }

    // Move (don't clone) the heavy vecs out of `message`; they are unused after this call.
    let source_chain_selector = message.source_chain_selector;
    let sender = std::mem::take(&mut message.sender);
    let data_field = std::mem::take(&mut message.data);
    let data = if receiver_is_v2 {
        Any2SVMMessageV2 {
            message_id,
            source_chain_selector,
            sender,
            data: data_field,
            token_transfer: delivered_token_transfer,
        }
        .build_receiver_discriminator_and_data()?
    } else {
        Any2SVMMessage {
            message_id,
            source_chain_selector,
            sender,
            data: data_field,
            // 0 or 1 element: a v2 message carries at most one token transfer. The v1 receiver
            // interface only exposes the bare amount, so the `token_receiver` + usage are dropped.
            token_amounts: delivered_token_transfer
                .map(|t| t.token_amount)
                .into_iter()
                .collect(),
        }
        .build_receiver_discriminator_and_data()?
    };

    // CPI accounts: the external_execution_signer (as signer), the offramp and allowed_offramp
    // (fixed `ExecuteV2` accounts), then the receiver-specific accounts, each with the writability
    // taken from the CCV-authenticated dest_blob bitmap (validated above).
    // SAFETY: a capacity hint only — a wrong value costs a realloc, never correctness, and the operands are account counts bounded by the per-tx account limit.
    let mut acc_metas = Vec::with_capacity(3 + receiver_accounts.len());
    // SAFETY: a capacity hint only — a wrong value costs a realloc, never correctness, and the operands are account counts bounded by the per-tx account limit.
    let mut acc_infos = Vec::with_capacity(4 + receiver_accounts.len());

    acc_metas.push(AccountMeta::new_readonly(
        external_execution_signer.key(),
        true,
    ));
    acc_metas.push(AccountMeta::new_readonly(ctx.accounts.offramp.key(), false));
    acc_metas.push(AccountMeta::new_readonly(
        ctx.accounts.allowed_offramp.key(),
        false,
    ));
    acc_infos.push(external_execution_signer.to_account_info());
    acc_infos.push(ctx.accounts.offramp.to_account_info());
    acc_infos.push(ctx.accounts.allowed_offramp.to_account_info());

    for (i, acc) in receiver_accounts.iter().enumerate() {
        // Writability comes from the authenticated dest_blob bitmap, not the executor-supplied
        // tx-level flag, so the receiver sees exactly the writability the sender signed at source.
        //
        // SAFETY: `receiver_accounts.len() == dest_blob.accounts.len() <= MAX_MESSAGING_ACCOUNTS`
        // (64) per the checks at the top of this function, so the index cast cannot truncate.
        acc_metas.push(
            if is_writable(&dest_blob.account_is_writable_bitmap, i as u8) {
                AccountMeta::new(acc.key(), false)
            } else {
                AccountMeta::new_readonly(acc.key(), false)
            },
        );
        acc_infos.push(acc.to_account_info());
    }

    // The invoked program's account must be available to the runtime for the CPI.
    acc_infos.push(receiver_program.to_account_info());

    let instruction = Instruction {
        program_id: receiver_program.key(),
        accounts: acc_metas,
        data,
    };

    let signer_seeds: &[&[u8]] = &[
        seed::EXTERNAL_EXECUTION_CONFIG,
        receiver_program.key.as_ref(),
        &[signer_bump],
    ];
    invoke_signed(&instruction, &acc_infos, &[signer_seeds])?;

    Ok(())
}

fn ensure_ccv_quorum_is_reached(
    input_ccvs: &[Pubkey],
    ccv_requirements: &GetCcvsForMsgResponse,
) -> Result<Vec<usize>> {
    let mut ccv_indexes_to_query = Vec::with_capacity(
        ccv_requirements.required_ccvs.len() + ccv_requirements.optional_threshold as usize,
    );

    for required_ccv in &ccv_requirements.required_ccvs {
        let index = input_ccvs
            .iter()
            .position(|ccv| ccv == required_ccv)
            .ok_or(CcipOfframpError::RequiredCCVMissing)?;
        ccv_indexes_to_query.push(index);
    }

    let mut optional_ccvs_to_find = ccv_requirements.optional_threshold as usize;
    for optional_ccv in &ccv_requirements.optional_ccvs {
        if optional_ccvs_to_find == 0 {
            break;
        }

        if let Some(index) = input_ccvs.iter().position(|ccv| ccv == optional_ccv) {
            optional_ccvs_to_find -= 1;
            ccv_indexes_to_query.push(index);
        }
    }

    require_eq!(
        optional_ccvs_to_find,
        0,
        CcipOfframpError::OptionalCCVQuorumNotReached
    );

    Ok(ccv_indexes_to_query)
}

pub(crate) fn parse_pubkey(bytes: &[u8]) -> Result<Pubkey> {
    let bytes: [u8; 32] = bytes
        .try_into()
        .map_err(|_| CcipOfframpError::InvalidInputPubkeyBytes)?;
    Ok(Pubkey::new_from_array(bytes))
}

pub(crate) fn create_idempotent_ata<'info>(
    wallet: &AccountInfo<'info>,
    payer: &AccountInfo<'info>,
    mint: &AccountInfo<'info>,
    token_program: &AccountInfo<'info>,
    ata: &AccountInfo<'info>,
    system_program: &AccountInfo<'info>,
) -> Result<()> {
    let ix = spl_associated_token_account::instruction::create_associated_token_account_idempotent(
        payer.key,
        wallet.key,
        mint.key,
        token_program.key,
    );

    invoke(
        &ix,
        &[
            payer.clone(),
            ata.clone(),
            wallet.clone(),
            mint.clone(),
            system_program.clone(),
            token_program.clone(),
        ],
    )
    .map_err(Into::into)
}

pub(crate) struct ExecuteRemainingAccountsLayout<'a, 'info> {
    pub buffer_account: Option<&'a AccountInfo<'info>>,
    pub token_transfer_accounts: &'a [AccountInfo<'info>],
    /// Accounts for the receiver `ccip_receive` CPI, laid out as
    /// `[receiver_program, external_execution_signer, ..receiver_accounts]` (see `resolve_accounts::second`).
    /// Empty when the message is a pure token transfer (no receiver).
    pub messaging_accounts: &'a [AccountInfo<'info>],
    /// Accounts for the receiver's `get_ccvs_and_finality_config` CPI (CCIP v2), laid out as
    /// `[receiver_program, source_chain_ccv_config]` (see `resolve_accounts::second`). Empty for a
    /// v1 receiver or a token-only transfer.
    pub receiver_ccv_accounts: &'a [AccountInfo<'info>],
    /// The router-owned `ReceiverRegistry`: a single account for an arbitrary
    /// message, empty for a token-only transfer. Read to select the v1 vs v2 receiver flow.
    pub receiver_registry_account: Option<&'a AccountInfo<'info>>,
    pub ccv_accounts_per_ccv: Vec<&'a [AccountInfo<'info>]>,
}

impl<'a, 'info> ExecuteRemainingAccountsLayout<'a, 'info> {
    pub fn new(
        mut remaining_accounts: &'a [AccountInfo<'info>],
        acc_lengths: &OfframpAccountListLengths,
    ) -> Result<Self> {
        acc_lengths
            .validate_with(remaining_accounts)
            .inspect_err(|_| {
                msg!(
                    "Account list lengths validation failed. AccountListLengths: {:?}. Remaining accounts: {:?}",
                    acc_lengths,
                    remaining_accounts
                );
            })?;

        let buffer_accounts = &remaining_accounts[..acc_lengths.buffer as usize];
        remaining_accounts = &remaining_accounts[acc_lengths.buffer as usize..];

        let messaging_accounts = &remaining_accounts[..acc_lengths.messaging as usize];
        remaining_accounts = &remaining_accounts[acc_lengths.messaging as usize..];

        // The `ReceiverRegistry` PDA and the receiver-CCV-query segment sit between messaging and
        // token_transfer (matching the order `resolve_accounts::second` appends them:
        // registry first, then receiver_ccv).
        let receiver_registry_accounts =
            &remaining_accounts[..acc_lengths.receiver_registry as usize];
        remaining_accounts = &remaining_accounts[acc_lengths.receiver_registry as usize..];

        let receiver_ccv_accounts = &remaining_accounts[..acc_lengths.receiver_ccv as usize];
        remaining_accounts = &remaining_accounts[acc_lengths.receiver_ccv as usize..];

        let token_transfer_accounts = &remaining_accounts[..acc_lengths.token_transfer as usize];
        remaining_accounts = &remaining_accounts[acc_lengths.token_transfer as usize..];

        let mut ccv_accounts_per_ccv = Vec::with_capacity(acc_lengths.ccvs.len());
        for &len in acc_lengths.ccvs.iter() {
            let ccv_accounts = &remaining_accounts[..len as usize];
            remaining_accounts = &remaining_accounts[len as usize..];

            ccv_accounts_per_ccv.push(ccv_accounts);
        }

        // This is redundant here given the check at the beginning, but can help catch dev errors.
        // It can be removed later.
        require_eq!(
            0,
            remaining_accounts.len(),
            CcipOfframpError::InvalidAccountListLengths
        );

        Ok(Self {
            buffer_account: buffer_accounts.first(),
            token_transfer_accounts,
            messaging_accounts,
            receiver_ccv_accounts,
            receiver_registry_account: receiver_registry_accounts.first(),
            ccv_accounts_per_ccv,
        })
    }
}

fn call_verify<'info>(
    ccv: Pubkey,
    ccv_accounts: &[AccountInfo<'info>],
    encoded_message: Vec<u8>,
    message_id: [u8; 32],
    verifier_results: Vec<u8>,
) -> Result<()> {
    // Only the CCV program account itself is required here, so this check makes no assumption
    // about any particular verifier's account layout: a verifier that needs more accounts (the
    // committee-verifier, for instance, also takes its config and remote_chain_config) validates
    // them itself.
    require_gte!(
        ccv_accounts.len(),
        // At minimum the CCV program account itself; per-CCV account validation is the verifier's responsibility.
        1,
        CcipOfframpError::InvalidAccountListLengths
    );

    let params = verifier::VerifyParams {
        encoded_message,
        message_id,
        verifier_results,
    };

    require_keys_eq!(
        ccv_accounts[0].key(),
        ccv,
        CcipOfframpError::InvalidAccountListLengths
    );

    // Only allow ccv_signer (derived per CCV to avoid cross-CCV confusion) as a signer in the verifier call
    let (ccv_signer, ccv_signer_bump) =
        Pubkey::find_program_address(&[seed::CCV_SIGNER, ccv.as_ref()], &crate::ID);

    let participant_accounts = &ccv_accounts[1..];
    let mut account_metas = Vec::with_capacity(participant_accounts.len());
    for acc_info in participant_accounts {
        let is_ccv_signer = acc_info.key() == ccv_signer;
        if !is_ccv_signer {
            require!(
                !acc_info.is_signer,
                CcipOfframpError::InvalidInputsUnexpectedSigner,
            );
        }
        account_metas.push(AccountMeta {
            pubkey: *acc_info.key,
            is_signer: is_ccv_signer,
            is_writable: acc_info.is_writable,
        });
    }

    let ix = Instruction {
        program_id: ccv,
        accounts: account_metas,
        data: verifier::instruction::Verify { params }.data(),
    };
    let ccv_signer_seeds: &[&[u8]] = &[seed::CCV_SIGNER, ccv.as_ref(), &[ccv_signer_bump]];
    invoke_signed(&ix, participant_accounts, &[ccv_signer_seeds])?;
    Ok(())
}

/// Helper used inside the `seeds = [...]` constraint of `message_exec_state`.
/// Anchor evaluates account constraints with no access to handler state, so the seed must come
/// out of `params.resolution_metadata` directly. Returns zeros on malformed input so the PDA
/// mismatch surfaces as a normal seed-mismatch error rather than a panic; the handler
/// re-validates the asserted id against the resolved message either way.
fn message_id_from_params(params: &ExecuteParams) -> [u8; 32] {
    ExecuteResolutionMetadata::try_from_slice(&params.resolution_metadata)
        .map(|m| m.message_id)
        .unwrap_or_else(|_| {
            msg!("Failed to deserialize resolution metadata for PDA seed; returning zeros");
            [0; 32]
        })
}

#[event_cpi]
#[derive(Accounts)]
#[instruction(params: ExecuteParams)]
pub struct ExecuteV2<'info> {
    #[account(
        seeds = [seed::CONFIG],
        bump = config.bump,
        constraint = config.version == Config::MAX_V @ CcipOfframpError::InvalidVersion,
    )]
    pub config: Account<'info, Config>,

    #[account(
        seeds = [seed::REFERENCE_ADDRESSES],
        bump = reference_addresses.bump,
        constraint = reference_addresses.version == ReferenceAddresses::MAX_V @ CcipOfframpError::InvalidVersion,
    )]
    pub reference_addresses: Account<'info, ReferenceAddresses>,

    #[account(
        // The selector seed is self-referencing — it is read off the account being validated — so
        // this constraint only proves the account is a canonical `SourceChain` PDA, not that it is
        // the one for the message being executed. `execute_v2` closes that gap with an explicit
        // `require_eq!` against `message.source_chain_selector` before it reads any lane policy.
        seeds = [seed::SOURCE_CHAIN, source_chain.chain_selector.to_le_bytes().as_ref()],
        bump = source_chain.bump,
        constraint = source_chain.version == SourceChain::MAX_V @ CcipOfframpError::InvalidVersion,
    )]
    pub source_chain: Account<'info, SourceChain>,

    // PDA seed comes from the message_id bundled in resolution_metadata.
    // The handler re-derives the id from the resolved message bytes and rejects mismatches, so
    // buffered execution can satisfy the constraint without Anchor needing buffer-account access.
    #[account(
        init_if_needed,
        seeds = [seed::MESSAGE_EXEC_STATE, &message_id_from_params(&params)],
        bump,
        constraint = {
            message_exec_state.version == 0 // uninitialized case
            || message_exec_state.version == MessageState::MAX_V // initialized case
        } @ CcipOfframpError::InvalidVersion,
        payer = authority,
        space = MessageState::DISCRIMINATOR.len() + MessageState::INIT_SPACE, // if the account is being initialized, it needs space allocated; if it's not being initialized, this doesn't do anything
    )]
    pub message_exec_state: Account<'info, MessageState>,

    pub offramp: Program<'info, CcipOfframp>,

    // PERMISSIONLESS: Execution is permissionless in CCIP V2
    #[account(mut)]
    pub authority: Signer<'info>,
    pub system_program: Program<'info, System>,

    /// CHECK: This is a sysvar account
    #[account(address = Instructions::id() @ CcipOfframpError::InvalidInputsSysvarAccount)]
    pub sysvar_instructions: UncheckedAccount<'info>,

    /// CHECK: When there is a token transfer, this contains the token admin registry entry for the transferred
    /// token. When there isn't, it's the zero pubkey. This is stored separately to the token accounts as it's
    /// necessary to determine the schema for the token account layout.
    pub token_admin_registry: UncheckedAccount<'info>,

    /// CHECK PDA of the router program verifying the signer is an allowed offramp.
    /// If PDA does not exist, the router doesn't allow this offramp. This is just used
    /// so that token pools and receivers can then check that the caller is an actual offramp that
    /// has been registered in the router as such for that source chain.
    /// The selector seed comes from `source_chain`, which `execute_v2` binds to the message, so this
    /// authorization is effectively checked against the message's own source chain.
    #[account(
        owner = reference_addresses.router @ CcipOfframpError::InvalidInputsAllowedOfframpAccount, // this guarantees that it was initialized
        seeds = [seed::ALLOWED_OFFRAMP, source_chain.chain_selector.to_le_bytes().as_ref(), offramp.key().as_ref()],
        bump,
        seeds::program = reference_addresses.router,
    )]
    pub allowed_offramp: UncheckedAccount<'info>,

    ////////////////////
    // RMN Remote CPI //
    ////////////////////
    /// CHECK: This is the account for the RMN Remote program
    #[account(
        address = reference_addresses.rmn_remote @ CcipOfframpError::InvalidRMNRemoteAddress,
    )]
    pub rmn_remote: UncheckedAccount<'info>,

    /// CHECK: This account is just used in the CPI to the RMN Remote program
    #[account(
        seeds = [seed::CURSES],
        bump,
        seeds::program = reference_addresses.rmn_remote,
    )]
    pub rmn_remote_curses: UncheckedAccount<'info>,

    /// CHECK: This account is just used in the CPI to the RMN Remote program
    #[account(
        seeds = [seed::CONFIG],
        bump,
        seeds::program = reference_addresses.rmn_remote,
    )]
    pub rmn_remote_config: UncheckedAccount<'info>,
    // remaining accounts (most are optional), in this exact order — see
    // `ExecuteRemainingAccountsLayout::new` and the `AccountListLengths` field order:
    // 1. ExecInputs buffer (0 or 1)
    // 2. Messaging things
    // 3. Pool / token-transfer things
    // 4. CCV things
}
