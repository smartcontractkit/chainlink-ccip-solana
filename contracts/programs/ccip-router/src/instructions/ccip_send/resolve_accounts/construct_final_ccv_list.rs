use anchor_lang::prelude::*;

use crate::{
    context::{valid_version, MAX_CHAINSTATECCIPV2_V, MAX_CONFIG_V},
    instruction::{self as router_instruction},
    instructions::common::{OnrampRemainingAccountListLengths, OnrampResolutionMetadata},
    state::{Config, DestChainCcipV2},
    CcipRouterError,
};
use ccip_common::{
    ccvs::get_pool_required_ccvs_cpi,
    pool_ccvs_with_default_fallback,
    resolution::*,
    seed::{self, vault_ata},
    v2::{MessageDirection, ProtocolAmount},
    INCLUDE_DEFAULT_CCVS,
};

use super::ccv::begin_nested_ccv_stages;
use super::common::{end_resolution, parse_onramp_resolution_inputs};

pub(crate) fn begin_construct_final_ccv_list_stage(
    dest_chain_selector: u64,
    pool_ccv_accounts: Vec<Pubkey>,
    accounts_to_save: Vec<CcipAccountMeta>,
    account_list_lengths: OnrampRemainingAccountListLengths,
    fee_token_mint: Pubkey,
) -> Result<ResolveAccountsResponse> {
    let dest_selector_seed = dest_chain_selector.to_le_bytes();
    let mut ask_again_with = vec![
        find(&[seed::CONFIG], crate::ID),
        find(
            &[seed::DEST_CHAIN_STATE_V2, dest_selector_seed.as_ref()],
            crate::ID,
        ),
        fee_token_mint,
    ];
    // For token transfers, the pool accounts are appended as remaining accounts so the stage can
    // CPI into the pool's get_required_ccvs. Empty when there's no transfer, or when the pool is
    // legacy (v1) and has no such interface — the stage tells those cases apart via the message.
    ask_again_with.extend(pool_ccv_accounts);

    Ok(ResolveAccountsResponse {
        ask_again_with,
        accounts_to_save,
        lookup_tables_to_save: vec![],
        next_ix_discriminator: Some(
            router_instruction::ResolveAccountsConstructFinalCcvListStage::DISCRIMINATOR
                .try_into()
                .unwrap(),
        ),
        metadata: serialize_metadata(&OnrampResolutionMetadata {
            account_list_lengths,
        })?,
    })
}

#[derive(Accounts)]
#[instruction(params: ResolveAccountsParams)]
pub struct ResolveAccountsCcipSendConstructFinalCcvListStage<'info> {
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
    pub fee_token_mint: UncheckedAccount<'info>,
    // For token transfers, `remaining_accounts` carries the pool accounts needed to CPI into the
    // pool's get_required_ccvs: [pool_program, pool_config, pool_chain_config].
}

// Stage immediately after the executor stage.
// Constructs the final CCV list by merging the user/default CCVs (from extra args) with the
// lane-mandated CCVs (from dest chain config) and the pool-required CCVs (via a CPI into the token
// pool). This mirrors the merge `ccip_send_v2` performs at execution time, so the resolved account
// list covers every CCV that will actually be invoked. The merged list is then threaded through the
// nested CCV stages via the metadata.
//
// This stage also saves all vault PDAs required for billing, which begin the CCV block before
// the per-CCV nested resolution.
pub fn construct_final_ccv_list_stage<'info>(
    ctx: Context<'info, ResolveAccountsCcipSendConstructFinalCcvListStage<'info>>,
    params: ResolveAccountsParams,
) -> Result<ResolveAccountsResponse> {
    let send_params = parse_onramp_resolution_inputs(&params)?.params;
    let dest_selector_seed = send_params.dest_chain_selector.to_le_bytes();
    require_keys_eq!(
        ctx.accounts.dest_chain.key(),
        find(
            &[seed::DEST_CHAIN_STATE_V2, dest_selector_seed.as_ref()],
            crate::ID,
        ),
        CcipRouterError::InvalidAccountListLengths
    );

    let dest_chain_config_v2 = &ctx.accounts.dest_chain.config;

    let mut extra_args =
        dest_chain_config_v2.parse_extra_args_with_defaults(&send_params.message)?;

    // Mirrors the pool contribution in `ccip_send_v2` so the resolved account list covers every
    // CCV the send will invoke: no token transfer contributes nothing; a transfer through a
    // legacy (v1) pool — recognizable here because the token stage appends no pool accounts for
    // it — has no `get_required_ccvs` interface and collapses to the INCLUDE_DEFAULT_CCVS
    // sentinel; a v2 pool is queried via CPI, with an empty response collapsing to the sentinel
    // as well. `merge_ccv_lists` then expands the sentinel to the lane defaults.
    let pool_required_ccvs = if send_params.message.token_amounts.is_empty() {
        vec![]
    } else if ctx.remaining_accounts.is_empty() {
        vec![INCLUDE_DEFAULT_CCVS]
    } else {
        let amount = send_params
            .message
            .token_amounts
            .first()
            .ok_or(CcipRouterError::InvalidInstructionData)?;
        pool_ccvs_with_default_fallback(get_pool_required_ccvs_cpi(
            ctx.remaining_accounts,
            ProtocolAmount::from(amount.amount),
            amount.token,
            send_params.dest_chain_selector,
            extra_args.finality,
            MessageDirection::Outbound,
        )?)
    };

    extra_args.merge_ccv_lists(
        &pool_required_ccvs,
        &ctx.accounts.dest_chain.config.lane_mandated_ccvs,
        &ctx.accounts.dest_chain.config.default_ccvs,
    )?;

    // vault atas precede the per-ccv account list. There is always one per CCV
    let accounts_to_save: Vec<_> = extra_args
        .ccvs
        .iter()
        .map(|ccv| vault_ata(*ccv, None, &ctx.accounts.fee_token_mint.to_account_info()).writable())
        .collect();

    let mut metadata = deserialize_metadata::<OnrampResolutionMetadata>(&params.metadata)?;
    // One vault ATA per CCV, and the merged CCV list is not capped anywhere in this path, so the
    // count is checked rather than cast: an account-list length that silently wrapped would
    // mis-segment the remaining accounts the handler splits by.
    metadata.account_list_lengths.ccv_vault_atas = checked_add_u8(
        metadata.account_list_lengths.ccv_vault_atas,
        accounts_to_save.len(),
    )?;

    if extra_args.ccvs.is_empty() {
        end_resolution(accounts_to_save, metadata.account_list_lengths)
    } else {
        begin_nested_ccv_stages(
            send_params,
            accounts_to_save,
            extra_args.ccvs,
            metadata.account_list_lengths,
        )
    }
}
