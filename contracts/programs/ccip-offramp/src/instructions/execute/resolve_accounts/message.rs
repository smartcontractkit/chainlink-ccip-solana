use anchor_lang::prelude::*;

use super::super::token_transfer_legacy::{is_writable, MAX_MESSAGING_ACCOUNTS};
use crate::error::CcipOfframpError;
use crate::instruction as offramp_instruction;
use crate::instructions::execute::parse_pubkey;
use crate::messages::OfframpAccountListLengths;
use crate::state::{Config, ReferenceAddresses};
use ccip_common::messages::{MessageV1, SVMDestBlob, TokenTransferV1};
use ccip_common::state::AccountSchemaVersion;
use ccip_common::{resolution::*, seed};

use super::ccv::begin_ccv_stages;
use super::common::{end_resolution, parse_exec_inputs, ExecuteTokenStaticStageMetadata};

// This has to have a long descriptive name because Anchor does not support yet using qualified paths for Contexts.
// Even more, we need to import everything in this module (with *) to correctly import in the lib.rs the things
// that Anchor creates here when expanding the macros.
#[derive(Accounts)]
pub struct ResolveAccountsExecuteMessageStageContext<'info> {
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

/// Cursor carried between `message_stage` rounds while the messaging segment is paginated. Absent
/// (empty metadata) on the first round.
#[derive(Clone, Debug, PartialEq, Eq, AnchorSerialize, AnchorDeserialize)]
pub struct MessageStageMetadata {
    /// How many `dest_blob.accounts` entries earlier rounds already emitted.
    pub messaging_accounts_emitted: u8,
}

