use anchor_lang::prelude::*;
use anchor_spl::associated_token::{self, get_associated_token_address_with_program_id};
use ccip_common::v2::TOKEN_POOL_LUT_REQUIRED_ENTRIES;
use solana_address_lookup_table_interface as address_lookup_table;
use solana_address_lookup_table_interface::state::AddressLookupTable;

use crate::error::CcipOfframpError;
use crate::instruction as offramp_instruction;
use crate::messages::OfframpAccountListLengths;
use crate::state::{Config, ReferenceAddresses};
use ccip_common::messages::{MessageV1, SVMDestBlob};
use ccip_common::resolution::*;
use ccip_common::router_accounts::TokenAdminRegistry;
use ccip_common::state::{AccountSchemaVersion, PoolChainConfigOverrideType};
use ccip_common::v1::{load_token_admin_registry_checked, token_admin_registry_writable};
use ccip_common::{seed, CommonCcipError};

use super::ccv::begin_ccv_stages;
use super::common::{
    end_resolution, parse_exec_inputs, ExecuteNestedTokenStageMetadata,
    ExecuteTokenStaticStageMetadata,
};

#[derive(Accounts)]
pub struct ResolveAccountsExecuteTokenTransferStaticAccountsStage<'info> {
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

    pub admin_registry: UncheckedAccount<'info>,
    #[account(owner = address_lookup_table::program::id() @ CommonCcipError::InvalidInputsLookupTableAccounts)]
    pub token_lut: UncheckedAccount<'info>,
}

