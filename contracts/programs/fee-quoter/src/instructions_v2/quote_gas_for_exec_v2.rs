use anchor_lang::prelude::*;
use ccip_common::{
    seed,
    types::{CrossChainGas, UsdCents},
};

use crate::{
    context::{valid_version, MAX_CHAINSTATE_V as LEGACY_MAX_CHAINSTATE_V},
    instructions::PackedPrice,
    instructions_v2::{V2_MAX_BILLING_TOKEN_CONFIG_V, V2_MAX_CHAINSTATE_V, V2_MAX_CONFIG_V},
    messages_v2::{QuoteGasForExecV2Params, QuoteGasForExecV2Result},
    state::{BillingTokenConfigWrapper, Config, DestChain},
    state_v2::DestChainV2,
    FeeQuoterError,
};

pub fn quote_gas_for_exec_v2<'info>(
    ctx: Context<'info, QuoteGasForExecV2<'info>>,
    params: QuoteGasForExecV2Params,
) -> Result<QuoteGasForExecV2Result> {
    require!(
        ctx.accounts.billing_token_config.config.enabled,
        FeeQuoterError::FeeTokenDisabled
    );

    require!(
        ctx.accounts.dest_chain.config.is_enabled,
        FeeQuoterError::DestChainDisabled
    );

    // Note that the legacy dest chain is deliberately not checked for enablement: the 2.0 lane
    // is the one being quoted, and the 1.6 account is read only as the source of the gas price
    // (see below). Whether the 1.6 lane itself is open to traffic says nothing about the 2.0 one.

    // A total that doesn't fit in a u32 is, by construction, above any configurable
    // `max_per_msg_gas_limit` (itself a u32), so the overflow reports as an over-limit message
    // instead of panicking under `overflow-checks`.
    let corrected_total_gas = CrossChainGas::new(
        params
            .bytes_overhead_sum
            .checked_mul(
                ctx.accounts
                    .dest_chain
                    .config
                    .dest_gas_per_payload_byte_base,
            )
            .and_then(|payload_gas| payload_gas.checked_add(params.gas_limit_sum.convert::<u32>()))
            .ok_or(FeeQuoterError::MessageGasLimitTooHigh)?,
    );

    require_gte!(
        ctx.accounts.dest_chain.config.max_per_msg_gas_limit,
        corrected_total_gas,
        FeeQuoterError::MessageGasLimitTooHigh
    );

    require_gte!(
        ctx.accounts.dest_chain.config.max_data_bytes,
        params.bytes_overhead_sum,
        FeeQuoterError::MessageTooLarge
    );

    // Uses 1.6 state for gas price, as it's the only price updated by the offramp.
    require_gt!(
        ctx.accounts
            .legacy_dest_chain
            .state
            .usd_per_unit_gas
            .timestamp,
        0,
        FeeQuoterError::PriceNeverSet
    );

    // Note that Data Availability gas price (L2) is not currently billed. Instead, we
    // factor it in `dest_gas_per_payload_byte_base`, which accounts for data availability
    // costs. The upper 112 bits from the packed price here can be discarded.
    let price = PackedPrice::from(&ctx.accounts.legacy_dest_chain.state.usd_per_unit_gas);

    // Rounded up, so a non-zero gas cost is never billed as zero cents.
    let gas_cost = UsdCents::round_up_from_usd18(
        price.execution_gas_price * corrected_total_gas.convert::<u32>(),
    )?;

    let multiplier = if params.fee_token_mint == ctx.accounts.config.link_token_mint {
        ctx.accounts.dest_chain.config.link_fee_multiplier_percent
    } else {
        100
    };

    Ok(QuoteGasForExecV2Result {
        total_gas: corrected_total_gas,
        total_gas_cost: gas_cost,
        premium_percent_multiplier: multiplier as u64,
    })
}

#[derive(Accounts)]
#[instruction(params: QuoteGasForExecV2Params)]
pub struct QuoteGasForExecV2<'info> {
    #[account(
        seeds = [seed::CONFIG],
        bump,
        constraint = valid_version(config.version, V2_MAX_CONFIG_V) @ FeeQuoterError::InvalidVersion,
    )]
    pub config: Account<'info, Config>,

    #[account(
        seeds = [seed::DEST_CHAIN_V2, params.dest_chain_selector.to_le_bytes().as_ref()],
        bump = dest_chain.bump,
        constraint = valid_version(dest_chain.version, V2_MAX_CHAINSTATE_V) @ FeeQuoterError::InvalidVersion,
    )]
    pub dest_chain: Account<'info, DestChainV2>,

    #[account(
        seeds = [seed::DEST_CHAIN, params.dest_chain_selector.to_le_bytes().as_ref()],
        bump,
        constraint = valid_version(legacy_dest_chain.version, LEGACY_MAX_CHAINSTATE_V) @ FeeQuoterError::InvalidVersion,
    )]
    pub legacy_dest_chain: Account<'info, DestChain>,

    #[account(
        seeds = [seed::FEE_BILLING_TOKEN_CONFIG, params.fee_token_mint.as_ref()],
        bump,
        constraint = valid_version(billing_token_config.version, V2_MAX_BILLING_TOKEN_CONFIG_V) @ FeeQuoterError::InvalidVersion,
    )]
    pub billing_token_config: Account<'info, BillingTokenConfigWrapper>,
}
