use std::{
    fmt::{self, Display, Formatter},
    str::FromStr,
};

use anchor_lang::prelude::*;
use anchor_lang::solana_program::{instruction::Instruction, program::invoke};
use anchor_spl::{
    associated_token::get_associated_token_address_with_program_id,
    token::spl_token::{self},
};
use ccip_common::cpi::parse_verified_cpi_return_as;
use ccip_common::v1::{load_token_admin_registry_checked, token_admin_registry_writable};
use ccip_common::CommonCcipError;
use ccip_common::{messages::NormalizeWsol, seed};
use solana_address_lookup_table_interface::state::AddressLookupTable;

use crate::{
    context::ViewConfigOnly,
    instructions_v1::v1::{fees::get_fee_cpi, messages::pools::LockOrBurnInV1},
    messages::TOKENPOOL_DERIVE_LOCK_OR_BURN_DISCRIMINATOR,
    state::{CcipAccountMeta, DeriveAccountsCcipSendParams, DeriveAccountsResponse, ToMeta},
    CcipRouterError,
};

use super::helpers::peek_total_nonce;

const MIN_PAGE_SIZE: usize = 4;

// Splits caller-provided remaining accounts into the fixed prefix and the rest,
// failing instead of panicking when there are fewer than `fixed_len` accounts.
fn split_fixed_accounts<'a, 'info>(
    accounts: &'a [AccountInfo<'info>],
    fixed_len: usize,
) -> Result<(&'a [AccountInfo<'info>], &'a [AccountInfo<'info>])> {
    require_gte!(
        accounts.len(),
        fixed_len,
        CcipRouterError::InvalidAccountListForPdaDerivation
    );
    Ok(accounts.split_at(fixed_len))
}

fn lut_address(lut: &AddressLookupTable, index: usize) -> Result<Pubkey> {
    lut.addresses
        .get(index)
        .copied()
        .ok_or(CommonCcipError::InvalidInputsLookupTableAccounts.into())
}

// Local helper to find a readonly CCIP meta for a given seed + program_id combo.
// Short name for compactness.
fn find(seeds: &[&[u8]], program_id: Pubkey) -> CcipAccountMeta {
    Pubkey::find_program_address(seeds, &program_id)
        .0
        .readonly()
}

#[derive(Clone, Debug)]
pub enum DeriveAccountsCcipSendStage {
    Start,
    FinishMainAccountList,
    RetrieveTokenLUTs,
    RetrievePoolPrograms,
    // N stages, one per token for the ones below.
    TokenTransferStaticAccounts {
        // Index of the current token being derived.
        token: u32,
        // Might be too many to fit in one response, so the user
        // may be required to request multiple pages.
        page: u32,
    },
    NestedTokenDerive {
        token: u32,
        token_substage: String,
    },
}

impl Display for DeriveAccountsCcipSendStage {
    fn fmt(&self, f: &mut Formatter) -> fmt::Result {
        match self {
            DeriveAccountsCcipSendStage::Start => f.write_str("Start"),
            DeriveAccountsCcipSendStage::FinishMainAccountList => {
                f.write_str("FinishMainAccountList")
            }
            DeriveAccountsCcipSendStage::RetrieveTokenLUTs => {
                f.write_str("RetrieveTokenLookupTables")
            }
            DeriveAccountsCcipSendStage::RetrievePoolPrograms => {
                f.write_str("RetrievePoolPrograms")
            }
            DeriveAccountsCcipSendStage::TokenTransferStaticAccounts { token, page } => {
                f.write_fmt(format_args!("TokenTransferStaticAccounts/{token}/{page}"))
            }
            DeriveAccountsCcipSendStage::NestedTokenDerive {
                token_substage,
                token,
            } => f.write_fmt(format_args!("NestedTokenDerive/{token}/{token_substage}")),
        }
    }
}

impl FromStr for DeriveAccountsCcipSendStage {
    type Err = CcipRouterError;

