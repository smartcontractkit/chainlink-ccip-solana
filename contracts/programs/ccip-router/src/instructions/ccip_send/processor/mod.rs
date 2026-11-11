use anchor_lang::prelude::*;
use anchor_lang::solana_program::{instruction::Instruction, program::invoke_signed};
use anchor_lang::InstructionData;
use anchor_spl::associated_token::get_associated_token_address_with_program_id;
use anchor_spl::token::spl_token::native_mint;
use anchor_spl::token_interface::{Mint, TokenAccount, TokenInterface};

use base_token_pool::v2::{LockOrBurnInV2, LockOrBurnOutV2};
use ccip_common::cpi::{get_verified_cpi_return_data, parse_verified_cpi_return_as};
use ccip_common::extra_args::GenericExtraArgsV3;
use ccip_common::types::{CrossChainGas, ToTxData};
use ccip_common::v1::CcipV1OnrampTokenAccounts;
use ccip_common::v2::{CcipV2OnrampTokenAccounts, ProtocolAmount};
use committee_verifier as verifier;

use ccip_common::messages::{
    normalized_address, MessageV1, NormalizeWsol, SVMTokenAmount, TokenTransferV1,
};
use ccip_common::seed;
use ethnum::U256;
use fee_quoter::messages::TokenTransferAdditionalData;
use fee_quoter::messages_v2::ValidatedTokenPriceV2;

use crate::context::{valid_version, MAX_CHAINSTATECCIPV2_V, MAX_CONFIG_V};
use crate::error::CcipRouterError;
use crate::instructions::common::{
    collect_receipts, get_token_price_from_fee_quoter_cpi, validate_token_transfer,
    FeeQuoterCpiAccounts, OnrampRemainingAccountsLayout, OnrampResolutionMetadata,
    PreflightValidatedTokenTransfer, QuoteGasForExecV2CpiInputs, TokenTransferContext,
};
use crate::messages::{Receipt, SVM2AnyMessage, SVM2AnyTokenTransfer};
use crate::rmn::verify_uncursed_cpi;
use crate::state::{Config, DestChainCcipV2};
use crate::{
    build_token_transfer_legacy, events, interact_with_pool_legacy, transfer_token_cpi_legacy,
    LockOrBurnInLegacy, LockOrBurnOutLegacy,
};

pub mod extra_args;
pub mod fees;

#[derive(AnchorSerialize, AnchorDeserialize, Debug)]
pub struct CcipSendV2Params {
    pub dest_chain_selector: u64,
    pub message: SVM2AnyMessage,

    /// Opaque account-resolution metadata produced by account resolution.
    pub resolution_metadata: Vec<u8>,
}

