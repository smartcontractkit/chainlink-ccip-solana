use anchor_lang::prelude::program::invoke;
use anchor_lang::prelude::*;
use anchor_lang::solana_program::{instruction::Instruction, program::invoke_signed};
use anchor_lang::InstructionData;
use base_token_pool::v2::GetFeeParams;
use ccip_common::ccvs::get_pool_required_ccvs_cpi;
use ccip_common::cpi::parse_verified_cpi_return_as;
use ccip_common::messages::NormalizeWsol;
use ccip_common::types::{CrossChainGas, Exponential, ToTxData, Usd18Decimals, UsdCents};
use ccip_common::v1::{
    load_token_admin_registry_checked,
    validate_and_parse_onramp_token_accounts as validate_and_parse_token_accounts_legacy,
    CcipV1OnrampTokenAccounts,
};
use ccip_common::v2::{
    validate_and_parse_onramp_token_accounts_v2, CcipV2OnrampTokenAccounts, MessageDirection,
    ProtocolAmount,
};
use ccip_common::{
    extra_args::{GenericExtraArgsV3, NO_EXECUTION_TAG},
    messages::{
        FinalityConfig, SVMTokenAmount, MESSAGE_V1_REMOTE_CHAIN_ADDRESSES,
        MESSAGE_V1_SVM_SOURCE_BASE_SIZE, TOKEN_TRANSFER_V1_SVM_SOURCE_BASE_SIZE,
    },
    pool_ccvs_with_default_fallback, seed,
    v2::Receipt,
    INCLUDE_DEFAULT_CCVS,
};
use committee_verifier as verifier;
use ethnum::U256;
use fee_quoter::messages::SVM2AnyMessage;
use fee_quoter::messages_v2::{
    FallbackTokenTransferFeeV2, GetFallbackTokenTransferFeeV2Params, QuoteGasForExecV2Params,
    QuoteGasForExecV2Result, ValidatedTokenPriceV2,
};

use crate::error::CcipRouterError;
use crate::state::DestChainConfigCcipV2;

/// Layout of remaining accounts as passed to ccip_send_v2 and get_fee_v2, broken
/// down into its specific sections. In some cases they will undergo further parsing
/// and validation (i.e. token transfer accounts).
pub(crate) struct OnrampRemainingAccountsLayout<'a, 'info> {
    pub token_transfer_accounts: &'a [AccountInfo<'info>],
    pub executor_accounts: &'a [AccountInfo<'info>],
    // Unlike for TT and executor, which have their ATA account as
    // part of their slice, the CCV fee vault ATAs precede the per-ccv lists.
    // They're left here for clarity as they're not read directly (see the
    // comment at the top of `transfer_receipt_fee_to_issuer_ata` for why.)
    pub _ccv_vault_atas: &'a [AccountInfo<'info>],
    pub ccv_accounts_per_ccv: Vec<&'a [AccountInfo<'info>]>,
}

impl<'a, 'info> OnrampRemainingAccountsLayout<'a, 'info> {
    pub(crate) fn new(
        remaining_accounts: &'a [AccountInfo<'info>],
        acc_lengths: &OnrampRemainingAccountListLengths,
    ) -> Result<Self> {
        let tt_end = acc_lengths.token_transfer as usize;
        let exec_end = tt_end
            .checked_add(acc_lengths.executor as usize)
            .ok_or(CcipRouterError::InvalidAccountListLengths)?;
        let ccv_vault_atas_end = exec_end
            .checked_add(acc_lengths.ccv_vault_atas as usize)
            .ok_or(CcipRouterError::InvalidAccountListLengths)?;

        let token_transfer_accounts = remaining_accounts
            .get(..tt_end)
            .ok_or(CcipRouterError::InvalidAccountListLengths)?;
        let executor_accounts = remaining_accounts
            .get(tt_end..exec_end)
            .ok_or(CcipRouterError::InvalidAccountListLengths)?;
        let ccv_vault_atas = remaining_accounts
            .get(exec_end..ccv_vault_atas_end)
            .ok_or(CcipRouterError::InvalidAccountListLengths)?;
        let mut offset = ccv_vault_atas_end;

        let mut ccv_accounts_per_ccv = Vec::with_capacity(acc_lengths.ccvs.len());
        for &len in acc_lengths.ccvs.iter() {
            let end = offset
                .checked_add(len as usize)
                .ok_or(CcipRouterError::InvalidAccountListLengths)?;
            let ccv_accounts = remaining_accounts
                .get(offset..end)
                .ok_or(CcipRouterError::InvalidAccountListLengths)?;
            ccv_accounts_per_ccv.push(ccv_accounts);
            offset = end;
        }

        require_eq!(
            offset,
            remaining_accounts.len(),
            CcipRouterError::InvalidAccountListLengths
        );

        Ok(Self {
            token_transfer_accounts,
            executor_accounts,
            ccv_accounts_per_ccv,
            _ccv_vault_atas: ccv_vault_atas,
        })
    }
}

#[derive(Clone, AnchorSerialize, AnchorDeserialize, Debug, PartialEq, Eq, Default)]
pub(crate) struct OnrampRemainingAccountListLengths {
    pub token_transfer: u8,
    pub executor: u8,
    pub ccv_vault_atas: u8,
    pub ccvs: Vec<u8>,
}

#[derive(Clone, AnchorSerialize, AnchorDeserialize, Debug)]
pub struct OnrampResolutionMetadata {
    pub account_list_lengths: OnrampRemainingAccountListLengths,
}

