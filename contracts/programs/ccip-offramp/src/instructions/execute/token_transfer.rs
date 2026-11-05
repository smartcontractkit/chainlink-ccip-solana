use anchor_lang::prelude::program::invoke_signed;
use anchor_lang::prelude::*;
use anchor_spl::token_interface::TokenAccount;
use solana_program::instruction::Instruction;

use base_token_pool::state::RemoteAddress;
use base_token_pool::v2::{ReleaseOrMintInV2, ReleaseOrMintOutV2};
use ccip_common::cpi::get_verified_cpi_return_data;
use ccip_common::messages::{
    FinalityConfig, MessageV1, SVMDestBlob, SVMTokenReceiverUsage, TokenTransferV1,
};
use ccip_common::router_accounts::TokenAdminRegistry;
use ccip_common::seed;
use ccip_common::types::ToTxData;
use ccip_common::v1::{
    load_token_admin_registry_checked,
    validate_and_parse_offramp_token_accounts as validate_and_parse_offramp_token_accounts_legacy,
    CcipV1OfframpTokenAccounts,
};
use ccip_common::v2::{validate_and_parse_offramp_token_accounts_v2, CcipV2OfframpTokenAccounts};
use fee_quoter::messages::SVMTokenAmount;

use super::token_transfer_legacy::{
    interact_with_pool as interact_with_pool_legacy, ReleaseOrMintInV1 as ReleaseOrMintInLegacy,
    ReleaseOrMintOutV1 as ReleaseOrMintOutLegacy, CCIP_POOL_V1_RET_BYTES,
};
use super::ExecuteRemainingAccountsLayout;
use super::ExecuteV2;

use crate::error::CcipOfframpError;
use crate::instructions::ccv_resolution;
use crate::instructions::ccv_resolution::{PoolCcvSource, TokenPoolCcvCpiIn};
use crate::instructions::execute::parse_pubkey;
use crate::messages::TokenTransfer;

// Holds context related to token transfers during execution.
//
// Different token transfer cases (legacy, v2, and no transfer) require different
// handling at multiple points of execution. In order to avoid re-checking what
// case we're in at each of those points, this type internally carries the relevant
// context for each of the cases, so the execution logic can abstract away the details.
pub(crate) enum TokenTransferContext<'info> {
    None,
    V1(CcipV1OfframpTokenAccounts<'info>, SVMTokenReceiverUsage),
    V2(
        CcipV2OfframpTokenAccounts<'info>,
        SVMTokenReceiverUsage,
        Option<TokenPoolCcvCpiIn<'info>>,
    ),
}

impl<'a, 'info> TokenTransferContext<'info> {
    /// The source of token-pool-required CCVs for this transfer, for `get_ccvs_for_message`.
    /// A legacy v1 pool maps to `LegacyV1` (→ lane defaults, EVM parity) rather than "no transfer",
    /// so a v1 token transfer is still verified by the default CCVs.
    pub fn take_pool_ccv_source(&mut self) -> PoolCcvSource<'info> {
        match self {
            Self::None => PoolCcvSource::NoTokenTransfer,
            Self::V1(..) => PoolCcvSource::LegacyV1,
            Self::V2(.., p) => p
                .take()
                .map(PoolCcvSource::V2)
                .unwrap_or(PoolCcvSource::NoTokenTransfer),
        }
    }

    /// `dest_blob` is the message's dest blob, parsed once by `execute_v2` and shared with the
    /// receiver CPI. It must be `Some` whenever the message carries a token transfer: the blob
    /// carries the token-receiver ATA usage, which drives both the canonical token-account
    /// validation below — whose account-slice layout depends on the usage — and the transfer
    /// dispatch, so the two cannot disagree on the expected accounts.
    pub fn new(
        message: &'a MessageV1,
        ctx: &'a Context<'info, ExecuteV2<'info>>,
        tar_account: &'info AccountInfo<'info>,
        layout: &'a ExecuteRemainingAccountsLayout<'info, 'info>,
        dest_blob: Option<&'a SVMDestBlob>,
    ) -> Result<Self> {
        Ok(match &message.token_transfer {
            Some(transfer) => {
                let tar = load_token_admin_registry_checked(
                    tar_account,
                    parse_pubkey(&transfer.dest_token_address)?,
                )?;
                let use_ata = dest_blob.ok_or(CcipOfframpError::InvalidDestBlob)?.use_ata;

                if tar.interface_version > 1 {
                    let (accounts, ccv_cpi_params) =
                        validate_v2_token_transfer(ctx, message, layout, &tar, use_ata)?;
                    TokenTransferContext::V2(accounts, use_ata, Some(ccv_cpi_params))
                } else {
                    let accounts = validate_v1_token_transfer(ctx, message, layout, use_ata)?;
                    require!(
                        !tar.account_resolution.is_enabled(),
                        CcipOfframpError::UnsupportedPoolV1ResolutionOnLaneV2
                    );
                    TokenTransferContext::V1(accounts, use_ata)
                }
            }
            None => TokenTransferContext::None,
        })
    }

    /// If the context carries a token transfer (v1 or v2) it's executed and the delivered
    /// [`TokenTransfer`] is returned (the local `SVMTokenAmount` plus the `token_receiver` + usage,
    /// for the receiver's `Any2SVMMessage`). Otherwise it's a noop returning `None`.
    /// `init_bal` is the receiver's initial token account balance snapshotted at the start of execute.
    pub fn execute(
        self,
        ctx: &Context<'info, ExecuteV2<'info>>,
        message: &'a MessageV1,
        init_bal: Option<u64>,
    ) -> Result<Option<TokenTransfer>> {
        match self {
            TokenTransferContext::None => Ok(None),
            TokenTransferContext::V1(accounts, token_receiver_usage) => transfer_token_legacy(
                message.token_transfer.as_ref().unwrap(),
                ctx,
                message,
                &accounts,
                token_receiver_usage,
                init_bal.expect("token transfer always has an init_bal"),
            )
            .map(Some),
            TokenTransferContext::V2(accounts, token_receiver_usage, _) => transfer_token_ccipv2(
                message.token_transfer.as_ref().unwrap(),
                &accounts,
                ctx,
                message,
                token_receiver_usage,
                init_bal.expect("token transfer always has an init_bal"),
            )
            .map(Some),
        }
    }
}

