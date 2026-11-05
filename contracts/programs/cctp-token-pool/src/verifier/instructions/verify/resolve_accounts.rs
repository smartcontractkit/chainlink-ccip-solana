use anchor_lang::prelude::*;

use anchor_spl::associated_token::get_associated_token_address_with_program_id;
use anchor_spl::token_interface::Mint;
use ccip_common::messages::MessageV1;
use ccip_common::messages::SVMDestBlob;
use ccip_common::resolution::*;
use ccip_common::seed::CCV_SIGNER;

use crate::error::CctpPoolVerifierError;
use crate::verifier::cctp_constants::MESSAGE_TRANSMITTER_V2;
use crate::verifier::cctp_constants::TOKEN_MESSENGER_MINTER_V2;
use crate::verifier::instructions::resolve_accounts_start::CctpCcvCpiResolutionInputs;
use crate::verifier::instructions::verify::DecodedVerifierResult;
use crate::verifier::state::*;

/// First stage of account resolution for the `verify` instruction.
pub fn start(
    config_key: Pubkey,
    _config: Config,
    source_chain_selector: u64,
    encoded_message: Vec<u8>,
    verifier_results: Vec<u8>,
) -> Result<NestedResolveAccountsResponse> {
    let remote_chain_pda = find(
        &[
            verifier_seed::REMOTE_CHAIN,
            &source_chain_selector.to_le_bytes(),
        ],
        crate::ID,
    );

    let message = MessageV1::from_bytes(&encoded_message).map_err(|e| {
        msg!("Failed to decode message: {:?}", e);
        CctpPoolVerifierError::InvalidResolutionInputs
    })?;

    let offramp = Pubkey::new_from_array(message.off_ramp_address.try_into().map_err(|e| {
        msg!("Failed to parse offramp from off_ramp_address: {:?}", e);
        CctpPoolVerifierError::InvalidResolutionInputs
    })?);

    let token_transfer = message
        .token_transfer
        .ok_or(CctpPoolVerifierError::MissingTokenTransfer)?;

    let mint =
        Pubkey::new_from_array(token_transfer.dest_token_address.try_into().map_err(|e| {
            msg!("Failed to parse mint from dest_token_address: {:?}", e);
            CctpPoolVerifierError::InvalidResolutionInputs
        })?);

    let cpi_signer = find(
        &[verifier_seed::POOL_SIGNER_SEED, &mint.to_bytes()],
        crate::ID,
    );

    let pool_ata = get_associated_token_address_with_program_id(
        &cpi_signer,
        &mint,
        // assumed that every CCTP token is a standard SPL token, so the ATA program is always the standard one
        // - this can be changed later by adding an additional resolution round and checking the actual mint account
        &anchor_spl::token::ID,
    );

    let dest_blob = SVMDestBlob::from_bytes(&message.dest_blob).map_err(|e| {
        msg!("Failed to decode dest_blob: {:?}", e);
        CctpPoolVerifierError::InvalidResolutionInputs
    })?;

    let token_receiver =
        Pubkey::new_from_array(token_transfer.token_receiver.try_into().map_err(|e| {
            msg!("Failed to parse token_receiver : {:?}", e);
            CctpPoolVerifierError::InvalidResolutionInputs
        })?);

    let end_receiver = if dest_blob.use_ata.should_derive_ata() {
        // assumed to be SPL token program for all CCTP tokens, see comment above
        get_associated_token_address_with_program_id(&token_receiver, &mint, &anchor_spl::token::ID)
    } else {
        token_receiver
    };

    let cctp_authority_pda = find(
        &[
            b"message_transmitter_authority",
            TOKEN_MESSENGER_MINTER_V2.as_ref(),
        ],
        MESSAGE_TRANSMITTER_V2,
    );

    let cctp_message_transmitter_pda = find(&[b"message_transmitter"], MESSAGE_TRANSMITTER_V2);

    let result = DecodedVerifierResult::from_bytes(&verifier_results)?;

    let cctp_used_nonce_pda = find(
        &[b"used_nonce", result.cctp_message.header.nonce().as_ref()],
        MESSAGE_TRANSMITTER_V2,
    );

    let cctp_token_messenger = find(&[b"token_messenger"], TOKEN_MESSENGER_MINTER_V2);

    let remote_burn_token = result.cctp_message.body.burn_token();

    Ok(NestedResolveAccountsResponse {
        ask_again_with: vec![config_key, remote_chain_pda, mint, cctp_token_messenger],
        accounts_to_save: vec![
            config_key.readonly(),
            remote_chain_pda.readonly(),
            mint.readonly(),
            cpi_signer.writable(),
            pool_ata.writable(),
            end_receiver.writable(),
            anchor_spl::token::ID.readonly(), // assumed to be SPL token program for all CCTP tokens, see comment above
            cctp_authority_pda.readonly(),
            cctp_message_transmitter_pda.readonly(),
            cctp_used_nonce_pda.writable(),
            system_program::ID.readonly(),
            cctp_token_messenger.readonly(),
        ],
        lookup_tables_to_save: vec![],
        next_cpi_resolution_inputs: serialize_metadata(
            &CctpCcvCpiResolutionInputs::VerifyWithRemoteChain {
                source_chain_selector,
                mint,
                remote_burn_token,
                offramp,
            },
        )?,
    })
}