pub fn message_stage(
    ctx: Context<ResolveAccountsExecuteMessageStageContext>,
    params: ResolveAccountsParams,
) -> Result<ResolveAccountsResponse> {
    // Empty on the first round; a cursor once the messaging segment spills into further rounds.
    let messaging_accounts_emitted = if params.metadata.is_empty() {
        0usize
    } else {
        deserialize_metadata::<MessageStageMetadata>(&params.metadata)?.messaging_accounts_emitted
            as usize
    };
    // Everything that precedes the messaging accounts is emitted once, on the first page; the
    // driver accumulates `accounts_to_save` across rounds, so later pages carry only their slice.
    let first_page = messaging_accounts_emitted == 0;

    let exec_params = parse_exec_inputs(&params, ctx.remaining_accounts)?;
    let msg = MessageV1::from_bytes(&exec_params.encoded_message)
        .map_err(|_| CcipOfframpError::InvalidInstructionData)?;
    let src_selector_seed = msg.source_chain_selector.to_le_bytes();

    let ReferenceAddresses {
        router,
        rmn_remote,
        offramp_lookup_table,
        ..
    } = *ctx.accounts.reference_addresses;

    // The message's data receiver, if any. A token-only transfer is never delivered to a receiver.
    let data_receiver: Option<Pubkey> = if msg.receiver.is_empty() {
        None
    } else {
        require_eq!(
            msg.receiver.len(),
            32,
            CcipOfframpError::InvalidReceiverAddress
        );
        let bytes: [u8; 32] = msg
            .receiver
            .as_slice()
            .try_into()
            .map_err(|_| CcipOfframpError::InvalidReceiverAddress)?;
        msg.is_offramp_arbitrary_message()
            .then(|| Pubkey::new_from_array(bytes))
    };

    let receiver_is_v2 = if let Some(receiver) = data_receiver {
        // Receiver-version resolution: the OffRamp selects the v1/v2 receiver flow from the
        // router-owned `ReceiverRegistry` PDA. Reading it on-chain requires the account be
        // provided, so when this is a data message whose registry has not been supplied yet we
        // re-request `message_stage` with the registry included and read it on re-entry.
        let expected_registry_pda = find(&[seed::RECEIVER_REGISTRY, receiver.as_ref()], router);
        let registry = ctx
            .remaining_accounts
            .iter()
            .find(|acc| acc.key() == expected_registry_pda);

        match registry {
            None => {
                let mut ask_again_with = vec![
                    ctx.accounts.config.key(),
                    find(&[seed::REFERENCE_ADDRESSES], crate::ID),
                ];
                // Keep the buffer at remaining_accounts[0] (parse_exec_inputs convention); the registry
                // is found by key on re-entry, so its position after the buffer is irrelevant.
                if let Some(buffer_account) = exec_params.buffer_account {
                    ask_again_with.push(buffer_account);
                }
                ask_again_with.push(expected_registry_pda);
                return Ok(ResolveAccountsResponse {
                    ask_again_with,
                    accounts_to_save: vec![], // emitted in full on re-entry, so nothing is duplicated
                    lookup_tables_to_save: vec![],
                    next_ix_discriminator: Some(
                        offramp_instruction::ResolveAccountsExecuteMessageStage::DISCRIMINATOR
                            .try_into()
                            .unwrap(),
                    ),
                    metadata: vec![],
                });
            }
            Some(registry) => ccip_common::router_accounts::receiver_is_v2_from_account(
                registry, receiver, router,
            )?,
        }
    } else {
        false
    };

    // If there's a transfer, we add the registry to the response, and the zero pubkey if not.
    // This is not bundled inside the token accounts as it's used to determine the schema between
    // 1.6 and 2.0 token transfer accounts.
    let token_admin_registry = if let Some(token_transfer) = &msg.token_transfer {
        token_admin_registry_pda(&token_transfer.dest_token_address, router)?
    } else {
        Pubkey::default()
    };

    let allowed_offramp = find(
        &[
            seed::ALLOWED_OFFRAMP,
            src_selector_seed.as_ref(),
            crate::ID.as_ref(),
        ],
        router,
    );

    let buffer_accounts_len = u8::from(exec_params.buffer_account.is_some());
    let mut accounts_to_save = if first_page {
        let mut fixed = vec![
            token_admin_registry.readonly(),
            allowed_offramp.readonly(),
            rmn_remote.readonly(),
            find(&[seed::CURSES], rmn_remote).readonly(),
            find(&[seed::CONFIG], rmn_remote).readonly(),
            find(&[seed::EVENT_AUTHORITY], crate::ID).readonly(), // the CPI event authority
            crate::ID.readonly(), // the offramp itself for the CPI event
        ];
        if let Some(buffer_account) = exec_params.buffer_account {
            fixed.push(buffer_account.writable());
        }
        fixed
    } else {
        vec![]
    };

    let mut messaging_accounts_length = 0;
    let mut receiver_ccv_length = 0u8;
    let mut receiver_registry_length = 0u8;
    let message_id = msg.compute_message_id();

    if let Some(receiver) = data_receiver {
        // Messaging accounts are laid out as `[receiver_program, external_execution_signer,
        // ..dest_blob.accounts]`. The offramp itself derives the `external_execution_signer`
        // PDA here (rather than trusting the executor-supplied dest_blob) because it is the
        // authority the offramp signs the `ccip_receive` CPI with; `execute_v2` re-derives and
        // validates it. The `offramp` and `allowed_offramp` accounts the receiver also expects
        // are the fixed `ExecuteV2` accounts, prepended to the CPI at execution time.
        if first_page {
            accounts_to_save.push(receiver.readonly());
            accounts_to_save.push(
                find(
                    &[seed::EXTERNAL_EXECUTION_CONFIG, receiver.as_ref()],
                    crate::ID,
                )
                .readonly(),
            );
        }
        let dest_blob = SVMDestBlob::from_bytes(&msg.dest_blob).map_err(|e| {
            // Kept deliberately: `InvalidDestBlob` alone says nothing about why the executor's blob
            // failed to parse, and this is the only place the raw bytes are still in scope.
            msg!("Failed to parse dest blob: {:?}", e);
            msg!(
                "Dest blob was {} bytes long: {:?}",
                msg.dest_blob.len(),
                msg.dest_blob
            );
            CcipOfframpError::InvalidDestBlob
        })?;

        // The writablity bitmap is a u64, so max 64 accounts for messaging
        // Solana currently enforces a max of 64 accounts per tx anyway (with plans to increase to 128), and
        // CCIP's executor args for now enforce that same limit. This can be increased in the future with
        // a new version of the SVMExecutorArgs with a longer bitmap.
        //
        // Checked before the loop below so an oversized list gets this error rather than panicking
        // on the `u8` index conversion.
        require_gte!(
            MAX_MESSAGING_ACCOUNTS,
            dest_blob.accounts.len(),
            CcipOfframpError::TooManyMessagingAccounts
        );

        // The messaging accounts may not fit in one 1kB response, so emit them a page at a time and
        // re-enter this stage until they are exhausted. `execute_v2` is unaffected: it sees the
        // concatenation the driver accumulates, in the same order as before.
        //
        // The trailing registry/receiver-CCV accounts must land in the same response as the last
        // messaging page (they follow the messaging segment and precede token_transfer), so the
        // page that can also fit them is the final one.
        let trailing_accounts = 1 + if receiver_is_v2 { 2 } else { 0 };
        let pending = dest_blob
            .accounts
            .len()
            .saturating_sub(messaging_accounts_emitted);
        let (take, is_final_page) =
            plan_account_page(pending, accounts_to_save.len(), trailing_accounts)?;

        for offset in 0..take {
            let i = messaging_accounts_emitted + offset;
            // SAFETY: the cast cannot truncate — `i < MAX_MESSAGING_ACCOUNTS` (64) per the
            // `require_gte!` above.
            let is_writable = is_writable(&dest_blob.account_is_writable_bitmap, i as u8);
            accounts_to_save.push(dest_blob.accounts[i].with_writable(is_writable));
        }

        if !is_final_page {
            // Keep asking for the same accounts so the final page can still read the registry, and
            // keep the buffer at index 0 per the `parse_exec_inputs` convention.
            let mut ask_again_with = vec![
                ctx.accounts.config.key(),
                find(&[seed::REFERENCE_ADDRESSES], crate::ID),
            ];
            if let Some(buffer_account) = exec_params.buffer_account {
                ask_again_with.push(buffer_account);
            }
            ask_again_with.push(find(&[seed::RECEIVER_REGISTRY, receiver.as_ref()], router));

            return Ok(ResolveAccountsResponse {
                ask_again_with,
                accounts_to_save,
                lookup_tables_to_save: vec![],
                next_ix_discriminator: Some(
                    offramp_instruction::ResolveAccountsExecuteMessageStage::DISCRIMINATOR
                        .try_into()
                        .unwrap(),
                ),
                metadata: serialize_metadata(&MessageStageMetadata {
                    // SAFETY: the sum never exceeds `dest_blob.accounts.len()`, which the
                    // `require_gte!` above caps at `MAX_MESSAGING_ACCOUNTS` (64), so it neither
                    // overflows nor truncates.
                    messaging_accounts_emitted: (messaging_accounts_emitted + take) as u8,
                })?,
            });
        }

        // count the receiver program and external_execution_signer PDA in addition to the
        // dest_blob accounts
        //
        // SAFETY: `dest_blob.accounts.len() <= MAX_MESSAGING_ACCOUNTS` (64) per the `require_gte!`
        // above, so `len + 2 <= 66` and the cast cannot truncate.
        messaging_accounts_length = (dest_blob.accounts.len() + 2) as u8;

        // The router-owned `ReceiverRegistry` PDA: the authoritative source
        // `execute_v2` reads to pick the v1/v2 flow.
        accounts_to_save
            .push(find(&[seed::RECEIVER_REGISTRY, receiver.as_ref()], router).readonly());
        receiver_registry_length = 1;

        // Only a v2 receiver needs the `get_ccvs_and_finality_config` query accounts, decided from
        // the registry above rather than a caller hint.
        if receiver_is_v2 {
            // Dedicated `receiver_ccv` segment for the receiver's `get_ccvs_and_finality_config`
            // CPI: `[receiver_program, source_chain_ccv_config]`. The config PDA is derived
            // under the receiver program from the shared interface seed + source chain selector,
            // so a v2 receiver's CCV requirements can be queried on-chain during execution.
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
            receiver_ccv_length = 2;
        }
    }

    let account_list_lengths = OfframpAccountListLengths {
        buffer: buffer_accounts_len,
        messaging: messaging_accounts_length,
        token_transfer: 0, // no token transfer accounts at this stage
        receiver_ccv: receiver_ccv_length,
        receiver_registry: receiver_registry_length,
        ccvs: vec![], // no CCV accounts at this stage
    };

    let mut response = if let Some(token_transfer) = msg.token_transfer {
        begin_token_transfer_stages(
            ctx,
            router,
            &token_transfer,
            accounts_to_save,
            account_list_lengths,
            exec_params.buffer_account,
        )?
    } else if exec_params.ccvs.is_empty() {
        end_resolution(accounts_to_save, account_list_lengths, vec![], message_id)?
    } else {
        begin_ccv_stages(exec_params, accounts_to_save, account_list_lengths, vec![])?
    };

    response.lookup_tables_to_save.push(offramp_lookup_table);
    Ok(response)
}

