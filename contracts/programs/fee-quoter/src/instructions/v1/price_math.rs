use anchor_lang::prelude::*;
use ccip_common::types::Usd18Decimals;

use crate::state::BillingTokenConfig;
use crate::FeeQuoterError;

pub(crate) fn get_validated_token_price(
    token_config: &BillingTokenConfig,
) -> Result<Usd18Decimals> {
    let timestamp = token_config.usd_per_token.timestamp;
    let price: Usd18Decimals = (&token_config.usd_per_token).into();

    // NOTE: There's no validation done with respect to token price staleness since data feeds are not
    // supported in solana. Only the existence of `any` timestamp is checked, to ensure the price
    // was set at least once.
    // require! instead of require_neq!: U256 does not implement Display
    require!(price.get() != 0, FeeQuoterError::InvalidTokenPrice);
    require_neq!(timestamp, 0, FeeQuoterError::InvalidTokenPrice);

    Ok(price)
}