/// The receipt of a token transfer, issued by the pool program handling it.
pub struct TokenPoolReceipt {
    pub receipt: Receipt,
    /// Mint of the transferred token, used as the qualifier seed of the pool's fee vault so
    /// that a pool program managing multiple tokens accrues fees separately for each of them.
    pub mint: Pubkey,
    /// Legacy (1.6 interface) pools aren't paid out: their fee is kept in the onramp,
    /// equivalent to 1.6 behaviour where the additional fee simply accrues there.
    pub is_legacy: bool,
}

pub struct CombinedReceipts {
    pub executor_receipt: Receipt,
    pub network_fee_receipt: Receipt,
    pub token_pool_receipt: Option<TokenPoolReceipt>,
    pub ccv_receipts: Vec<Receipt>,

    pub execution_gas_limit_sum: CrossChainGas,
    pub fee_token_amount_sum: u64,
}

impl CombinedReceipts {
    pub fn in_order(&self) -> impl Iterator<Item = &Receipt> {
        // The CCIP receipt list must follow the canonical order expected by the off-chain receipt
        // parser: [CCV receipts.., token receipts.., executor, network fee].
        self.ccv_receipts
            .iter()
            .chain(self.token_pool_receipt.as_ref().map(|t| &t.receipt))
            .chain([&self.executor_receipt, &self.network_fee_receipt])
    }

    pub fn in_order_mut(&mut self) -> impl Iterator<Item = &mut Receipt> {
        // The CCIP receipt list must follow the canonical order expected by the off-chain receipt
        // parser: [CCV receipts.., token receipts.., executor, network fee].
        self.ccv_receipts
            .iter_mut()
            .chain(self.token_pool_receipt.as_mut().map(|t| &mut t.receipt))
            .chain([&mut self.executor_receipt, &mut self.network_fee_receipt])
    }

    // Returns only the payable receipts, together with the qualifier seed of the issuer's
    // fee vault (see `ccip_common::seed::vault_ata`). Payable receipts are:
    //
    // * Those that have positive fees
    // * And aren't legacy pools
    // * And aren't the network fee receipt (which is kept in the onramp)
    pub fn payable_only(&self) -> impl Iterator<Item = (&Receipt, Option<&Pubkey>)> {
        let maybe_token_pool_receipt = self
            .token_pool_receipt
            .as_ref()
            .filter(|t| !t.is_legacy)
            .map(|t| (&t.receipt, Some(&t.mint)));

        self.ccv_receipts
            .iter()
            .map(|r| (r, None))
            .chain(maybe_token_pool_receipt)
            .chain(
                (self.executor_receipt.issuer != NO_EXECUTION_TAG)
                    .then_some((&self.executor_receipt, None)),
            )
            .filter(|(r, _)| r.fee_token_amount != 0)
    }
}

#[allow(clippy::too_many_arguments)]
pub(crate) fn collect_receipts<'info>(
    layout: &OnrampRemainingAccountsLayout<'_, 'info>,
    dest_chain_config: &DestChainConfigCcipV2,
    dest_chain_selector: u64,
    message: &SVM2AnyMessage,
    finality: FinalityConfig,
    token_pool_receipt: Option<Receipt>,
    extra_args: &GenericExtraArgsV3,
    fee_token_price: &Usd18Decimals,
    token_transfer_context: &TokenTransferContext,
    mut quote_gas_inputs: QuoteGasForExecV2CpiInputs,
) -> Result<CombinedReceipts> {
    // Every requested CCV must come with its own account group and args: a shorter
    // account or args list would silently truncate the zip below and skip the fee
    // receipts of the unmatched CCVs.
    require_eq!(
        extra_args.ccvs.len(),
        layout.ccv_accounts_per_ccv.len(),
        CcipRouterError::InvalidAccountListLengths
    );
    require_eq!(
        extra_args.ccvs.len(),
        extra_args.ccv_args.len(),
        CcipRouterError::InvalidAccountListLengths
    );

    let mut ccv_receipts = Vec::with_capacity(extra_args.ccvs.len());
    for ((ccv, ccv_accounts), ccv_args) in extra_args
        .ccvs
        .iter()
        .zip(layout.ccv_accounts_per_ccv.iter())
        .zip(extra_args.ccv_args.iter())
    {
        ccv_receipts.push(call_ccv_get_fee(
            *ccv,
            ccv_accounts,
            dest_chain_selector,
            message,
            ccv_args,
            finality,
        )?);
    }

    let executor_receipt = executor_receipt(
        dest_chain_config,
        message,
        extra_args,
        layout.executor_accounts,
        executor::GetFeeParams {
            dest_chain_selector,
            requested_finality: finality,
            ccvs: extra_args.ccvs.clone(),
            extra_args: extra_args.executor_args.clone(),
            fee_token: message.fee_token,
        },
    )?;

    let network_fee_receipt = network_fee_receipt(dest_chain_config, message)?;

    let token_pool_receipt = token_pool_receipt
        .map(|receipt| {
            Ok::<_, Error>(TokenPoolReceipt {
                receipt,
                mint: token_transfer_context
                    .token_mint()
                    .ok_or(CcipRouterError::InvalidReceiptData)?,
                is_legacy: token_transfer_context.is_legacy_token_transfer(),
            })
        })
        .transpose()?;

    let mut receipts = CombinedReceipts {
        execution_gas_limit_sum: CrossChainGas::ZERO,
        fee_token_amount_sum: 0,
        executor_receipt,
        network_fee_receipt,
        token_pool_receipt,
        ccv_receipts,
    };

    receipts.execution_gas_limit_sum = CrossChainGas::new(safe_sum(
        receipts.in_order().map(|r| r.dest_gas_limit.convert()),
        CcipRouterError::InvalidReceiptData,
    )?);

    let execution_bytes_overhead_sum = safe_sum(
        receipts.in_order().map(|r| r.dest_bytes_overhead),
        CcipRouterError::InvalidReceiptData,
    )?;

    quote_gas_inputs.gas_limit_sum = receipts.execution_gas_limit_sum;
    quote_gas_inputs.bytes_overhead_sum = execution_bytes_overhead_sum;
    let gas_quote = quote_gas_for_exec_fee_quoter_cpi(quote_gas_inputs)?;
    receipts.execution_gas_limit_sum = gas_quote.total_gas;

    // Receipts carry USD cents up to here, and are all converted with one multiplier, as EVM's
    // `OnRamp._getReceipts`.
    let fee_multiplier = fee_multiplier(gas_quote.premium_percent_multiplier, fee_token_price)?;
    for receipt in receipts.in_order_mut() {
        receipt.fee_token_amount = (U256::from(receipt.fee_token_amount) * fee_multiplier)
            .try_into()
            .map_err(|_| CcipRouterError::InvalidTokenArithmetic)?;
    }

    // Add execution cost to the executor's fee. Execution cost should not be multiplied by the multiplier.
    if extra_args.executor != NO_EXECUTION_TAG {
        receipts.executor_receipt.fee_token_amount = receipts
            .executor_receipt
            .fee_token_amount
            .checked_add(usd_fee_to_fee_token_amount(
                gas_quote.total_gas_cost,
                fee_token_price,
            )?)
            .ok_or(CcipRouterError::InvalidTokenArithmetic)?;
    }

    receipts.fee_token_amount_sum = safe_sum(
        receipts.in_order().map(|r| r.fee_token_amount),
        CcipRouterError::InvalidReceiptData,
    )?;

    require_gte!(
        usd_fee_to_fee_token_amount(dest_chain_config.max_fee_per_message, fee_token_price)?,
        receipts.fee_token_amount_sum,
        CcipRouterError::FeeExceedsMaxAllowed
    );

    Ok(receipts)
}