fn begin_token_transfer_stages(
    ctx: Context<'_, ResolveAccountsExecuteMessageStageContext<'_>>,
    router: Pubkey,
    token_transfer: &TokenTransferV1,
    accounts_to_save: Vec<CcipAccountMeta>,
    account_list_lengths: OfframpAccountListLengths,
    buffer_account: Option<Pubkey>,
) -> Result<ResolveAccountsResponse> {
    let mut ask_again_with = vec![
        ctx.accounts.config.key(),
        token_admin_registry_pda(&token_transfer.dest_token_address, router)?,
    ];

    // In buffered mode, keep the buffer at `remaining_accounts[0]` for the next stage so it can
    // deserialize the execution inputs. The token LUT stage's context consumes the two named
    // accounts above, so appending the buffer here places it first among that stage's
    // remaining accounts (matching the convention used by `parse_exec_inputs`).
    if let Some(buffer) = buffer_account {
        ask_again_with.push(buffer);
    }

    Ok(ResolveAccountsResponse {
        ask_again_with,
        accounts_to_save,
        lookup_tables_to_save: vec![],
        next_ix_discriminator: Some(
            offramp_instruction::ResolveAccountsExecuteTokenTransferRetrieveLutStage::DISCRIMINATOR
                .try_into()
                .unwrap(),
        ),
        metadata: serialize_metadata(&ExecuteTokenStaticStageMetadata {
            account_list_lengths,
            token_accounts_emitted: 0,
        })?,
    })
}

