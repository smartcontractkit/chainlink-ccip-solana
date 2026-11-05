use anchor_lang::prelude::*;
use anchor_spl::associated_token::get_associated_token_address_with_program_id;
use anchor_spl::{token, token_2022, token_interface};
use base_token_pool::constants::{POOL_CHAINCONFIG_SEED, POOL_SIGNER_SEED, POOL_STATE_SEED};
use ccip_common::messages::{MessageV1, SVMDestBlob, SVMTokenReceiverUsage};
use ccip_common::resolution::*;
use ccip_common::seed::ALLOWED_OFFRAMP;

use crate::constants::VERIFIER_REMOTE_CHAIN_SEED;
use crate::errors::LombardTokenPoolError;
use crate::state::VerifierRemoteChainConfig;

fn remote_chain_config_pda(remote_chain_selector: u64) -> Pubkey {
    find(
        &[
            VERIFIER_REMOTE_CHAIN_SEED,
            &remote_chain_selector.to_le_bytes(),
        ],
        crate::ID,
    )
}

/// The accounts read below come from the caller's `remaining_accounts`, so their contents are only
/// trusted once they are known to be owned by the expected program.
fn require_owner(account: &AccountInfo, owners: &[Pubkey]) -> Result<()> {
    require!(
        owners.contains(account.owner),
        LombardTokenPoolError::InvalidResolutionAccountOwner
    );
    Ok(())
}

fn lookup_tables(lookup_table: Option<Pubkey>) -> Vec<Pubkey> {
    lookup_table.into_iter().collect()
}

/// Round 1 of `verify`'s resolution.
pub fn verify_start(
    verifier_config_key: Pubkey,
    caller: Pubkey,
    caller_program: Pubkey,
    source_chain_selector: u64,
    encoded_message: &[u8],
    verifier_results: &[u8],
) -> Result<NestedResolveAccountsResponse> {
    let message = MessageV1::from_bytes(encoded_message)
        .map_err(|_| LombardTokenPoolError::InvalidPayload)?;
    let token_transfer = message
        .token_transfer
        .as_ref()
        .ok_or(LombardTokenPoolError::InvalidPath)?;
    let mint = Pubkey::try_from(token_transfer.dest_token_address.as_slice())
        .map_err(|_| LombardTokenPoolError::InvalidPath)?;
    let receiver = Pubkey::try_from(token_transfer.token_receiver.as_slice())
        .map_err(|_| LombardTokenPoolError::InvalidPath)?;
    let receiver_usage = SVMDestBlob::from_bytes(&message.dest_blob)
        .map_err(|_| LombardTokenPoolError::InvalidResolutionInputs)?
        .use_ata;
    require!(
        verifier_results.len() == 32,
        LombardTokenPoolError::InvalidPayloadLength
    );
    let mut payload_hash = [0u8; 32];
    payload_hash.copy_from_slice(verifier_results);

    let remote_chain_config = remote_chain_config_pda(source_chain_selector);
    // `state`'s address only depends on `mint` (already known here), so it can be requested
    // alongside remote_chain_config/mint in this same round - no need for a third round just to
    // read its data (its `config.router` is needed to predict `allowed_offramp`'s PDA below).
    let state = find(&[POOL_STATE_SEED, mint.as_ref()], crate::ID);

    Ok(NestedResolveAccountsResponse {
        // Re-request verifier_config (needed every round so nested_resolve_accounts_start's own
        // State-vs-VerifierConfig discriminator check keeps passing - it always binds the first
        // passed account to that check), then remote_chain_config/mint/state as extra accounts,
        // which land in ctx.remaining_accounts for verify_stage2 to read.
        ask_again_with: vec![verifier_config_key, remote_chain_config, mint, state],
        // `caller` is the offramp-derived `ccv_signer` PDA
        accounts_to_save: vec![
            // Writable: forwarded as the real mailbox's `HandleMessage.handler`
            // (`Signer` + `#[account(mut)]`).
            verifier_config_key.writable(),
            remote_chain_config.readonly(),
            caller.signer(),
            caller_program.readonly(),
        ],
        lookup_tables_to_save: vec![],
        next_cpi_resolution_inputs: serialize_metadata(
            &crate::instructions::nested_resolution::LombardVerifierCpiResolutionInputs::VerifyStage2 {
                mint,
                receiver,
                receiver_usage,
                payload_hash,
                caller_program,
            },
        )?,
    })
}