/// Converts a USD fee to the fee token's smallest denomination (e.g. lamports for SOL).
///
/// The fee token price is expressed as 18-decimals USD for 1e18 smallest token denominations,
/// so the USD fee (18 decimals) is raised to 36 decimals before dividing by the price.
fn usd_fee_to_fee_token_amount(fee: UsdCents, fee_token_price: &Usd18Decimals) -> Result<u64> {
    let fee_usd_36_decimals = Usd18Decimals::from(fee).get() * 1u32.e(18);
    Ok((fee_usd_36_decimals / non_zero_price(fee_token_price)?).try_into()?)
}

/// The fee token amount per USD cent, premium included: `premium% * 1e32 / price`, where 1e32 is
/// 1e36 less the two decimals each of the cents and the percentage. As EVM's `OnRamp._getReceipts`.
fn fee_multiplier(
    premium_percent_multiplier: u64,
    fee_token_price: &Usd18Decimals,
) -> Result<U256> {
    Ok(U256::from(premium_percent_multiplier) * 1u32.e(32) / non_zero_price(fee_token_price)?)
}

fn non_zero_price(fee_token_price: &Usd18Decimals) -> Result<U256> {
    let price = fee_token_price.get();
    // The fee quoter validates the price on retrieval; this guards against a zero price
    // reaching the division through any other path.
    require_neq!(price, U256::ZERO, CcipRouterError::InvalidTokenPrice);
    Ok(price)
}

fn executor_receipt<'info>(
    dest_chain_config: &DestChainConfigCcipV2,
    message: &SVM2AnyMessage,
    extra_args: &GenericExtraArgsV3,
    executor_accounts: &[AccountInfo<'info>],
    params: executor::GetFeeParams,
) -> Result<Receipt> {
    // Executor receipt: issued by the executor.
    // Lane configuration enforces `offramp.len() == address_bytes_length`, so every destination
    // address on the wire (offramp included) is this wide.
    require!(
        !dest_chain_config.offramp.is_empty(),
        CcipRouterError::MissingOfframpAddress
    );
    let remote_address_len = dest_chain_config.address_bytes_length as usize;

    // The wire payload is identical on source and destination with the V1 codec, so this predicts
    // the encoded MessageV1 size, excluding parts billed by other receipts (token extraData → pool
    // receipt, CCV bytes → CCV receipts).
    let dest_bytes_overhead: u32 = (MESSAGE_V1_SVM_SOURCE_BASE_SIZE
        + message.data.len()
        + extra_args.executor_args.len() // destBlob == executorArgs
        + MESSAGE_V1_REMOTE_CHAIN_ADDRESSES * remote_address_len
        + message.token_amounts.len()
            * (TOKEN_TRANSFER_V1_SVM_SOURCE_BASE_SIZE + remote_address_len * 2))
        .try_into()
        .map_err(|_| CcipRouterError::InvalidReceiptData)?;

    let fee = if extra_args.executor == NO_EXECUTION_TAG {
        UsdCents::ZERO
    } else {
        get_executor_fee_cpi(executor_accounts, extra_args.executor, params)?
    };
    Ok(Receipt {
        issuer: extra_args.executor,
        dest_gas_limit: CrossChainGas::new(
            dest_chain_config
                .base_execution_gas_cost
                .convert::<u32>()
                .checked_add(extra_args.gas_limit.convert())
                .ok_or(CcipRouterError::InvalidReceiptData)?,
        ),
        dest_bytes_overhead,
        fee_token_amount: fee.convert(),
        extra_args: extra_args.executor_args.clone(),
    })
}

