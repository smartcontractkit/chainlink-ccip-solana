use anchor_lang::prelude::*;
use solana_address_lookup_table_interface::state::AddressLookupTable;

use ccip_common::resolution::{find, ResolveAccountsParams, ResolveAccountsResponse, ToMeta};
use ccip_common::state::AccountSchemaVersion;
use ccip_common::v1::load_token_admin_registry_checked;
use ccip_common::{seed, CommonCcipError};

use super::super::resolve_accounts_start::ResolveAccountsStart;
use super::processor::GetCcvsForMsgParams;
use crate::instruction as offramp_instruction;

use crate::error::CcipOfframpError;
use crate::instructions::execute::parse_pubkey;
use crate::state::{Config, ReferenceAddresses};

#[derive(Accounts)]
pub struct ResolveAccountsGetCcvsForMsgStage<'info> {
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
}

pub fn start(
    ctx: Context<ResolveAccountsStart>,
    params: ResolveAccountsParams,
) -> Result<ResolveAccountsResponse> {
    require!(
        params.metadata.is_empty(),
        CcipOfframpError::InvalidInstructionData
    );

    let ix_params = parse_get_ccvs_for_msg_params(&params)?;
    let src_selector_seed = ix_params.remote_chain_selector.to_le_bytes();

    Ok(ResolveAccountsResponse {
        ask_again_with: vec![
            ctx.accounts.config.key(),
            find(&[seed::REFERENCE_ADDRESSES], crate::ID),
        ],
        accounts_to_save: vec![
            ctx.accounts.config.key().readonly(),
            find(&[seed::REFERENCE_ADDRESSES], crate::ID).readonly(),
            find(&[seed::SOURCE_CHAIN, src_selector_seed.as_ref()], crate::ID).readonly(),
        ],
        lookup_tables_to_save: vec![],
        next_ix_discriminator: Some(
            offramp_instruction::ResolveAccountsGetCcvsForMsgStage::DISCRIMINATOR
                .try_into()
                .unwrap(),
        ),
        metadata: vec![],
    })
}

