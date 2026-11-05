use anchor_lang::prelude::*;
use anchor_lang::solana_program::instruction::{AccountMeta, Instruction};
use anchor_lang::solana_program::program::invoke_signed;

use anchor_spl::associated_token::get_associated_token_address_with_program_id;
use anchor_spl::token_interface::{Mint, TokenAccount, TokenInterface};

use base_token_pool::{constants::*, errors::*, events::*, helpers::*, state::*, v1::*};
use ccip_common::seed;
use ccip_common::state::AccountSchemaVersion;

use super::cctp_message::*;
use super::token_pool_extra_data::*;

use crate::error::CctpPoolVerifierError;
use crate::pool::instructions::{MESSAGE_TRANSMITTER_V1, TOKEN_MESSENGER_MINTER_V1};
use crate::pool::state::{ChainConfig, State};
use crate::SOLANA_DOMAIN_ID;

pub const RECEIVE_MESSAGE_DISCRIMINATOR: [u8; 8] = [38, 144, 127, 225, 31, 225, 238, 25]; // global:receive_message

pub fn mint_tokens<'info>(
    ctx: Context<'info, TokenOfframp<'info>>,
    release_or_mint: ReleaseOrMintInV1,
) -> Result<ReleaseOrMintOutV1> {
    require!(
        !release_or_mint.offchain_token_data.is_empty(),
        CctpPoolVerifierError::InvalidTokenData
    );

    let msg_att = MessageAndAttestation::try_from_slice(&release_or_mint.offchain_token_data)
        .map_err(|_| CctpPoolVerifierError::InvalidTokenData)?;

    let remote_token_address_bytes =
        to_solana_pubkey(&ctx.accounts.chain_config.base.remote.token_address).to_bytes();

    let remaining = parse_remaining_release_accounts(
        ctx.remaining_accounts,
        release_or_mint.local_token,
        &msg_att.message,
        ctx.accounts.chain_config.cctp.domain_id,
        remote_token_address_bytes,
    )?;

    let parsed_amount = cross_chain_amount_to_svm_token_amount(
        CrossChainAmount::from_le_bytes(release_or_mint.amount),
        ctx.accounts.chain_config.base.remote.decimals,
        ctx.accounts.state.config.decimals,
    )?;

    let BaseChain {
        remote,
        inbound_rate_limit,
        ..
    } = &mut ctx.accounts.chain_config.base;

    validate_release_or_mint(
        &release_or_mint,
        parsed_amount,
        ctx.accounts.state.config.mint,
        &remote.pool_addresses,
        inbound_rate_limit,
    )?;

    validate_cctp_and_ccip_messages(&msg_att.message, &release_or_mint)?;

    let message_and_attestation_bytes = borsh::to_vec(&msg_att).unwrap();
    cctp_receive_message(&ctx, &remaining, &message_and_attestation_bytes)?;
    transfer_cctp_tokens(&ctx, parsed_amount, ctx.accounts.state.config.decimals)?;

    emit!(Minted {
        sender: ctx.accounts.authority.key(),
        recipient: release_or_mint.receiver,
        amount: parsed_amount,
        mint: ctx.accounts.state.config.mint,
    });

    Ok(ReleaseOrMintOutV1 {
        destination_amount: parsed_amount,
    })
}

pub fn to_solana_pubkey(address: &RemoteAddress) -> Pubkey {
    let mut solana_address = [0; 32];
    let remote_bytes = address.len();
    solana_address[32 - remote_bytes..].copy_from_slice(address);
    Pubkey::new_from_array(solana_address)
}

