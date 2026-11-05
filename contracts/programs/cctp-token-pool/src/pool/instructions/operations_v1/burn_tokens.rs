use anchor_lang::prelude::instruction::Instruction;
use anchor_lang::prelude::program::get_return_data;
use anchor_lang::prelude::*;
use anchor_lang::solana_program::program::invoke_signed;

use anchor_spl::token_interface::{Mint, TokenAccount, TokenInterface};

use base_token_pool::{constants::*, errors::*, events::*, v1::*};
use ccip_common::seed;
use ccip_common::state::AccountSchemaVersion;

use crate::error::CctpPoolVerifierError;
use crate::pool::instructions::{MESSAGE_TRANSMITTER_V1, TOKEN_MESSENGER_MINTER_V1};
use crate::pool::state::{ChainConfig, State, MESSAGE_SENT_EVENT_SEED};
use crate::SOLANA_DOMAIN_ID;

use super::token_pool_extra_data::*;

pub const DEPOSIT_FOR_BURN_WITH_CALLER_DISCRIMINATOR: [u8; 8] =
    [167, 222, 19, 114, 85, 21, 14, 118]; // global:deposit_for_burn_with_caller

pub fn burn_tokens(
    ctx: Context<TokenOnramp>,
    lock_or_burn: LockOrBurnInV1,
) -> Result<LockOrBurnOutV1> {
    validate_lock_or_burn(
        &lock_or_burn,
        ctx.accounts.state.config.mint,
        &mut ctx.accounts.chain_config.base.outbound_rate_limit,
        ctx.accounts.state.config.list_enabled,
        &ctx.accounts.state.config.allow_list,
    )?;

    require_eq!(
        lock_or_burn.receiver.len(),
        32,
        CctpPoolVerifierError::InvalidReceiver
    );

    let cctp_nonce = cctp_deposit_for_burn_with_caller(&ctx, &lock_or_burn)?;

    let cctp_event_data = ctx.accounts.cctp_message_sent_event.try_borrow_data()?;
    // The first 8 bytes are the discriminator, so we skip them
    // Then, the next 32 bytes are the rent payer, which we also skip
    let cctp_message_bytes_len = &cctp_event_data[40..44]; // the next 4 bytes encode the message length
    let cctp_message_bytes = &cctp_event_data[44..]; // then, the rest is the message bytes
    require_eq!(
        cctp_message_bytes.len(),
        u32::from_le_bytes(
            cctp_message_bytes_len
                .try_into()
                .expect("slice [40..44] is always exactly 4 bytes")
        ) as usize,
        CctpPoolVerifierError::InvalidMessageSentEventAccount
    );

    // This event is specific to this CCTP Token Pool program
    emit!(CcipCctpMessageSentEvent {
        original_sender: lock_or_burn.original_sender,
        remote_chain_selector: lock_or_burn.remote_chain_selector,
        msg_total_nonce: lock_or_burn.msg_total_nonce,
        event_address: ctx.accounts.cctp_message_sent_event.key(),
        message_sent_bytes: cctp_message_bytes.to_vec(),
        source_domain: SOLANA_DOMAIN_ID,
        cctp_nonce,
    });

    // This event is standardized with the BurnmintTokenPool program
    emit!(Burned {
        sender: ctx.accounts.authority.key(),
        amount: lock_or_burn.amount,
        mint: ctx.accounts.state.config.mint,
    });

    let extra_data = TokenPoolExtraData {
        nonce: cctp_nonce,
        source_domain: SOLANA_DOMAIN_ID,
    };

    Ok(LockOrBurnOutV1 {
        dest_token_address: ctx.accounts.chain_config.base.remote.token_address.clone(),
        // The dest_pool_data is then read by the remote pool, so we standardize on ABI-encoding
        dest_pool_data: extra_data.abi_encode().to_vec(),
    })
}