/// Second stage of account resolution for the `verify` instruction.
pub fn with_remote_chain<'info>(
    mut remaining_accounts: &'info [AccountInfo<'info>],
    source_chain_selector: u64,
    mint: Pubkey,
    remote_burn_token: Pubkey,
    offramp: Pubkey,
) -> Result<NestedResolveAccountsResponse> {
    let bumps =
        &mut <NestedResolveAccountsVerifyWithRemoteChain as anchor_lang::Bumps>::Bumps::default();
    let reallocs = &mut std::collections::BTreeSet::new();
    let accounts = NestedResolveAccountsVerifyWithRemoteChain::try_accounts(
        &crate::ID,
        &mut remaining_accounts,
        &[
            source_chain_selector.to_le_bytes().as_ref(),
            mint.to_bytes().as_ref(),
        ]
        .concat(),
        bumps,
        reallocs,
    )?;

    let remote_domain_seed = accounts.remote_chain.cctp.domain_identifier.to_string();

    let remote_token_messenger = find(
        &[b"remote_token_messenger", remote_domain_seed.as_ref()],
        TOKEN_MESSENGER_MINTER_V2,
    );

    let token_minter = find(&[b"token_minter"], TOKEN_MESSENGER_MINTER_V2);

    let local_token = find(
        &[b"local_token", accounts.mint.key().as_ref()],
        TOKEN_MESSENGER_MINTER_V2,
    );

    let token_pair = find(
        &[
            b"token_pair",
            remote_domain_seed.as_ref(),
            remote_burn_token.as_ref(),
        ],
        TOKEN_MESSENGER_MINTER_V2,
    );

    let fee_recipient_ata = get_associated_token_address_with_program_id(
        &accounts.cctp_token_messenger.fee_recipient,
        &accounts.mint.key(),
        &anchor_spl::token::ID, // assumed that every CCTP token is a standard SPL token
    );

    let custody_token_account = find(
        &[b"custody", accounts.mint.key().as_ref()],
        TOKEN_MESSENGER_MINTER_V2,
    );

    let tmm_event_authority = find(&[b"__event_authority"], TOKEN_MESSENGER_MINTER_V2);
    let mt_event_authority = find(&[b"__event_authority"], MESSAGE_TRANSMITTER_V2);

    let lookup_tables_to_save = if let Some(alt) = accounts.remote_chain.cctp.lookup_table {
        vec![alt]
    } else {
        vec![]
    };

    let allowed_offramp = find(
        &[
            ccip_common::seed::ALLOWED_OFFRAMP,
            source_chain_selector.to_le_bytes().as_ref(),
            offramp.as_ref(),
        ],
        accounts.remote_chain.admin_config.router,
    );
    let expected_authority = find(&[CCV_SIGNER, crate::ID.as_ref()], offramp);

    Ok(NestedResolveAccountsResponse {
        ask_again_with: vec![],
        accounts_to_save: vec![
            remote_token_messenger.readonly(),
            token_minter.readonly(),
            local_token.writable(),
            token_pair.readonly(),
            fee_recipient_ata.writable(),
            custody_token_account.writable(),
            mt_event_authority.readonly(),
            MESSAGE_TRANSMITTER_V2.readonly(),
            tmm_event_authority.readonly(),
            TOKEN_MESSENGER_MINTER_V2.readonly(),
            expected_authority.readonly(), // the offramp will sign with this PDA always, regardless of the flags we put here
            offramp.readonly(),
            allowed_offramp.readonly(),
        ],
        lookup_tables_to_save,
        next_cpi_resolution_inputs: vec![],
    })
}

#[derive(Accounts)]
#[instruction(source_chain_selector: u64, ix_data_mint: Pubkey)]
struct NestedResolveAccountsVerifyWithRemoteChain<'info> {
    #[account(
        seeds = [verifier_seed::REMOTE_CHAIN, &source_chain_selector.to_le_bytes()],
        bump = remote_chain.bump,
    )]
    pub remote_chain: Account<'info, RemoteChainConfig>,

    #[account(address = ix_data_mint)]
    pub mint: InterfaceAccount<'info, Mint>,

    #[account(
        seeds = [b"token_messenger"],
        bump,
        seeds::program = TOKEN_MESSENGER_MINTER_V2,
    )]
    pub cctp_token_messenger: Account<'info, TokenMessenger>,
}

// This comes for CCTP - defining the schema here so we can deserialize it to resolve the fee_recipient.
// Manually setting the discriminator & owner to match the CCTP PDA
#[account("cctp_v2", discriminator = [162, 4, 242, 52, 147, 243, 221, 96])]
#[derive(Debug, InitSpace)]
struct TokenMessenger {
    pub _denylister: Pubkey,
    pub _owner: Pubkey,
    pub _pending_owner: Pubkey,
    pub _message_body_version: u32,
    pub _authority_bump: u8,
    pub fee_recipient: Pubkey,
    pub _min_fee_controller: Pubkey,
    pub _min_fee: u32,
}

impl anchor_lang::Owner for TokenMessenger {
    fn owner() -> Pubkey {
        TOKEN_MESSENGER_MINTER_V2
    }
}
