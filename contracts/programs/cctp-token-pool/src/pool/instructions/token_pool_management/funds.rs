use anchor_lang::prelude::instruction::Instruction;
use anchor_lang::prelude::program::invoke_signed;
use anchor_lang::prelude::system_instruction::transfer;
use anchor_lang::prelude::*;
use anchor_spl::token_interface::Mint;

use base_token_pool::constants::{POOL_SIGNER_SEED, POOL_STATE_SEED};
use base_token_pool::errors::CcipTokenPoolError;

use ccip_common::state::AccountSchemaVersion;

use crate::error::CctpPoolVerifierError;
use crate::pool::instructions::MESSAGE_TRANSMITTER_V1;
use crate::pool::state::{FundingConfig, State, MESSAGE_SENT_EVENT_SEED};

const RECLAIM_EVENT_ACCOUNT_DISCRIMINATOR: [u8; 8] = [94, 198, 180, 159, 131, 236, 15, 174]; // global:reclaim_event_account

pub fn set_fund_manager(ctx: Context<SetFundConfig>, fund_manager: Pubkey) -> Result<()> {
    let old = ctx.accounts.state.funding;
    ctx.accounts.state.funding.manager = fund_manager;

    emit!(CcipCctpFundingConfigChanged {
        old,
        new: ctx.accounts.state.funding
    });
    Ok(())
}

pub fn set_minimum_signer_funds(ctx: Context<SetFundConfig>, amount: u64) -> Result<()> {
    let old = ctx.accounts.state.funding;
    ctx.accounts.state.funding.minimum_signer_funds = amount;

    emit!(CcipCctpFundingConfigChanged {
        old,
        new: ctx.accounts.state.funding
    });
    Ok(())
}

pub fn set_fund_reclaim_destination(
    ctx: Context<SetFundConfig>,
    fund_reclaim_destination: Pubkey,
) -> Result<()> {
    let old = ctx.accounts.state.funding;
    ctx.accounts.state.funding.reclaim_destination = fund_reclaim_destination;

    emit!(CcipCctpFundingConfigChanged {
        old,
        new: ctx.accounts.state.funding
    });
    Ok(())
}

#[derive(Accounts)]
pub struct SetFundConfig<'info> {
    #[account(
        mut,
        seeds = [
            POOL_STATE_SEED,
            mint.key().as_ref()
        ],
        bump,
        constraint = state.is_latest_version() @ CcipTokenPoolError::InvalidVersion,
    )]
    pub state: Account<'info, State>,

    pub mint: InterfaceAccount<'info, Mint>, // underlying token that the pool wraps

    #[account(constraint = state.config.owner == authority.key() @ CcipTokenPoolError::Unauthorized)]
    pub authority: Signer<'info>,
}

#[event]
pub struct CcipCctpFundingConfigChanged {
    old: FundingConfig,
    new: FundingConfig,
}

pub fn reclaim_event_account(
    ctx: Context<ReclaimEventAccount>,
    mint: Pubkey,
    original_sender: Pubkey,
    remote_chain_selector: u64,
    msg_nonce: u64,
    attestation: Vec<u8>,
) -> Result<()> {
    let mut instruction_data = Vec::with_capacity(RECLAIM_EVENT_ACCOUNT_DISCRIMINATOR.len());
    instruction_data.extend_from_slice(&RECLAIM_EVENT_ACCOUNT_DISCRIMINATOR);
    borsh::to_writer(&mut instruction_data, &attestation)?;

    let acc_infos = vec![
        ctx.accounts.pool_signer.to_account_info(),
        ctx.accounts.message_transmitter.to_account_info(),
        ctx.accounts.message_sent_event_account.to_account_info(),
    ];

    let acc_metas: Vec<AccountMeta> = acc_infos
        .iter()
        .flat_map(|acc_info| {
            let is_signer = acc_info.key() == ctx.accounts.pool_signer.key();
            acc_info.to_account_metas(Some(is_signer))
        })
        .collect();

    let instruction = Instruction {
        program_id: ctx.accounts.cctp_message_transmitter.key(),
        accounts: acc_metas,
        data: instruction_data,
    };

    let signer_seeds: &[&[u8]] = &[
        POOL_SIGNER_SEED,
        &mint.key().to_bytes(),
        &[ctx.bumps.pool_signer],
    ];

    invoke_signed(&instruction, &acc_infos, &[signer_seeds])?;

    emit!(CcipCctpMessageEventAccountClosed {
        original_sender,
        remote_chain_selector,
        msg_total_nonce: msg_nonce,
        address: ctx.accounts.message_sent_event_account.key()
    });

    Ok(())
}