fn validate_v2_token_transfer<'a, 'info>(
    ctx: &'a Context<'info, ExecuteV2<'info>>,
    message: &'a MessageV1,
    layout: &'a ExecuteRemainingAccountsLayout<'info, 'info>,
    tar: &'a TokenAdminRegistry,
    token_receiver_usage: SVMTokenReceiverUsage,
) -> Result<(CcipV2OfframpTokenAccounts<'info>, TokenPoolCcvCpiIn<'info>)> {
    let token_transfer = message
        .token_transfer
        .as_ref()
        .expect("Must exist if we're here");

    // SECURITY: the required-CCV set for a token transfer must be driven by the *canonical* pool for
    // the message's token — not by accounts the (permissionless) executor supplies. We therefore
    // validate the full v2 token-pool account set against the message mint here (the same validation
    // the release/mint path performs), and feed the CCV CPI the canonical pool accounts derived from
    // it. This mirrors EVM, where `_getCCVsFromPool` resolves the pool from the trusted
    // TokenAdminRegistry keyed by the token, so the caller cannot substitute a pool that under-reports
    // required CCVs.
    let message_mint = parse_pubkey(&token_transfer.dest_token_address)?;
    let (router, fee_quoter) = {
        let reference_addresses = &ctx.accounts.reference_addresses;
        (reference_addresses.router, reference_addresses.fee_quoter)
    };

    let token_receiver = parse_pubkey(&token_transfer.token_receiver)?;
    // `validate_and_parse_offramp_token_accounts_v2` derives the pool program from the mint-bound
    // TokenAdminRegistry's lookup table and validates every pool PDA against the mint, so
    // the returned pool accounts are canonical and cannot be substituted by the caller.
    // Validate with the real `use_ata` so the parsed account set matches the resolved layout
    // and can be reused as-is by the transfer (no second validation).
    let validated_token_accounts_v2 = validate_and_parse_offramp_token_accounts_v2(
        token_receiver,
        message_mint,
        message.source_chain_selector,
        router,
        fee_quoter,
        crate::ID,
        token_receiver_usage,
        layout.token_transfer_accounts,
    )?;

    // The accounts the `get_required_ccvs` CPI needs
    // (`[pool_program, pool_config, pool_chain_config, chain_config_v2]`) are all already resolved and
    // mint-validated by `validate_and_parse_offramp_token_accounts_v2` above — in particular
    // `chain_config_v2` is PDA-checked as `POOL_CHAINCONFIG_SEED_2_0` under the canonical (LUT-derived,
    // mint-bound) pool program by `OfframpTokenAccountsValidationContext`. So we reuse them directly;
    // no dedicated segment or re-validation is needed, and a permissionless executor cannot substitute
    // a pool that under-reports CCVs. (`release_or_mint` simply does not pass `chain_config_v2` to its
    // own CPI — burnmint's `TokenOfframpV2` has no such account — only `get_required_ccvs` consumes it.)
    let pool_ccv_accounts: Vec<AccountInfo<'info>> = validated_token_accounts_v2
        .common
        .get_pool_required_ccvs_accounts();

    // Only the token-transfer path has a real `token_admin_registry` account; messaging-only
    // messages pass the default/system pubkey, so loading (and ownership-checking) it
    // unconditionally would fail. `get_token_pool_required_ccvs_inputs` returns `None` for
    // non-token-transfer messages anyway, so skip the load entirely in that case.
    let token_pool_params = get_token_pool_required_ccvs_inputs(
        pool_ccv_accounts,
        token_transfer,
        message.source_chain_selector,
        message.finality,
        tar,
    )?
    .expect("We are in a v2 token transfer");

    Ok((validated_token_accounts_v2, token_pool_params))
}