fn network_fee_receipt(
    dest_chain_config: &DestChainConfigCcipV2,
    message: &SVM2AnyMessage,
) -> Result<Receipt> {
    let network_fee = if message.token_amounts.is_empty() {
        dest_chain_config.message_network_fee
    } else {
        dest_chain_config.token_transfer_network_fee
    };

    Ok(Receipt {
        // Network-fee receipt: issued by the router/onramp itself.
        issuer: crate::ID,
        dest_gas_limit: CrossChainGas::ZERO,
        dest_bytes_overhead: 0,
        fee_token_amount: network_fee.convert(),
        extra_args: vec![],
    })
}

#[allow(clippy::too_many_arguments)]
fn call_ccv_get_fee<'info>(
    ccv: Pubkey,
    ccv_accounts: &[AccountInfo<'info>],
    dest_chain_selector: u64,
    message: &SVM2AnyMessage,
    ccv_args: &[u8],
    finality: FinalityConfig,
) -> Result<Receipt> {
    require_gte!(
        ccv_accounts.len(),
        4,
        CcipRouterError::InvalidAccountListLengths
    );

    let params = verifier::GetFeeParams {
        dest_chain_selector,
        message: message.clone(),
        extra_args: ccv_args.to_vec(),
        finality,
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
        data: verifier::instruction::GetVerifierFee { params }.data(),
    };
    let ccv_signer_seeds: &[&[u8]] = &[seed::CCV_SIGNER, ccv.as_ref(), &[ccv_signer_bump]];
    invoke_signed(&ix, participant_accounts, &[ccv_signer_seeds])?;

    let quote: verifier::GetFeeResult = parse_verified_cpi_return_as(
        ccv,
        CcipRouterError::InvalidCpiReturnProgram,
        CcipRouterError::InvalidInstructionData,
    )?;

    // The receipt that ends up in the message is the router's view of the CCV's fee quote,
    // stamped with the issuing CCV. We adapt here rather than have every CCV know about the
    // router's `Receipt` shape.
    Ok(Receipt {
        issuer: ccv,
        dest_gas_limit: quote.gas_for_verification,
        dest_bytes_overhead: quote.payload_size_bytes,
        fee_token_amount: quote.fee.convert(),
        extra_args: ccv_args.to_vec(),
    })
}

fn safe_sum<T>(i: impl IntoIterator<Item = T>, err: CcipRouterError) -> Result<T>
where
    T: Into<u64> + TryFrom<u64>,
{
    let sum = i
        .into_iter()
        .try_fold(0u64, |acc, val| acc.checked_add(val.into()).ok_or(err))?;

    sum.try_into().map_err(|_| err.into())
}

/// `AccountMeta`s for a CPI that passes the caller's writable flags through unchanged.
/// Equivalent to `to_account_metas(None)` per account, but without the one-element `Vec` each of
/// those calls allocates: the program heap is a bump allocator, so such temporaries stay occupied
/// for the rest of the instruction, and the send path runs close to the 32KB limit.
///
/// This method drops all signer flags from the account list.
fn account_metas_preserving_writability(acc_infos: &[AccountInfo<'_>]) -> Vec<AccountMeta> {
    let mut acc_metas = Vec::with_capacity(acc_infos.len());
    for acc_info in acc_infos {
        acc_metas.push(AccountMeta {
            pubkey: *acc_info.key,
            is_signer: false,
            is_writable: acc_info.is_writable,
        });
    }
    acc_metas
}

pub(crate) fn get_token_price_from_fee_quoter_cpi<'info>(
    fee_quoter: Pubkey,
    fee_quoter_config: AccountInfo<'info>,
    billing_token_config: AccountInfo<'info>,
    mint: Pubkey,
) -> Result<ValidatedTokenPriceV2> {
    let cpi_accounts = fee_quoter::cpi::accounts::GetValidatedTokenPriceV2 {
        config: fee_quoter_config,
        billing_token_config,
    };

    let cpi_ctx = CpiContext::new(fee_quoter, cpi_accounts);
    let price = fee_quoter::cpi::get_validated_token_price_v2(cpi_ctx, mint)?.get();

    Ok(price)
}

pub struct QuoteGasForExecV2CpiInputs<'info> {
    fee_quoter: Pubkey,
    fee_quoter_config: AccountInfo<'info>,
    billing_token_config: AccountInfo<'info>,
    fee_quoter_dest_chain_v2: AccountInfo<'info>,
    fee_quoter_dest_chain_legacy: AccountInfo<'info>,
    dest_chain_selector: u64,
    fee_token_mint: Pubkey,
    gas_limit_sum: CrossChainGas,
    bytes_overhead_sum: u32,
}

impl<'info> QuoteGasForExecV2CpiInputs<'info> {
    pub fn new(
        fee_quoter: Pubkey,
        fee_quoter_config: AccountInfo<'info>,
        billing_token_config: AccountInfo<'info>,
        fee_quoter_dest_chain_v2: AccountInfo<'info>,
        fee_quoter_dest_chain_legacy: AccountInfo<'info>,
        fee_token_mint: Pubkey,
        dest_chain_selector: u64,
    ) -> Self {
        Self {
            fee_quoter,
            fee_quoter_config,
            billing_token_config,
            fee_quoter_dest_chain_v2,
            dest_chain_selector,
            fee_token_mint: fee_token_mint.normalize_wsol(),
            // Filled later in the receipt collection phase
            gas_limit_sum: CrossChainGas::ZERO,
            bytes_overhead_sum: 0,
            fee_quoter_dest_chain_legacy,
        }
    }
}