fn token_admin_registry_pda(dest_token_address: &[u8], router: Pubkey) -> Result<Pubkey> {
    let dest_mint = parse_pubkey(dest_token_address)?;
    Ok(seed::token_admin_registry(&dest_mint, router))
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::instructions::execute::token_transfer_legacy::MAX_MESSAGING_ACCOUNTS;

    /// The first page carries the fixed accounts (7, plus an optional buffer, plus the receiver and
    /// its execution signer), so it has the least room for messaging accounts.
    const FIRST_PAGE_PREFIX: usize = 10;

    /// Walks the pagination the way the resolver does, returning the size of each page. Mirrors
    /// `message_stage`: the first round starts with the fixed prefix already queued, later rounds
    /// start empty.
    fn paginate(total: usize, trailing: usize) -> Vec<usize> {
        let mut pages = vec![];
        let mut emitted = 0;
        loop {
            let already = if emitted == 0 { FIRST_PAGE_PREFIX } else { 0 };
            let (take, final_page) = plan_account_page(total - emitted, already, trailing).unwrap();
            pages.push(take);
            emitted += take;
            if final_page {
                break;
            }
            assert!(take > 0, "a non-final page must make progress");
        }
        assert_eq!(emitted, total, "every messaging account must be emitted");
        pages
    }

    #[test]
    fn small_messages_still_resolve_in_one_round() {
        // The common case must not regress into extra round trips.
        let (take, is_final) = plan_account_page(3, FIRST_PAGE_PREFIX, 3).unwrap();
        assert_eq!(take, 3);
        assert!(is_final);
    }

    #[test]
    fn the_last_page_leaves_room_for_the_trailing_accounts() {
        // The 10-account prefix leaves 8 slots, so 5 pending + 3 trailing exactly fills the page:
        // it must still count as final rather than spilling the registry into another round.
        let (take, is_final) = plan_account_page(5, FIRST_PAGE_PREFIX, 3).unwrap();
        assert_eq!(take, 5);
        assert!(is_final);

        // One more and it no longer fits alongside the trailing accounts.
        let (take, is_final) = plan_account_page(6, FIRST_PAGE_PREFIX, 3).unwrap();
        assert!(!is_final);
        assert_eq!(take, 6.min(MAX_ACCOUNTS_PER_RESPONSE - FIRST_PAGE_PREFIX));
    }

    #[test]
    fn every_page_respects_the_response_budget() {
        for total in 0..=MAX_MESSAGING_ACCOUNTS {
            for trailing in [1, 3] {
                let pages = paginate(total, trailing);
                for (i, &page) in pages.iter().enumerate() {
                    let prefix = if i == 0 { FIRST_PAGE_PREFIX } else { 0 };
                    let is_last = i == pages.len() - 1;
                    let used = prefix + page + if is_last { trailing } else { 0 };
                    assert!(
                        used <= MAX_ACCOUNTS_PER_RESPONSE,
                        "total={total} page {i} would emit {used} accounts"
                    );
                }
            }
        }
    }

    #[test]
    fn the_maximum_message_terminates_in_a_bounded_number_of_rounds() {
        // 64 messaging accounts is the ceiling the writability bitmap allows; resolving it must
        // stay a handful of simulations, not dozens.
        let pages = paginate(MAX_MESSAGING_ACCOUNTS, 3);
        assert!(
            pages.len() <= 6,
            "expected a bounded page count, got {pages:?}"
        );
    }

    #[test]
    fn token_admin_registry_rejects_overlong_dest_token_address() {
        let overlong_dest_token_address = [7u8; 33];
        let err = token_admin_registry_pda(&overlong_dest_token_address, Pubkey::new_unique())
            .expect_err("overlong token address must be rejected before PDA derivation");

        assert_eq!(err, error!(CcipOfframpError::InvalidInputPubkeyBytes));
    }
}