pub fn ccip_send_v2<'info>(
    mut ctx: Context<'info, CcipSendV2<'info>>,
    params: CcipSendV2Params,
) -> Result<[u8; 32]> {
    verify_uncursed_cpi(
        &ctx.accounts.rmn_remote,
        &ctx.accounts.rmn_remote_curses,
        &ctx.accounts.rmn_remote_config,
        params.dest_chain_selector,
    )?;

    let sender = ctx.accounts.authority.key();

    let dest_chain_config = &ctx.accounts.dest_chain_state.config;
    require!(
        !dest_chain_config.allow_list_enabled
            || dest_chain_config.allowed_senders.contains(&sender),
        CcipRouterError::SenderNotAllowed
    );

    let fee_token = if params.message.fee_token == Pubkey::default() {
        native_mint::ID
    } else {
        params.message.fee_token
    };

    let resolution_metadata = OnrampResolutionMetadata::try_from_slice(&params.resolution_metadata)
        .map_err(|_| CcipRouterError::InvalidAccountListLengths)?;

    let layout = OnrampRemainingAccountsLayout::new(
        ctx.remaining_accounts,
        &resolution_metadata.account_list_lengths,
    )?;

    let mut extra_args = dest_chain_config.parse_extra_args_with_defaults(&params.message)?;

    let ValidatedTokenPriceV2 {
        price: fee_token_price,
        billing_enabled,
    } = get_token_price_from_fee_quoter_cpi(
        ctx.accounts.config.fee_quoter,
        ctx.accounts.fee_quoter_config.to_account_info(),
        ctx.accounts
            .fee_quoter_billing_token_config
            .to_account_info(),
        fee_token,
    )?;

    require!(billing_enabled, CcipRouterError::UnsupportedFeeToken);

    let fee_quoter_cpi_accounts = FeeQuoterCpiAccounts {
        fee_quoter: ctx.accounts.config.fee_quoter,
        config: ctx.accounts.fee_quoter_config.to_account_info(),
        dest_chain: ctx.accounts.fee_quoter_dest_chain.to_account_info(),
    };

    let mut token_transfer_context = TokenTransferContext::new(
        &ctx.accounts.token_admin_registry,
        &params.message.token_amounts,
    )?;

    let (token_pool_required_ccvs, performed_transfer, token_pool_receipt) =
        if token_transfer_context.is_token_transfer() {
            let validated = validate_token_transfer(
                ctx.accounts.authority.key(),
                params.dest_chain_selector,
                fee_token,
                extra_args.finality,
                ctx.program_id.key(),
                &fee_quoter_cpi_accounts,
                &mut token_transfer_context,
                layout.token_transfer_accounts,
                &extra_args,
                true,
            )?;
            let pool_required_ccvs = validated.pool_required_ccvs();
            match &validated {
                PreflightValidatedTokenTransfer::Legacy {
                    token_accounts,
                    receipt,
                } => {
                    let token_pool_receipt = receipt.clone();
                    let (transfer, _pool_program) = transfer_token_legacy(
                        &token_transfer_context,
                        &ctx,
                        &params.message,
                        params.dest_chain_selector,
                        token_accounts,
                        &extra_args,
                        receipt,
                    )?;
                    (pool_required_ccvs, Some(transfer), Some(token_pool_receipt))
                }
                PreflightValidatedTokenTransfer::V2 {
                    ref token_accounts,
                    ref receipt,
                    ..
                } => {
                    let token_pool_receipt = receipt.clone();
                    (
                        pool_required_ccvs,
                        Some(transfer_token_v2(
                            &token_transfer_context,
                            &ctx,
                            token_accounts,
                            &params.message,
                            params.dest_chain_selector,
                            &extra_args,
                            receipt,
                        )?),
                        Some(token_pool_receipt),
                    )
                }
            }
        } else {
            (vec![], None, None)
        };

    extra_args.merge_ccv_lists(
        &token_pool_required_ccvs,
        &ctx.accounts.dest_chain_state.config.lane_mandated_ccvs,
        &ctx.accounts.dest_chain_state.config.default_ccvs,
    )?;

    // Assign the next message number for this lane from account state
    let next_message_number = ctx
        .accounts
        .dest_chain_state
        .state
        .message_number
        .checked_add(1)
        .ok_or(error!(CcipRouterError::ReachedMaxMessageNumber))?;
    ctx.accounts.dest_chain_state.state.message_number = next_message_number;

    let receipts = collect_receipts(
        &layout,
        &ctx.accounts.dest_chain_state.config,
        params.dest_chain_selector,
        &params.message,
        extra_args.finality,
        token_pool_receipt,
        &extra_args,
        &fee_token_price,
        &token_transfer_context,
        QuoteGasForExecV2CpiInputs::new(
            ctx.accounts.fee_quoter.key(),
            ctx.accounts.fee_quoter_config.to_account_info(),
            ctx.accounts
                .fee_quoter_billing_token_config
                .to_account_info(),
            ctx.accounts.fee_quoter_dest_chain.to_account_info(),
            ctx.accounts.fee_quoter_dest_chain_legacy.to_account_info(),
            params.message.fee_token,
            params.dest_chain_selector,
        ),
    )?;

    fees::distribute_fees(&mut ctx, &receipts, params.message.fee_token)?;

    let msg = build_message(
        &ctx.accounts.config,
        &ctx.accounts.dest_chain_state,
        sender,
        &params,
        performed_transfer,
        &extra_args,
        receipts.execution_gas_limit_sum,
    )?;
    let encoded_message = msg
        .to_bytes()
        .map_err(|_| CcipRouterError::InvalidInstructionData)?;
    // `compute_message_id()` would encode the message a second time; the id is just the hash of
    // the encoding we already have, and a second encoding is never freed off the bump heap.
    let message_id = MessageV1::compute_message_id_from_bytes(&encoded_message);

    let verifier_blobs = collect_verifier_blobs(
        &layout,
        &extra_args.ccvs,
        &extra_args.ccv_args,
        &msg,
        message_id,
        SVMTokenAmount {
            token: fee_token,
            amount: receipts.fee_token_amount_sum,
        },
    )?;

    emit_cpi!(events::on_ramp::CCIPMessageSentV2 {
        dest_chain_selector: params.dest_chain_selector,
        sender,
        message_id,
        fee_token,
        // EVM parity: the sender's input amount, before any in-token BPS fee the pool deducts.
        // The post-fee amount travels in the encoded message's token transfer.
        token_amount_before_token_pool_fees: params
            .message
            .token_amounts
            .first()
            .map(|tt| ProtocolAmount::from(Into::<U256>::into(tt.amount)))
            .unwrap_or_default(),
        encoded_message,
        receipts: receipts.in_order().cloned().collect(),
        verifier_blobs,
    });

    Ok(message_id)
}

