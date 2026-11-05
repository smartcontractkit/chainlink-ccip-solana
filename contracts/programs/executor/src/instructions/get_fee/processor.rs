use anchor_discriminator::anchor_discriminator;
use anchor_lang::prelude::*;

use anchor_spl::token_interface::{Mint, TokenAccount};
use ccip_common::messages::FinalityConfig;
use ccip_common::seed;
use ccip_common::types::{ToTxData, UsdCents};

use crate::error::ExecutorError;
use crate::state::{valid_version, ChainConfig, Config, MAX_CHAIN_CONFIG_V, MAX_CONFIG_V};

#[derive(AnchorSerialize, AnchorDeserialize, Debug)]
pub struct GetFeeParams {
    pub dest_chain_selector: u64,
    pub requested_finality: FinalityConfig,
    pub ccvs: Vec<Pubkey>,
    pub extra_args: Vec<u8>,
    pub fee_token: Pubkey,
}

impl ToTxData for GetFeeParams {
    fn to_tx_data(&self) -> Vec<u8> {
        let mut data = Vec::with_capacity(8);
        data.extend_from_slice(&anchor_discriminator!("get_fee"));
        borsh::to_writer(&mut data, &self).unwrap();
        data
    }
}

pub fn get_fee(ctx: Context<GetFee>, params: GetFeeParams) -> Result<UsdCents> {
    require!(
        ctx.accounts.chain_config.enabled,
        ExecutorError::InvalidDestChain
    );

    let finality_allowed = ctx
        .accounts
        .config
        .allowed_finality_config
        .ensure_requested_finality_allowed(params.requested_finality);
    require!(finality_allowed, ExecutorError::InvalidFinality);

    if ctx.accounts.config.ccv_allowlist_enabled {
        for ccv in &params.ccvs {
            ctx.accounts
                .config
                .allowed_ccvs
                .binary_search(ccv)
                .map_err(|_| ExecutorError::InvalidCCV)?;
        }
    }

    require_gte!(
        ctx.accounts.config.max_ccvs_per_msg as usize,
        params.ccvs.len(),
        ExecutorError::ExceedsMaxCCVs
    );

    Ok(ctx.accounts.chain_config.usd_cents_fee)
}

#[event_cpi]
#[derive(Accounts)]
#[instruction(params: GetFeeParams)]
pub struct GetFee<'info> {
    #[account(
        seeds = [seed::CONFIG],
        bump = config.bump,
        constraint = valid_version(config.version, MAX_CONFIG_V) @ ExecutorError::InvalidVersion,
    )]
    pub config: Account<'info, Config>,

    #[account(
        seeds = [seed::REMOTE_CHAIN, &params.dest_chain_selector.to_le_bytes()],
        bump = chain_config.bump,
        constraint = valid_version(chain_config.version, MAX_CHAIN_CONFIG_V) @ ExecutorError::InvalidVersion,
    )]
    pub chain_config: Account<'info, ChainConfig>,
}

#[derive(Accounts)]
pub struct WithdrawFeeTokens<'info> {
    #[account(
        seeds = [seed::CONFIG],
        bump = config.bump,
        constraint = valid_version(config.version, MAX_CONFIG_V) @ ExecutorError::InvalidVersion,
    )]
    pub config: Account<'info, Config>,

    /// The account the router pays this executor's fees into: the ATA of `fee_vault` for the fee
    /// token. There is one per fee token, since the sender picks which token to bill in.
    #[account(
        mut,
        associated_token::mint = fee_token_mint,
        associated_token::authority = fee_vault,
        associated_token::token_program = fee_token_program,
    )]
    pub fee_vault_token_account: InterfaceAccount<'info, TokenAccount>,

    /// CHECK: PDA that owns the fee vault ATA, and therefore the CPI signer for the withdrawal.
    /// The router derives the destination of every executor receipt fee from these exact seeds
    /// (see `ccip_common::seed::vault_ata`), so this is the only authority that can move the
    /// accrued fees out.
    #[account(
        seeds = [seed::FEE_VAULT],
        bump,
    )]
    pub fee_vault: UncheckedAccount<'info>,

    #[account(
        mut,
        associated_token::mint = fee_token_mint,
        associated_token::token_program = fee_token_program,
        associated_token::authority = config.fee_aggregator,
    )]
    pub fee_aggregator_token_account: InterfaceAccount<'info, TokenAccount>,

    #[account(owner = fee_token_program.key())]
    pub fee_token_mint: InterfaceAccount<'info, Mint>,

    /// CHECK: CPI to token program
    pub fee_token_program: Program<'info>,
}