#[derive(Accounts)]
#[instruction(lock_or_burn: LockOrBurnInV1)]
pub struct TokenOnramp<'info> {
    // CCIP accounts ------------------------
    #[account(address = state.config.router_onramp_authority @ CcipTokenPoolError::InvalidPoolCaller)]
    pub authority: Signer<'info>,

    // Token pool accounts ------------------
    // consistent set + token pool program
    #[account(
        seeds = [POOL_STATE_SEED, mint.key().as_ref()],
        bump,
        constraint = state.is_latest_version() @ CcipTokenPoolError::InvalidVersion,
    )]
    pub state: Box<Account<'info, State>>,

    pub token_program: Interface<'info, TokenInterface>,

    #[account(mut)]
    pub mint: InterfaceAccount<'info, Mint>,

    /// CHECK: CPI signer. This account is intentionally not initialized, and it will
    /// hold a balance to pay for the rent of initializing the CCTP MessageSentEvent account
    #[account(
        mut,
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

    ////////////////////
    // RMN Remote CPI //
    ////////////////////
    /// CHECK: RMNRemote program, invoked to check for curses
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
        seeds = [
            POOL_CHAINCONFIG_SEED,
            lock_or_burn.remote_chain_selector.to_le_bytes().as_ref(),
            mint.key().as_ref()
        ],
        bump,
        constraint = chain_config.is_latest_version() @ CcipTokenPoolError::InvalidVersion,
    )]
    pub chain_config: Box<Account<'info, ChainConfig>>,

    // CCTP pool-specific accounts in the lookup table ----------------
    /// CHECK this is not read by the pool, just forwarded to CCTP
    #[account(
        mut,
        seeds = [b"message_transmitter"],
        bump,
        seeds::program = cctp_message_transmitter,
    )]
    pub cctp_message_transmitter_account: UncheckedAccount<'info>,

    /// CHECK this is CCTP's TokenMessengerMinter program, which
    /// is invoked by this program.
    #[account(
        address = TOKEN_MESSENGER_MINTER_V1 @ CctpPoolVerifierError::InvalidTokenMessengerMinter
    )]
    pub cctp_token_messenger_minter: UncheckedAccount<'info>,

    pub system_program: Program<'info, System>,

    /// CHECK this is CCTP's MessageTransmitter program, which
    /// is invoked transitively by CCTP's TokenMessengerMinter,
    /// which in turn is invoked explicitly by this program.
    #[account(
        address = MESSAGE_TRANSMITTER_V1 @ CctpPoolVerifierError::InvalidMessageTransmitter
    )]
    pub cctp_message_transmitter: UncheckedAccount<'info>,

    /// CHECK this is not read by the pool, just forwarded to CCTP
    #[account(
        seeds = [b"token_messenger"],
        bump,
        seeds::program = cctp_token_messenger_minter,
    )]
    pub cctp_token_messenger_account: UncheckedAccount<'info>,

    /// CHECK this is not read by the pool, just forwarded to CCTP
    #[account(
        seeds = [b"token_minter"],
        bump,
        seeds::program = cctp_token_messenger_minter,
    )]
    pub cctp_token_minter_account: UncheckedAccount<'info>,

    /// CHECK this is not read by the pool, just forwarded to CCTP
    #[account(
        mut,
        seeds = [b"local_token", mint.key().as_ref()],
        bump,
        seeds::program = cctp_token_messenger_minter,
    )]
    pub cctp_local_token: UncheckedAccount<'info>,

    /// CHECK this is not read by the pool, just forwarded to CCTP
    #[account(
        seeds = [b"__event_authority"],
        bump,
        seeds::program = cctp_token_messenger_minter,
    )]
    pub cctp_event_authority: UncheckedAccount<'info>,

    // CCTP pool-specific accounts NOT in the lookup table ----------------
    /// CHECK this is not read by the pool, just forwarded to CCTP
    #[account(
        seeds = [b"sender_authority"],
        bump,
        seeds::program = cctp_token_messenger_minter,
    )]
    pub cctp_authority_pda: UncheckedAccount<'info>,

    /// CHECK this is not read by the pool, just forwarded to CCTP
    #[account(
        seeds = [b"remote_token_messenger", chain_config.cctp.domain_id.to_string().as_bytes()],
        bump,
        seeds::program = cctp_token_messenger_minter,
    )]
    pub cctp_remote_token_messenger_key: UncheckedAccount<'info>,

    /// CHECK this is the account in which CCTP will store the event. It is not a PDA of CCTP,
    /// but CCTP will initialize it and become the owner for it.
    #[account(
        mut,
        seeds = [
            MESSAGE_SENT_EVENT_SEED,
            &lock_or_burn.original_sender.to_bytes(),
            &lock_or_burn.remote_chain_selector.to_le_bytes(),
            &lock_or_burn.msg_total_nonce.to_le_bytes(),
        ],
        bump,
        owner = System::id(), // this is not initialized yet, it will later be owned by CCTP
    )]
    pub cctp_message_sent_event: UncheckedAccount<'info>,
}

