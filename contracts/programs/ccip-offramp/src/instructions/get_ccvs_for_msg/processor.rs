use anchor_lang::prelude::*;
use ccip_common::messages::{FinalityConfig, TokenTransferV1};
use ccip_common::seed;
use ccip_common::state::AccountSchemaVersion;
use ccip_common::types::CrossChainGas;
use ccip_common::v1::load_token_admin_registry_checked;

use crate::error::CcipOfframpError;
use crate::instructions::ccv_resolution::{self, PoolCcvSource};
use crate::instructions::execute::parse_pubkey;
use crate::instructions::execute::token_transfer::get_token_pool_required_ccvs_inputs;
use crate::state::{Config, ReferenceAddresses, SourceChain};

/// Off-chain helper that *predicts* the CCV set an executor should supply to `execute_v2`.
///
/// IMPORTANT: this view is **non-authoritative**. `execute_v2` recomputes the required set
/// independently and is the only place quorum is enforced; the view exists solely so executors can
/// pre-resolve CCV accounts. It shares the receiver-CCV resolution helper with `execute_v2`
/// (`resolve_receiver_required_ccvs`) so the predicted receiver contribution can't diverge from the
/// authoritative one — including the receiver-program target binding (`expected_receiver`).
///
/// It does NOT bind the supplied *pool* accounts to the message's token — it queries whatever pool
/// the caller passes. That is acceptable precisely because it is non-authoritative: an honest
/// executor passes the canonical pool accounts (the same ones the LUT / account resolution yields)
/// to get a correct prediction, and `execute_v2` re-derives and enforces the real set, so the view
/// can never be used to weaken on-chain enforcement.
pub fn get_ccvs_for_msg<'info>(
    ctx: Context<'info, GetCcvsForMsg<'info>>,
    params: GetCcvsForMsgParams,
) -> Result<GetCcvsForMsgResponse> {
    require_eq!(
        params.remote_chain_selector,
        ctx.accounts.source_chain.chain_selector,
        CcipOfframpError::InvalidMessageSourceChainSelector
    );

    let source_chain_config = &ctx.accounts.source_chain.config;

    // The receiver is consulted only for arbitrary (data) messages; token-only transfers are driven
    // by the pool. The v1/v2 decision comes from the router-owned `ReceiverRegistry` PDA.
    let is_arbitrary = !ccip_common::messages::is_offramp_token_only_transfer(
        &params.message_receiver.to_bytes(),
        params.data_len as usize,
        CrossChainGas::new(params.ccip_receive_gas_limit),
    );
    let router = ctx.accounts.reference_addresses.router;

    // `remaining_accounts` layout, in order (matching `execute_v2`'s
    // registry → receiver_ccv → token ordering):
    //   [receiver_registry (1, iff an arbitrary receiver)]
    //     ++ [receiver_ccv (2, iff a v2 arbitrary receiver)]
    //     ++ [pool (5, iff a token transfer)]
    // where receiver_ccv is `[receiver_program, source_chain_ccv_config]`. See resolve_accounts.
    let (receiver_is_v2, receiver_registry_len) = if is_arbitrary {
        let registry = ctx
            .remaining_accounts
            .first()
            .ok_or(CcipOfframpError::InvalidAccountListLengths)?;
        // `receiver_is_v2_from_account` re-derives the expected registry PDA from the raw receiver,
        // so pass `message_receiver` (not the derived PDA) as the receiver argument.
        let is_v2 = ccip_common::router_accounts::receiver_is_v2_from_account(
            registry,
            params.message_receiver,
            router,
        )?;
        (is_v2, 1usize)
    } else {
        (false, 0usize)
    };

    let receiver_ccv_len = if is_arbitrary && receiver_is_v2 { 2 } else { 0 };
    let pool_len = if params.token_transfer.is_some() {
        5
    } else {
        0
    };
    // Covers the registry, receiver-CCV and pool segments, so use the layout-level error rather than
    // a pool-specific one — the mismatch may be in any segment.
    require_eq!(
        ctx.remaining_accounts.len(),
        receiver_registry_len + receiver_ccv_len + pool_len,
        CcipOfframpError::InvalidAccountListLengths
    );

    let receiver_ccv_accounts =
        &ctx.remaining_accounts[receiver_registry_len..receiver_registry_len + receiver_ccv_len];

    // Mirror `execute_v2`'s pool-CCV source so the predicted set matches execution:
    // legacy v1 pools enforce the lane defaults (placeholder), v2 pools are queried.
    let pool_ccv_source = if let Some(token_transfer) = &params.token_transfer {
        // Expected layout: [token_admin_registry, pool_program, pool_config, pool_chain_config, chain_config_v2].
        // The 5 pool accounts are only consulted for v2 pools; for a legacy v1 pool they are ignored,
        // but the caller must still pass all 5 so the layout is fixed and predictable.
        let pool_start = receiver_registry_len + receiver_ccv_len;
        let pool_accounts = &ctx.remaining_accounts[pool_start..pool_start + pool_len];
        let tar = load_token_admin_registry_checked(
            &pool_accounts[0],
            parse_pubkey(&token_transfer.dest_token_address)?,
        )?;
        if tar.interface_version <= 1 {
            PoolCcvSource::LegacyV1
        } else {
            match get_token_pool_required_ccvs_inputs(
                pool_accounts[1..5].to_vec(),
                token_transfer,
                params.remote_chain_selector,
                params.requested_finality,
                &tar,
            )? {
                Some(inputs) => PoolCcvSource::V2(inputs),
                // Unreachable for a v2 token transfer (always yields Some), but stay safe.
                None => PoolCcvSource::NoTokenTransfer,
            }
        }
    } else {
        PoolCcvSource::NoTokenTransfer
    };

    // Resolve the receiver contribution through the SAME helper `execute_v2` uses, so the predicted
    // set can never diverge from the authoritative one (v1 → lane-defaults marker; v2 → CPI query).
    let receiver_required_ccvs = ccv_resolution::resolve_receiver_required_ccvs(
        is_arbitrary,
        receiver_is_v2,
        receiver_ccv_accounts,
        params.message_receiver,
        params.remote_chain_selector,
        params.sender.clone(),
        params.requested_finality,
    )?;

    ccv_resolution::get_ccvs_for_message(
        &source_chain_config.default_ccvs,
        &source_chain_config.lane_mandated_ccvs,
        pool_ccv_source,
        receiver_required_ccvs,
    )
}