/// Round 2 (final). `remaining_accounts` is `[remote_chain_config, mint, state]`.
pub fn verify_stage2(
    remaining_accounts: &[AccountInfo],
    mint: Pubkey,
    receiver: Pubkey,
    receiver_usage: SVMTokenReceiverUsage,
    payload_hash: [u8; 32],
    caller_program: Pubkey,
) -> Result<NestedResolveAccountsResponse> {
    let [remote_chain_config_info, mint_info, state_info] = remaining_accounts else {
        return Err(LombardTokenPoolError::InvalidResolutionInputs.into());
    };
    require_owner(remote_chain_config_info, &[crate::ID])?;
    require_owner(mint_info, &[token::ID, token_2022::ID])?;
    require_owner(state_info, &[crate::ID])?;

    let remote_chain_config = VerifierRemoteChainConfig::try_deserialize(
        &mut &remote_chain_config_info.try_borrow_data()?[..],
    )?;
    let chain_id = remote_chain_config.path.l_chain_id;
    let remote_chain_selector = remote_chain_config.remote_chain_selector;

    let mint_data = &mut &mint_info.try_borrow_data()?[..];
    let mint_account = token_interface::Mint::try_deserialize(mint_data)?;
    let token_program = *mint_info.owner;
    let mint_authority = mint_account
        .mint_authority
        .ok_or(LombardTokenPoolError::InvalidPath)?;

    let receiver_token_account = if receiver_usage.should_derive_ata() {
        get_associated_token_address_with_program_id(&receiver, &mint, &token_program)
    } else {
        receiver
    };

    let state = crate::state::State::try_deserialize(&mut &state_info.try_borrow_data()?[..])?;
    let router = state.config.router;

    let pool_signer = find(&[POOL_SIGNER_SEED, mint.as_ref()], crate::ID);
    let token_authority = find(&[b"token_authority"], crate::bridge::ID);
    let bridge_config = find(&[b"bridge_config"], crate::bridge::ID);
    let local_token_config = find(&[b"local_token_config", mint.as_ref()], crate::bridge::ID);
    let remote_bridge_config = find(&[b"remote_bridge_config", &chain_id], crate::bridge::ID);
    let remote_token_config = find(
        &[b"remote_token_config", mint.as_ref(), &chain_id],
        crate::bridge::ID,
    );
    let chain_config = find(
        &[
            POOL_CHAINCONFIG_SEED,
            &remote_chain_selector.to_le_bytes(),
            mint.as_ref(),
        ],
        crate::ID,
    );
    let inbound_message_path = find(&[b"inbound_message_path", &chain_id], crate::mailbox::ID);
    let message_info = find(&[b"message", &payload_hash], crate::mailbox::ID);
    let message_handled = find(&[b"message_handled", &payload_hash], crate::bridge::ID);
    let mailbox_config = find(&[b"mailbox_config"], crate::mailbox::ID);
    let allowed_offramp = find(
        &[
            ALLOWED_OFFRAMP,
            &remote_chain_selector.to_le_bytes(),
            caller_program.as_ref(),
        ],
        router,
    );

    Ok(NestedResolveAccountsResponse {
        ask_again_with: vec![],
        // Order and mutability must match `Verify`'s Accounts struct exactly (verify.rs), minus
        // verifier_config/remote_chain_config/authority/offramp_program already saved in round 1.
        accounts_to_save: vec![
            allowed_offramp.readonly(),
            state_info.key().readonly(),
            token_program.readonly(),
            mint.writable(),
            // Writable: forwarded as the real bridge's `GMPReceive.handler`
            // (`Signer` + `#[account(mut)]`, pays `message_handled`'s rent via `init`).
            pool_signer.writable(),
            receiver_token_account.writable(),
            mint_authority.readonly(),
            token_authority.readonly(),
            crate::bridge::ID.readonly(),
            bridge_config.readonly(),
            crate::mailbox::ID.readonly(),
            mailbox_config.readonly(),
            local_token_config.readonly(),
            anchor_lang::solana_program::system_program::ID.readonly(),
            remote_bridge_config.readonly(),
            // Writable: the real bridge's `gmp_receive` consumes `inbound_rate_limit` on it.
            remote_token_config.writable(),
            chain_config.readonly(),
            inbound_message_path.readonly(),
            message_info.writable(),
            message_handled.writable(),
        ],
        lookup_tables_to_save: lookup_tables(remote_chain_config.lookup_table),
        next_cpi_resolution_inputs: vec![],
    })
}