pub fn message_stage(
    ctx: Context<ResolveAccountsGetCcvsForMsgStage>,
    params: ResolveAccountsParams,
) -> Result<ResolveAccountsResponse> {
    require!(
        params.metadata.is_empty(),
        CcipOfframpError::InvalidInstructionData
    );

    let lookup_tables_to_save = vec![ctx.accounts.reference_addresses.offramp_lookup_table];
    let rmn_remote = ctx.accounts.reference_addresses.rmn_remote;
    let mut accounts_to_save = vec![
        rmn_remote.readonly(),
        find(&[seed::CURSES], rmn_remote).readonly(),
        find(&[seed::CONFIG], rmn_remote).readonly(),
    ];

    let ix_params = parse_get_ccvs_for_msg_params(&params)?;
    let receiver = if ccip_common::messages::is_offramp_token_only_transfer(
        &ix_params.message_receiver.to_bytes(),
        ix_params.data_len as usize,
        ccip_common::types::CrossChainGas::new(ix_params.ccip_receive_gas_limit),
    ) {
        Pubkey::default()
    } else {
        ix_params.message_receiver
    };
    let router = ctx.accounts.reference_addresses.router;

    // The segments below are emitted in the order `get_ccvs_for_msg` reads them back:
    // [receiver_registry (iff arbitrary)] ++ [receiver_ccv (iff v2 arbitrary)] ++ [pool (iff token
    // transfer)]. Both segments may need an account this stage does not have yet, in which case we
    // re-enter with a longer `ask_again_with`. Every such request must repeat what earlier rounds
    // already asked for, since the next round sees exactly what it requests and nothing more.
    let mut ask_again_with = vec![
        ctx.accounts.config.key(),
        ctx.accounts.reference_addresses.key(),
    ];

    // ---- Receiver segment ----
    if receiver != Pubkey::default() {
        let pda = find(&[seed::RECEIVER_REGISTRY, receiver.as_ref()], router);
        ask_again_with.push(pda);

        let Some(registry) = ctx.remaining_accounts.iter().find(|acc| acc.key() == pda) else {
            return Ok(ask_again(ask_again_with));
        };

        let receiver_is_v2 =
            ccip_common::router_accounts::receiver_is_v2_from_account(registry, receiver, router)?;

        accounts_to_save.push(pda.readonly());
        if receiver_is_v2 {
            let src_selector_seed = ix_params.remote_chain_selector.to_le_bytes();
            accounts_to_save.push(receiver.readonly());
            accounts_to_save.push(
                find(
                    &[
                        ccip_common::ccvs::RECEIVER_CCV_CONFIG_SEED,
                        src_selector_seed.as_ref(),
                    ],
                    receiver,
                )
                .readonly(),
            );
        }
    }

    // ---- Pool segment ----
    // `get_ccvs_for_msg` requires exactly 5 pool accounts for a token transfer, laid out as
    // [token_admin_registry, pool_program, pool_config, pool_chain_config, chain_config_v2].
    // Only the registry PDA is derivable up front: the pool program and mint live in the token
    // lookup table, whose address in turn lives in the registry. So this costs two extra rounds —
    // one to read the registry, one to read the lookup table it names.
    if let Some(token_transfer) = &ix_params.token_transfer {
        let dest_mint = parse_pubkey(&token_transfer.dest_token_address)?;
        let admin_registry_pda = seed::token_admin_registry(&dest_mint, router);
        ask_again_with.push(admin_registry_pda);

        let Some(registry_account) = ctx
            .remaining_accounts
            .iter()
            .find(|acc| acc.key() == admin_registry_pda)
        else {
            return Ok(ask_again(ask_again_with));
        };

        let token_admin_registry = load_token_admin_registry_checked(
            registry_account,
            parse_pubkey(&token_transfer.dest_token_address)?,
        )?;
        let token_lut = token_admin_registry.lookup_table;
        ask_again_with.push(token_lut);

        let Some(lut_account) = ctx
            .remaining_accounts
            .iter()
            .find(|acc| acc.key() == token_lut)
        else {
            return Ok(ask_again(ask_again_with));
        };

        let lut_data = &mut &lut_account.data.borrow()[..];
        let lut = AddressLookupTable::deserialize(lut_data)
            .map_err(|_| CommonCcipError::InvalidInputsLookupTableAccounts)?;
        // Indices 2 and 7 are the pool program and the mint, matching the layout
        // `execute_v2`'s token stage reads (see `token_transfer_static.rs`). The router already
        // validates the table's mandatory prefix when the pool is registered (`set_pool`), so no
        // length check is repeated here.
        let pool_program = lut.addresses[2];
        let token_mint = lut.addresses[7];
        let selector = ix_params.remote_chain_selector.to_le_bytes();

        accounts_to_save.push(admin_registry_pda.readonly());
        accounts_to_save.push(pool_program.readonly());
        accounts_to_save
            .push(find(&[seed::POOL_STATE_SEED, token_mint.as_ref()], pool_program).readonly());
        accounts_to_save.push(
            find(
                &[seed::POOL_CHAINCONFIG_SEED, &selector, token_mint.as_ref()],
                pool_program,
            )
            .readonly(),
        );
        accounts_to_save.push(
            find(
                &[
                    seed::POOL_CHAINCONFIG_SEED_2_0,
                    &selector,
                    token_mint.as_ref(),
                ],
                pool_program,
            )
            .readonly(),
        );
    }

    Ok(ResolveAccountsResponse {
        ask_again_with: vec![],
        accounts_to_save,
        lookup_tables_to_save,
        next_ix_discriminator: None,
        metadata: vec![],
    })
}

/// Re-enter this same stage with a longer account list. Partial rounds save nothing: the final
/// round rebuilds `accounts_to_save` in full.
fn ask_again(ask_again_with: Vec<Pubkey>) -> ResolveAccountsResponse {
    ResolveAccountsResponse {
        ask_again_with,
        accounts_to_save: vec![],
        lookup_tables_to_save: vec![],
        next_ix_discriminator: Some(
            offramp_instruction::ResolveAccountsGetCcvsForMsgStage::DISCRIMINATOR
                .try_into()
                .unwrap(),
        ),
        metadata: vec![],
    }
}

fn parse_get_ccvs_for_msg_params(params: &ResolveAccountsParams) -> Result<GetCcvsForMsgParams> {
    let (_, data) = params
        .ix_data
        .split_at_checked(crate::instruction::GetCcvsForMsg::DISCRIMINATOR.len())
        .ok_or(CcipOfframpError::InvalidInstructionData)?;

    GetCcvsForMsgParams::try_from_slice(data)
        .map_err(|_| CcipOfframpError::InvalidInstructionData.into())
}