fn cctp_deposit_for_burn_with_caller(
    ctx: &Context<TokenOnramp>,
    lock_or_burn: &LockOrBurnInV1,
) -> Result<u64> {
    require_gte!(
        lock_or_burn.receiver.len(),
        32,
        CctpPoolVerifierError::InvalidReceiver
    );
    let mint_recipient = Pubkey::new_from_array(
        lock_or_burn.receiver[0..32]
            .try_into()
            .map_err(|_| error!(CctpPoolVerifierError::InvalidReceiver))?,
    );

    // A lane whose CCTP config was never set still has a zero destination caller.
    require_keys_neq!(
        ctx.accounts.chain_config.cctp.destination_caller,
        Pubkey::default(),
        CctpPoolVerifierError::UnknownDomain
    );
    let destination_domain = ctx.accounts.chain_config.cctp.domain_id;

    let deposit_for_burn_params = DepositForBurnWithCallerParams {
        amount: lock_or_burn.amount,
        destination_domain,
        mint_recipient,
        destination_caller: ctx.accounts.chain_config.cctp.destination_caller,
    };

    let mut instruction_data = Vec::with_capacity(8);
    instruction_data.extend_from_slice(&DEPOSIT_FOR_BURN_WITH_CALLER_DISCRIMINATOR);
    borsh::to_writer(&mut instruction_data, &deposit_for_burn_params)?;

    let acc_infos = vec![
        ctx.accounts.pool_signer.to_account_info(),
        ctx.accounts.pool_signer.to_account_info(),
        ctx.accounts.cctp_authority_pda.to_account_info(),
        ctx.accounts.pool_token_account.to_account_info(),
        ctx.accounts
            .cctp_message_transmitter_account
            .to_account_info(),
        ctx.accounts.cctp_token_messenger_account.to_account_info(),
        ctx.accounts
            .cctp_remote_token_messenger_key
            .to_account_info(),
        ctx.accounts.cctp_token_minter_account.to_account_info(),
        ctx.accounts.cctp_local_token.to_account_info(),
        ctx.accounts.mint.to_account_info(),
        ctx.accounts.cctp_message_sent_event.to_account_info(),
        ctx.accounts.cctp_message_transmitter.to_account_info(),
        ctx.accounts.cctp_token_messenger_minter.to_account_info(),
        ctx.accounts.token_program.to_account_info(),
        ctx.accounts.system_program.to_account_info(),
        ctx.accounts.cctp_event_authority.to_account_info(),
        ctx.accounts.cctp_token_messenger_minter.to_account_info(),
    ];

    let signer_keys = [
        ctx.accounts.pool_signer.key(),
        ctx.accounts.cctp_message_sent_event.key(),
    ];

    let acc_metas: Vec<AccountMeta> = acc_infos
        .iter()
        .flat_map(|acc_info| {
            let is_signer = signer_keys.contains(acc_info.key);
            acc_info.to_account_metas(Some(is_signer))
        })
        .collect();

    let instruction = Instruction {
        program_id: ctx.accounts.cctp_token_messenger_minter.key(),
        accounts: acc_metas,
        data: instruction_data,
    };

    let pool_signer_seeds: &[&[u8]] = &[
        POOL_SIGNER_SEED,
        &ctx.accounts.mint.key().to_bytes(),
        &[ctx.bumps.pool_signer],
    ];

    let message_sent_event_seeds: &[&[u8]] = &[
        MESSAGE_SENT_EVENT_SEED,
        &lock_or_burn.original_sender.to_bytes(),
        &lock_or_burn.remote_chain_selector.to_le_bytes(),
        &lock_or_burn.msg_total_nonce.to_le_bytes(),
        &[ctx.bumps.cctp_message_sent_event],
    ];

    invoke_signed(
        &instruction,
        &acc_infos,
        &[pool_signer_seeds, message_sent_event_seeds],
    )?;

    // Solana's get_return_data() returns data from whichever program last called
    // set_return_data() in the call tree, so we must verify both that return data
    // exists and that it came from the expected CCTP program.
    let (returning_program, cctp_nonce_bytes) =
        get_return_data().ok_or(CctpPoolVerifierError::MissingCpiReturnData)?;
    require_keys_eq!(
        returning_program,
        ctx.accounts.cctp_token_messenger_minter.key(),
        CctpPoolVerifierError::InvalidCpiReturnProgram
    );
    let nonce_bytes: [u8; 8] = cctp_nonce_bytes
        .as_slice()
        .try_into()
        .map_err(|_| CctpPoolVerifierError::InvalidCpiReturnDataLength)?;
    Ok(u64::from_le_bytes(nonce_bytes))
}

#[derive(AnchorSerialize, AnchorDeserialize, Clone)]
pub struct DepositForBurnWithCallerParams {
    pub amount: u64,
    pub destination_domain: u32,
    pub mint_recipient: Pubkey,
    pub destination_caller: Pubkey,
}

#[event]
pub struct CcipCctpMessageSentEvent {
    // Seeds for the CCTP message sent event account
    pub original_sender: Pubkey,
    pub remote_chain_selector: u64,
    pub msg_total_nonce: u64,

    // Actual event account address, derived from the seeds above
    pub event_address: Pubkey,

    // CCTP values identifying the message
    pub source_domain: u32, // The source chain domain ID, which for Solana is always 5
    pub cctp_nonce: u64,

    // CCTP message bytes, used to get the attestation offchain and receive the message on dest
    pub message_sent_bytes: Vec<u8>,
}