pub fn retrieve_token_static_accounts_stage<'info>(
    ctx: Context<'info, ResolveAccountsExecuteTokenTransferStaticAccountsStage<'info>>,
    params: ResolveAccountsParams,
) -> Result<ResolveAccountsResponse> {
    let exec_params = parse_exec_inputs(&params, ctx.remaining_accounts)?;
    let msg = MessageV1::from_bytes(&exec_params.encoded_message)
        .map_err(|_| CcipOfframpError::InvalidInstructionData)?;
    let message_id = msg.compute_message_id();
    let token_transfer = msg
        .token_transfer
        .ok_or(CcipOfframpError::InvalidInstructionData)?;

    let lookup_table_data = &mut &ctx.accounts.token_lut.data.borrow()[..];
    let lut_account: AddressLookupTable = AddressLookupTable::deserialize(lookup_table_data)
        .map_err(|_| CommonCcipError::InvalidInputsLookupTableAccounts)?;
    require_gte!(
        lut_account.addresses.len(),
        TOKEN_POOL_LUT_REQUIRED_ENTRIES,
        CommonCcipError::InvalidInputsLookupTableAccounts
    );
    let token_mint = lut_account.addresses[7];
    let pool_program = lut_account.addresses[2];
    let token_program = lut_account.addresses[6];

    // The registry is derived from the mint the table claims, and it is the registry that names
    // the table actually registered for that mint; any other table is rejected here.
    let token_admin_registry =
        load_token_admin_registry_checked(&ctx.accounts.admin_registry, token_mint)?;
    require_keys_eq!(
        ctx.accounts.token_lut.key(),
        token_admin_registry.lookup_table,
        CommonCcipError::InvalidInputsLookupTableAccounts
    );

    let ReferenceAddresses { fee_quoter, .. } = *ctx.accounts.reference_addresses;

    let ccip_offramp_pool_signer = find(
        &[seed::EXTERNAL_TOKEN_POOLS_SIGNER, pool_program.as_ref()],
        crate::ID,
    )
    .readonly();

    let dest_blob =
        SVMDestBlob::from_bytes(&msg.dest_blob).map_err(|_| CcipOfframpError::InvalidDestBlob)?;

    let token_receiver_bytes: [u8; 32] = token_transfer.token_receiver.try_into().map_err(|e| {
        msg!("Error converting token receiver: {:?}", e);
        CcipOfframpError::InvalidInstructionData
    })?;
    let msg_token_receiver = Pubkey::new_from_array(token_receiver_bytes);

    let user_token_account = if dest_blob.use_ata.should_derive_ata() {
        get_associated_token_address_with_program_id(
            &msg_token_receiver,
            &token_mint.key(),
            &token_program.key(),
        )
        .writable()
    } else {
        msg_token_receiver.writable()
    };

    let selector = msg.source_chain_selector.to_le_bytes();
    let token_billing_config = find(
        &[
            seed::PER_CHAIN_PER_TOKEN_CONFIG,
            &selector,
            token_mint.as_ref(),
        ],
        fee_quoter,
    )
    .readonly();

    let pool_chain_config = find(
        &[seed::POOL_CHAINCONFIG_SEED, &selector, token_mint.as_ref()],
        pool_program,
    )
    .writable();

    let metadata = deserialize_metadata::<ExecuteTokenStaticStageMetadata>(&params.metadata)?;
    // Non-zero only when an earlier round already emitted part of the lookup table; see the
    // pagination below.
    let lut_entries_emitted = metadata.token_accounts_emitted as usize;
    let first_page = lut_entries_emitted == 0;

    let mut accounts_to_save = if first_page {
        vec![
            ccip_offramp_pool_signer,
            user_token_account,
            token_billing_config,
            pool_chain_config,
        ]
    } else {
        vec![]
    };

    if first_page && token_admin_registry.interface_version > 1 {
        let pool_chain_config_v2 = find(
            &[
                seed::POOL_CHAINCONFIG_SEED_2_0,
                &selector,
                token_mint.as_ref(),
            ],
            pool_program,
        )
        .readonly();
        accounts_to_save.push(pool_chain_config_v2);

        let pool_chain_config_override = find(
            &[
                seed::POOL_OVERRIDE_CHAINCONFIG_SEED,
                &selector,
                token_mint.as_ref(),
                PoolChainConfigOverrideType::FTF.as_bytes(),
            ],
            pool_program,
        )
        .writable();
        accounts_to_save.push(pool_chain_config_override);
    }

    // The parsers (`validate_and_parse_offramp_token_accounts{,_v2}`) expect the required lookup
    // table entries, then the ATA pair when the ATA is to be created, and only then any additional
    // lookup table entries, which they forward to the pool as extra accounts. So the first page
    // always carries the required entries and the ATA pair, and only the additional entries page.
    if first_page {
        push_lut_entries(
            &mut accounts_to_save,
            &lut_account,
            &token_admin_registry,
            0,
            TOKEN_POOL_LUT_REQUIRED_ENTRIES,
        )?;

        if dest_blob.use_ata.should_create_ata() {
            // Only when deriving & creating the ATA, we need the underlying wallet as an additional account
            // as the ATA program requires it. We also need the AssociatedTokenProgram itself to CPI into it,
            // so it has to be available, though we never access its value directly.
            accounts_to_save.push(msg_token_receiver.readonly());
            accounts_to_save.push(associated_token::ID.readonly());
        }
    }

    // The additional entries can outnumber what fits in one 1kB response, so emit them a page at a
    // time and re-enter this stage until they are exhausted, exactly as the messaging segment does.
    let lut_cursor = if first_page {
        TOKEN_POOL_LUT_REQUIRED_ENTRIES
    } else {
        lut_entries_emitted
    };
    let pending = lut_account.addresses.len().saturating_sub(lut_cursor);

    let (take, is_final_page) = if token_admin_registry
        .account_resolution
        .use_lookup_table_additional_entries()
    {
        plan_lut_extras_page(pending, accounts_to_save.len())?
    } else {
        (0, true)
    };

    push_lut_entries(
        &mut accounts_to_save,
        &lut_account,
        &token_admin_registry,
        lut_cursor,
        take,
    )?;

    let mut account_list_lengths = metadata.account_list_lengths.clone();

    if !is_final_page {
        // Re-enter this stage with the cursor advanced. `ask_again_with` repeats this stage's own
        // four named accounts, plus the buffer at `remaining_accounts[0]` per `parse_exec_inputs`.
        let mut ask_again_with = vec![
            ctx.accounts.config.key(),
            find(&[seed::REFERENCE_ADDRESSES], crate::ID),
            ctx.accounts.admin_registry.key(),
            ctx.accounts.token_lut.key(),
        ];
        if let Some(buffer) = exec_params.buffer_account {
            ask_again_with.push(buffer);
        }

        // The token segment spans rounds, so accumulate its length rather than assigning it. A
        // segment that no longer fits a u8 is unresolvable — the account-list lengths are the wire
        // format `execute_v2` splits the accounts by — so fail rather than clamp.
        account_list_lengths.token_transfer =
            checked_add_u8(account_list_lengths.token_transfer, accounts_to_save.len())?;

        return Ok(ResolveAccountsResponse {
            ask_again_with,
            accounts_to_save,
            lookup_tables_to_save: vec![],
            next_ix_discriminator: Some(
                offramp_instruction::ResolveAccountsExecuteTokenTransferStaticAccountsStage::DISCRIMINATOR
                    .try_into()
                    .unwrap(),
            ),
            metadata: serialize_metadata(&ExecuteTokenStaticStageMetadata {
                account_list_lengths,
                // Advance the cursor past the entries this page emitted. A table whose entries do
                // not fit a u8 was already rejected by the `checked_add_u8` above.
                token_accounts_emitted: checked_len_u8(lut_cursor + take)?,
            })?,
        });
    }

    // Token transfer section: add this final page to whatever earlier pages already contributed.
    // Checked for the same reason as above.
    account_list_lengths.token_transfer =
        checked_add_u8(account_list_lengths.token_transfer, accounts_to_save.len())?;

    if token_admin_registry.interface_version > 1
        && token_admin_registry.account_resolution.is_enabled()
    {
        begin_nested_token_stages(
            exec_params.buffer_account,
            accounts_to_save,
            pool_program,
            token_mint,
            account_list_lengths,
        )
    } else if token_admin_registry.interface_version == 1
        && token_admin_registry.account_resolution.is_enabled()
    {
        Err(CcipOfframpError::UnsupportedPoolV1ResolutionOnLaneV2.into())
    } else {
        // No nested pool resolution from here on. NOTE: v2 pools expose `get_required_ccvs`, but the
        // OffRamp needs no dedicated segment for it — the accounts that CPI requires
        // (`[pool_program, pool_config, pool_chain_config, chain_config_v2]`) are all already part of
        // the mint-validated `token_transfer` accounts resolved above (`chain_config_v2` is the
        // `POOL_CHAINCONFIG_SEED_2_0` PDA validated by `OfframpTokenAccountsValidationContext`), and
        // `execute_v2` extracts them via `get_pool_required_ccvs_accounts`. See the design note on
        // `OfframpAccountListLengths`.
        if exec_params.ccvs.is_empty() {
            end_resolution(accounts_to_save, account_list_lengths, vec![], message_id)
        } else {
            begin_ccv_stages(exec_params, accounts_to_save, account_list_lengths, vec![])
        }
    }
}