// Returns the token transfer plus the token pool program that handled it (used as the issuer of
// the token receipt emitted in CCIPMessageSentV2).
#[allow(clippy::too_many_arguments)]
fn transfer_token_legacy<'info>(
    token_transfer_context: &TokenTransferContext,
    ctx: &Context<'info, CcipSendV2<'info>>,
    message: &SVM2AnyMessage,
    dest_chain_selector: u64,
    accounts: &CcipV1OnrampTokenAccounts<'info>,
    extra_args: &GenericExtraArgsV3,
    receipt: &Receipt,
) -> Result<(SVM2AnyTokenTransfer, Pubkey)> {
    let TokenTransferContext::Legacy { amount, .. } = token_transfer_context else {
        unreachable!("Only called in legacy case");
    };
    require!(
        extra_args.finality.is_wait_for_finality(),
        CcipRouterError::FinalityNotSupportedOnPoolV1
    );
    require!(
        extra_args.token_args.is_empty(),
        CcipRouterError::TokenArgsNotSupportedOnPoolV1
    );

    let transfer_seeds = &[seed::FEE_BILLING_SIGNER, &[ctx.bumps.fee_billing_signer]];

    // CPI: transfer token amount from user to token pool
    transfer_token_cpi_legacy(
        amount.amount,
        accounts.common.token_program,
        accounts.common.mint,
        accounts.common.user_token_account,
        accounts.common.pool_token_account,
        &ctx.accounts.fee_billing_signer.to_account_info(),
        transfer_seeds,
    )?;

    let token_receiver = token_receiver(extra_args, message, &ctx.accounts.dest_chain_state)?;

    // CPI: call lockOrBurn on token pool
    let lock_or_burn = LockOrBurnInLegacy {
        receiver: token_receiver,
        remote_chain_selector: dest_chain_selector,
        original_sender: ctx.accounts.authority.key(),
        amount: amount.amount,
        local_token: amount.token,
        // Deprecated: `ccip_send_v2` tracks no per-sender nonces. Sound only because the one pool
        // that reads this field (CCTP v1, which seeds a PDA with it) cannot be reached from here —
        // `TokenTransferContext::new` rejects v1 pools implementing account resolution before any
        // pool CPI. See the field's docs on `LockOrBurnInV1` for the full argument.
        msg_total_nonce: 0,
    };

    // Sized up front: growing this Vec would leak every intermediate buffer on the bump-allocated
    // program heap (10 fixed accounts plus whatever the pool asked to be passed through).
    // SAFETY: a capacity hint only — a wrong value costs a realloc, never correctness, and the operands are account counts bounded by the per-tx account limit.
    let mut acc_infos = Vec::with_capacity(10 + accounts.remaining_accounts.len());
    acc_infos.extend_from_slice(&[
        accounts.common.ccip_router_pool_signer.to_account_info(),
        accounts.common.pool_config.to_account_info(),
        accounts.common.token_program.to_account_info(),
        accounts.common.mint.to_account_info(),
        accounts.common.pool_signer.to_account_info(),
        accounts.common.pool_token_account.to_account_info(),
        ctx.accounts.rmn_remote.to_account_info(),
        ctx.accounts.rmn_remote_curses.to_account_info(),
        ctx.accounts.rmn_remote_config.to_account_info(),
        accounts.common.pool_chain_config.to_account_info(),
    ]);
    acc_infos.extend_from_slice(accounts.remaining_accounts);

    let pool_seeds = &[
        seed::EXTERNAL_TOKEN_POOLS_SIGNER,
        accounts.common.pool_program.key.as_ref(),
        &[accounts.ccip_router_pool_signer_bump],
    ];

    let return_data = interact_with_pool_legacy(
        accounts.common.pool_program.key(),
        accounts.common.ccip_router_pool_signer.key(),
        acc_infos,
        lock_or_burn,
        pool_seeds,
    )?;

    let lock_or_burn_out_data = LockOrBurnOutLegacy::try_from_slice(&return_data)?;

    let transfer = build_token_transfer_legacy(
        lock_or_burn_out_data,
        accounts.common.pool_config.key(),
        amount,
        &TokenTransferAdditionalData {
            dest_bytes_overhead: receipt.dest_bytes_overhead,
            dest_gas_overhead: receipt.dest_gas_limit,
        },
    )?;
    Ok((transfer, accounts.common.pool_program.key()))
}