fn validate_v1_token_transfer<'a, 'info>(
    ctx: &'a Context<'info, ExecuteV2<'info>>,
    message: &'a MessageV1,
    layout: &'a ExecuteRemainingAccountsLayout<'info, 'info>,
    token_receiver_usage: SVMTokenReceiverUsage,
) -> Result<CcipV1OfframpTokenAccounts<'info>> {
    let token_transfer = message
        .token_transfer
        .as_ref()
        .expect("Must exist if we're here");

    // PoolsV1 generally validate that the receiver is the ATA to the wallet specified in the ReleaseOrMintInV1.
    // Unless we relax that validation and upgrade all pools, we cannot support the UseAsIs variant, where there's
    // no wallet and the receiver might be a Token Account (not Associated). If we're already upgrading pools,
    // we might as well just upgrade them to PoolV2, which no longer uses this legacy handler at all.
    require_neq!(
        token_receiver_usage,
        SVMTokenReceiverUsage::UseAsIs,
        CcipOfframpError::UnsupportedTokenReceiverUseAsIsForPoolV1,
    );

    let reference_addresses = &ctx.accounts.reference_addresses;

    let receiver = parse_pubkey(&token_transfer.token_receiver)?;
    let mint = parse_pubkey(&token_transfer.dest_token_address)?;
    let accs = validate_and_parse_offramp_token_accounts_legacy(
        receiver,
        mint,
        message.source_chain_selector,
        reference_addresses.router,
        reference_addresses.fee_quoter,
        crate::ID,
        &token_receiver_usage,
        layout.token_transfer_accounts,
    )?;

    Ok(accs)
}

/// Packages the pool accounts + amount/selector/finality into the input the CCV aggregation needs to
/// CPI the pool's `get_required_ccvs`. Returns `None` for non-token / legacy-v1 transfers.
///
/// IMPORTANT: this helper does NOT verify that `pool_accounts` are the canonical pool accounts for
/// the message's token. Callers are responsible for that binding:
/// - `execute_v2` passes accounts derived from `validate_v2_token_pool_accounts` (canonical, mint-bound).
/// - The off-chain `get_ccvs_for_msg` view is non-authoritative and passes caller-supplied accounts;
///   `execute_v2` re-derives and enforces the real set, so the view cannot be used to weaken it.
pub fn get_token_pool_required_ccvs_inputs<'info>(
    pool_accounts: Vec<AccountInfo<'info>>,
    token_transfer: &TokenTransferV1,
    source_chain_selector: u64,
    requested_finality_config: FinalityConfig,
    tar: &TokenAdminRegistry,
) -> Result<Option<ccv_resolution::TokenPoolCcvCpiIn<'info>>> {
    if tar.interface_version <= 1 {
        return Ok(None); // Legacy v1 pool: no `get_required_ccvs` interface; handled by the legacy transfer path.
    }

    require_eq!(
        pool_accounts.len(),
        4,
        CcipOfframpError::InvalidInputsPoolAccounts
    );

    Ok(Some(ccv_resolution::TokenPoolCcvCpiIn {
        pool_accounts,
        source_denominated_amount: token_transfer.amount,
        remote_chain_selector: source_chain_selector,
        requested_finality_config,
        local_token: parse_pubkey(&token_transfer.dest_token_address)?,
    }))
}

