use anchor_lang::prelude::*;

use anchor_lang::prelude::instruction::Instruction;
use anchor_lang::prelude::program::invoke_signed;
use anchor_spl::associated_token::get_associated_token_address_with_program_id;
use anchor_spl::token_interface::{Mint, TokenAccount, TokenInterface};
use ccip_common::seed::{ALLOWED_OFFRAMP, CCV_SIGNER};

use ccip_common::messages::{MessageV1, SVMDestBlob, TokenTransferV1};

use crate::error::CctpPoolVerifierError;
use crate::utils::DisplaySlice;
use crate::verifier::cctp_constants::*;
use crate::verifier::instructions::utils;
use crate::verifier::state::{
    verifier_seed, AccountSchemaVersion, Config, RemoteChainConfig, RemoteLocalPair,
};
use crate::SOLANA_DOMAIN_ID;

use super::cctp_message::*;

const CCIP_MESSAGE_ID_LEN: usize = 32;

#[derive(AnchorSerialize, AnchorDeserialize, Debug)]
pub struct VerifyParams {
    pub message_id: [u8; CCIP_MESSAGE_ID_LEN],
    pub encoded_message: Vec<u8>,

    // The verifier result is a concatenation of bytes with the format:
    // - version tag --> We validate this, it's CCIP-specific
    // - CCTP message --> We do consistency checks on this and forward it to CCTP
    // - CCTP attestation --> Opaque to CCIP, forwarded without inspection for CCTP to validate
    pub verifier_results: Vec<u8>,
}

pub fn verify(ctx: Context<Verify>, params: VerifyParams) -> Result<()> {
    let message = MessageV1::from_bytes(&params.encoded_message)
        .map_err(|_| CctpPoolVerifierError::InvalidMessage)?;

    let token_transfer = message
        .token_transfer
        .as_ref()
        .ok_or(CctpPoolVerifierError::MissingTokenTransfer)?;

    let mint = utils::parse_pubkey(token_transfer.dest_token_address.as_slice())?;

    // CCIP-only validations. Authorizing the caller as a valid offramp is already done in the context constraints.
    validate_remote_chain_pda(&ctx, message.source_chain_selector)?;
    validate_mint_account(&ctx, &mint)?;
    validate_end_receiver_account(&ctx, message.dest_blob.as_slice(), token_transfer)?;

    // Decode result and perform CCTP consistency checks
    let result = DecodedVerifierResult::from_bytes(&params.verifier_results)?.validate(
        &ctx,
        &params.message_id,
        &mint,
        token_transfer,
    )?;

    // The CCTP message always mints the tokens to the pool's token account. After the CCTP part is done,
    // we need to forward those tokens along to the end token receiver. We do that in the verifier and not in the pool
    // so that we can take a snapshot of the balance before the mint, and only forward the exact amount. Even though
    // the pool token account should generally be empty, this is a defense-in-depth measure to ensure that the pool
    // doesn't accidentally forward more than it should. And because of the destination-side fees of CCTP with
    // faster-than-finality transfers, we can't rely on the token transfer amount in the message from within the pool
    // without accessing the verifier result as well (which is not exposed to the pool).
    let starting_balance = ctx.accounts.pool_token_account.amount;
    invoke_cctp_receive_message(&ctx, &result)?;
    ctx.accounts.pool_token_account.reload()?; // refresh the account after the CPI to CCTP, so we can get the new balance
    forward_funds_to_end_receiver(&ctx, &result, starting_balance)?;

    Ok(())
}

fn validate_mint_account(ctx: &Context<Verify>, mint: &Pubkey) -> Result<()> {
    require_keys_eq!(
        *mint,
        ctx.accounts.mint.key(),
        CctpPoolVerifierError::InvalidInstructionData
    );

    Ok(())
}