#[allow(clippy::too_many_arguments)]
fn transfer_token_v2<'info>(
    token_transfer_context: &TokenTransferContext,
    ctx: &Context<'info, CcipSendV2<'info>>,
    accounts: &CcipV2OnrampTokenAccounts<'info>,
    message: &SVM2AnyMessage,
    dest_chain_selector: u64,
    extra_args: &GenericExtraArgsV3,
    token_pool_receipt: &Receipt,
) -> Result<SVM2AnyTokenTransfer> {
    let TokenTransferContext::V2(amount) = token_transfer_context else {
        unreachable!("Only called in v2 case");
    };

    let transfer_seeds = &[seed::FEE_BILLING_SIGNER, &[ctx.bumps.fee_billing_signer]];

    // CPI: transfer token amount from user to token pool. Nothing
    // changes from the legacy transfer, so we can reuse the legacy helper.
    transfer_token_cpi_legacy(
        amount.amount,
        accounts.common.token_program,
        accounts.common.mint,
        accounts.common.user_token_account,
        accounts.common.pool_token_account,
        &ctx.accounts.fee_billing_signer.to_account_info(),
        transfer_seeds,
    )?;

    let token_receiver = token_receiver(extra_args, message, &ctx.accounts.dest_chain_state)?;

    // CPI: call lockOrBurn on token pool
    let lock_or_burn = LockOrBurnInV2 {
        receiver: token_receiver,
        remote_chain_selector: dest_chain_selector,
        original_sender: ctx.accounts.authority.key(),
        amount: amount.amount,
        local_token: amount.token,
        requested_finality_config: extra_args.finality,
        token_args: extra_args.token_args.clone(),
    };

    // Sized up front: see the note in `transfer_token_legacy` (9 fixed accounts plus the pool's
    // auto-resolved extras).
    // SAFETY: a capacity hint only — a wrong value costs a realloc, never correctness, and the operands are account counts bounded by the per-tx account limit.
    let mut acc_infos = Vec::with_capacity(9 + accounts.extra_cpi_accounts.len());
    acc_infos.extend_from_slice(&[
        accounts.common.ccip_router_pool_signer.to_account_info(),
        accounts.common.pool_config.to_account_info(),
        accounts.common.token_program.to_account_info(),
        accounts.common.mint.to_account_info(),
        accounts.common.pool_signer.to_account_info(),
        accounts.common.pool_token_account.to_account_info(),
        accounts.common.pool_chain_config.to_account_info(),
        accounts.common.pool_chain_config_v2.to_account_info(),
        accounts.common.pool_chain_config_override.to_account_info(),
    ]);
    acc_infos.extend_from_slice(accounts.extra_cpi_accounts);

    let pool_seeds = &[
        seed::EXTERNAL_TOKEN_POOLS_SIGNER,
        accounts.common.pool_program.key.as_ref(),
        &[accounts.ccip_router_pool_signer_bump],
    ];

    let return_data = interact_with_pool_v2(
        accounts.common.pool_program.key(),
        accounts.common.ccip_router_pool_signer.key(),
        acc_infos,
        lock_or_burn,
        pool_seeds,
    )?;

    let lock_or_burn_out_data = LockOrBurnOutV2::try_from_slice(&return_data)?;

    let token_transfer_additional_data = TokenTransferAdditionalData {
        dest_bytes_overhead: token_pool_receipt.dest_bytes_overhead,
        dest_gas_overhead: token_pool_receipt.dest_gas_limit,
    };

    build_token_transfer_v2(
        lock_or_burn_out_data,
        accounts.common.pool_config.key(),
        &token_transfer_additional_data,
    )
}