/// Round 1 of `start_verification_at_source`'s resolution.
pub fn start_verification_at_source_start(
    verifier_config_key: Pubkey,
    caller: Pubkey,
    dest_chain_selector: u64,
    token: Pubkey,
) -> Result<NestedResolveAccountsResponse> {
    let remote_chain_config = remote_chain_config_pda(dest_chain_selector);

    Ok(NestedResolveAccountsResponse {
        // Re-request verifier_config (needed every round, see verify_start's comment on the same
        // pattern), then remote_chain_config and mint (token) as extra accounts, which land in
        // ctx.remaining_accounts for stage 2 to read.
        ask_again_with: vec![verifier_config_key, remote_chain_config, token],
        accounts_to_save: vec![
            verifier_config_key.readonly(),
            caller.signer(),
            remote_chain_config.readonly(),
        ],
        lookup_tables_to_save: vec![],
        next_cpi_resolution_inputs: serialize_metadata(
            &crate::instructions::nested_resolution::LombardVerifierCpiResolutionInputs::StartVerificationAtSourceStage2 {
                mint: token,
            },
        )?,
    })
}

/// Round 2. `remaining_accounts` is `[remote_chain_config, mint]`.
pub fn start_verification_at_source_stage2(
    verifier_config_key: Pubkey,
    remaining_accounts: &[AccountInfo],
    mint: Pubkey,
) -> Result<NestedResolveAccountsResponse> {
    let [remote_chain_config_info, mint_info] = remaining_accounts else {
        return Err(LombardTokenPoolError::InvalidResolutionInputs.into());
    };
    require_owner(remote_chain_config_info, &[crate::ID])?;
    require_owner(mint_info, &[token::ID, token_2022::ID])?;

    let remote_chain_config = VerifierRemoteChainConfig::try_deserialize(
        &mut &remote_chain_config_info.try_borrow_data()?[..],
    )?;
    let chain_id = remote_chain_config.path.l_chain_id;
    let token_program = *mint_info.owner;

    let pool_signer = find(&[POOL_SIGNER_SEED, mint.as_ref()], crate::ID);
    let pool_token_account =
        crate::bridge::utils::token_actions::get_token_account(token_program, mint, pool_signer)?;
    let bridge_config = find(&[b"bridge_config"], crate::bridge::ID);
    let mailbox_config = find(&[b"mailbox_config"], crate::mailbox::ID);

    Ok(NestedResolveAccountsResponse {
        // Re-request verifier_config (needed every round, see verify_start's comment on the same
        // pattern), then mailbox_config as an extra account, which lands in ctx.remaining_accounts
        // for stage 3 to read.
        ask_again_with: vec![verifier_config_key, mailbox_config],
        // Order and mutability must match `StartVerificationAtSource`'s Accounts struct exactly
        // (start_verification_at_source.rs), minus verifier_config/authority/remote_chain_config
        // already saved in round 1 and the mailbox-nonce-dependent accounts saved in round 3.
        accounts_to_save: vec![
            token_program.readonly(),
            mint.writable(),
            // Writable: used as both `fee_payer` and `sender` in the `bridge::deposit` CPI, both
            // `Signer` + `#[account(mut)]` on the real bridge.
            pool_signer.writable(),
            pool_token_account.writable(),
            crate::bridge::ID.readonly(),
            bridge_config.readonly(),
            find(&[b"sender_config", pool_signer.as_ref()], crate::bridge::ID).readonly(), // bridge_sender_config
            find(&[b"remote_bridge_config", &chain_id], crate::bridge::ID).readonly(),
            find(&[b"local_token_config", mint.as_ref()], crate::bridge::ID).readonly(),
            find(
                &[b"remote_token_config", mint.as_ref(), &chain_id],
                crate::bridge::ID,
            )
            .readonly(),
            crate::mailbox::ID.readonly(),
            mailbox_config.writable(),
            find(&[b"outbound_message_path", &chain_id], crate::mailbox::ID).readonly(),
        ],
        lookup_tables_to_save: lookup_tables(remote_chain_config.lookup_table),
        next_cpi_resolution_inputs: serialize_metadata(
            &crate::instructions::nested_resolution::LombardVerifierCpiResolutionInputs::StartVerificationAtSourceStage3,
        )?,
    })
}