fn validate_end_receiver_account(
    ctx: &Context<Verify>,
    dest_blob_bytes: &[u8],
    token_transfer: &TokenTransferV1,
) -> Result<()> {
    let end_receiver_token_account = &ctx.accounts.end_receiver_token_account;

    let SVMDestBlob { use_ata, .. } = SVMDestBlob::from_bytes(dest_blob_bytes)
        .map_err(|_| CctpPoolVerifierError::InvalidInstructionData)?;

    let token_receiver = utils::parse_pubkey(token_transfer.token_receiver.as_slice())?;

    let expected = if use_ata.should_derive_ata() {
        get_associated_token_address_with_program_id(
            &token_receiver,
            &ctx.accounts.mint.key(),
            &ctx.accounts.token_program.key(),
        )
    } else {
        token_receiver
    };

    require_keys_eq!(
        end_receiver_token_account.key(),
        expected,
        CctpPoolVerifierError::InvalidInstructionData
    );

    Ok(())
}

fn validate_remote_chain_pda(ctx: &Context<Verify>, chain_selector: u64) -> Result<()> {
    let expected_pda = Pubkey::create_program_address(
        &[
            verifier_seed::REMOTE_CHAIN,
            &chain_selector.to_le_bytes(),
            &[ctx.accounts.remote_chain_config.bump],
        ],
        &crate::ID,
    )
    .map_err(|_| CctpPoolVerifierError::InvalidInstructionData)?;

    require_keys_eq!(
        ctx.accounts.remote_chain_config.key(),
        expected_pda,
        CctpPoolVerifierError::InvalidInstructionData
    );

    require!(
        ctx.accounts.remote_chain_config.cctp.enabled,
        CctpPoolVerifierError::UnknownDomain
    );

    Ok(())
}