pub fn build_token_transfer_v2(
    lock_or_burn_out_data: LockOrBurnOutV2,
    source_pool_address: Pubkey,
    additional_data: &TokenTransferAdditionalData,
) -> Result<SVM2AnyTokenTransfer> {
    let dest_gas_amount = additional_data.dest_gas_overhead;

    let extra_data = lock_or_burn_out_data.dest_pool_data;
    // SAFETY: extra_data from CPI return, bounded by Solana tx size
    let extra_data_length = extra_data.len() as u32;

    require_gte!(
        LockOrBurnOutV2::default_ret_bytes().max(additional_data.dest_bytes_overhead),
        extra_data_length,
        CcipRouterError::SourceTokenDataTooLarge
    );

    let dest_exec_data = dest_gas_amount.convert::<u32>().to_be_bytes().to_vec();

    Ok(SVM2AnyTokenTransfer {
        source_pool_address,
        dest_token_address: lock_or_burn_out_data.dest_token_address.address.clone(),
        extra_data,
        // The pool's post-fee destination amount: V2 pools deduct their in-token BPS fee before
        // locking or burning the rest. The pool on the receiver chain handles decimals.
        amount: lock_or_burn_out_data.dest_token_amount.into(),
        dest_exec_data,
    })
}

fn interact_with_pool_v2(
    pool_program: Pubkey,
    signer: Pubkey,
    acc_infos: Vec<AccountInfo>,
    data: LockOrBurnInV2,
    seeds: &[&[u8]],
) -> Result<Vec<u8>> {
    // Built by hand rather than through `to_account_metas`, which allocates a one-element Vec per
    // account (see the note in `interact_with_pool`).
    let mut acc_metas = Vec::with_capacity(acc_infos.len());
    for acc_info in &acc_infos {
        acc_metas.push(AccountMeta {
            pubkey: *acc_info.key,
            // Check signer from PDA External Execution config
            is_signer: acc_info.key() == signer,
            is_writable: acc_info.is_writable,
        });
    }

    let ix = Instruction {
        program_id: pool_program,
        accounts: acc_metas,
        data: data.to_tx_data(),
    };

    // CPI call to pool is expected to return data using set_return_data
    // anchor does this automatically but is limited to max 256 bytes
    // https://github.com/coral-xyz/anchor/blob/0109f4a3cf4117570f627e3ae465b6247d504f69/lang/syn/src/codegen/program/handlers.rs#L113
    invoke_signed(&ix, &acc_infos, &[seeds])?;

    get_verified_cpi_return_data(pool_program, CcipRouterError::InvalidCpiReturnProgram)
}

pub(crate) fn build_message(
    config: &Config,
    dest_chain_state: &DestChainCcipV2,
    sender: Pubkey,
    params: &CcipSendV2Params,
    performed_transfer: Option<SVM2AnyTokenTransfer>,
    extra_args: &GenericExtraArgsV3,
    execution_gas_limit: CrossChainGas,
) -> Result<MessageV1> {
    let address_bytes_length = dest_chain_state.config.address_bytes_length as usize;
    let receiver = normalized_address(
        &params.message.receiver,
        address_bytes_length,
        CcipRouterError::InvalidReceiverAddress,
    )?;

    Ok(MessageV1 {
        source_chain_selector: config.svm_chain_selector,
        dest_chain_selector: params.dest_chain_selector,
        message_number: dest_chain_state.state.message_number,
        execution_gas_limit,
        ccip_receive_gas_limit: extra_args.gas_limit,
        finality: extra_args.finality,
        ccv_and_executor_hash: compute_ccv_exec_hash(&extra_args.ccvs, &extra_args.executor),
        on_ramp_address: crate::ID.to_bytes().to_vec(),
        off_ramp_address: dest_chain_state.config.offramp.clone(),
        sender: sender.to_bytes().to_vec(),
        receiver,
        dest_blob: extra_args.executor_args.clone(),
        data: params.message.data.clone(),
        token_transfer: if params.message.token_amounts.is_empty() {
            None
        } else {
            let tt = &params.message.token_amounts[0];
            let performed_transfer = performed_transfer
                .expect("Must exist at this point if params.message.token_amounts is not empty");

            let dest_token_address = normalized_address(
                &performed_transfer.dest_token_address,
                address_bytes_length,
                CcipRouterError::InvalidDestTokenAddress,
            )?;

            let token_receiver = token_receiver(extra_args, &params.message, dest_chain_state)?;
            Some(TokenTransferV1 {
                version: 1,
                // The pool's post-fee destination amount, which may be less than the sender's
                // input amount when the pool charges an in-token BPS fee.
                amount: performed_transfer.amount.into(),
                source_pool_address: performed_transfer.source_pool_address.to_bytes().to_vec(),
                source_token_address: tt.token.to_bytes().to_vec(),
                dest_token_address,
                token_receiver,
                extra_data: performed_transfer.extra_data,
            })
        },
    })
}