fn transfer_token_legacy<'info>(
    token_transfer: &TokenTransferV1,
    ctx: &Context<'info, ExecuteV2<'info>>,
    message: &MessageV1,
    accounts: &CcipV1OfframpTokenAccounts<'info>,
    token_receiver_usage: SVMTokenReceiverUsage,
    init_bal: u64,
) -> Result<TokenTransfer> {
    let receiver = parse_pubkey(&token_transfer.token_receiver)?;
    let mint = parse_pubkey(&token_transfer.dest_token_address)?;

    let release_or_mint = ReleaseOrMintInLegacy {
        original_sender: message.sender.clone(),
        receiver,
        amount: token_transfer.amount.into(),
        local_token: mint,
        remote_chain_selector: message.source_chain_selector,
        source_pool_address: token_transfer.source_pool_address.clone(),
        source_pool_data: token_transfer.extra_data.clone(),
        // Unsupported for legacy transfers in 2.0
        offchain_token_data: vec![],
    };

    let mut acc_infos = vec![
        accounts.ccip_offramp_pool_signer.to_account_info(),
        ctx.accounts.offramp.to_account_info(),
        ctx.accounts.allowed_offramp.to_account_info(),
        accounts.common.pool_config.to_account_info(),
        accounts.common.token_program.to_account_info(),
        accounts.common.mint.to_account_info(),
        accounts.common.pool_signer.to_account_info(),
        accounts.common.pool_token_account.to_account_info(),
        accounts.common.pool_chain_config.to_account_info(),
        ctx.accounts.rmn_remote.to_account_info(),
        ctx.accounts.rmn_remote_curses.to_account_info(),
        ctx.accounts.rmn_remote_config.to_account_info(),
        accounts.common.user_token_account.to_account_info(),
    ];

    // LUT extras
    // NOTE: In the pre 2.0 offramp, we'd also extend `acc_infos` with the
    // pools auto-derived additional accounts, but we don't support
    // these on 2.0 legacy token transfers.
    acc_infos.extend_from_slice(accounts.remaining_accounts);

    let seeds = &[
        seed::EXTERNAL_TOKEN_POOLS_SIGNER,
        accounts.common.pool_program.key.as_ref(),
        &[accounts.ccip_offramp_pool_signer_bump],
    ];

    let return_data = interact_with_pool_legacy(
        accounts.common.pool_program.key(),
        accounts.ccip_offramp_pool_signer.key(),
        acc_infos,
        release_or_mint,
        seeds,
    )?;

    require_eq!(
        return_data.len(),
        CCIP_POOL_V1_RET_BYTES,
        CcipOfframpError::OfframpInvalidDataLength
    );

    let mint_output = ReleaseOrMintOutLegacy::try_from_slice(&return_data)?;

    // When the receiver is the pool itself, we skip the balance validation. This contemplates cases such as
    // LockRelease pools, where the pool itself may be the receiver of the tokens, so it would just "unlock"
    // the tokens back to itself, thus incurring no actual balance change.
    if accounts.common.user_token_account.key() != accounts.common.pool_token_account.key() {
        // Validate user received tokens according to the amount returned by the token pool.
        // For cases such as CCTPv2 FTF, where it's the CCV who does the minting _and_ it also charges
        // dest-side fees, use a PoolV2 instead of a legacy one - we don't support that case in this
        // legacy token transfer path here.
        let post_bal = get_balance(accounts.common.user_token_account)?;

        require_gte!(
            post_bal,
            init_bal,
            CcipOfframpError::OfframpReleaseMintBalanceMismatch
        );

        require_eq!(
            // SAFETY: the pool just credited the account, so `post_bal >= init_bal`; an underflow would abort
            // under overflow-checks rather than wrap, and the require_eq! below rejects any unexpected delta.
            post_bal - init_bal,
            mint_output.destination_amount,
            CcipOfframpError::OfframpReleaseMintBalanceMismatch
        );
    }

    Ok(TokenTransfer {
        token_amount: SVMTokenAmount {
            token: mint,
            amount: mint_output.destination_amount,
        },
        token_receiver: receiver,
        token_receiver_usage,
    })
}

