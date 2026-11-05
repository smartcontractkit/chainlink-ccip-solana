use anchor_lang::prelude::*;
use anchor_spl::associated_token::AssociatedToken;
use anchor_spl::token_interface::{Mint, TokenAccount, TokenInterface};
use base_token_pool::v2::{require_no_nulls_nor_duplicates, SetChainConfigV2Params};
use base_token_pool::{constants::*, errors::*, events::*};
use ccip_common::seed;

use crate::state::*;
use crate::{ChainConfigV2, State};

#[event_cpi]
#[derive(Accounts)]
#[instruction(params: SetChainConfigV2Params)]
pub struct InitChainConfigV2<'info> {
    #[account(
        seeds = [POOL_STATE_SEED, params.mint.as_ref()],
        bump,
        constraint = valid_version(state.version, MAX_POOL_STATE_V) @ CcipTokenPoolError::InvalidVersion,
    )]
    pub state: Account<'info, State>,
    #[account(
        init,
        seeds = [POOL_CHAINCONFIG_SEED_2_0, params.remote_chain_selector.to_le_bytes().as_ref(), state.config.mint.as_ref()],
        bump,
        payer = authority,
        space = ANCHOR_DISCRIMINATOR + ChainConfigV2::INIT_SPACE + ChainConfigV2::dynamic_space(&params),
    )]
    pub chain_config_v2: Account<'info, ChainConfigV2>,
    #[account(mut, address = state.config.owner)]
    pub authority: Signer<'info>,
    pub system_program: Program<'info, System>,

    /// The token this pool wraps. Needed to create the fee vault ATA the lane's in-token BPS fee
    /// accrues into.
    #[account(address = state.config.mint, owner = token_program.key())]
    pub mint: InterfaceAccount<'info, Mint>,

    /// CHECK: PDA that owns the pool's fee vault ATAs, and the authority of the one created here.
    #[account(
        seeds = [seed::FEE_VAULT, mint.key().as_ref()],
        bump,
    )]
    pub fee_vault: UncheckedAccount<'info>,

    /// The fee vault's ATA for the pool's own mint, where `lock_or_burn_tokens_v2` accrues the
    /// in-token BPS fee. Created here so that configuring a
    /// lane and being able to collect (and withdraw) its BPS fee are the same operation.
    #[account(
        init_if_needed,
        payer = authority,
        associated_token::mint = mint,
        associated_token::authority = fee_vault,
        associated_token::token_program = token_program,
    )]
    pub fee_vault_token_account: InterfaceAccount<'info, TokenAccount>,

    pub token_program: Interface<'info, TokenInterface>,
    pub associated_token_program: Program<'info, AssociatedToken>,
}

#[event_cpi]
#[derive(Accounts)]
#[instruction(params: SetChainConfigV2Params)]
pub struct EditChainConfigV2<'info> {
    #[account(
        seeds = [POOL_STATE_SEED, params.mint.as_ref()],
        bump,
        constraint = valid_version(state.version, MAX_POOL_STATE_V) @ CcipTokenPoolError::InvalidVersion,
    )]
    pub state: Account<'info, State>,
    #[account(
        mut,
        seeds = [POOL_CHAINCONFIG_SEED_2_0, params.remote_chain_selector.to_le_bytes().as_ref(), state.config.mint.as_ref()],
        bump = chain_config_v2.bump,
        realloc = ANCHOR_DISCRIMINATOR + ChainConfigV2::INIT_SPACE + ChainConfigV2::dynamic_space(&params),
        realloc::payer = authority,
        realloc::zero = false,
    )]
    pub chain_config_v2: Account<'info, ChainConfigV2>,
    #[account(mut, address = state.config.owner)]
    pub authority: Signer<'info>,
    pub system_program: Program<'info, System>,
}

pub fn init_chain_config_v2(
    ctx: Context<InitChainConfigV2>,
    params: SetChainConfigV2Params,
) -> Result<()> {
    let config = validated_config(&params, ctx.bumps.chain_config_v2)?;
    ctx.accounts.chain_config_v2.set_inner(config);

    emit_cpi!(ConfigV2Updated {
        remote_chain_selector: params.remote_chain_selector,
        mint: params.mint,
        ccv: params.ccv_config,
        allowed_finality: params.allowed_finality_config,
        token_transfer_fee: params.token_transfer_fee_config
    });

    Ok(())
}

pub fn edit_chain_config_v2(
    ctx: Context<EditChainConfigV2>,
    params: SetChainConfigV2Params,
) -> Result<()> {
    let config = validated_config(&params, ctx.accounts.chain_config_v2.bump)?;
    ctx.accounts.chain_config_v2.set_inner(config);

    emit_cpi!(ConfigV2Updated {
        remote_chain_selector: params.remote_chain_selector,
        mint: params.mint,
        ccv: params.ccv_config,
        allowed_finality: params.allowed_finality_config,
        token_transfer_fee: params.token_transfer_fee_config
    });

    Ok(())
}

fn validated_config(params: &SetChainConfigV2Params, bump: u8) -> Result<ChainConfigV2> {
    params.token_transfer_fee_config.validate()?;

    require_valid_threshold(
        &params.ccv_config.threshold_inbound_ccvs,
        &params.ccv_config.inbound_ccvs,
    )?;
    require_valid_threshold(
        &params.ccv_config.threshold_outbound_ccvs,
        &params.ccv_config.outbound_ccvs,
    )?;

    // A non-zero threshold amount is required whenever threshold CCVs are configured; otherwise
    // `get_required_ccvs` would silently never apply them (it gates on `threshold_amount > 0`),
    // making the threshold lists dead config.
    require!(
        params.ccv_config.threshold_amount > 0
            || (params.ccv_config.threshold_outbound_ccvs.is_empty()
                && params.ccv_config.threshold_inbound_ccvs.is_empty()),
        CcipTokenPoolError::ThresholdAmountRequiredForThresholdCcvs
    );

    Ok(ChainConfigV2 {
        bump,
        version: MAX_V2_CONFIG_V,
        ccv_config: params.ccv_config.clone(),
        remote_chain_selector: params.remote_chain_selector,
        mint: params.mint,
        allowed_finality_config: params.allowed_finality_config,
        token_transfer_fee_config: params.token_transfer_fee_config.clone(),
    })
}

fn require_valid_threshold(threshold_ccvs: &[Pubkey], base_ccvs: &[Pubkey]) -> Result<()> {
    if threshold_ccvs.is_empty() {
        return require_no_nulls_nor_duplicates(base_ccvs.iter());
    }

    require!(
        !base_ccvs.is_empty(),
        CcipTokenPoolError::MustSpecifyUnderThresholdCCVsForThresholdCCVs
    );

    require_no_nulls_nor_duplicates(base_ccvs.iter().chain(threshold_ccvs.iter()))
}