#[derive(Accounts)]
#[instruction(release_or_mint: ReleaseOrMintInV1)]
pub struct TokenOfframp<'info> {
    // CCIP accounts ------------------------
    #[account(
        seeds = [EXTERNAL_TOKEN_POOLS_SIGNER, crate::ID.as_ref()],
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
        owner = state.config.router @ CcipTokenPoolError::InvalidPoolCaller, // this guarantees that it was initialized
        seeds = [
            ALLOWED_OFFRAMP,
            release_or_mint.remote_chain_selector.to_le_bytes().as_ref(),
            offramp_program.key().as_ref()
        ],
        bump,
        seeds::program = state.config.router,
    )]
    pub allowed_offramp: UncheckedAccount<'info>,

    // Token pool accounts ------------------
    // consistent set + token pool program
    #[account(
        seeds = [
            POOL_STATE_SEED,
            mint.key().as_ref()
        ],
        bump,
        constraint = state.is_latest_version() @ CcipTokenPoolError::InvalidVersion,
    )]
    pub state: Box<Account<'info, State>>,

    #[account(address = state.config.token_program)]
    pub token_program: Interface<'info, TokenInterface>,

    #[account(mut)]
    pub mint: InterfaceAccount<'info, Mint>,

    /// CHECK: CPI signer
    #[account(
        mut, // CCTP's receive_message method requires that this account is mutable,
             // although it does not really mutate it
        seeds = [POOL_SIGNER_SEED, mint.key().as_ref()],
        bump,
        address = state.config.pool_signer,
    )]
    pub pool_signer: UncheckedAccount<'info>,

    #[account(
        mut,
        associated_token::mint = mint,
        associated_token::authority = pool_signer,
        associated_token::token_program = token_program,
    )]
    pub pool_token_account: Box<InterfaceAccount<'info, TokenAccount>>,

    #[account(
        mut,
        seeds = [
            POOL_CHAINCONFIG_SEED,
            &release_or_mint.remote_chain_selector.to_le_bytes(),
            mint.key().as_ref(),
        ],
        bump,
        constraint = chain_config.is_latest_version() @ CcipTokenPoolError::InvalidVersion,
    )]
    pub chain_config: Box<Account<'info, ChainConfig>>,

    ////////////////////
    // RMN Remote CPI //
    ////////////////////
    /// CHECK: This is the account for the RMN Remote program
    #[account(
        address = state.config.rmn_remote @ CcipTokenPoolError::InvalidRMNRemoteAddress,
    )]
    pub rmn_remote: UncheckedAccount<'info>,

    /// CHECK: This account is just used in the CPI to the RMN Remote program
    #[account(
        seeds = [seed::CURSES],
        bump,
        seeds::program = state.config.rmn_remote,
    )]
    pub rmn_remote_curses: UncheckedAccount<'info>,

    /// CHECK: This account is just used in the CPI to the RMN Remote program
    #[account(
        seeds = [seed::CONFIG],
        bump,
        seeds::program = state.config.rmn_remote,
    )]
    pub rmn_remote_config: UncheckedAccount<'info>,

    #[account(
        mut,
        address = get_associated_token_address_with_program_id(&release_or_mint.receiver, &mint.key(), &token_program.key())
    )]
    pub receiver_token_account: InterfaceAccount<'info, TokenAccount>,
}

fn parse_remaining_release_accounts<'info>(
    remaining: &'info [AccountInfo<'info>],
    mint: Pubkey,
    cctp_msg: &CctpMessage,
    cctp_domain_id: u32,
    remote_token_address: [u8; 32],
) -> Result<TokenOfframpRemainingAccounts<'info>> {
    let mut remaining_accounts = remaining.iter();
    let result = TokenOfframpRemainingAccounts {
        // Accounts in lookup table
        cctp_message_transmitter_account: next_account_info(&mut remaining_accounts)?,
        cctp_token_messenger_minter: next_account_info(&mut remaining_accounts)?,
        system_program: next_account_info(&mut remaining_accounts)?,
        cctp_message_transmitter: next_account_info(&mut remaining_accounts)?,
        cctp_token_messenger_account: next_account_info(&mut remaining_accounts)?,
        cctp_token_minter_account: next_account_info(&mut remaining_accounts)?,
        cctp_local_token: next_account_info(&mut remaining_accounts)?,
        cctp_token_messenger_minter_event_authority: next_account_info(&mut remaining_accounts)?,

        // Account not in lookup table
        cctp_authority_pda: next_account_info(&mut remaining_accounts)?,
        cctp_event_authority: next_account_info(&mut remaining_accounts)?,
        cctp_custody_token_account: next_account_info(&mut remaining_accounts)?,
        cctp_remote_token_messenger_key: next_account_info(&mut remaining_accounts)?,
        cctp_token_pair: next_account_info(&mut remaining_accounts)?,
        cctp_used_nonces: next_account_info(&mut remaining_accounts)?,
    };

    result.validate(mint, cctp_msg, cctp_domain_id, remote_token_address)?;

    Ok(result)
}