pub fn token_receiver(
    extra_args: &GenericExtraArgsV3,
    message: &SVM2AnyMessage,
    dest_chain_state: &DestChainCcipV2,
) -> Result<Vec<u8>> {
    // When the sender didn't specify a token receiver, the tokens go to the message receiver: a
    // validation failure is then about the message receiver, so it is reported as such rather than
    // blaming a token receiver that was never provided.
    let (raw, err) = if extra_args.token_receiver.is_empty() {
        (&message.receiver, CcipRouterError::InvalidReceiverAddress)
    } else {
        (
            &extra_args.token_receiver,
            CcipRouterError::InvalidDestTokenReceiverAddress,
        )
    };

    normalized_address(
        raw,
        dest_chain_state.config.address_bytes_length as usize,
        err,
    )
}

fn collect_verifier_blobs<'info>(
    layout: &OnrampRemainingAccountsLayout<'_, 'info>,
    ccvs: &[Pubkey],
    ccv_args: &[Vec<u8>],
    msg: &MessageV1,
    message_id: [u8; 32],
    fee_token: SVMTokenAmount,
) -> Result<Vec<Vec<u8>>> {
    let mut verifier_blobs = Vec::with_capacity(ccvs.len());

    for ((ccv, ccv_accounts), ccv_args) in ccvs
        .iter()
        .zip(layout.ccv_accounts_per_ccv.iter())
        .zip(ccv_args.iter())
    {
        verifier_blobs.push(call_start_verification_at_source(
            *ccv,
            ccv_args.clone(),
            ccv_accounts,
            msg,
            message_id,
            fee_token.clone(),
        )?);
    }

    Ok(verifier_blobs)
}

fn call_start_verification_at_source<'info>(
    ccv: Pubkey,
    ccv_args: Vec<u8>,
    ccv_accounts: &[AccountInfo<'info>],
    msg: &MessageV1,
    message_id: [u8; 32],
    fee_token: SVMTokenAmount,
) -> Result<Vec<u8>> {
    require_gte!(
        ccv_accounts.len(),
        4,
        CcipRouterError::InvalidAccountListLengths
    );

    let params = verifier::StartVerificationAtSourceParams {
        message: msg.clone(),
        message_id,
        fee_token,
        verifier_args: ccv_args,
    };

    require_keys_eq!(
        ccv_accounts[0].key(),
        ccv,
        CcipRouterError::InvalidInputsCcvAccounts
    );

    let (ccv_signer, ccv_signer_bump) =
        Pubkey::find_program_address(&[seed::CCV_SIGNER, ccv.as_ref()], &crate::ID);

    let participant_accounts = &ccv_accounts[1..];
    let mut signer_seen = false;
    let mut account_metas = Vec::with_capacity(participant_accounts.len());
    for acc_info in participant_accounts {
        let is_ccv_signer = acc_info.key() == ccv_signer;
        if is_ccv_signer {
            require!(!signer_seen, CcipRouterError::InvalidInputsCcvAccounts);
            signer_seen = true;
        } else {
            require!(
                !acc_info.is_signer,
                CcipRouterError::InvalidInputsCcvAccounts
            );
        }
        account_metas.push(AccountMeta {
            pubkey: *acc_info.key,
            is_signer: is_ccv_signer,
            is_writable: acc_info.is_writable,
        });
    }
    require!(signer_seen, CcipRouterError::InvalidInputsCcvAccounts);

    let ix = Instruction {
        program_id: ccv,
        accounts: account_metas,
        data: verifier::instruction::StartVerificationAtSource { params }.data(),
    };
    let ccv_signer_seeds: &[&[u8]] = &[seed::CCV_SIGNER, ccv.as_ref(), &[ccv_signer_bump]];
    invoke_signed(&ix, participant_accounts, &[ccv_signer_seeds])?;

    parse_verified_cpi_return_as::<Vec<u8>, _>(
        ccv,
        CcipRouterError::InvalidCpiReturnProgram,
        CcipRouterError::InvalidInstructionData,
    )
}