pub(crate) fn quote_gas_for_exec_fee_quoter_cpi(
    inputs: QuoteGasForExecV2CpiInputs,
) -> Result<QuoteGasForExecV2Result> {
    let cpi_accounts = fee_quoter::cpi::accounts::QuoteGasForExecV2 {
        config: inputs.fee_quoter_config,
        billing_token_config: inputs.billing_token_config,
        dest_chain: inputs.fee_quoter_dest_chain_v2,
        legacy_dest_chain: inputs.fee_quoter_dest_chain_legacy,
    };

    let cpi_ctx = CpiContext::new(inputs.fee_quoter, cpi_accounts);
    let params = QuoteGasForExecV2Params {
        dest_chain_selector: inputs.dest_chain_selector,
        fee_token_mint: inputs.fee_token_mint,
        gas_limit_sum: inputs.gas_limit_sum,
        bytes_overhead_sum: inputs.bytes_overhead_sum,
    };
    let quote = fee_quoter::cpi::quote_gas_for_exec_v2(cpi_ctx, params)?.get();

    Ok(quote)
}

/// Fee quoter accounts required by the fallback token-transfer-fee CPI, bundled so that both
/// `ccip_send_v2` and `get_fee_v2` pass them around uniformly.
pub(crate) struct FeeQuoterCpiAccounts<'info> {
    pub fee_quoter: Pubkey,
    pub config: AccountInfo<'info>,
    pub dest_chain: AccountInfo<'info>,
}

pub(crate) fn get_fallback_token_transfer_fee_cpi<'info>(
    fee_quoter_accounts: &FeeQuoterCpiAccounts<'info>,
    per_chain_per_token_config: AccountInfo<'info>,
    dest_chain_selector: u64,
    mint: Pubkey,
) -> Result<FallbackTokenTransferFeeV2> {
    let cpi_accounts = fee_quoter::cpi::accounts::GetFallbackTokenTransferFeeV2 {
        config: fee_quoter_accounts.config.clone(),
        dest_chain: fee_quoter_accounts.dest_chain.clone(),
        per_chain_per_token_config,
    };

    let cpi_ctx = CpiContext::new(fee_quoter_accounts.fee_quoter, cpi_accounts);
    let fallback_fee = fee_quoter::cpi::get_fallback_token_transfer_fee_v2(
        cpi_ctx,
        GetFallbackTokenTransferFeeV2Params {
            dest_chain_selector,
            mint,
        },
    )?
    .get();

    Ok(fallback_fee)
}

pub(crate) fn get_pool_fee_cpi<'info>(
    accounts: &CcipV2OnrampTokenAccounts<'info>,
    get_fee_in: GetFeeParams,
) -> Result<base_token_pool::v2::GetFeeResult> {
    // Sized up front, and the metas built by hand: both `Vec` growth and the one-element Vec that
    // `to_account_metas` allocates per account are permanently lost on the bump-allocated heap.
    let mut acc_infos = Vec::with_capacity(3 + accounts.extra_cpi_accounts.len());
    acc_infos.extend_from_slice(&[
        accounts.common.pool_config.clone(),
        accounts.common.pool_chain_config.clone(),
        accounts.common.pool_chain_config_v2.clone(),
    ]);
    // NOTE we pass all the auto-resolved extra accounts to the token pool
    // here. They're likely unneeded, but token pools may have a use
    // for them during fee quoting as well
    acc_infos.extend_from_slice(accounts.extra_cpi_accounts);
    let acc_metas = account_metas_preserving_writability(&acc_infos);
    let pool_program = accounts.common.pool_program.key();

    let ix = Instruction {
        program_id: pool_program,
        accounts: acc_metas,
        data: get_fee_in.to_tx_data(),
    };

    // CPI call to pool is expected to return data using set_return_data
    // anchor does this automatically but is limited to max 256 bytes
    // https://github.com/coral-xyz/anchor/blob/0109f4a3cf4117570f627e3ae465b6247d504f69/lang/syn/src/codegen/program/handlers.rs#L113
    invoke(&ix, &acc_infos)?;

    parse_verified_cpi_return_as(
        pool_program,
        CcipRouterError::InvalidCpiReturnProgram,
        CcipRouterError::InvalidInstructionData,
    )
}

pub(crate) fn get_executor_fee_cpi<'info>(
    executor_accounts: &[AccountInfo<'info>],
    executor: Pubkey,
    params: executor::GetFeeParams,
) -> Result<UsdCents> {
    // The last resolved executor account is the vault ATA, which is not needed for the
    // get_fee CPI, so we exclude it from here:
    let executor_accounts_for_cpi = &executor_accounts[..executor_accounts.len().saturating_sub(1)];

    let acc_metas = account_metas_preserving_writability(executor_accounts_for_cpi);

    let ix = Instruction {
        program_id: executor,
        accounts: acc_metas,
        data: params.to_tx_data(),
    };

    // CPI call to pool is expected to return data using set_return_data
    // anchor does this automatically but is limited to max 256 bytes
    // https://github.com/coral-xyz/anchor/blob/0109f4a3cf4117570f627e3ae465b6247d504f69/lang/syn/src/codegen/program/handlers.rs#L113
    invoke(&ix, executor_accounts)?;

    parse_verified_cpi_return_as(
        executor,
        CcipRouterError::InvalidCpiReturnProgram,
        CcipRouterError::InvalidInstructionData,
    )
}

