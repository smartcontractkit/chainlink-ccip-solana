use anchor_lang::prelude::*;
use anchor_spl::associated_token::get_associated_token_address_with_program_id;
use solana_address_lookup_table_interface as address_lookup_table;
use solana_address_lookup_table_interface::state::AddressLookupTable;

use crate::{
    context::{valid_version, MAX_CONFIG_V},
    instruction::{self as router_instruction},
    instructions::common::OnrampRemainingAccountListLengths,
    state::Config,
    CcipRouterError,
};
use ccip_common::{
    messages::NormalizeWsol,
    resolution::*,
    seed::{self, vault_ata, EXTERNAL_TOKEN_POOLS_SIGNER},
    state::PoolChainConfigOverrideType,
    v1::{load_token_admin_registry_checked, token_admin_registry_writable},
    v2::TOKEN_POOL_LUT_REQUIRED_ENTRIES,
    CommonCcipError,
};

use super::common::{
    parse_onramp_resolution_inputs, pool_ccv_resolution_accounts, CcipSendTokenStagesMetadata,
    CcipSendTokenStaticStageMetadata,
};
use super::executor::begin_executor_stage;

#[derive(Accounts)]
pub struct ResolveAccountsCcipSendTokenTransferStaticAccountsStage<'info> {
    #[account(
        seeds = [seed::CONFIG],
        bump,
        constraint = valid_version(config.version, MAX_CONFIG_V) @ CcipRouterError::InvalidVersion,
    )]
    pub config: Account<'info, Config>,
    pub admin_registry: UncheckedAccount<'info>,
    #[account(owner = address_lookup_table::program::id() @ CommonCcipError::InvalidInputsLookupTableAccounts)]
    pub token_lut: UncheckedAccount<'info>,
    pub fee_token_mint: UncheckedAccount<'info>,
}