    fn from_str(s: &str) -> std::result::Result<Self, Self::Err> {
        let mut s = s.split('/');
        let (a, b, c) = (s.next(), s.next(), s.next());

        match (a, b, c) {
            (Some("Start"), None, None) => Ok(Self::Start),
            (Some("FinishMainAccountList"), None, None) => Ok(Self::FinishMainAccountList),
            (Some("RetrieveTokenLookupTables"), None, None) => Ok(Self::RetrieveTokenLUTs),
            (Some("RetrievePoolPrograms"), None, None) => Ok(Self::RetrievePoolPrograms),
            (Some("TokenTransferStaticAccounts"), Some(token), Some(page)) => {
                Ok(Self::TokenTransferStaticAccounts {
                    page: str::parse::<u32>(page)
                        .or(Err(CcipRouterError::InvalidDerivationStage))?,
                    token: str::parse::<u32>(token)
                        .or(Err(CcipRouterError::InvalidDerivationStage))?,
                })
            }
            (Some("NestedTokenDerive"), Some(token), token_substage) => {
                Ok(Self::NestedTokenDerive {
                    token_substage: token_substage.unwrap_or("Start").to_string(),
                    token: str::parse::<u32>(token)
                        .or(Err(CcipRouterError::InvalidDerivationStage))?,
                })
            }
            _ => Err(CcipRouterError::InvalidDerivationStage),
        }
    }
}

pub fn derive_ccip_send_accounts_start(
    params: DeriveAccountsCcipSendParams,
) -> Result<DeriveAccountsResponse> {
    let selector = params.dest_chain_selector.to_le_bytes();
    let accounts_to_save = vec![
        find(&[seed::CONFIG], crate::ID),
        find(&[seed::DEST_CHAIN_STATE, &selector], crate::ID).writable(),
        find(
            &[
                seed::NONCE,
                &selector,
                params.ccip_send_caller.key().as_ref(),
            ],
            crate::ID,
        )
        .writable(),
        params.ccip_send_caller.writable().signer(),
        anchor_lang::solana_program::system_program::ID.readonly(),
    ];

    let ask_again_with = vec![
        params.message.fee_token.normalize_wsol().readonly(),
        find(&[seed::FEE_BILLING_SIGNER], crate::ID),
    ];

    Ok(DeriveAccountsResponse {
        accounts_to_save,
        ask_again_with,
        look_up_tables_to_save: vec![],
        current_stage: DeriveAccountsCcipSendStage::Start.to_string(),
        next_stage: DeriveAccountsCcipSendStage::FinishMainAccountList.to_string(),
    })
}

