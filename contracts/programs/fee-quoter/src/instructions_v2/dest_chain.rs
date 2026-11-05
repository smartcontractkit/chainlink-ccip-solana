use anchor_lang::prelude::*;

use ccip_common::seed;
use ccip_common::types::{CrossChainGas, TimestampedPackedU224};
use ccip_common::{
    CommonCcipError, CHAIN_FAMILY_SELECTOR_APTOS, CHAIN_FAMILY_SELECTOR_EVM,
    CHAIN_FAMILY_SELECTOR_SUI, CHAIN_FAMILY_SELECTOR_SVM, CHAIN_FAMILY_SELECTOR_TVM,
};

use crate::context::{uninitialized, valid_version, ANCHOR_DISCRIMINATOR};
use crate::event::{DestChainAddedV2, DestChainConfigDisabledV2, DestChainConfigUpdatedV2};
use crate::instructions_v2::{V2_MAX_CHAINSTATE_V, V2_MAX_CONFIG_V};
use crate::state::{CodeVersion, Config};
use crate::state_v2::{DestChainConfigV2, DestChainStateV2, DestChainV2};
use crate::FeeQuoterError;

#[derive(AnchorSerialize, AnchorDeserialize, Clone)]
pub struct UpsertDestChainConfigV2Params {
    pub chain_selector: u64,
    pub dest_chain_config: DestChainConfigV2,
}

pub fn upsert_dest_chain_config_v2(
    ctx: Context<UpsertDestChainConfigV2>,
    params: UpsertDestChainConfigV2Params,
) -> Result<()> {
    let is_new_dest_chain = ctx.accounts.dest_chain.chain_selector == 0;

    if is_new_dest_chain {
        // Init: PDA was just created by init_if_needed
        validate_dest_chain_config_v2(params.chain_selector, &params.dest_chain_config)?;
        ctx.accounts.dest_chain.set_inner(DestChainV2 {
            version: V2_MAX_CHAINSTATE_V,
            bump: ctx.bumps.dest_chain,
            chain_selector: params.chain_selector,
            state: DestChainStateV2 {
                _usd_per_unit_gas: TimestampedPackedU224 {
                    timestamp: 0,
                    value: [0; 28],
                },
            },
            config: params.dest_chain_config.clone(),
        });
        emit_cpi!(DestChainAddedV2 {
            dest_chain_selector: params.chain_selector,
            dest_chain_config: params.dest_chain_config,
        });
    } else {
        // Update: validate and replace full config
        validate_dest_chain_config_v2(params.chain_selector, &params.dest_chain_config)?;
        ctx.accounts.dest_chain.config = params.dest_chain_config.clone();
        emit_cpi!(DestChainConfigUpdatedV2 {
            dest_chain_selector: params.chain_selector,
            dest_chain_config: params.dest_chain_config,
        });
    }

    Ok(())
}

pub fn disable_dest_chain_v2(
    ctx: Context<DisableDestChainConfigV2>,
    chain_selector: u64,
) -> Result<()> {
    ctx.accounts.dest_chain.config.is_enabled = false;
    emit_cpi!(DestChainConfigDisabledV2 {
        dest_chain_selector: chain_selector,
        dest_chain_config: ctx.accounts.dest_chain.config.clone(),
    });
    Ok(())
}

fn validate_dest_chain_config_v2(
    dest_chain_selector: u64,
    config: &DestChainConfigV2,
) -> Result<()> {
    require!(
        matches!(
            config.lane_code_version,
            CodeVersion::Default | CodeVersion::V1
        ),
        FeeQuoterError::InvalidVersion
    );
    require_neq!(
        dest_chain_selector,
        0,
        FeeQuoterError::InvalidInputsChainSelector
    );
    require_neq!(
        config.default_tx_gas_limit,
        CrossChainGas::ZERO,
        FeeQuoterError::ZeroGasLimit
    );
    require_gte!(
        config.max_per_msg_gas_limit,
        config.default_tx_gas_limit,
        FeeQuoterError::DefaultGasLimitExceedsMaximum
    );
    require_neq!(
        config.link_fee_multiplier_percent,
        0,
        FeeQuoterError::ZeroLinkFeeMultiplier
    );
    // Matches EVM, where this field is a uint8.
    require_gte!(
        u32::from(u8::MAX),
        config.dest_gas_per_payload_byte_base,
        FeeQuoterError::DestGasPerPayloadByteBaseTooLarge
    );
    // Matches EVM, where this field is a uint16.
    require_gte!(
        u32::from(u16::MAX),
        config.network_fee.convert::<u32>(),
        FeeQuoterError::NetworkFeeTooLarge
    );
    require!(
        matches!(
            u32::from_be_bytes(config.chain_family_selector),
            CHAIN_FAMILY_SELECTOR_APTOS
                | CHAIN_FAMILY_SELECTOR_EVM
                | CHAIN_FAMILY_SELECTOR_SUI
                | CHAIN_FAMILY_SELECTOR_SVM
                | CHAIN_FAMILY_SELECTOR_TVM
        ),
        CommonCcipError::InvalidChainFamilySelector
    );
    Ok(())
}