#[derive(Accounts)]
pub struct Verify<'info> {
    // ----------
    // Verifier accounts
    // ----------
    #[account(
        seeds = [verifier_seed::CONFIG],
        bump = config.bump,
        constraint = config.is_latest_version() @ CctpPoolVerifierError::InvalidVersion,
    )]
    pub config: Account<'info, Config>,

    // NOTE: The remote_chain_config PDA is not validated via a seeds constraint
    // here because deriving the seeds requires the source_chain_selector, which
    // is only available after decoding the message. The processor validates the
    // PDA address manually to avoid decoding the message twice.
    #[account(
        constraint = remote_chain_config.is_latest_version() @ CctpPoolVerifierError::InvalidVersion,
    )]
    pub remote_chain_config: Account<'info, RemoteChainConfig>,

    // NOTE: Checked inside the method handler for consistency with the message's dest token.
    // This is not done as a context constraint to avoid decoding the message twice.
    #[account(owner = token_program.key())]
    pub mint: InterfaceAccount<'info, Mint>,

    #[account(
        mut,
        seeds = [verifier_seed::POOL_SIGNER_SEED, &mint.key().to_bytes()],
        bump,
    )]
    pub cpi_signer: UncheckedAccount<'info>,

    // We receive the minted tokens in the pool/verifier's ATA.
    // Then the pool, during the actual "mint_or_release" instruction, will transfer the tokens to the user.
    #[account(
        mut,
        associated_token::mint = mint,
        associated_token::authority = cpi_signer,
        associated_token::token_program = token_program,
    )]
    pub pool_token_account: Box<InterfaceAccount<'info, TokenAccount>>,

    // Validated inside the method handler for consistency with the message's token receiver,
    // to avoid decoding the message repeatedly
    #[account(
        mut,
        owner = token_program.key(),
        has_one = mint,
    )]
    pub end_receiver_token_account: Box<InterfaceAccount<'info, TokenAccount>>,

    pub token_program: Interface<'info, TokenInterface>,

    // ----------
    // CCTP accounts for receive_message CPI
    // ----------
    /// CHECK: Forwarded to CCTP and checked there
    pub authority_pda: UncheckedAccount<'info>,

    /// CHECK: Forwarded to CCTP and checked there
    pub message_transmitter: UncheckedAccount<'info>,

    /// CHECK: Forwarded to CCTP and checked there
    #[account(mut)]
    pub used_nonce: UncheckedAccount<'info>,

    pub system_program: Program<'info, System>,

    /// CHECK: Forwarded to CCTP and checked there
    pub token_messenger: UncheckedAccount<'info>,

    /// CHECK: Forwarded to CCTP and checked there
    pub remote_token_messenger: UncheckedAccount<'info>,

    /// CHECK: Forwarded to CCTP and checked there
    pub token_minter: UncheckedAccount<'info>,

    /// CHECK: Forwarded to CCTP and checked there
    #[account(mut)]
    pub local_token: UncheckedAccount<'info>,

    /// CHECK: Forwarded to CCTP and checked there
    pub token_pair: UncheckedAccount<'info>,

    /// CHECK: Forwarded to CCTP and checked there
    #[account(mut)]
    pub fee_recipient_ata: UncheckedAccount<'info>,

    /// CHECK: Forwarded to CCTP and checked there
    #[account(mut)]
    pub custody_token_account: UncheckedAccount<'info>,

    /// CHECK: Forwarded to CCTP and checked there
    pub message_transmitter_event_authority: UncheckedAccount<'info>,

    #[account(address = MESSAGE_TRANSMITTER_V2)]
    pub message_transmitter_program: UncheckedAccount<'info>,

    /// CHECK: Forwarded to CCTP and checked there
    pub token_messenger_minter_event_authority: UncheckedAccount<'info>,

    /// CHECK: Receiver program address, i.e. TokenMessenger
    #[account(address = TOKEN_MESSENGER_MINTER_V2)]
    pub token_messenger_minter_program: UncheckedAccount<'info>,

    // ----------
    // CCIP accounts to authorize offramp
    // ----------
    #[account(
        seeds = [CCV_SIGNER, crate::ID.as_ref()],
        bump,
        seeds::program = offramp_program.key(),
    )]
    pub authority: Signer<'info>,

    /// CHECK offramp program: exists only to derive the allowed offramp PDA
    /// and the authority PDA.
    pub offramp_program: UncheckedAccount<'info>,

    /// CHECK PDA of the router program verifying the signer is an allowed offramp.
    /// If PDA does not exist, the router doesn't allow this offramp
    #[account(
        owner = remote_chain_config.admin_config.router @ CctpPoolVerifierError::Unauthorized,
        seeds = [
            ALLOWED_OFFRAMP,
            remote_chain_config.remote_chain_selector.to_le_bytes().as_ref(),
            offramp_program.key().as_ref()
        ],
        bump,
        seeds::program = remote_chain_config.admin_config.router,
    )]
    pub allowed_offramp: UncheckedAccount<'info>,
}

#[derive(Debug)]
pub(super) struct DecodedVerifierResult<'a> {
    pub _version_tag: [u8; 4],
    pub cctp_message: CctpMessage<'a>,
    pub attestation: &'a [u8],
}
struct ValidatedVerifierResult<'a>(DecodedVerifierResult<'a>);

const HOOK_DATA_LEN: usize = VERSION_TAG.len() + CCIP_MESSAGE_ID_LEN; // 36
const VERSION_TAG_SLICE: SliceRange = slice_range(0, VERSION_TAG.len());
const MESSAGE_SLICE: SliceRange =
    slice_range(VERSION_TAG.len(), CctpMessage::MIN_SIZE + HOOK_DATA_LEN);
const MIN_ECDSA_SIGNATURE_LEN: usize = 65; // ecdsa signature with recovery byte