pub fn derive_ccip_send_accounts_finish_main_account_list<'info>(
    ctx: Context<'info, ViewConfigOnly<'info>>,
    params: DeriveAccountsCcipSendParams,
) -> Result<DeriveAccountsResponse> {
    let selector = params.dest_chain_selector.to_le_bytes();
    let [fee_token_mint_info, fee_billing_signer, ..] = ctx.remaining_accounts else {
        return Err(CcipRouterError::InvalidAccountListForPdaDerivation.into());
    };

    let is_fee_in_sol = params.message.fee_token == Pubkey::default();

    let (fee_token_user_ata, fee_token_program) = if is_fee_in_sol {
        (Pubkey::default().readonly(), spl_token::ID.readonly())
    } else {
        let fee_token_program = fee_token_mint_info.owner;
        (
            get_associated_token_address_with_program_id(
                &params.ccip_send_caller.key(),
                &params.message.fee_token,
                &fee_token_program.key(),
            )
            .writable(),
            fee_token_program.readonly(),
        )
    };

    let fee_token_receiver = get_associated_token_address_with_program_id(
        fee_billing_signer.key,
        fee_token_mint_info.key,
        &fee_token_program.pubkey,
    );

    let config = &ctx.accounts.config;

    let accounts_to_save = vec![
        fee_token_program.readonly(),
        fee_token_mint_info.key.readonly(),
        fee_token_user_ata,
        fee_token_receiver.writable(),
        fee_billing_signer.key.readonly(),
        config.fee_quoter.readonly(),
        find(&[seed::CONFIG], config.fee_quoter).readonly(),
        find(&[seed::DEST_CHAIN, &selector], config.fee_quoter).readonly(),
        find(
            &[
                seed::FEE_BILLING_TOKEN_CONFIG,
                fee_token_mint_info.key().as_ref(),
            ],
            config.fee_quoter,
        ),
        find(
            &[
                seed::FEE_BILLING_TOKEN_CONFIG,
                config.link_token_mint.key().as_ref(),
            ],
            config.fee_quoter,
        ),
        config.rmn_remote.readonly(),
        find(&[seed::CURSES], config.rmn_remote),
        find(&[seed::CONFIG], config.rmn_remote),
    ];

    // If there are no tokens, we're done.
    let (next_stage, ask_again_with) = if params.message.token_amounts.is_empty() {
        ("".to_string(), vec![])
    } else {
        let mints_of_transferred_tokens = params.message.token_amounts.iter().map(|ta| ta.token);
        let next_stage_accounts_per_transferred_token =
            mints_of_transferred_tokens.flat_map(|mint| {
                [
                    seed::token_admin_registry(&mint, crate::ID).readonly(),
                    find(
                        &[seed::FEE_BILLING_TOKEN_CONFIG, mint.as_ref()],
                        config.fee_quoter,
                    ),
                    find(
                        &[seed::PER_CHAIN_PER_TOKEN_CONFIG, &selector, mint.as_ref()],
                        config.fee_quoter,
                    ),
                ]
                .into_iter()
            });

        let mut ask_again_with = vec![
            config.fee_quoter.readonly(),
            find(&[seed::CONFIG], config.fee_quoter).readonly(),
            find(&[seed::DEST_CHAIN, &selector], config.fee_quoter).readonly(),
            find(
                &[
                    seed::FEE_BILLING_TOKEN_CONFIG,
                    params.message.fee_token.normalize_wsol().as_ref(),
                ],
                config.fee_quoter,
            ),
            find(
                &[
                    seed::FEE_BILLING_TOKEN_CONFIG,
                    config.link_token_mint.key().as_ref(),
                ],
                config.fee_quoter,
            ),
        ];
        ask_again_with.extend(next_stage_accounts_per_transferred_token);
        (
            DeriveAccountsCcipSendStage::RetrieveTokenLUTs.to_string(),
            ask_again_with,
        )
    };

    Ok(DeriveAccountsResponse {
        ask_again_with,
        accounts_to_save,
        look_up_tables_to_save: vec![],
        current_stage: DeriveAccountsCcipSendStage::FinishMainAccountList.to_string(),
        next_stage,
    })
}

pub fn derive_ccip_send_accounts_retrieve_luts<'info>(
    ctx: Context<'info, ViewConfigOnly<'info>>,
    params: &DeriveAccountsCcipSendParams,
) -> Result<DeriveAccountsResponse> {
    let fee_quoter_fixed_accounts_len = 5usize;
    let accounts_per_token_len = 3usize;

    // Accounts needed for get_fee cpi
    // + [registry, billing_config, per_chain_per_token_config, lookup_table]
    let (fee_quoter_fixed_accounts, token_accounts) =
        split_fixed_accounts(ctx.remaining_accounts, fee_quoter_fixed_accounts_len)?;
    let mut ask_again_with = vec![];
    ask_again_with.extend(fee_quoter_fixed_accounts.iter().map(|a| a.key.readonly()));
    ask_again_with.push(find(
        &[
            seed::NONCE,
            params.dest_chain_selector.to_le_bytes().as_ref(),
            params.ccip_send_caller.key().as_ref(),
        ],
        crate::ID,
    ));

    for (i, accs) in token_accounts.chunks(accounts_per_token_len).enumerate() {
        let [registry, _billing_config, _per_chain_per_token_config] = accs else {
            return Err(CcipRouterError::InvalidAccountListForPdaDerivation.into());
        };
        let token_amount = params
            .message
            .token_amounts
            .get(i)
            .ok_or(CcipRouterError::InvalidAccountListForPdaDerivation)?;
        let token_admin_registry_account =
            load_token_admin_registry_checked(registry, token_amount.token)?;
        let lut = token_admin_registry_account.lookup_table;
        ask_again_with.extend(accs.iter().map(|a| a.key.readonly()));
        ask_again_with.push(lut.readonly());
    }

    Ok(DeriveAccountsResponse {
        accounts_to_save: vec![],
        ask_again_with,
        look_up_tables_to_save: vec![],
        current_stage: DeriveAccountsCcipSendStage::RetrieveTokenLUTs.to_string(),
        next_stage: DeriveAccountsCcipSendStage::RetrievePoolPrograms.to_string(),
    })
}