fn cctp_receive_message<'info>(
    ctx: &Context<TokenOfframp<'info>>,
    remaining: &TokenOfframpRemainingAccounts<'info>,
    message_and_attestation_bytes: &[u8],
) -> Result<()> {
    let mut instruction_data = Vec::with_capacity(
        RECEIVE_MESSAGE_DISCRIMINATOR.len() + message_and_attestation_bytes.len(),
    );
    instruction_data.extend_from_slice(&RECEIVE_MESSAGE_DISCRIMINATOR);
    instruction_data.extend_from_slice(message_and_attestation_bytes);

    let acc_infos = vec![
        ctx.accounts.pool_signer.clone().to_account_info(),
        ctx.accounts.pool_signer.clone().to_account_info(),
        remaining.cctp_authority_pda.to_account_info(),
        remaining.cctp_message_transmitter_account.to_account_info(),
        remaining.cctp_used_nonces.to_account_info(),
        remaining.cctp_token_messenger_minter.to_account_info(),
        remaining.system_program.to_account_info(),
        remaining.cctp_event_authority.to_account_info(),
        remaining.cctp_message_transmitter.to_account_info(),
        remaining.cctp_token_messenger_account.to_account_info(),
        remaining.cctp_remote_token_messenger_key.to_account_info(),
        remaining.cctp_token_minter_account.to_account_info(),
        remaining.cctp_local_token.to_account_info(),
        remaining.cctp_token_pair.to_account_info(),
        ctx.accounts.pool_token_account.clone().to_account_info(), // always mint to the tp ATA first
        remaining.cctp_custody_token_account.to_account_info(),
        ctx.accounts.token_program.clone().to_account_info(),
        remaining
            .cctp_token_messenger_minter_event_authority
            .to_account_info(),
        remaining.cctp_token_messenger_minter.to_account_info(),
    ];

    let acc_metas: Vec<AccountMeta> = acc_infos
        .iter()
        .flat_map(|acc_info| {
            let is_signer = acc_info.key() == ctx.accounts.pool_signer.key();
            acc_info.to_account_metas(Some(is_signer))
        })
        .collect();

    let instruction = Instruction {
        program_id: remaining.cctp_message_transmitter.key(),
        accounts: acc_metas,
        data: instruction_data,
    };

    let signer_seeds: &[&[u8]] = &[
        POOL_SIGNER_SEED,
        &ctx.accounts.mint.key().to_bytes(),
        &[ctx.bumps.pool_signer],
    ];

    invoke_signed(&instruction, &acc_infos, &[signer_seeds])
        .map_err(|_| CctpPoolVerifierError::FailedCctpCpi.into())
}

// This contains the CCTP-specific accounts that are used in the offramp. As they are too many, they don't fit inside
// a single Context struct (as Anchor would require too much memory to validate all of them), so we use a separate struct
// and do the address validations manually.
pub struct TokenOfframpRemainingAccounts<'info> {
    // Accounts that are in the lookup table, as they are static and shared between onramp & offramp
    pub cctp_message_transmitter_account: &'info AccountInfo<'info>,
    pub cctp_token_messenger_minter: &'info AccountInfo<'info>,
    pub system_program: &'info AccountInfo<'info>,
    pub cctp_message_transmitter: &'info AccountInfo<'info>,
    pub cctp_token_messenger_account: &'info AccountInfo<'info>,
    pub cctp_token_minter_account: &'info AccountInfo<'info>,
    pub cctp_local_token: &'info AccountInfo<'info>,
    pub cctp_token_messenger_minter_event_authority: &'info AccountInfo<'info>,

    // Accounts that are not in the lookup table, as they are either dynamic or not shared with onramp
    pub cctp_authority_pda: &'info AccountInfo<'info>,
    pub cctp_event_authority: &'info AccountInfo<'info>,
    pub cctp_custody_token_account: &'info AccountInfo<'info>,
    pub cctp_remote_token_messenger_key: &'info AccountInfo<'info>,
    pub cctp_token_pair: &'info AccountInfo<'info>,
    pub cctp_used_nonces: &'info AccountInfo<'info>,
}