pub(crate) fn legacy_fallback_fee_to_receipt(
    pool_program: Pubkey,
    fallback_fee: FallbackTokenTransferFeeV2,
) -> Result<Receipt> {
    Ok(Receipt {
        // The pool program issues the receipt, but its fee vault is qualified by the mint of
        // the managed token, so a program managing multiple tokens accrues fees separately for
        // each of them (see `ccip_common::seed::vault_ata`).
        issuer: pool_program,
        dest_gas_limit: fallback_fee.dest_gas_limit,
        dest_bytes_overhead: fallback_fee.dest_bytes_overhead,
        fee_token_amount: fallback_fee.flat_fee.convert(),
        extra_args: vec![],
    })
}

/// Retrieves the fallback token transfer fee from the fee quoter and adapts it into a token
/// receipt issued by the pool program. Used when the pool is a legacy (V1) pool or a V2 pool
/// that has custom fees disabled.
fn legacy_fallback_token_pool_receipt<'info>(
    fee_quoter_accounts: &FeeQuoterCpiAccounts<'info>,
    per_chain_per_token_config: AccountInfo<'info>,
    dest_chain_selector: u64,
    mint: Pubkey,
    pool_program: Pubkey,
) -> Result<Receipt> {
    let fallback = get_fallback_token_transfer_fee_cpi(
        fee_quoter_accounts,
        per_chain_per_token_config,
        dest_chain_selector,
        mint,
    )?;

    legacy_fallback_fee_to_receipt(pool_program, fallback)
}

/// The validated token accounts of a token transfer, together with the fee accounting for it.
pub(crate) enum PreflightValidatedTokenTransfer<'info> {
    Legacy {
        token_accounts: CcipV1OnrampTokenAccounts<'info>,
        receipt: Receipt,
    },
    V2 {
        token_accounts: CcipV2OnrampTokenAccounts<'info>,
        required_ccvs: Vec<Pubkey>,
        receipt: Receipt,
    },
}

impl PreflightValidatedTokenTransfer<'_> {
    /// The pool's contribution to the message's required-CCV list, in the shape
    /// `merge_ccv_lists` expects. A legacy pool has no `get_required_ccvs` interface, and a v2
    /// pool answering with an empty list specifies nothing: both collapse to the
    /// `INCLUDE_DEFAULT_CCVS` sentinel so the transfer is still verified by the lane defaults.
    /// Mirrors EVM `_getCCVsForPool` and the offramp's `PoolCcvSource` handling, and is shared
    /// by `ccip_send_v2` and `get_fee_v2` so quote and send can never diverge.
    pub(crate) fn pool_required_ccvs(&self) -> Vec<Pubkey> {
        match self {
            PreflightValidatedTokenTransfer::Legacy { .. } => vec![INCLUDE_DEFAULT_CCVS],
            PreflightValidatedTokenTransfer::V2 { required_ccvs, .. } => {
                pool_ccvs_with_default_fallback(required_ccvs.clone())
            }
        }
    }
}

/// Validates the accounts of the single transferred token and produces its token pool receipt
/// (for V2 pools, also the CCVs the pool requires). Fee accounting is identical for quoting
/// (`get_fee_v2`) and sending (`ccip_send_v2`), so both flows share this function.
///
/// The token pool receipt comes from the pool itself when it's a V2 pool with custom fees
/// enabled, and from the fee quoter fallback otherwise.
#[allow(clippy::too_many_arguments)]
pub(crate) fn validate_token_transfer<'info>(
    sender: Pubkey,
    dest_chain_selector: u64,
    fee_token: Pubkey,
    finality: FinalityConfig,
    router: Pubkey,
    fee_quoter_accounts: &FeeQuoterCpiAccounts<'info>,
    context: &mut TokenTransferContext,
    token_transfer_accounts: &'info [AccountInfo<'info>],
    extra_args: &GenericExtraArgsV3,
    // Relevant only for `ccip_send`. `get_fee` does not need to
    // validate that the ATA exists.
    validate_sender_ata: bool,
) -> Result<PreflightValidatedTokenTransfer<'info>> {
    match context {
        TokenTransferContext::None => {
            unreachable!("Never called in the no transfer case")
        }
        TokenTransferContext::Legacy {
            amount,
            pool_program,
        } => {
            require_gt!(amount.amount, 0, CcipRouterError::InvalidInputsTokenAmount);

            let token_accounts = validate_and_parse_token_accounts_legacy(
                sender,
                amount.token,
                dest_chain_selector,
                router,
                fee_quoter_accounts.fee_quoter,
                token_transfer_accounts,
                validate_sender_ata,
            )?;

            let receipt = legacy_fallback_token_pool_receipt(
                fee_quoter_accounts,
                token_accounts.common.per_chain_per_token_config.clone(),
                dest_chain_selector,
                amount.token,
                token_accounts.common.pool_program.key(),
            )?;

            *pool_program = Some(token_accounts.common.pool_program.key());

            Ok(PreflightValidatedTokenTransfer::Legacy {
                token_accounts,
                receipt,
            })
        }
        TokenTransferContext::V2(amount) => {
            require_gt!(amount.amount, 0, CcipRouterError::InvalidInputsTokenAmount);

            let token_accounts = validate_and_parse_onramp_token_accounts_v2(
                sender,
                amount.token,
                dest_chain_selector,
                router,
                fee_quoter_accounts.fee_quoter,
                token_transfer_accounts,
                validate_sender_ata,
            )?;

            let get_fee_result = get_pool_fee_cpi(
                &token_accounts,
                GetFeeParams {
                    remote_chain_selector: dest_chain_selector,
                    amount: amount.amount,
                    local_token: amount.token,
                    fee_token,
                    requested_finality_config: finality,
                    token_args: extra_args.token_args.clone(),
                },
            )?;

            let receipt = if get_fee_result.is_enabled {
                token_pool_get_fee_result_to_receipt(
                    get_fee_result,
                    token_accounts.common.pool_program.key(),
                    &extra_args.token_args,
                )?
            } else {
                legacy_fallback_token_pool_receipt(
                    fee_quoter_accounts,
                    token_accounts.common.per_chain_per_token_config.clone(),
                    dest_chain_selector,
                    amount.token,
                    token_accounts.common.pool_program.key(),
                )?
            };

            let required_ccvs = get_pool_required_ccvs_cpi(
                token_accounts
                    .common
                    .get_pool_required_ccvs_accounts()
                    .as_slice(),
                ProtocolAmount::from(amount.amount),
                amount.token,
                dest_chain_selector,
                finality,
                MessageDirection::Outbound,
            )?;

            Ok(PreflightValidatedTokenTransfer::V2 {
                token_accounts,
                required_ccvs,
                receipt,
            })
        }
    }
}