impl<'a> DecodedVerifierResult<'a> {
    pub fn from_bytes(bytes: &'a [u8]) -> Result<Self> {
        // We always require at least 1 sig for the attestation, besides the version tag prefix and the message.
        // Circle does a similar (stricter) check in their contracts, but we still check just in case
        // https://github.com/circlefin/solana-cctp-contracts/blob/ec16e95d28ee47f7832df4203ae07b5981d146fc/programs/v2/message-transmitter-v2/src/state.rs#L84-L91
        require_gte!(
            bytes.len(),
            MESSAGE_SLICE.end + MIN_ECDSA_SIGNATURE_LEN,
            CctpPoolVerifierError::MalformedVerifierResults
        );

        let version_tag: [u8; 4] = VERSION_TAG_SLICE
            .of(bytes)
            .try_into()
            .expect("bytes.len() >= VERSION_TAG.len() (verified above)");

        require_eq!(
            DisplaySlice(&version_tag),
            DisplaySlice(&VERSION_TAG),
            CctpPoolVerifierError::InvalidVersionTag
        );

        Ok(Self {
            _version_tag: version_tag,
            cctp_message: CctpMessage::from_bytes(MESSAGE_SLICE.of(bytes))?,
            attestation: &bytes[MESSAGE_SLICE.end..],
        })
    }

    fn validate(
        self,
        ctx: &Context<Verify>,
        message_id: &[u8; CCIP_MESSAGE_ID_LEN],
        mint: &Pubkey,
        token_transfer: &TokenTransferV1,
    ) -> Result<ValidatedVerifierResult<'a>> {
        let remote = &ctx.accounts.remote_chain_config.cctp;

        self.validate_hook_data(message_id)?;
        self.validate_domain_consistency(remote.domain_identifier)?;
        self.validate_source_dest_token(&remote.supported_tokens, token_transfer, mint)?;
        self.validate_recipient()?;
        self.validate_mint_recipient(ctx.accounts.pool_token_account.key())?;
        self.validate_message_sender(remote.allowed_caller_deposit)?;