/// Appends the `take` pool lookup table entries starting at `cursor`, each writable as configured
/// in the token admin registry.
fn push_lut_entries(
    accounts: &mut Vec<CcipAccountMeta>,
    lut: &AddressLookupTable,
    registry: &TokenAdminRegistry,
    cursor: usize,
    take: usize,
) -> Result<()> {
    let page = lookup_table_page(&lut.addresses, cursor, take)?;
    for (offset, a) in page.iter().enumerate() {
        // SAFETY: `lookup_table_page` checked `cursor + take` against the table length, and
        // `offset < take`, so the sum cannot overflow.
        let i = checked_len_u8(cursor + offset)?;
        accounts.push(match token_admin_registry_writable::is(registry, i) {
            true => a.writable(),
            false => a.readonly(),
        });
    }
    Ok(())
}

/// Plans the next page of additional lookup table entries, as [`plan_account_page`] does, except
/// that a response already full is not an error. The first page can be filled exactly by the fixed
/// prefix, the required entries and the ATA pair, in which case every additional entry moves to the
/// next round. Later rounds start empty, so they always make progress.
fn plan_lut_extras_page(pending: usize, already_in_response: usize) -> Result<(usize, bool)> {
    if pending > 0 && already_in_response >= MAX_ACCOUNTS_PER_RESPONSE {
        return Ok((0, false));
    }
    plan_account_page(pending, already_in_response, 0)
}