pub(crate) enum TokenTransferContext {
    None,
    Legacy {
        amount: SVMTokenAmount,
        // Gets written later in the context lifecycle, hence the option.
        pool_program: Option<Pubkey>,
    },
    V2(SVMTokenAmount),
}

impl TokenTransferContext {
    pub(crate) fn new(
        token_admin_registry: &AccountInfo<'_>,
        token_amounts: &[SVMTokenAmount],
    ) -> Result<Self> {
        let amount = match &token_amounts {
            [] => return Ok(Self::None),
            [amount] => amount,
            _ => return Err(CcipRouterError::InvalidInputsMultipleTokenTransfers.into()),
        };

        let registry = load_token_admin_registry_checked(token_admin_registry, amount.token)?;
        let is_legacy = registry.interface_version == 1;
        if is_legacy && registry.account_resolution.is_enabled() {
            return Err(CcipRouterError::UnsupportedPoolV1ResolutionOnLaneV2.into());
        }

        if is_legacy {
            Ok(Self::Legacy {
                amount: amount.clone(),
                // Will be added later on account parsing
                pool_program: None,
            })
        } else {
            Ok(Self::V2(amount.clone()))
        }
    }

    pub fn is_token_transfer(&self) -> bool {
        !matches!(self, Self::None)
    }

    pub fn is_legacy_token_transfer(&self) -> bool {
        matches!(self, Self::Legacy { .. })
    }

    /// Mint of the transferred token, if any.
    pub fn token_mint(&self) -> Option<Pubkey> {
        match self {
            Self::None => None,
            Self::Legacy { amount, .. } => Some(amount.token),
            Self::V2(amount) => Some(amount.token),
        }
    }
}

/// Adapts a V2 pool's fee quote into the token receipt issued by the pool program.
///
/// The receipt charges only the pool's flat fee, converted to the fee token. The quote's
/// `token_fee_bps` is deliberately excluded: as on EVM (where `OnRamp._getReceipts` ignores
/// `tokenFeeBps` and `TokenPool.lockOrBurn` deducts `amount * bps / BPS_DIVIDER` in-token),
/// the BPS fee is charged by the pool in its own token before locking or burning the rest.
pub fn token_pool_get_fee_result_to_receipt(
    get_fee_result: base_token_pool::v2::GetFeeResult,
    pool_program: Pubkey,
    token_args: &[u8],
) -> Result<Receipt> {
    Ok(Receipt {
        // The pool program issues the receipt, but its fee vault is qualified by the mint of
        // the managed token, so a program managing multiple tokens accrues fees separately for
        // each of them (see `ccip_common::seed::vault_ata`).
        issuer: pool_program,
        dest_gas_limit: get_fee_result.dest_gas_limit,
        dest_bytes_overhead: get_fee_result.dest_bytes_overhead,
        fee_token_amount: get_fee_result.fee.convert(),
        extra_args: token_args.to_vec(),
    })
}

#[cfg(test)]
mod tests {
    use super::*;
    use ccip_common::state::CodeVersion;

    // `TokenTransferContext::new` (and every other reader of a `TokenAdminRegistry`) relies on
    // `load_token_admin_registry_checked` binding the account to the transferred token's registry
    // PDA. That derivation happens inside `ccip_common`, against `ccip_common::ID`, while the PDA is
    // really owned by and derived under this program. The two `declare_id!`s must therefore match:
    // if they ever diverge, the binding would reject every legitimate registry.
    #[test]
    fn test_common_shares_the_router_program_id() {
        assert_eq!(ccip_common::ID, crate::ID);
    }

    // $150 per whole SOL (9 decimals): 18-decimals USD for 1e18 lamports = 150e9 * 1e18 = 1.5e29.
    fn sol_price() -> Usd18Decimals {
        Usd18Decimals::from(15u32.e(28))
    }

