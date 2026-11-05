use anchor_lang::prelude::*;

use crate::{
    context::{valid_version, MAX_CHAINSTATECCIPV2_V, MAX_CONFIG_V, MAX_REFERENCE_ADDRESSES_V},
    instruction::{self as router_instruction},
    instructions::common::{OnrampRemainingAccountListLengths, OnrampResolutionMetadata},
    state::{Config, DestChainCcipV2, ReferenceAddresses},
    CcipRouterError,
};
use ccip_common::{
    extra_args::NO_EXECUTION_TAG,
    messages::NormalizeWsol,
    resolution::*,
    seed::{self, vault_ata},
};

use super::common::parse_onramp_resolution_inputs;
use super::construct_final_ccv_list::begin_construct_final_ccv_list_stage;

pub(crate) fn begin_executor_stage(
    dest_chain_selector: u64,
    pool_ccv_accounts: Vec<Pubkey>,
    accounts_to_save: Vec<CcipAccountMeta>,
    account_list_lengths: OnrampRemainingAccountListLengths,
    lookup_tables_to_save: Vec<Pubkey>,
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
        find(&[seed::REFERENCE_ADDRESSES], crate::ID),
    ];
    // The pool accounts are only consumed by the construct-final-ccv-list stage that follows;
    // they ride along as remaining accounts so this stage can thread them through.
    ask_again_with.extend(pool_ccv_accounts);

    Ok(ResolveAccountsResponse {
        ask_again_with,
        accounts_to_save,
        lookup_tables_to_save,
        next_ix_discriminator: Some(
            router_instruction::ResolveAccountsExecutorStage::DISCRIMINATOR
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
pub struct ResolveAccountsCcipSendExecutorStage<'info> {
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
    /// The router's `ReferenceAddresses`, which do not exist until `update_reference_addresses`
    /// creates them. Their address is validated in the handler below.
    pub reference_addresses: UncheckedAccount<'info>,
    // For token transfers, `remaining_accounts` carries the pool accounts the next stage needs
    // to CPI into the pool's get_required_ccvs; they are passed through untouched.
}

// Stage immediately after token transfers, or after `start` if there are no token transfers.
// Resolves the accounts the onramp needs to CPI into the executor's `get_fee`. The executor is
// taken from the message extra args, falling back to the lane's default executor. There is no
// nested resolution here: `get_fee` is a simple method, so its account list is standardized to
// the executor's `GetFee` context (config + chain config, plus the event CPI accounts).
pub fn executor_stage<'info>(
    ctx: Context<'info, ResolveAccountsCcipSendExecutorStage<'info>>,
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

    let extra_args = dest_chain_config_v2.parse_extra_args_with_defaults(&send_params.message)?;

    let mut metadata = deserialize_metadata::<OnrampResolutionMetadata>(&params.metadata)?;

    // The no-execution tag means no automated execution: the onramp skips the executor `get_fee`
    // CPI entirely, so no executor accounts are resolved and the executor section stays empty.
    let accounts_to_save = if extra_args.executor == NO_EXECUTION_TAG {
        vec![]
    } else {
        let mut accounts_to_save =
            executor_get_fee_accounts(extra_args.executor, send_params.dest_chain_selector);
        // The vault ATA is not part of the accounts passed to the `get_fee` CPI, but must exist
        // in the list to pay the receipt.
        accounts_to_save.push(
            vault_ata(
                extra_args.executor,
                None,
                &ctx.accounts.fee_token_mint.to_account_info(),
            )
            .writable(),
        );
        accounts_to_save
    };
    metadata.account_list_lengths.executor = checked_len_u8(accounts_to_save.len())?;

    let pool_ccv_accounts = ctx
        .remaining_accounts
        .iter()
        .map(|account| account.key())
        .collect();

    let mut response = begin_construct_final_ccv_list_stage(
        send_params.dest_chain_selector,
        pool_ccv_accounts,
        accounts_to_save,
        metadata.account_list_lengths,
        send_params.message.fee_token.normalize_wsol(),
    )?;
    // Every `ccip_send_v2` and `get_fee_v2` resolution passes through this stage exactly once,
    // so the onramp lookup table is returned once.
    if let Some(lookup_table) = onramp_lookup_table(&ctx.accounts.reference_addresses)? {
        response.lookup_tables_to_save.push(lookup_table);
    }
    Ok(response)
}

// The onramp lookup table the `ReferenceAddresses` hold, if the admin has set one.
fn onramp_lookup_table(reference_addresses: &UncheckedAccount) -> Result<Option<Pubkey>> {
    require_keys_eq!(
        reference_addresses.key(),
        find(&[seed::REFERENCE_ADDRESSES], crate::ID),
        CcipRouterError::InvalidInputsLookupTableAccounts
    );
    // Not created yet: resolution keeps working, it just returns no onramp table.
    if reference_addresses.owner != &crate::ID {
        return Ok(None);
    }
    let reference_addresses =
        ReferenceAddresses::try_deserialize(&mut &reference_addresses.try_borrow_data()?[..])?;
    require!(
        valid_version(reference_addresses.version, MAX_REFERENCE_ADDRESSES_V),
        CcipRouterError::InvalidVersion
    );
    let lookup_table = reference_addresses.onramp_lookup_table;
    Ok((lookup_table != Pubkey::default()).then_some(lookup_table))
}

// The accounts of the executor's `GetFee` context, in the order its own resolver returns them:
// config, chain config, then the event CPI accounts (`GetFee` is `#[event_cpi]`) including the
// executor program itself, which the onramp also needs in scope to `invoke` it.
fn executor_get_fee_accounts(executor: Pubkey, dest_chain_selector: u64) -> Vec<CcipAccountMeta> {
    let dest_selector_seed = dest_chain_selector.to_le_bytes();
    vec![
        find(&[seed::CONFIG], executor).readonly(),
        find(&[seed::REMOTE_CHAIN, dest_selector_seed.as_ref()], executor).readonly(),
        find(&[seed::EVENT_AUTHORITY], executor).readonly(),
        executor.readonly(),
    ]
}