pub fn derive_ccip_send_accounts_retrieve_pool_programs<'info>(
    ctx: Context<'info, ViewConfigOnly<'info>>,
) -> Result<DeriveAccountsResponse> {
    let token_derivation_fixed_accounts_len = 6usize;
    let accounts_per_token_len = 4usize;

    // Accounts needed for get_fee cpi
    // + [registry, billing_config, per_chain_per_token_config, lookup_table, pool_program]
    let (token_derivation_fixed_accounts, token_accounts) =
        split_fixed_accounts(ctx.remaining_accounts, token_derivation_fixed_accounts_len)?;
    let mut ask_again_with = vec![];
    ask_again_with.extend(
        token_derivation_fixed_accounts
            .iter()
            .map(|a| a.key.readonly()),
    );

    for accs in token_accounts.chunks(accounts_per_token_len) {
        let [_registry, _billing_config, _per_chain_per_token_config, lut] = accs else {
            return Err(CcipRouterError::InvalidAccountListForPdaDerivation.into());
        };
        let lut_data = &mut &lut.data.borrow()[..];
        let lut_account: AddressLookupTable = AddressLookupTable::deserialize(lut_data)
            .map_err(|_| CommonCcipError::InvalidInputsLookupTableAccounts)?;
        let pool_program = lut_address(&lut_account, 2)?;
        ask_again_with.extend(accs.iter().map(|a| a.key.readonly()));
        ask_again_with.push(pool_program.readonly());
    }

    Ok(DeriveAccountsResponse {
        accounts_to_save: vec![],
        ask_again_with,
        look_up_tables_to_save: vec![],
        current_stage: DeriveAccountsCcipSendStage::RetrievePoolPrograms.to_string(),
        next_stage: DeriveAccountsCcipSendStage::TokenTransferStaticAccounts { page: 0, token: 0 }
            .to_string(),
    })
}