#[event_cpi]
#[derive(Accounts)]
#[instruction(params: UpsertDestChainConfigV2Params)]
pub struct UpsertDestChainConfigV2<'info> {
    #[account(
        seeds = [seed::CONFIG],
        bump,
        constraint = valid_version(config.version, V2_MAX_CONFIG_V) @ FeeQuoterError::InvalidVersion,
    )]
    pub config: Account<'info, Config>,

    #[account(
        init_if_needed,
        payer = authority,
        space = ANCHOR_DISCRIMINATOR + DestChainV2::INIT_SPACE,
        seeds = [seed::DEST_CHAIN_V2, params.chain_selector.to_le_bytes().as_ref()],
        bump,
        constraint = uninitialized(dest_chain.version) || valid_version(dest_chain.version, V2_MAX_CHAINSTATE_V) @ FeeQuoterError::InvalidVersion,
    )]
    pub dest_chain: Account<'info, DestChainV2>,

    // Only the registered admin can perform this action
    #[account(mut, address = config.owner @ FeeQuoterError::Unauthorized)]
    pub authority: Signer<'info>,

    pub system_program: Program<'info, System>,
}

#[event_cpi]
#[derive(Accounts)]
#[instruction(chain_selector: u64)]
pub struct DisableDestChainConfigV2<'info> {
    #[account(
        seeds = [seed::CONFIG],
        bump,
        constraint = valid_version(config.version, V2_MAX_CONFIG_V) @ FeeQuoterError::InvalidVersion,
    )]
    pub config: Account<'info, Config>,

    #[account(
        mut,
        seeds = [seed::DEST_CHAIN_V2, chain_selector.to_le_bytes().as_ref()],
        bump = dest_chain.bump,
        constraint = valid_version(dest_chain.version, V2_MAX_CHAINSTATE_V) @ FeeQuoterError::InvalidVersion,
    )]
    pub dest_chain: Account<'info, DestChainV2>,

    // Only the registered admin can perform this action
    #[account(mut, address = config.owner @ FeeQuoterError::Unauthorized)]
    pub authority: Signer<'info>,
}

#[cfg(test)]
mod tests {
    use super::*;
    use ccip_common::types::UsdCents;

    fn sample_config() -> DestChainConfigV2 {
        DestChainConfigV2 {
            is_enabled: true,
            lane_code_version: CodeVersion::Default,
            max_data_bytes: 30_000,
            max_per_msg_gas_limit: CrossChainGas::new(3_000_000),
            dest_gas_overhead: CrossChainGas::new(100_000),
            dest_gas_per_payload_byte_base: 16,
            chain_family_selector: CHAIN_FAMILY_SELECTOR_EVM.to_be_bytes(),
            default_token_fee_usd_cents: 50,
            default_token_dest_gas_overhead: CrossChainGas::new(50_000),
            default_tx_gas_limit: CrossChainGas::new(200_000),
            network_fee: UsdCents::new(50),
            link_fee_multiplier_percent: 90,
        }
    }

    #[test]
    fn validate_dest_chain_config_v2_accepts_valid_config() {
        validate_dest_chain_config_v2(1, &sample_config()).unwrap();
    }

    #[test]
    fn validate_dest_chain_config_v2_rejects_zero_link_fee_multiplier() {
        let mut config = sample_config();
        config.link_fee_multiplier_percent = 0;

        assert_eq!(
            validate_dest_chain_config_v2(1, &config).unwrap_err(),
            FeeQuoterError::ZeroLinkFeeMultiplier.into()
        );
    }

    #[test]
    fn validate_dest_chain_config_v2_bounds_dest_gas_per_payload_byte_base() {
        let mut config = sample_config();
        config.dest_gas_per_payload_byte_base = u32::from(u8::MAX);
        validate_dest_chain_config_v2(1, &config).unwrap();

        config.dest_gas_per_payload_byte_base = u32::from(u8::MAX) + 1;
        assert_eq!(
            validate_dest_chain_config_v2(1, &config).unwrap_err(),
            FeeQuoterError::DestGasPerPayloadByteBaseTooLarge.into()
        );
    }

    #[test]
    fn validate_dest_chain_config_v2_bounds_network_fee() {
        let mut config = sample_config();
        config.network_fee = UsdCents::new(u32::from(u16::MAX));
        validate_dest_chain_config_v2(1, &config).unwrap();

        config.network_fee = UsdCents::new(u32::from(u16::MAX) + 1);
        assert_eq!(
            validate_dest_chain_config_v2(1, &config).unwrap_err(),
            FeeQuoterError::NetworkFeeTooLarge.into()
        );
    }
}