impl TokenOfframpRemainingAccounts<'_> {
    pub fn validate(
        &self,
        mint: Pubkey,
        cctp_msg: &super::cctp_message::CctpMessage,
        domain_id: u32,
        remote_token_address: [u8; 32],
    ) -> Result<()> {
        let nonce_seed = Self::first_nonce_seed(cctp_msg);
        let domain_str = domain_id.to_string();
        let domain_seed = domain_str.as_bytes();

        require_keys_eq!(
            self.cctp_message_transmitter_account.key(),
            get_message_transmitter_pda(&[b"message_transmitter"])
        );

        require_keys_eq!(
            self.cctp_token_messenger_minter.key(),
            TOKEN_MESSENGER_MINTER_V1,
        );

        require_keys_eq!(self.system_program.key(), System::id());

        require_keys_eq!(self.cctp_message_transmitter.key(), MESSAGE_TRANSMITTER_V1);

        require_keys_eq!(
            self.cctp_token_messenger_account.key(),
            get_token_messenger_minter_pda(&[b"token_messenger"])
        );

        require_keys_eq!(
            self.cctp_token_minter_account.key(),
            get_token_messenger_minter_pda(&[b"token_minter"])
        );

        require_keys_eq!(
            self.cctp_local_token.key(),
            get_token_messenger_minter_pda(&[b"local_token", mint.as_ref()])
        );

        require_keys_eq!(
            self.cctp_token_messenger_minter_event_authority.key(),
            get_token_messenger_minter_pda(&[b"__event_authority"])
        );

        require_keys_eq!(
            self.cctp_authority_pda.key(),
            get_message_transmitter_pda(&[
                b"message_transmitter_authority",
                TOKEN_MESSENGER_MINTER_V1.as_ref(),
            ])
        );

        require_keys_eq!(
            self.cctp_event_authority.key(),
            get_message_transmitter_pda(&[b"__event_authority"])
        );

        require_keys_eq!(
            self.cctp_custody_token_account.key(),
            get_token_messenger_minter_pda(&[b"custody", mint.key().as_ref()])
        );

        require_keys_eq!(
            self.cctp_remote_token_messenger_key.key(),
            get_token_messenger_minter_pda(&[b"remote_token_messenger", domain_seed])
        );

        require_keys_eq!(
            self.cctp_token_pair.key(),
            get_token_messenger_minter_pda(&[
                b"token_pair",
                domain_seed,
                remote_token_address.as_ref()
            ])
        );

        require_keys_eq!(
            self.cctp_used_nonces.key(),
            get_message_transmitter_pda(&[b"used_nonces", domain_seed, nonce_seed.as_ref()])
        );

        Ok(())
    }

    pub fn first_nonce_seed(msg: &CctpMessage) -> Vec<u8> {
        msg.first_nonce().to_string().as_bytes().to_vec()
    }
}

// Checks consistency in the data between the CCTP and CCIP messages
fn validate_cctp_and_ccip_messages(
    cctp_msg: &CctpMessage,
    release_or_mint: &ReleaseOrMintInV1,
) -> Result<()> {
    let source_extra_data = TokenPoolExtraData::abi_decode(&release_or_mint.source_pool_data)?;

    require_gte!(
        cctp_msg.data.len(),
        CctpMessage::MINIMUM_LENGTH,
        CctpPoolVerifierError::MalformedCctpV1Message
    );

    require_eq!(
        cctp_msg.version(),
        super::cctp_message::SUPPORTED_CCTP_MESSAGE_VERSION,
        CctpPoolVerifierError::InvalidCctpV1MessageVersion
    );

    require_eq!(
        cctp_msg.source_domain(),
        source_extra_data.source_domain,
        CctpPoolVerifierError::InvalidSourceDomain
    );

    require_eq!(
        cctp_msg.destination_domain(),
        SOLANA_DOMAIN_ID,
        CctpPoolVerifierError::InvalidDestDomain
    );

    require_eq!(
        cctp_msg.nonce(),
        source_extra_data.nonce,
        CctpPoolVerifierError::InvalidNonce
    );

    Ok(())
}

#[derive(AnchorSerialize, AnchorDeserialize, Clone)]
pub struct MessageAndAttestation {
    pub message: CctpMessage,
    pub attestation: Vec<u8>,
}

fn transfer_cctp_tokens(ctx: &Context<TokenOfframp>, amount: u64, decimals: u8) -> Result<()> {
    let seeds = &[
        POOL_SIGNER_SEED,
        &ctx.accounts.mint.key().to_bytes(),
        &[ctx.bumps.pool_signer],
    ];
    let signer_seeds: &[&[&[u8]]] = &[seeds];
    let cpi_ctx = CpiContext::new_with_signer(
        ctx.accounts.token_program.key(),
        anchor_spl::token::TransferChecked {
            from: ctx.accounts.pool_token_account.to_account_info(),
            to: ctx.accounts.receiver_token_account.to_account_info(),
            authority: ctx.accounts.pool_signer.to_account_info(),
            mint: ctx.accounts.mint.to_account_info(),
        },
        signer_seeds,
    );

    anchor_spl::token::transfer_checked(cpi_ctx, amount, decimals)
}
