use anchor_lang::prelude::*;
use anchor_spl::token_interface::{Mint, TokenAccount};
use ccip_common::seed;

use crate::error::CommitteeVerifierError;
use crate::program::CommitteeVerifier;
use crate::state::{valid_version, Config, StorageLocations, ANCHOR_DISCRIMINATOR, MAX_CONFIG_V};

#[derive(Accounts)]
pub struct InitializeConfig<'info> {
    #[account(
        init,
        seeds = [seed::CONFIG],
        bump,
        payer = authority,
        space = ANCHOR_DISCRIMINATOR + Config::INIT_SPACE,
    )]
    pub config: Account<'info, Config>,

    #[account(
        init,
        seeds = [seed::STORAGE_LOCATIONS],
        bump,
        payer = authority,
        space = ANCHOR_DISCRIMINATOR + StorageLocations::INIT_SPACE,
    )]
    pub storage_locations: Account<'info, StorageLocations>,

    #[account(mut)]
    pub authority: Signer<'info>,

    pub system_program: Program<'info, System>,

    #[account(constraint = program.programdata_address()? == Some(program_data.key()))]
    pub program: Program<'info, CommitteeVerifier>,

    #[account(constraint = program_data.upgrade_authority_address == Some(authority.key()) @ CommitteeVerifierError::Unauthorized)]
    pub program_data: Account<'info, ProgramData>,
}

#[event_cpi]
#[derive(Accounts)]
pub struct UpdateConfig<'info> {
    #[account(
        mut,
        seeds = [seed::CONFIG],
        bump = config.bump,
        constraint = valid_version(config.version, MAX_CONFIG_V) @ CommitteeVerifierError::InvalidVersion,
    )]
    pub config: Account<'info, Config>,

    #[account(address = config.owner @ CommitteeVerifierError::Unauthorized)]
    pub authority: Signer<'info>,
}

#[derive(Accounts)]
pub struct WithdrawFeeTokens<'info> {
    #[account(
        seeds = [seed::CONFIG],
        bump = config.bump,
        constraint = valid_version(config.version, MAX_CONFIG_V) @ CommitteeVerifierError::InvalidVersion,
    )]
    pub config: Account<'info, Config>,

    /// The account the router pays this verifier's fees into: the ATA of `fee_vault` for the fee
    /// token. There is one per fee token, since the sender picks which token to bill in.
    #[account(
        mut,
        associated_token::mint = fee_token_mint,
        associated_token::authority = fee_vault,
        associated_token::token_program = token_program,
    )]
    pub fee_vault_token_account: InterfaceAccount<'info, TokenAccount>,

    /// CHECK: PDA that owns the fee vault ATA, and therefore the CPI signer for the withdrawal.
    /// The router derives the destination of every CCV receipt fee from these exact seeds (see
    /// `ccip_common::seed::vault_ata`), so this is the only authority that can move the accrued
    /// fees out.
    #[account(
        seeds = [seed::FEE_VAULT],
        bump,
    )]
    pub fee_vault: UncheckedAccount<'info>,

    #[account(
        mut,
        associated_token::mint = fee_token_mint,
        associated_token::token_program = token_program,
        associated_token::authority = config.fee_aggregator,
    )]
    pub fee_aggregator_token_account: InterfaceAccount<'info, TokenAccount>,

    #[account(owner = token_program.key())]
    pub fee_token_mint: InterfaceAccount<'info, Mint>,

    /// CHECK: CPI to token program
    pub token_program: Program<'info>,
}