        Ok(ValidatedVerifierResult(self))
    }

    fn validate_message_sender(&self, allowed_sender: Pubkey) -> Result<()> {
        // The messageSender property of the messageBody must align with the allowedCallerOnSource.
        // This check is critical to ensure that CCIP is unable to process burn messages generated by other systems.
        let cctp_sender = self.cctp_message.body.message_sender();
        require_keys_eq!(
            cctp_sender,
            allowed_sender,
            CctpPoolVerifierError::InvalidCctpSender
        );

        Ok(())
    }

    fn validate_mint_recipient(&self, pool_ata: Pubkey) -> Result<()> {
        // This CCV expects the tokens to be minted to a proxy ATA owned by the pool (in this same program)
        // and then the funds are forwarded separately to the end receiver. This is to accommodate cases where
        // CCIP works with wallets and ATAs but CCTP works with plain token accounts, and there are no consistency
        // checks between the two on the source chains.
        require_keys_eq!(
            self.cctp_message.body.mint_recipient(),
            pool_ata,
            CctpPoolVerifierError::InvalidCctpRecipient
        );
        Ok(())
    }

    fn validate_recipient(&self) -> Result<()> {
        // The messaging-layer recipient must be the TokenMessengerMinter program. CCTP's message transmitter is a generic messaging
        // layer that can carry messages other than token transfers, so this check is critical to ensure that we are
        // processing a message addressed to the token bridge, and not some other message on the message transmitter.
        let recipient = self.cctp_message.header.recipient();
        require_keys_eq!(
            recipient,
            TOKEN_MESSENGER_MINTER_V2,
            CctpPoolVerifierError::InvalidCctpRecipient
        );
        // the context constraints enforce that the passed-in account also matches the TokenMessengerMinter program
        Ok(())
    }

    fn validate_source_dest_token(
        &self,
        supported_tokens: &[RemoteLocalPair],
        token_transfer: &TokenTransferV1,
        mint: &Pubkey,
    ) -> Result<()> {
        // Circle's canonical mapping for the attested sourceDomain and burnToken must resolve to this chain's equivalent token (e.g. USDC).
        // This is a defense-in-depth check confirming that the burned token is actually recognized as that token by CCTP itself,
        // rather than relying solely on the local configuration of this verifier.
        // In Solana, this is achieved by having many consistency checks between these things:
        // - The remote token in CCTP's message
        // - The remote token in CCIP's message
        // - The local token in CCIP's message
        // - The local token in this verifier _for that given remote token_
        // - The local token in this instruction's account
        // - The local token in CCTP's token minter TokenPair PDA
        //    ╰→ delegated to CCTP - we only forward the consistent mint-related token accounts, so it can't release any other token

        // CCTP limits the address to 32 bytes, see
        // https://developers.circle.com/cctp/references/technical-guide#message-body
        let source_token =
            utils::left_padded_pubkey(token_transfer.source_token_address.as_slice())?;

        let burn_token = self.cctp_message.body.burn_token();
        require_keys_eq!(
            burn_token,
            source_token,
            CctpPoolVerifierError::InvalidSourceTokenAddress
        );
        require!(
            supported_tokens
                .iter()
                .any(|pair| pair.matches(mint, &token_transfer.source_token_address)),
            CctpPoolVerifierError::UnknownDomain
        );

        Ok(())
    }

    fn validate_domain_consistency(&self, remote_domain_identifier: u32) -> Result<()> {
        let attested_source_domain = self.cctp_message.header.source_domain();
        require_eq!(
            attested_source_domain,
            remote_domain_identifier,
            CctpPoolVerifierError::InvalidCctpSourceDomain
        );

        let attested_destination_domain = self.cctp_message.header.destination_domain();
        require_eq!(
            attested_destination_domain,
            SOLANA_DOMAIN_ID,
            CctpPoolVerifierError::InvalidCctpDestinationDomain
        );

        Ok(())
    }

    fn validate_hook_data(&self, message_id: &[u8; CCIP_MESSAGE_ID_LEN]) -> Result<()> {
        // This check is ensures that the verifier being used here is using the same
        // version of the verifier in the source chain (via the VERSION_TAG prefix)
        // and that the CCTP message was generated by this CCIP message

        let hook_data = self.cctp_message.body.hook_data();
        require_eq!(
            hook_data.len(),
            VERSION_TAG.len() + message_id.len(), // 36
            CctpPoolVerifierError::InvalidCctpHookData
        );

        let attested_version = &hook_data[..VERSION_TAG.len()];
        require_eq!(
            DisplaySlice(attested_version),
            DisplaySlice(&VERSION_TAG),
            CctpPoolVerifierError::InvalidCctpHookData
        );

        let attested_message_id = &hook_data[VERSION_TAG.len()..];
        require_eq!(
            DisplaySlice(attested_message_id),
            DisplaySlice(message_id),
            CctpPoolVerifierError::InvalidCctpHookData
        );

        Ok(())
    }
}

