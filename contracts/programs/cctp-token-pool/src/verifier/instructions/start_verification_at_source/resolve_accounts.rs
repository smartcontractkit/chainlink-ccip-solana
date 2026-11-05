use anchor_lang::prelude::*;

use anchor_spl::associated_token::get_associated_token_address;
use anchor_spl::token_interface::Mint;
use ccip_common::messages::MessageV1FieldsAtResolutionTime;
use ccip_common::resolution::*;

use crate::error::CctpPoolVerifierError;
use crate::verifier::cctp_constants::{MESSAGE_TRANSMITTER_V2, TOKEN_MESSENGER_MINTER_V2};
use crate::verifier::instructions::resolve_accounts_start::CctpCcvCpiResolutionInputs;
use crate::verifier::state::*;

/// First (and only) stage of account resolution for the `start_verification_at_source`
/// instruction.
pub fn start(
    config_key: Pubkey,
    _config: Config,
    caller: Pubkey,
    fields: MessageV1FieldsAtResolutionTime,
) -> Result<NestedResolveAccountsResponse> {
    let remote_chain_pda = find(
        &[
            verifier_seed::REMOTE_CHAIN,
            &fields.dest_chain_selector.to_le_bytes(),
        ],
        crate::ID,
    );

    let mint = fields
        .token_transfer
        .ok_or(CctpPoolVerifierError::MissingTokenTransfer)?
        .source_token_address;

    let cpi_signer = find(&[verifier_seed::POOL_SIGNER_SEED, mint.as_ref()], crate::ID);

    let nonce = find(
        &[
            verifier_seed::CCTPV2_NONCE,
            fields.sender.as_slice(),
            &fields.dest_chain_selector.to_le_bytes(),
            mint.as_ref(),
        ],
        crate::ID,
    );

    Ok(NestedResolveAccountsResponse {
        ask_again_with: vec![config_key, mint, remote_chain_pda, nonce],
        accounts_to_save: vec![
            config_key.readonly(),
            caller.readonly(), // this is the router CPI signer
            remote_chain_pda.readonly(),
            mint.writable(),
            // all CCTP tokens are SPL Tokens currently. If we ever want to onboard a
            // CCTP token that's Token2022, we'll need to add another resolution round
            // that looks at the mint account to fetch the owning token program
            anchor_spl::token::ID.readonly(),
            cpi_signer.writable(),
            nonce.writable(),
        ],
        lookup_tables_to_save: vec![],
        next_cpi_resolution_inputs: serialize_metadata(
            &CctpCcvCpiResolutionInputs::StartVerificationAtSourceWithNonce {
                sender: Pubkey::new_from_array(
                    fields
                        .sender
                        .try_into()
                        .map_err(|_| CctpPoolVerifierError::InvalidResolutionInputs)?,
                ),
            },
        )?,
    })
}

pub fn with_nonce<'info>(
    mut remaining_accounts: &'info [AccountInfo<'info>],
    sender: Pubkey,
) -> Result<NestedResolveAccountsResponse> {
    let bumps =
        &mut <NestedResolveAccountsVerificationWithNonce as anchor_lang::Bumps>::Bumps::default();
    let reallocs = &mut std::collections::BTreeSet::new();
    let accounts = NestedResolveAccountsVerificationWithNonce::try_accounts(
        &crate::ID,
        &mut remaining_accounts,
        &[],
        bumps,
        reallocs,
    )?;

    let nonce_seed = match *accounts.nonce.owner {
        // Account not initialized yet: start the sequence
        system_program::ID => 1,
        // Already initialized: load it
        crate::ID_CONST => {
            let account_info = accounts.nonce.to_account_info();
            let data = account_info
                .try_borrow_data()
                .map_err(|_| CctpPoolVerifierError::InvalidNonceAccountData)?;

            let nonce_pda = Nonce::try_deserialize(&mut &data[..])
                .map_err(|_| CctpPoolVerifierError::InvalidNonceAccountData)?;

            require!(
                nonce_pda.is_latest_version(),
                CctpPoolVerifierError::InvalidVersion
            );
            nonce_pda.nonce + 1
        }
        // Any other case (not valid)
        _ => {
            return Err(CctpPoolVerifierError::InvalidNoncePDAOwner.into());
        }
    };
    let mint = accounts.mint.key();

    let event_pda = find(
        &[
            verifier_seed::CCTPV2_EVENT,
            sender.as_ref(),
            accounts
                .remote_chain
                .remote_chain_selector
                .to_le_bytes()
                .as_ref(),
            &mint.to_bytes(),
            &nonce_seed.to_le_bytes(),
        ],
        crate::ID,
    );

    let cpi_signer = find(&[verifier_seed::POOL_SIGNER_SEED, mint.as_ref()], crate::ID);
    let ata = get_associated_token_address(&cpi_signer, &mint);

    let cctp_sender_authority = find(&[b"sender_authority"], TOKEN_MESSENGER_MINTER_V2);
    let cctp_denylist = find(
        &[b"denylist_account", cpi_signer.as_ref()],
        TOKEN_MESSENGER_MINTER_V2,
    );
    let cctp_message_transmitter_pda = find(&[b"message_transmitter"], MESSAGE_TRANSMITTER_V2);
    let cctp_token_messenger_pda = find(&[b"token_messenger"], TOKEN_MESSENGER_MINTER_V2);
    let cctp_remote_token_messenger_pda = find(
        &[
            b"remote_token_messenger",
            accounts
                .remote_chain
                .cctp
                .domain_identifier
                .to_string()
                .as_ref(),
        ],
        TOKEN_MESSENGER_MINTER_V2,
    );
    let cctp_token_minter_pda = find(&[b"token_minter"], TOKEN_MESSENGER_MINTER_V2);
    let local_token = find(&[b"local_token", mint.as_ref()], TOKEN_MESSENGER_MINTER_V2);

    let token_messenger_minter_event_authority =
        find(&[b"__event_authority"], TOKEN_MESSENGER_MINTER_V2);
    let message_transmitter_event_authority = find(&[b"__event_authority"], MESSAGE_TRANSMITTER_V2);

    let lookup_tables: Vec<Pubkey> = if let Some(alt) = accounts.remote_chain.cctp.lookup_table {
        vec![alt]
    } else {
        vec![]
    };

    Ok(NestedResolveAccountsResponse {
        ask_again_with: vec![],
        accounts_to_save: vec![
            event_pda.writable(),
            ata.writable(),
            system_program::ID.readonly(),
            cctp_sender_authority.readonly(),
            cctp_denylist.readonly(),
            cctp_message_transmitter_pda.writable(),
            cctp_token_messenger_pda.readonly(),
            cctp_remote_token_messenger_pda.readonly(),
            cctp_token_minter_pda.readonly(),
            local_token.writable(),
            token_messenger_minter_event_authority.readonly(),
            message_transmitter_event_authority.readonly(),
            MESSAGE_TRANSMITTER_V2.readonly(),
            TOKEN_MESSENGER_MINTER_V2.readonly(),
        ],
        lookup_tables_to_save: lookup_tables,
        next_cpi_resolution_inputs: vec![],
    })
}

#[derive(Accounts)]
pub struct NestedResolveAccountsVerificationWithNonce<'info> {
    pub mint: InterfaceAccount<'info, Mint>,
    pub remote_chain: Account<'info, RemoteChainConfig>,
    /// CHECK: Unchecked because it may not be initialized yet
    pub nonce: UncheckedAccount<'info>,
}