fn begin_nested_token_stages(
    buffer_account: Option<Pubkey>,
    accounts_to_save: Vec<CcipAccountMeta>,
    pool_program: Pubkey,
    token_mint: Pubkey,
    account_list_lengths: OfframpAccountListLengths,
) -> Result<ResolveAccountsResponse> {
    Ok(ResolveAccountsResponse {
        ask_again_with: ask_again_with_for_nested_tokens(
            find(&[seed::CONFIG], crate::ID),
            pool_program,
            find(
                &[seed::EXTERNAL_TOKEN_POOLS_SIGNER, pool_program.as_ref()],
                crate::ID,
            ),
            buffer_account,
            vec![find(
                &[seed::POOL_STATE_SEED, token_mint.as_ref()],
                pool_program,
            )],
        ),
        accounts_to_save,
        lookup_tables_to_save: vec![],
        next_ix_discriminator: Some(
            offramp_instruction::ResolveAccountsExecuteNestedTokenStage::DISCRIMINATOR
                .try_into()
                .unwrap(),
        ),
        metadata: serialize_metadata(&ExecuteNestedTokenStageMetadata {
            account_list_lengths,
            inner_cpi_resolution_inputs: vec![],
        })?,
    })
}

pub(crate) fn ask_again_with_for_nested_tokens(
    offramp_config: Pubkey,
    pool_program: Pubkey,
    pool_signer: Pubkey,
    buffer_account: Option<Pubkey>,
    inner_accounts: Vec<Pubkey>,
) -> Vec<Pubkey> {
    let mut ask_again_with =
        // SAFETY: a capacity hint only — a wrong value costs a realloc, never correctness, and the operands are account counts bounded by the per-tx account limit.
        Vec::with_capacity(3 + buffer_account.is_some() as usize + inner_accounts.len());
    ask_again_with.push(offramp_config);
    ask_again_with.push(pool_program);
    ask_again_with.push(pool_signer);
    if let Some(buffer) = buffer_account {
        ask_again_with.push(buffer);
    }
    ask_again_with.extend(inner_accounts);
    ask_again_with
}

#[cfg(test)]
mod tests {
    use super::*;

    /// The fixed prefix this stage queues before the table: pool signer, user token account,
    /// billing config and pool chain config, plus the two pool configs only v2 pools have.
    const V1_FIRST_PAGE_PREFIX: usize = 4;
    const V2_FIRST_PAGE_PREFIX: usize = 6;

    /// The wallet and Associated Token Program, which follow the required entries when the
    /// message asks for the receiver ATA to be created.
    const ATA_ACCOUNTS: usize = 2;

    /// A `token_transfer` segment is measured by a `u8`, so a table larger than this cannot be
    /// resolved at all: `checked_add_u8` rejects it rather than reporting a clamped length.
    const MAX_RESOLVABLE_ENTRIES: usize = u8::MAX as usize - V2_FIRST_PAGE_PREFIX - ATA_ACCOUNTS;

    /// Accounts the first round queues before any additional lookup table entry.
    fn first_page_fixed(prefix: usize, ata: usize) -> usize {
        prefix + TOKEN_POOL_LUT_REQUIRED_ENTRIES + ata
    }