pub fn derive_ccip_send_accounts_additional_tokens_static<'info>(
    ctx: Context<'info, ViewConfigOnly<'info>>,
    params: DeriveAccountsCcipSendParams,
    page: u32,
    token: u32,
) -> Result<DeriveAccountsResponse> {
    let mut response = DeriveAccountsResponse {
        current_stage: DeriveAccountsCcipSendStage::TokenTransferStaticAccounts { page, token }
            .to_string(),
        // It's possible we'll need to return to this function, so we
        // initialize with the same account list
        ask_again_with: ctx
            .remaining_accounts
            .iter()
            .map(|a| a.key().readonly())
            .collect(),
        ..Default::default()
    };
    let token_derivation_fixed_accounts_len = 6usize;
    let accounts_per_token_len = 5usize;

    // We extract the accounts for the current token
    let token_start_index =
        token_derivation_fixed_accounts_len + accounts_per_token_len * token as usize;
    let token_end_index = token_start_index + accounts_per_token_len;
    let Some(
        [token_registry, _billing_config, _per_chain_per_token_config, token_lut, _pool_program],
    ) = ctx
        .remaining_accounts
        .get(token_start_index..token_end_index)
    else {
        return Err(CcipRouterError::InvalidAccountListForPdaDerivation.into());
    };

    let lut_data = &mut &token_lut.data.borrow()[..];
    let lut_account: AddressLookupTable = AddressLookupTable::deserialize(lut_data)
        .map_err(|_| CommonCcipError::InvalidInputsLookupTableAccounts)?;
    let token_mint = lut_address(&lut_account, 7)?;
    let pool_program = lut_address(&lut_account, 2)?;
    let token_program = lut_address(&lut_account, 6)?;
    let selector = params.dest_chain_selector.to_le_bytes();
    let sender_token_account = get_associated_token_address_with_program_id(
        &params.ccip_send_caller,
        &token_mint,
        &token_program.key(),
    )
    .writable();

    let token_billing_config = find(
        &[
            seed::PER_CHAIN_PER_TOKEN_CONFIG,
            &selector,
            token_mint.as_ref(),
        ],
        ctx.accounts.config.fee_quoter,
    )
    .readonly();

    let pool_chain_config = find(
        &[seed::POOL_CHAINCONFIG_SEED, &selector, token_mint.as_ref()],
        pool_program,
    )
    .writable();

    if page == 0 {
        response.look_up_tables_to_save = vec![*token_lut.key];
    }

    let token_admin_registry = load_token_admin_registry_checked(token_registry, token_mint)?;

    response.accounts_to_save = vec![
        sender_token_account,
        token_billing_config,
        pool_chain_config,
    ];
    response
        .accounts_to_save
        .extend(lut_account.addresses.iter().enumerate().map(|(i, a)| {
            match token_admin_registry_writable::is(
                &token_admin_registry,
                i.try_into()
                    .expect("LUT addresses are capped at 256 entries, fits in any integer type"),
            ) {
                true => a.writable(),
                false => a.readonly(),
            }
        }));

    let max_response_accounts = 26;
    if response.ask_again_with.len() + response.accounts_to_save.len() > max_response_accounts {
        let total_accounts_to_save = response.accounts_to_save.len();
        // paging is necessary, because we can't fit everything in one response.
        let max_accounts_per_page = max_response_accounts
            .checked_sub(response.ask_again_with.len())
            .ok_or(CcipRouterError::AccountDerivationResponseTooLarge)?;
        require_gte!(
            max_accounts_per_page,
            MIN_PAGE_SIZE,
            CcipRouterError::AccountDerivationResponseTooLarge
        );
        let start_of_page = max_accounts_per_page
            .checked_mul(page as usize)
            .ok_or(CcipRouterError::InvalidAccountListForPdaDerivation)?;
        let end_of_page = total_accounts_to_save.min(start_of_page + max_accounts_per_page);
        response.accounts_to_save = response
            .accounts_to_save
            .get(start_of_page..end_of_page)
            .ok_or(CcipRouterError::InvalidAccountListForPdaDerivation)?
            .to_vec();
        if end_of_page < total_accounts_to_save {
            response.next_stage = DeriveAccountsCcipSendStage::TokenTransferStaticAccounts {
                page: page + 1,
                token,
            }
            .to_string();
            // Different pages take the same inputs to derive, so we just reiterate the request:
            return Ok(response);
        }
    }

    let token_admin_registry = load_token_admin_registry_checked(token_registry, token_mint)?;

    if token_admin_registry.account_resolution.is_enabled() {
        // Nested derivation is supported, so we go one level deeper
        response.next_stage = DeriveAccountsCcipSendStage::NestedTokenDerive {
            token_substage: "Start".to_string(),
            token,
        }
        .to_string();
    // SAFETY: token_amounts.len() bounded by max_number_of_tokens_per_msg (u32)
    } else if token + 1 < params.message.token_amounts.len() as u32 {
        // We aren't done yet with all tokens
        response.next_stage = DeriveAccountsCcipSendStage::TokenTransferStaticAccounts {
            page: 0,
            token: token + 1,
        }
        .to_string();
    }

    Ok(response)
}