/// Round 3 (final). `remaining_accounts` is `[mailbox_config]`.
pub fn start_verification_at_source_stage3(
    remaining_accounts: &[AccountInfo],
) -> Result<NestedResolveAccountsResponse> {
    let [mailbox_config_info] = remaining_accounts else {
        return Err(LombardTokenPoolError::InvalidResolutionInputs.into());
    };
    require_owner(mailbox_config_info, &[crate::mailbox::ID])?;

    let mailbox_config = crate::mailbox::state::Config::try_deserialize(
        &mut &mailbox_config_info.try_borrow_data()?[..],
    )?;
    let bridge_config = find(&[b"bridge_config"], crate::bridge::ID);

    Ok(NestedResolveAccountsResponse {
        ask_again_with: vec![],
        // Order and mutability must match `StartVerificationAtSource`'s Accounts struct exactly:
        // [.., outbound_message, mailbox_sender_config, treasury, system_program].
        accounts_to_save: vec![
            find(
                &[
                    b"outbound_message",
                    &mailbox_config.global_nonce.to_be_bytes(),
                ],
                crate::mailbox::ID,
            )
            .writable(),
            find(
                &[b"sender_config", bridge_config.as_ref()],
                crate::mailbox::ID,
            )
            .readonly(), // mailbox_sender_config
            mailbox_config.treasury.writable(),
            anchor_lang::solana_program::system_program::ID.readonly(),
        ],
        lookup_tables_to_save: vec![],
        next_cpi_resolution_inputs: vec![],
    })
}

#[cfg(test)]
mod tests {
    use super::*;

    fn owned_by(owner: Pubkey, check: impl FnOnce(&AccountInfo) -> Result<()>) -> Result<()> {
        let key = Pubkey::new_unique();
        let mut lamports = 0;
        let mut data = vec![];
        check(&AccountInfo::new(
            &key,
            false,
            false,
            &mut lamports,
            &mut data,
            &owner,
            false,
        ))
    }

    #[test]
    fn require_owner_accepts_any_listed_owner() {
        for owner in [token::ID, token_2022::ID] {
            owned_by(owner, |a| require_owner(a, &[token::ID, token_2022::ID])).unwrap();
        }
    }

    #[test]
    fn require_owner_rejects_other_owners() {
        let err = owned_by(Pubkey::new_unique(), |a| require_owner(a, &[crate::ID]))
            .err()
            .unwrap();
        assert_eq!(
            err,
            LombardTokenPoolError::InvalidResolutionAccountOwner.into()
        );
    }
}