fn compute_ccv_exec_hash(ccvs: &[Pubkey], executor: &Pubkey) -> [u8; 32] {
    const PUBKEY_SIZE: u8 = 32;
    // SAFETY: ccvs.len() is protocol-bounded (1-3 CCVs); 1 + 3*32 + 32 = 129 max, cannot overflow
    let mut preimage =
        Vec::<u8>::with_capacity(1 + ccvs.len() * PUBKEY_SIZE as usize + PUBKEY_SIZE as usize);
    preimage.push(PUBKEY_SIZE);
    ccvs.iter()
        .for_each(|pk| preimage.extend_from_slice(&pk.to_bytes()));
    preimage.extend_from_slice(&executor.to_bytes());
    solana_keccak_hasher::hash(&preimage).to_bytes()
}

#[event_cpi]
#[derive(Accounts)]
#[instruction(params: CcipSendV2Params)]
pub struct CcipSendV2<'info> {
    #[account(
        seeds = [seed::CONFIG],
        bump,
        constraint = valid_version(config.version, MAX_CONFIG_V) @ CcipRouterError::InvalidVersion,
    )]
    pub config: Box<Account<'info, Config>>,

    #[account(
        mut,
        seeds = [seed::DEST_CHAIN_STATE_V2, params.dest_chain_selector.to_le_bytes().as_ref()],
        bump = dest_chain_state.bump,
        constraint = valid_version(dest_chain_state.version, MAX_CHAINSTATECCIPV2_V) @ CcipRouterError::InvalidVersion,
    )]
    pub dest_chain_state: Box<Account<'info, DestChainCcipV2>>,

    /// CHECK: When there is a token transfer, this contains the token admin registry entry for the transferred
    /// token. When there isn't, it's the zero pubkey. This is stored separately to the token accounts as it's
    /// necessary to determine the schema for the token account layout.
    pub token_admin_registry: UncheckedAccount<'info>,

    // PERMISSIONLESS: Anyone may send a message through CCIP - the protocol is permissionless. The authority is the sender of the message.
    #[account(mut)]
    pub authority: Signer<'info>,

    pub system_program: Program<'info, System>,

    ////////////////////
    // fee quoter CPI //
    ////////////////////
    /// CHECK: This is the account for the Fee Quoter program
    #[account(
        address = config.fee_quoter @ CcipRouterError::InvalidFeeQuoterAddress,
    )]
    pub fee_quoter: UncheckedAccount<'info>,

    /// CHECK: This account is just used in the CPI to the Fee Quoter program
    #[account(
        seeds = [seed::CONFIG],
        bump,
        seeds::program = config.fee_quoter,
    )]
    pub fee_quoter_config: UncheckedAccount<'info>,

    /// CHECK: This account is just used in the CPI to the Fee Quoter program
    #[account(
        seeds = [seed::DEST_CHAIN_V2, params.dest_chain_selector.to_le_bytes().as_ref()],
        bump,
        seeds::program = config.fee_quoter,
    )]
    pub fee_quoter_dest_chain: UncheckedAccount<'info>,

    /// CHECK: This account is just used in the CPI to the Fee Quoter program
    #[account(
        seeds = [seed::DEST_CHAIN, params.dest_chain_selector.to_le_bytes().as_ref()],
        bump,
        seeds::program = config.fee_quoter,
    )]
    pub fee_quoter_dest_chain_legacy: UncheckedAccount<'info>,

    /// CHECK: This account is just used in the CPI to the Fee Quoter program
    #[account(
        seeds = [seed::FEE_BILLING_TOKEN_CONFIG, params.message.fee_token.normalize_wsol().as_ref()],
        bump,
        seeds::program = config.fee_quoter,
    )]
    pub fee_quoter_billing_token_config: UncheckedAccount<'info>,

    /// CHECK: This account is just used in the CPI to the Fee Quoter program
    #[account(
        seeds = [seed::FEE_BILLING_TOKEN_CONFIG,
            config.link_token_mint.key().as_ref(),
        ],
        bump,
        seeds::program = config.fee_quoter,
    )]
    pub fee_quoter_link_token_config: UncheckedAccount<'info>,

    /////////////
    // billing //
    /////////////
    pub fee_token_program: Interface<'info, TokenInterface>,

    #[account(
        owner = fee_token_program.key() @ CcipRouterError::InvalidInputsMint,
        address = params.message.fee_token.normalize_wsol() @ CcipRouterError::InvalidInputsMint,
    )]
    pub fee_token_mint: Box<InterfaceAccount<'info, Mint>>, // pass pre-2022 wSOL if using native SOL

    /// CHECK: This is the associated token account for the user paying the fee.
    /// If paying with native SOL, this must be the zero address.
    #[account(
        // address must be either zero (paying with native SOL) or must be a WRITABLE associated token account
        constraint = (params.message.fee_token == Pubkey::default() && fee_token_user_associated_account.key() == Pubkey::default())
            || fee_token_user_associated_account.is_writable @ CcipRouterError::InvalidInputsAtaWritable,
        // address must be either zero (paying with native SOL) or
        // the associated token account address for the caller and fee token used
        constraint = (params.message.fee_token == Pubkey::default() && fee_token_user_associated_account.key() == Pubkey::default())
            || fee_token_user_associated_account.key() == get_associated_token_address_with_program_id(
                &authority.key(),
                &fee_token_mint.key(),
                &fee_token_program.key(),
            ) @ CcipRouterError::InvalidInputsAtaAddress,
    )]
    pub fee_token_user_associated_account: UncheckedAccount<'info>,

    #[account(
        mut,
        associated_token::mint = fee_token_mint,
        associated_token::authority = fee_billing_signer,
        associated_token::token_program = fee_token_program,
    )]
    pub fee_vault: Box<InterfaceAccount<'info, TokenAccount>>, // pass pre-2022 wSOL receiver if using native SOL

    /// CHECK: This is the signer for the billing transfer CPI.
    #[account(
        seeds = [seed::FEE_BILLING_SIGNER],
        bump
    )]
    pub fee_billing_signer: UncheckedAccount<'info>,

    ////////////////////
    // RMN Remote CPI //
    ////////////////////
    /// CHECK: This is the account for the RMN Remote program
    #[account(
        address = config.rmn_remote @ CcipRouterError::InvalidRMNRemoteAddress,
    )]
    pub rmn_remote: UncheckedAccount<'info>,

    /// CHECK: This account is just used in the CPI to the RMN Remote program
    #[account(
        seeds = [seed::CURSES],
        bump,
        seeds::program = config.rmn_remote,
    )]
    pub rmn_remote_curses: UncheckedAccount<'info>,

    /// CHECK: This account is just used in the CPI to the RMN Remote program
    #[account(
        seeds = [seed::CONFIG],
        bump,
        seeds::program = config.rmn_remote,
    )]
    pub rmn_remote_config: UncheckedAccount<'info>,
    // Remaining accounts (CCIP 2.0, dynamic length)
    // Token pool accounts for token transfers
    // Executor accounts
    // CCV accounts
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_compute_ccv_exec_hash() {
        let addr_bytes =
            hex::decode("2222222222222222222222222222222222222222222222222222222222222222")
                .unwrap();
        let ccvs = vec![Pubkey::new_from_array(
            addr_bytes.clone().try_into().unwrap(),
        )];
        let executor = ccvs[0];
        let hash = compute_ccv_exec_hash(&ccvs, &executor);
        assert_eq!(
            hash,
            solana_keccak_hasher::hashv(&[&[32], &addr_bytes, &addr_bytes]).to_bytes()
        );
    }
}