fn invoke_cctp_receive_message(
    ctx: &Context<Verify>,
    result: &ValidatedVerifierResult,
) -> Result<()> {
    let DecodedVerifierResult {
        cctp_message,
        attestation,
        ..
    } = &result.0;

    let message_bytes = cctp_message.as_bytes();
    let mut ix_data = Vec::with_capacity(
        RECEIVE_MESSAGE_DISCRIMINATOR.len()
        + 8 // This is for the `Vec` length prefixes for the two `Vec`s that follow, which is 8 bytes in total (4+4)
        + attestation.len()
        + message_bytes.len(),
    );
    ix_data.extend_from_slice(&RECEIVE_MESSAGE_DISCRIMINATOR);
    borsh::to_writer(&mut ix_data, &message_bytes)?;
    borsh::to_writer(&mut ix_data, &attestation.to_vec())?;

    let signer_seeds: &[&[u8]] = &[
        verifier_seed::POOL_SIGNER_SEED,
        &ctx.accounts.mint.key().to_bytes(),
        &[ctx.bumps.cpi_signer],
    ];
    let signer_key = ctx.accounts.cpi_signer.key();

    let acc_infos = [
        ctx.accounts.cpi_signer.to_account_info(), // payer
        ctx.accounts.cpi_signer.to_account_info(), // caller
        ctx.accounts.authority_pda.to_account_info(),
        ctx.accounts.message_transmitter.to_account_info(),
        ctx.accounts.used_nonce.to_account_info(),
        ctx.accounts
            .token_messenger_minter_program
            .to_account_info(), // receiver (from MessageTransmitter PoV)
        ctx.accounts.system_program.to_account_info(),
        ctx.accounts
            .message_transmitter_event_authority
            .to_account_info(),
        ctx.accounts.message_transmitter_program.to_account_info(),
        // From here on, these remaining accounts are what the MessageTransmitter forwards to the
        // receiver (TokenMessenger) for the actual token minting
        ctx.accounts.token_messenger.to_account_info(),
        ctx.accounts.remote_token_messenger.to_account_info(),
        ctx.accounts.token_minter.to_account_info(),
        ctx.accounts.local_token.to_account_info(),
        ctx.accounts.token_pair.to_account_info(),
        ctx.accounts.fee_recipient_ata.to_account_info(),
        ctx.accounts.pool_token_account.to_account_info(),
        ctx.accounts.custody_token_account.to_account_info(),
        ctx.accounts.token_program.to_account_info(),
        ctx.accounts
            .token_messenger_minter_event_authority
            .to_account_info(),
        ctx.accounts
            .token_messenger_minter_program
            .to_account_info(),
    ];

    let acc_metas: Vec<AccountMeta> = acc_infos
        .iter()
        .flat_map(|acc_info| {
            let is_signer = signer_key == *acc_info.key;
            acc_info.to_account_metas(Some(is_signer))
        })
        .collect();

    let instruction = Instruction {
        program_id: MESSAGE_TRANSMITTER_V2,
        accounts: acc_metas,
        data: ix_data,
    };

    invoke_signed(&instruction, &acc_infos, &[signer_seeds])?;

    Ok(())
}

fn forward_funds_to_end_receiver(
    ctx: &Context<Verify>,
    result: &ValidatedVerifierResult,
    starting_balance: u64,
) -> Result<()> {
    let current_pool_balance = ctx.accounts.pool_token_account.amount;
    let amount = current_pool_balance
        .checked_sub(starting_balance)
        .ok_or(CctpPoolVerifierError::InvalidCctpMintAmount)?;

    // Sanity check: CCTP's TokenMessengerMinter mints the transferred amount minus whatever fees were actually
    // executed on destination due to faster-than-finality transfer fees. The EVM pools use `depositForBurnWithHook`
    // and never `depositForBurnWithHookAndFees`, so any FTF fees from CCTP are always deducted on the destination
    // chain (i.e. here)
    let expected = result
        .0
        .cctp_message
        .body
        .amount()?
        .checked_sub(result.0.cctp_message.body.fee_executed()?)
        .ok_or(CctpPoolVerifierError::FeeGreaterThanAmount)?;
    require_eq!(
        amount,
        expected,
        CctpPoolVerifierError::InvalidCctpMintAmount
    );

    let seeds = &[
        verifier_seed::POOL_SIGNER_SEED,
        &ctx.accounts.mint.key().to_bytes(),
        &[ctx.bumps.cpi_signer],
    ];

    let signer_seeds: &[&[&[u8]]] = &[seeds];
    let cpi_ctx = CpiContext::new_with_signer(
        ctx.accounts.token_program.key(),
        anchor_spl::token::TransferChecked {
            from: ctx.accounts.pool_token_account.to_account_info(),
            to: ctx.accounts.end_receiver_token_account.to_account_info(),
            authority: ctx.accounts.cpi_signer.to_account_info(),
            mint: ctx.accounts.mint.to_account_info(),
        },
        signer_seeds,
    );

    anchor_spl::token::transfer_checked(cpi_ctx, amount, ctx.accounts.mint.decimals)
}
