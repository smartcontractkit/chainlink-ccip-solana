use anchor_lang::prelude::*;
use base_token_pool::v2::{require_no_nulls_nor_duplicates, SetChainConfigV2Params};
use base_token_pool::{constants::*, errors::*, events::*};

use crate::error::CctpPoolVerifierError;
use crate::pool::state::*;

#[event_cpi]
#[derive(Accounts)]
#[instruction(params: SetChainConfigV2Params)]
pub struct InitChainConfigV2<'info> {
    #[account(
        seeds = [POOL_STATE_SEED, params.mint.as_ref()],
        bump,
        constraint = state.is_latest_version() @ CcipTokenPoolError::InvalidVersion,
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
}

#[event_cpi]
#[derive(Accounts)]
#[instruction(params: SetChainConfigV2Params)]
pub struct EditChainConfigV2<'info> {
    #[account(
        seeds = [POOL_STATE_SEED, params.mint.as_ref()],
        bump,
        constraint = state.is_latest_version() @ CcipTokenPoolError::InvalidVersion,
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
    {
        // CCTP-specific checks (not present in most other pools). We don't support BPS fees in this pool,
        // as there is no business need for them, though we still keep a lot of the bps code around for
        // similarity with other pool types.
        require_eq!(
            params.token_transfer_fee_config.finality_bps_fee,
            0,
            CctpPoolVerifierError::UnsupportedBpsFee
        );
        require_eq!(
            params.token_transfer_fee_config.fast_finality_bps_fee,
            0,
            CctpPoolVerifierError::UnsupportedBpsFee
        );
    }

    params.token_transfer_fee_config.validate()?;

    require_valid(
        &params.ccv_config.threshold_inbound_ccvs,
        &params.ccv_config.inbound_ccvs,
    )?;
    require_valid(
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
        version: ChainConfigV2::MAX_V,
        ccv_config: params.ccv_config.clone(),
        remote_chain_selector: params.remote_chain_selector,
        mint: params.mint,
        allowed_finality_config: params.allowed_finality_config,
        token_transfer_fee_config: params.token_transfer_fee_config.clone(),
    })
}

fn require_valid(threshold_ccvs: &[Pubkey], base_ccvs: &[Pubkey]) -> Result<()> {
    require!(
        base_ccvs.contains(&crate::ID),
        CctpPoolVerifierError::MissingConfigurationRequiredCctpCcv
    );

    if threshold_ccvs.is_empty() {
        return require_no_nulls_nor_duplicates(base_ccvs.iter());
    }

    require!(
        !base_ccvs.is_empty(),
        CcipTokenPoolError::MustSpecifyUnderThresholdCCVsForThresholdCCVs
    );

    require_no_nulls_nor_duplicates(base_ccvs.iter().chain(threshold_ccvs.iter()))
}
