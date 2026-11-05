//! Admin instructions for the pool's per-(mint, remote chain) v2 lane config (`ChainConfigV2`):
//! allowed finality, token transfer fee, and required CCVs. Mirrors `burnmint-token-pool`'s own
//! `init_chain_config_v2`/`edit_chain_config_v2` - same shared `SetChainConfigV2Params`, same PDA
//! seeds, same validation (for the most part, with the exception of bps fees).

use anchor_lang::prelude::*;
use base_token_pool::constants::{valid_version, ANCHOR_DISCRIMINATOR, POOL_STATE_SEED};
use base_token_pool::errors::CcipTokenPoolError;
use base_token_pool::events::ConfigV2Updated;
use base_token_pool::v2::{require_no_nulls_nor_duplicates, SetChainConfigV2Params};

use crate::constants::{MAX_POOL_STATE_V, POOL_CHAINCONFIG_SEED_2_0};
use crate::errors::LombardTokenPoolError;
use crate::state::{ChainConfigV2, State, MAX_CHAIN_CONFIG_V2_V};

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
        space = ANCHOR_DISCRIMINATOR + ChainConfigV2::INIT_SPACE + ChainConfigV2::dynamic_space(&params.ccv_config),
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
        constraint = valid_version(state.version, MAX_POOL_STATE_V) @ CcipTokenPoolError::InvalidVersion,
    )]
    pub state: Account<'info, State>,

    #[account(
        mut,
        seeds = [POOL_CHAINCONFIG_SEED_2_0, params.remote_chain_selector.to_le_bytes().as_ref(), state.config.mint.as_ref()],
        bump = chain_config_v2.bump,
        realloc = ANCHOR_DISCRIMINATOR + ChainConfigV2::INIT_SPACE + ChainConfigV2::dynamic_space(&params.ccv_config),
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
        token_transfer_fee: params.token_transfer_fee_config,
    });

    Ok(())
}

pub fn edit_chain_config_v2(
    ctx: Context<EditChainConfigV2>,
    params: SetChainConfigV2Params,
) -> Result<()> {
    // The account constraint already verified this stored bump against the PDA seeds.
    let config = validated_config(&params, ctx.accounts.chain_config_v2.bump)?;
    ctx.accounts.chain_config_v2.set_inner(config);

    emit_cpi!(ConfigV2Updated {
        remote_chain_selector: params.remote_chain_selector,
        mint: params.mint,
        ccv: params.ccv_config,
        allowed_finality: params.allowed_finality_config,
        token_transfer_fee: params.token_transfer_fee_config,
    });

    Ok(())
}

fn validated_config(params: &SetChainConfigV2Params, bump: u8) -> Result<ChainConfigV2> {
    {
        // Lombard-specific checks (not present in most other pools). This pool doesn't support BPS fees,
        // though it still keeps a lot of the bps code around for similarity with other pool types and for potential
        // future use. If BPS support is ever desired, these checks should be removed, and proper support should be
        // implemented then to accumulate those fees in a pool-owned token account and withdraw them later
        require_eq!(
            params.token_transfer_fee_config.finality_bps_fee,
            0,
            LombardTokenPoolError::UnsupportedBpsFee
        );
        require_eq!(
            params.token_transfer_fee_config.fast_finality_bps_fee,
            0,
            LombardTokenPoolError::UnsupportedBpsFee
        );
    }

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
        version: MAX_CHAIN_CONFIG_V2_V,
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

#[cfg(test)]
mod tests {
    use super::*;
    use base_token_pool::state::{CCVConfig, TokenTransferFeeConfig};
    use ccip_common::messages::FinalityConfig;

    #[test]
    fn validated_config_is_loadable_by_try_load() {
        let params = SetChainConfigV2Params {
            mint: Pubkey::new_unique(),
            remote_chain_selector: 1,
            allowed_finality_config: FinalityConfig::default(),
            token_transfer_fee_config: TokenTransferFeeConfig::default(),
            ccv_config: CCVConfig {
                threshold_amount: 0,
                outbound_ccvs: vec![],
                threshold_outbound_ccvs: vec![],
                inbound_ccvs: vec![],
                threshold_inbound_ccvs: vec![],
            },
        };
        let config = validated_config(&params, 255).unwrap();
        assert_eq!(config.version, MAX_CHAIN_CONFIG_V2_V);

        let mut data = Vec::new();
        config.try_serialize(&mut data).unwrap();
        let key = Pubkey::new_unique();
        let owner = crate::ID;
        let mut lamports = 0;
        let account_info =
            AccountInfo::new(&key, false, false, &mut lamports, &mut data, &owner, false);
        assert!(ChainConfigV2::try_load(&account_info).unwrap().is_some());
    }
}