    fn test_dest_chain_config() -> DestChainConfigCcipV2 {
        DestChainConfigCcipV2 {
            lane_code_version: CodeVersion::Default,
            address_bytes_length: 32,
            token_receiver_allowed: true,
            allowed_senders: vec![],
            allow_list_enabled: false,
            default_ccvs: vec![],
            lane_mandated_ccvs: vec![],
            default_executor: Pubkey::new_unique(),
            offramp: vec![7u8; 20],                 // EVM-sized address
            message_network_fee: UsdCents::new(50), // $0.50
            token_transfer_network_fee: UsdCents::new(150), // $1.50
            base_execution_gas_cost: CrossChainGas::new(100_000),
            max_fee_per_message: UsdCents::new(1000), // $10
        }
    }

    fn test_message(token_amounts: Vec<SVMTokenAmount>) -> SVM2AnyMessage {
        SVM2AnyMessage {
            receiver: vec![0u8; 20],
            data: vec![],
            token_amounts,
            fee_token: Pubkey::default(),
            extra_args: vec![],
        }
    }

    fn token_amount() -> SVMTokenAmount {
        SVMTokenAmount {
            token: Pubkey::new_unique(),
            amount: 1_000_000,
        }
    }

    #[test]
    fn usd_fee_conversion_yields_smallest_denominations() {
        // $0.50 at $150/SOL = 1/300 SOL, truncated to 3_333_333 lamports.
        assert_eq!(
            usd_fee_to_fee_token_amount(UsdCents::new(50), &sol_price()).unwrap(),
            3_333_333
        );
        // Fees above the u16 cents range must survive: $1000 = 6.66 SOL.
        assert_eq!(
            usd_fee_to_fee_token_amount(UsdCents::new(100_000), &sol_price()).unwrap(),
            6_666_666_666
        );
        assert_eq!(
            usd_fee_to_fee_token_amount(UsdCents::new(0), &sol_price()).unwrap(),
            0
        );
    }

    #[test]
    fn usd_fee_conversion_rejects_zero_price() {
        assert!(usd_fee_to_fee_token_amount(UsdCents::new(50), &Usd18Decimals::ZERO).is_err());
    }

    #[test]
    fn network_fee_receipt_charges_by_message_kind() {
        let config = test_dest_chain_config();

        // Receipts carry USD cents until `collect_receipts` converts them.
        let plain = network_fee_receipt(&config, &test_message(vec![])).unwrap();
        assert_eq!(plain.issuer, crate::ID);
        assert_eq!(plain.fee_token_amount, 50);
        assert_eq!(plain.dest_gas_limit, CrossChainGas::new(0));

        let with_token = network_fee_receipt(&config, &test_message(vec![token_amount()])).unwrap();
        assert_eq!(with_token.fee_token_amount, 150);
    }

    #[test]
    fn legacy_fallback_fee_receipt_converts_flat_fee() {
        let pool_program = Pubkey::new_unique();
        let fallback = FallbackTokenTransferFeeV2 {
            flat_fee: UsdCents::new(200), // $2.00
            dest_gas_limit: CrossChainGas::new(90_000),
            dest_bytes_overhead: 32,
        };

        let receipt = legacy_fallback_fee_to_receipt(pool_program, fallback).unwrap();
        assert_eq!(receipt.issuer, pool_program);
        assert_eq!(receipt.dest_gas_limit, CrossChainGas::new(90_000));
        assert_eq!(receipt.dest_bytes_overhead, 32);
        // The flat fee in cents, NOT the transferred token amount.
        assert_eq!(receipt.fee_token_amount, 200);
    }

    #[test]
    fn token_pool_receipt_charges_flat_fee_only() {
        let pool_program = Pubkey::new_unique();
        // A non-zero BPS quote must not surface in the receipt: the pool charges it in its own
        // token during lock/burn, exactly as on EVM.
        let receipt = token_pool_get_fee_result_to_receipt(
            base_token_pool::v2::GetFeeResult {
                dest_gas_limit: CrossChainGas::new(90_000),
                dest_bytes_overhead: 32,
                fee: UsdCents::new(50), // $0.50
                token_fee_bps: 250,
                is_enabled: true,
            },
            pool_program,
            &[],
        )
        .unwrap();

        assert_eq!(receipt.issuer, pool_program);
        assert_eq!(receipt.dest_gas_limit, CrossChainGas::new(90_000));
        assert_eq!(receipt.dest_bytes_overhead, 32);
        // $0.50 in cents, with no BPS component.
        assert_eq!(receipt.fee_token_amount, 50);
    }

    #[test]
    fn fee_multiplier_matches_evm() {
        // EVM's `OnRamp._getReceipts` example: $1.50 at $15/token with a 100% premium is ~0.1
        // tokens of an 18-decimals token, just under it because the multiplier is floored.
        let price = Usd18Decimals::from(15u32.e(18));
        assert_eq!(
            fee_multiplier(100, &price).unwrap() * 150,
            U256::from(99_999_999_999_999_900u64)
        );

        // 9-decimals SOL at $150: the multiplier is floored once, as on EVM.
        assert_eq!(
            fee_multiplier(100, &sol_price()).unwrap(),
            U256::from(66_666u32)
        );
        assert_eq!(
            fee_multiplier(150, &sol_price()).unwrap(),
            U256::from(100_000u32)
        );
    }

    #[test]
    fn fee_multiplier_rejects_zero_price() {
        assert!(fee_multiplier(100, &Usd18Decimals::ZERO).is_err());
    }
}