    /// Walks the pagination the way the resolver does, returning how many additional lookup table
    /// entries each round emits. Mirrors `retrieve_token_static_accounts_stage`: the first round
    /// starts with the prefix, the required entries and the ATA pair already queued, later rounds
    /// start empty.
    fn paginate(prefix: usize, total: usize, ata: usize) -> Vec<usize> {
        let extras = total - TOKEN_POOL_LUT_REQUIRED_ENTRIES;
        let mut pages = vec![];
        let mut emitted = 0;
        loop {
            let first = pages.is_empty();
            let already = if first {
                first_page_fixed(prefix, ata)
            } else {
                0
            };
            let (take, final_page) = plan_lut_extras_page(extras - emitted, already).unwrap();
            pages.push(take);
            emitted += take;
            if final_page {
                break;
            }
            assert!(
                first || take > 0,
                "a non-final page after the first must make progress"
            );
        }
        assert_eq!(emitted, extras, "every lookup table entry must be emitted");
        pages
    }

    #[test]
    fn a_table_with_only_the_required_entries_resolves_in_one_round() {
        // The common case stays a single round trip, ATA pair included.
        for prefix in [V1_FIRST_PAGE_PREFIX, V2_FIRST_PAGE_PREFIX] {
            for ata in [0, ATA_ACCOUNTS] {
                assert_eq!(
                    paginate(prefix, TOKEN_POOL_LUT_REQUIRED_ENTRIES, ata),
                    vec![0]
                );
            }
        }
    }

    #[test]
    fn a_full_first_page_defers_every_additional_entry_to_the_next_round() {
        // A v2 pool creating the ATA fills the first response exactly, so even a single additional
        // entry needs a second round rather than failing for lack of capacity.
        assert_eq!(
            first_page_fixed(V2_FIRST_PAGE_PREFIX, ATA_ACCOUNTS),
            MAX_ACCOUNTS_PER_RESPONSE
        );
        assert_eq!(
            paginate(
                V2_FIRST_PAGE_PREFIX,
                TOKEN_POOL_LUT_REQUIRED_ENTRIES + 1,
                ATA_ACCOUNTS
            ),
            vec![0, 1]
        );
        // Without the ATA pair there is room for the entry in the first round.
        assert_eq!(
            paginate(V2_FIRST_PAGE_PREFIX, TOKEN_POOL_LUT_REQUIRED_ENTRIES + 1, 0),
            vec![1]
        );
    }

    #[test]
    fn every_page_respects_the_response_budget() {
        for prefix in [V1_FIRST_PAGE_PREFIX, V2_FIRST_PAGE_PREFIX] {
            for total in TOKEN_POOL_LUT_REQUIRED_ENTRIES..=MAX_RESOLVABLE_ENTRIES {
                for ata in [0, ATA_ACCOUNTS] {
                    let pages = paginate(prefix, total, ata);
                    for (i, &page) in pages.iter().enumerate() {
                        let fixed = if i == 0 {
                            first_page_fixed(prefix, ata)
                        } else {
                            0
                        };
                        let used = fixed + page;
                        assert!(
                            used <= MAX_ACCOUNTS_PER_RESPONSE,
                            "prefix={prefix} total={total} ata={ata} page {i} would emit {used} accounts"
                        );
                    }
                }
            }
        }
    }

    #[test]
    fn the_largest_table_terminates_in_a_bounded_number_of_rounds() {
        // Resolving the biggest table the segment length can describe must stay a handful of
        // simulations, not dozens.
        let pages = paginate(V2_FIRST_PAGE_PREFIX, MAX_RESOLVABLE_ENTRIES, ATA_ACCOUNTS);
        assert!(
            pages.len() <= 16,
            "expected a bounded page count, got {} pages: {pages:?}",
            pages.len()
        );
    }
}