#[derive(Accounts)]
pub struct GetCcvsForMsg<'info> {
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
        // this constraint only proves the account is a canonical `SourceChain` PDA. `get_ccvs_for_msg`
        // ties it to `params.remote_chain_selector` with an explicit `require_eq!`.
        seeds = [seed::SOURCE_CHAIN, source_chain.chain_selector.to_le_bytes().as_ref()],
        bump = source_chain.bump,
        constraint = source_chain.version == SourceChain::MAX_V @ CcipOfframpError::InvalidVersion,
    )]
    pub source_chain: Account<'info, SourceChain>,

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
    // Optional accounts
}

#[derive(AnchorSerialize, AnchorDeserialize)]
pub struct GetCcvsForMsgParams {
    pub token_transfer: Option<TokenTransferV1>,
    pub message_receiver: Pubkey,
    /// Length of the message's `data` field, and the callback gas it requests. Together with
    /// `message_receiver` these decide whether the receiver is consulted at all.
    pub data_len: u32,
    pub ccip_receive_gas_limit: u32,
    /// Cross-chain sender, forwarded to a v2 receiver's `get_ccvs_and_finality_config` query so the
    /// predicted CCV set matches what `execute_v2` computes for the same message.
    pub sender: Vec<u8>,
    pub resolution_metadata: Vec<u8>,
    pub remote_chain_selector: u64,
    pub requested_finality: FinalityConfig,
}

/// Response from CCV resolution.
#[derive(AnchorSerialize, AnchorDeserialize, Debug)]
pub struct GetCcvsForMsgResponse {
    pub required_ccvs: Vec<Pubkey>,
    pub optional_ccvs: Vec<Pubkey>,
    pub optional_threshold: u8,
}