#[event]
pub struct CcipCctpMessageEventAccountClosed {
    // Seeds for the CCTP message sent event account
    original_sender: Pubkey,
    remote_chain_selector: u64,
    msg_total_nonce: u64,
    // Actual event account address, derived from the seeds above
    address: Pubkey,
}

#[derive(Accounts)]
#[instruction(mint: Pubkey, original_sender: Pubkey, remote_chain_selector: u64, msg_nonce: u64)]
pub struct ReclaimEventAccount<'info> {
    #[account(
        seeds = [
            POOL_STATE_SEED,
            mint.key().as_ref()
        ],
        bump,
        constraint = state.is_latest_version() @ CcipTokenPoolError::InvalidVersion,
    )]
    pub state: Account<'info, State>,

    /// CHECK: CPI signer for CCTP operations, validated by seeds constraint
    #[account(
        mut,
        seeds = [POOL_SIGNER_SEED, mint.key().as_ref()],
        bump,
    )]
    pub pool_signer: UncheckedAccount<'info>,

    /// CHECK: MessageSent event account to reclaim rent from
    #[account(
        mut,
        seeds = [
            MESSAGE_SENT_EVENT_SEED,
            &original_sender.to_bytes(),
            &remote_chain_selector.to_le_bytes(),
            &msg_nonce.to_le_bytes(),
        ],
        bump,
    )]
    pub message_sent_event_account: UncheckedAccount<'info>,

    /// CHECK: CCTP Message Transmitter program, validated by address constraint
    #[account(
        mut,
        seeds = [b"message_transmitter"],
        bump,
        seeds::program = cctp_message_transmitter,
    )]
    pub message_transmitter: UncheckedAccount<'info>,

    /// CHECK: CCTP Message Transmitter program account
    #[account(address = MESSAGE_TRANSMITTER_V1)]
    pub cctp_message_transmitter: UncheckedAccount<'info>,

    #[account(mut, address = state.funding.manager @ CctpPoolVerifierError::InvalidFundManager)]
    pub authority: Signer<'info>,

    pub system_program: Program<'info, System>,
}

pub fn reclaim_funds(ctx: Context<ReclaimFunds>, amount: u64) -> Result<()> {
    require_gt!(amount, 0, CctpPoolVerifierError::InvalidSolAmount);

    let pool_signer_lamports = ctx.accounts.pool_signer.lamports();

    let minimum_required = amount
        .checked_add(ctx.accounts.state.funding.minimum_signer_funds)
        .ok_or(CctpPoolVerifierError::InvalidSolAmount)?;
    require_gte!(
        pool_signer_lamports,
        minimum_required,
        CctpPoolVerifierError::InsufficientFunds
    );

    let mint_key = ctx.accounts.mint.key();
    let signer_seeds: &[&[u8]] = &[
        POOL_SIGNER_SEED,
        mint_key.as_ref(),
        &[ctx.bumps.pool_signer],
    ];

    let transfer_instruction = transfer(
        &ctx.accounts.pool_signer.key(),
        &ctx.accounts.fund_reclaim_destination.key(),
        amount,
    );

    invoke_signed(
        &transfer_instruction,
        &[
            ctx.accounts.pool_signer.to_account_info(),
            ctx.accounts.fund_reclaim_destination.to_account_info(),
        ],
        &[signer_seeds],
    )?;

    Ok(())
}

#[derive(Accounts)]
pub struct ReclaimFunds<'info> {
    #[account(
        seeds = [
            POOL_STATE_SEED,
            mint.key().as_ref()
        ],
        bump,
        constraint = state.is_latest_version() @ CcipTokenPoolError::InvalidVersion,
    )]
    pub state: Account<'info, State>,

    pub mint: InterfaceAccount<'info, Mint>,

    /// CHECK: CPI signer for SOL recovery, validated by seeds constraint
    #[account(
        mut,
        seeds = [POOL_SIGNER_SEED, mint.key().as_ref()],
        address = state.config.pool_signer,
        bump,
    )]
    pub pool_signer: UncheckedAccount<'info>,

    /// CHECK: Destination account to receive SOL. Preconfigured by the owner
    /// to be a particular fund reclaimer
    #[account(
        mut,
        address = state.funding.reclaim_destination @ CctpPoolVerifierError::InvalidReclaimDestination
    )]
    pub fund_reclaim_destination: UncheckedAccount<'info>,

    #[account(mut, constraint = authority.key() == state.funding.manager @ CctpPoolVerifierError::InvalidFundManager)]
    pub authority: Signer<'info>,

    pub system_program: Program<'info, System>,
}