fn transfer_token_ccipv2<'info>(
    token_transfer: &TokenTransferV1,
    accs: &CcipV2OfframpTokenAccounts<'info>,
    ctx: &Context<'info, ExecuteV2<'info>>,
    message: &MessageV1,
    token_receiver_usage: SVMTokenReceiverUsage,
    init_bal: u64,
) -> Result<TokenTransfer> {
    // `accs` was already validated against the message mint — with this same `token_receiver_usage`
    // — by the canonical token-account validation in `execute_v2` (the same accounts used to drive
    // the required-CCV query), so no re-validation is needed here.
    let receiver = parse_pubkey(&token_transfer.token_receiver)?;
    let message_mint = parse_pubkey(&token_transfer.dest_token_address)?;

    let release_or_mint = ReleaseOrMintInV2 {
        original_sender: RemoteAddress {
            address: message.sender.clone(),
        },
        receiver,
        amount: token_transfer.amount,
        local_token: message_mint,
        remote_chain_selector: message.source_chain_selector,
        source_pool_address: RemoteAddress {
            address: token_transfer.source_pool_address.clone(),
        },
        source_pool_data: token_transfer.extra_data.clone(),
        requested_finality_config: message.finality,
        receiver_usage: token_receiver_usage,
    };

    let offramp_token_pool_signer = accs.ccip_offramp_pool_signer;

    let initial_slice = [
        offramp_token_pool_signer.to_account_info(),
        ctx.accounts.offramp.to_account_info(),
        ctx.accounts.allowed_offramp.to_account_info(),
        accs.common.pool_config.to_account_info(),
        accs.common.token_program.to_account_info(),
        accs.common.mint.to_account_info(),
        accs.common.pool_signer.to_account_info(),
        accs.common.pool_token_account.to_account_info(),
        accs.common.pool_chain_config.to_account_info(),
        accs.common.pool_chain_config_override.to_account_info(),
        accs.common.user_token_account.to_account_info(),
    ];
    let mut acc_infos = Vec::with_capacity(initial_slice.len() + accs.extra_cpi_accounts.len());
    acc_infos.extend_from_slice(&initial_slice);

    // `extra_cpi_accounts` are the LUT entries beyond the required set (e.g. multisig); by
    // construction `pool_chain_config_v2` is NOT among them (it is a named field consumed earlier).
    acc_infos.extend_from_slice(accs.extra_cpi_accounts);

    let seeds = &[
        seed::EXTERNAL_TOKEN_POOLS_SIGNER,
        accs.common.pool_program.key.as_ref(),
        &[accs.ccip_offramp_pool_signer_bump],
    ];

    let return_data = interact_with_pool_v2(
        accs.common.pool_program.key(),
        offramp_token_pool_signer.key(),
        acc_infos,
        release_or_mint,
        seeds,
    )?;

    require_eq!(
        return_data.len(),
        ReleaseOrMintOutV2::ret_bytes(),
        CcipOfframpError::OfframpInvalidDataLength
    );

    let amount = if accs.common.user_token_account.key() == accs.common.pool_token_account.key() {
        // If a lock-release pool is the receiver, balancePost - balancePre would not reflect the amount transferred.
        // Therefore, if the receiver is the token pool, we trust the value returned by the pool.
        // Otherwise, we trust balancePost - balancePre as the amount given to the receiver.
        ReleaseOrMintOutV2::try_from_slice(&return_data)?.destination_amount
    } else {
        // The balance after minting must be at least the initial balance. However, on pools that charge fees
        // on destination, the delta may not match the amount returned by the pool, so we can't compare them
        // (e.g. CCTP with FTF may charge dest-side fees, and that's performed in the verifier and not the pool).
        // Instead, we compute the delta and use that as the effective amount that was actually received by the user,
        // which may be different than the amount returned by the pool.
        get_balance(accs.common.user_token_account)?
            .checked_sub(init_bal)
            .ok_or(CcipOfframpError::OfframpReleaseMintBalanceMismatch)?
    };

    Ok(TokenTransfer {
        token_amount: SVMTokenAmount {
            token: message_mint,
            amount,
        },
        token_receiver: receiver,
        token_receiver_usage,
    })
}

fn interact_with_pool_v2(
    pool_program: Pubkey,
    signer: Pubkey,
    acc_infos: Vec<AccountInfo>,
    data: ReleaseOrMintInV2,
    seeds: &[&[u8]],
) -> Result<Vec<u8>> {
    let acc_metas: Vec<AccountMeta> = acc_infos
        .to_vec()
        .iter()
        .flat_map(|acc_info| {
            // Check signer from PDA External Execution config
            let is_signer = acc_info.key() == signer;
            acc_info.to_account_metas(Some(is_signer))
        })
        .collect();

    let ix = Instruction {
        program_id: pool_program,
        accounts: acc_metas,
        data: data.to_tx_data(),
    };

    // CPI call to pool is expected to return data using set_return_data
    // anchor does this automatically but is limited to max 256 bytes
    // https://github.com/coral-xyz/anchor/blob/0109f4a3cf4117570f627e3ae465b6247d504f69/lang/syn/src/codegen/program/handlers.rs#L113
    invoke_signed(&ix, &acc_infos, &[seeds])?;

    get_verified_cpi_return_data(pool_program, CcipOfframpError::InvalidCpiReturnProgram)
}

pub fn get_balance<'a>(token_account: &'a AccountInfo<'a>) -> Result<u64> {
    let mut acc: InterfaceAccount<TokenAccount> = InterfaceAccount::try_from(token_account)?;
    acc.reload()?; // reload state to ensure latest balance
    Ok(acc.amount)
}