pub fn retrieve_token_static_accounts_stage<'info>(
    ctx: Context<'info, ResolveAccountsCcipSendTokenTransferStaticAccountsStage<'info>>,
    params: ResolveAccountsParams,
) -> Result<ResolveAccountsResponse> {
    let inputs = parse_onramp_resolution_inputs(&params)?;
    let send_params = &inputs.params;
    let message_token = send_params
        .message
        .token_amounts
        .first()
        .ok_or(CcipRouterError::InvalidInstructionData)?
        .token;
    let token_admin_registry =
        load_token_admin_registry_checked(&ctx.accounts.admin_registry, message_token)?;
    require_keys_eq!(
        ctx.accounts.token_lut.key(),
        token_admin_registry.lookup_table,
        CommonCcipError::InvalidInputsLookupTableAccounts
    );

    let lookup_table_data = &mut &ctx.accounts.token_lut.data.borrow()[..];
    let lut_account: AddressLookupTable = AddressLookupTable::deserialize(lookup_table_data)
        .map_err(|_| CommonCcipError::InvalidInputsLookupTableAccounts)?;
    require_gte!(
        lut_account.addresses.len(),
        TOKEN_POOL_LUT_REQUIRED_ENTRIES,
        CommonCcipError::InvalidInputsLookupTableAccounts
    );
    let token_mint = lut_account.addresses[7];
    require_keys_eq!(
        token_mint,
        message_token,
        CommonCcipError::InvalidInputsLookupTableAccounts
    );
    let pool_program = lut_account.addresses[2];
    let token_program = lut_account.addresses[6];

    let fee_quoter = ctx.accounts.config.fee_quoter;

    // Derived from the parsed sender rather than the resolution caller: the handlers validate
    // the token accounts against the send authority / the `get_fee_v2` placeholder sender.
    let sender_token_account = get_associated_token_address_with_program_id(
        &inputs.sender,
        &token_mint.key(),
        &token_program.key(),
    )
    .writable();

    let selector = send_params.dest_chain_selector.to_le_bytes();
    let per_chain_per_token_config = find(
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

    let metadata = deserialize_metadata::<CcipSendTokenStaticStageMetadata>(&params.metadata)?;
    // Non-zero only when an earlier round already emitted part of the lookup table; see the
    // pagination below.
    let lut_entries_emitted = metadata.token_accounts_emitted as usize;
    let first_page = lut_entries_emitted == 0;

    let mut accounts_to_save = vec![];
    if first_page {
        if token_admin_registry.interface_version > 1 {
            // The pool's fee vault is qualified by the mint of the managed token, so a pool
            // program managing multiple tokens is billed separately for each of them.
            let vault_ata = vault_ata(
                pool_program,
                Some(token_mint.as_ref()),
                &ctx.accounts.fee_token_mint.to_account_info(),
            );
            accounts_to_save.push(vault_ata.writable());
        }

        accounts_to_save.extend_from_slice(&[
            sender_token_account,
            per_chain_per_token_config,
            pool_chain_config,
        ]);

        if token_admin_registry.interface_version > 1 {
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
    }

    // The lookup table can hold more entries than fit in one 1kB response, so emit it a page at a
    // time and re-enter this stage until it is exhausted, mirroring the offramp's static token
    // stage. The first `TOKEN_POOL_LUT_REQUIRED_ENTRIES` are the required ones and any extras
    // follow, ending up in `extra_cpi_accounts`; paging preserves that order because the cursor
    // walks the table front to back. Nothing trails this segment, so every page may fill up.
    let pending = lut_account
        .addresses
        .len()
        .saturating_sub(lut_entries_emitted);

    let (take, is_final_page) = if token_admin_registry
        .account_resolution
        .use_lookup_table_additional_entries()
    {
        plan_account_page(pending, accounts_to_save.len(), 0)?
    } else {
        (TOKEN_POOL_LUT_REQUIRED_ENTRIES, true)
    };

    let page = lookup_table_page(&lut_account.addresses, lut_entries_emitted, take)?;
    for (offset, a) in page.iter().enumerate() {
        // SAFETY: `lookup_table_page` checked `lut_entries_emitted + take` against the table
        // length, and `offset < take`, so the sum cannot overflow.
        let i = checked_len_u8(lut_entries_emitted + offset)?;
        accounts_to_save.push(
            match token_admin_registry_writable::is(&token_admin_registry, i) {
                true => a.writable(),
                false => a.readonly(),
            },
        );
    }

    // The token segment spans rounds, so accumulate its length rather than assigning it. A segment
    // that no longer fits a u8 is unresolvable — the account-list lengths are the wire format the
    // handlers split the remaining accounts by — so fail rather than clamp.
    let mut account_list_lengths = metadata.account_list_lengths.clone();
    account_list_lengths.token_transfer =
        checked_add_u8(account_list_lengths.token_transfer, accounts_to_save.len())?;

    if !is_final_page {
        // Re-enter this stage with the cursor advanced, repeating its own four named accounts.
        return Ok(ResolveAccountsResponse {
            ask_again_with: vec![
                ctx.accounts.config.key(),
                ctx.accounts.admin_registry.key(),
                ctx.accounts.token_lut.key(),
                ctx.accounts.fee_token_mint.key(),
            ],
            accounts_to_save,
            lookup_tables_to_save: vec![],
            next_ix_discriminator: Some(
                router_instruction::ResolveAccountsTokenTransferStaticAccountsStage::DISCRIMINATOR
                    .try_into()
                    .unwrap(),
            ),
            metadata: serialize_metadata(&CcipSendTokenStaticStageMetadata {
                account_list_lengths,
                // Advance the cursor past the entries this page emitted. A table whose entries do
                // not fit a u8 was already rejected by the `checked_add_u8` above.
                token_accounts_emitted: checked_add_u8(metadata.token_accounts_emitted, take)?,
            })?,
        });
    }

    if token_admin_registry.interface_version > 1
        && token_admin_registry.account_resolution.is_enabled()
    {
        begin_nested_token_stages(
            accounts_to_save,
            pool_program,
            token_mint,
            account_list_lengths,
            send_params.dest_chain_selector,
        )
    } else if token_admin_registry.interface_version == 1
        && token_admin_registry.account_resolution.is_enabled()
    {
        // CCTP/Lombard are the only v1 pools that implement account resolution,
        // and we don't support invoking them through the v2 lane.
        Err(CcipRouterError::UnsupportedPoolV1ResolutionOnLaneV2.into())
    } else {
        // Only v2 pools expose get_required_ccvs, so only they get pool accounts appended for
        // the construct-final-ccv-list stage's CPI. A legacy (v1) pool passes none: that stage —
        // mirroring `ccip_send_v2` — treats a token transfer without pool accounts as the
        // INCLUDE_DEFAULT_CCVS sentinel, so the lane defaults are still resolved and enforced.
        let pool_ccv_accounts = if token_admin_registry.interface_version > 1 {
            pool_ccv_resolution_accounts(pool_program, token_mint, send_params.dest_chain_selector)
        } else {
            vec![]
        };
        begin_executor_stage(
            send_params.dest_chain_selector,
            pool_ccv_accounts,
            accounts_to_save,
            account_list_lengths,
            vec![],
            send_params.message.fee_token.normalize_wsol(),
        )
    }
}

fn begin_nested_token_stages(
    accounts_to_save: Vec<CcipAccountMeta>,
    pool_program: Pubkey,
    token_mint: Pubkey,
    account_list_lengths: OnrampRemainingAccountListLengths,
    dest_chain_selector: u64,
) -> Result<ResolveAccountsResponse> {
    Ok(ResolveAccountsResponse {
        ask_again_with: ask_again_with_for_nested_tokens(
            find(&[seed::CONFIG], crate::ID),
            pool_program,
            dest_chain_selector,
            vec![find(
                &[seed::POOL_STATE_SEED, token_mint.as_ref()],
                pool_program,
            )],
        ),
        accounts_to_save,
        lookup_tables_to_save: vec![],
        next_ix_discriminator: Some(
            router_instruction::ResolveAccountsNestedTokenStage::DISCRIMINATOR
                .try_into()
                .unwrap(),
        ),
        metadata: serialize_metadata(&CcipSendTokenStagesMetadata {
            account_list_lengths,
            inner_cpi_resolution_inputs: vec![],
        })?,
    })
}

pub(crate) fn ask_again_with_for_nested_tokens(
    router_config: Pubkey,
    pool_program: Pubkey,
    dest_chain_selector: u64,
    inner_accounts: Vec<Pubkey>,
) -> Vec<Pubkey> {
    // SAFETY: a capacity hint only — a wrong value costs a realloc, never correctness, and the operands are account counts bounded by the per-tx account limit.
    let mut ask_again_with = Vec::with_capacity(3 + inner_accounts.len());
    ask_again_with.push(router_config);
    ask_again_with.push(pool_program);
    ask_again_with.push(find(
        &[EXTERNAL_TOKEN_POOLS_SIGNER, pool_program.as_ref()],
        crate::ID,
    ));

    ask_again_with.push(find(
        &[
            seed::DEST_CHAIN_STATE_V2,
            dest_chain_selector.to_le_bytes().as_ref(),
        ],
        crate::ID,
    ));
    ask_again_with.extend(inner_accounts);
    ask_again_with
}

#[cfg(test)]
mod tests {
    use super::*;
    use ccip_common::v2::TOKEN_POOL_LUT_REQUIRED_ENTRIES;

    /// The widest fixed prefix this stage queues before the table: the pool's fee vault ATA, the
    /// sender token account, the billing config, the pool chain config and the two v2 pool
    /// configs.
    const FIRST_PAGE_PREFIX: usize = 6;

    /// A `token_transfer` segment is measured by a `u8`, so a table larger than this cannot be
    /// resolved at all: `checked_add_u8` rejects it rather than reporting a clamped length.
    const MAX_RESOLVABLE_ENTRIES: usize = u8::MAX as usize - FIRST_PAGE_PREFIX;

    /// Walks the pagination the way the resolver does, returning the size of each page. Mirrors
    /// `retrieve_token_static_accounts_stage`: the first round starts with the fixed prefix
    /// already queued, later rounds start empty. Nothing trails the segment here.
    fn paginate(total: usize) -> Vec<usize> {
        let mut pages = vec![];
        let mut emitted = 0;
        loop {
            let already = if emitted == 0 { FIRST_PAGE_PREFIX } else { 0 };
            let (take, final_page) = plan_account_page(total - emitted, already, 0).unwrap();
            pages.push(take);
            emitted += take;
            if final_page {
                break;
            }
            assert!(take > 0, "a non-final page must make progress");
        }
        assert_eq!(emitted, total, "every lookup table entry must be emitted");
        pages
    }

    #[test]
    fn a_table_that_fits_resolves_in_one_round() {
        // A pool with only the required entries must not regress into extra round trips: they fit
        // alongside the widest prefix.
        assert_eq!(
            paginate(TOKEN_POOL_LUT_REQUIRED_ENTRIES),
            vec![TOKEN_POOL_LUT_REQUIRED_ENTRIES]
        );
    }

    #[test]
    fn every_page_respects_the_response_budget() {
        for total in 0..=MAX_RESOLVABLE_ENTRIES {
            let pages = paginate(total);
            for (i, &page) in pages.iter().enumerate() {
                let used = if i == 0 { FIRST_PAGE_PREFIX } else { 0 } + page;
                assert!(
                    used <= MAX_ACCOUNTS_PER_RESPONSE,
                    "total={total} page {i} would emit {used} accounts"
                );
            }
        }
    }

    #[test]
    fn the_largest_table_terminates_in_a_bounded_number_of_rounds() {
        // Resolving the biggest table the segment length can describe must stay a handful of
        // simulations, not dozens.
        let pages = paginate(MAX_RESOLVABLE_ENTRIES);
        assert!(
            pages.len() <= 16,
            "expected a bounded page count, got {} pages: {pages:?}",
            pages.len()
        );
    }
}