pub fn derive_ccip_send_accounts_additional_token_nested<'info>(
    ctx: Context<'info, ViewConfigOnly<'info>>,
    params: &DeriveAccountsCcipSendParams,
    substage: &str,
    token: u32,
) -> Result<DeriveAccountsResponse> {
    let token_derivation_fixed_accounts_len = 6usize;
    let accounts_per_token_len = 5usize;
    let start_of_nested_accounts = token_derivation_fixed_accounts_len
        + params.message.token_amounts.len() * accounts_per_token_len;
    let mut response = DeriveAccountsResponse {
        ask_again_with: ctx
            .remaining_accounts
            .get(..start_of_nested_accounts)
            .ok_or(CcipRouterError::InvalidAccountListForPdaDerivation)?
            .iter()
            .map(|a| a.key().readonly())
            .collect(),
        current_stage: DeriveAccountsCcipSendStage::NestedTokenDerive {
            token_substage: substage.to_string(),
            token,
        }
        .to_string(),
        ..Default::default()
    };

    let token_derivation_fixed_accounts =
        &ctx.remaining_accounts[..token_derivation_fixed_accounts_len];

    let token_start_index =
        token_derivation_fixed_accounts_len + accounts_per_token_len * token as usize;
    let token_end_index = token_start_index + accounts_per_token_len;
    let Some(
        [_token_registry, _billing_config, _per_chain_per_token_config, _token_lut, pool_program],
    ) = ctx
        .remaining_accounts
        .get(token_start_index..token_end_index)
    else {
        return Err(CcipRouterError::InvalidAccountListForPdaDerivation.into());
    };

    // get_fee requires all tokens to have their billing config and per_chain_per_token configs available.
    let token_accounts_iter = ctx
        .remaining_accounts
        .iter()
        .skip(token_derivation_fixed_accounts_len);
    let billing_configs = token_accounts_iter
        .clone()
        .enumerate()
        .filter_map(|(i, a)| (i % accounts_per_token_len == 1).then_some(a.clone()));
    let per_chain_per_token_configs = token_accounts_iter
        .clone()
        .enumerate()
        .filter_map(|(i, a)| (i % accounts_per_token_len == 2).then_some(a.clone()));
    let cpi_remaining_accounts = billing_configs.chain(per_chain_per_token_configs).collect();

    let get_fee_result = get_fee_cpi(
        token_derivation_fixed_accounts[0].key(),
        token_derivation_fixed_accounts[1].clone(),
        token_derivation_fixed_accounts[2].clone(),
        token_derivation_fixed_accounts[3].clone(),
        token_derivation_fixed_accounts[4].clone(),
        params.dest_chain_selector,
        &params.message,
        cpi_remaining_accounts,
    )?;

    let nested_derivation_accounts = &ctx.remaining_accounts[start_of_nested_accounts..];

    // +1 because in `ccip_send` it will be bumped before `LockOrBurnInV1` reaches
    // the pool. `peek_total_nonce` mirrors the version handling of `load_migrate_nonce`,
    // so that senders whose nonce PDA is still on the legacy layout derive the same
    // nonce that `ccip_send` will assign after migrating it.
    let total_nonce = peek_total_nonce(&token_derivation_fixed_accounts[5])?
        .checked_add(1)
        .unwrap();

    let lock_or_burn = LockOrBurnInV1 {
        receiver: get_fee_result
            .processed_extra_args
            .token_receiver
            .as_ref()
            .unwrap_or(&params.message.receiver)
            .clone(),
        remote_chain_selector: params.dest_chain_selector,
        original_sender: params.ccip_send_caller,
        amount: params.message.token_amounts[token as usize].amount,
        local_token: params.message.token_amounts[token as usize].token,
        msg_total_nonce: total_nonce,
    };

    let acc_metas: Vec<AccountMeta> = nested_derivation_accounts
        .iter()
        .flat_map(|acc_info| acc_info.to_account_metas(None))
        .collect();

    let mut data = Vec::with_capacity(8);
    data.extend_from_slice(&TOKENPOOL_DERIVE_LOCK_OR_BURN_DISCRIMINATOR);
    borsh::to_writer(&mut data, &substage).unwrap();
    borsh::to_writer(&mut data, &lock_or_burn).unwrap();

    let ix = Instruction {
        program_id: pool_program.key(),
        accounts: acc_metas,
        data,
    };

    invoke(&ix, nested_derivation_accounts)?;
    let nested_response: DeriveAccountsResponse = parse_verified_cpi_return_as(
        pool_program.key(),
        CcipRouterError::InvalidCpiReturnProgram,
        CcipRouterError::InvalidTokenPoolAccountDerivationResponse,
    )?;

    response.accounts_to_save = nested_response.accounts_to_save;
    response.look_up_tables_to_save = nested_response.look_up_tables_to_save;

    // We're coming back from a nested derivation call, so we turn the stage reported
    // by it into our substage.
    if !nested_response.next_stage.is_empty() {
        response
            .ask_again_with
            .extend_from_slice(&nested_response.ask_again_with);
        response.next_stage = DeriveAccountsCcipSendStage::NestedTokenDerive {
            token_substage: nested_response.next_stage,
            token,
        }
        .to_string();
    // SAFETY: token_amounts.len() bounded by max_number_of_tokens_per_msg (u32)
    } else if token + 1 < params.message.token_amounts.len() as u32 {
        response.next_stage = DeriveAccountsCcipSendStage::TokenTransferStaticAccounts {
            page: 0,
            token: token + 1,
        }
        .to_string();
    } else {
        response.ask_again_with.clear();
        response.next_stage = "".to_string();
    }

    Ok(response)
}
