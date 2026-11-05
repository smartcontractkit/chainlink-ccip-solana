use anchor_lang::prelude::*;

use ccip_common::seed;

use crate::context::valid_version;
use crate::instructions::get_validated_token_price;
use crate::instructions_v2::{V2_MAX_BILLING_TOKEN_CONFIG_V, V2_MAX_CONFIG_V};
use crate::messages_v2::ValidatedTokenPriceV2;
use crate::state::{BillingTokenConfigWrapper, Config};
use crate::FeeQuoterError;

pub fn get_validated_token_price_v2(
    ctx: Context<GetValidatedTokenPriceV2>,
    _mint: Pubkey,
) -> Result<ValidatedTokenPriceV2> {
    // Price validation is identical to V1, only the account context differs.
    let price = get_validated_token_price(&ctx.accounts.billing_token_config.config)?;

    Ok(ValidatedTokenPriceV2 {
        price,
        billing_enabled: ctx.accounts.billing_token_config.config.enabled,
    })
}

#[derive(Accounts)]
#[instruction(mint: Pubkey)]
pub struct GetValidatedTokenPriceV2<'info> {
    #[account(
        seeds = [seed::CONFIG],
        bump,
        constraint = valid_version(config.version, V2_MAX_CONFIG_V) @ FeeQuoterError::InvalidVersion,
    )]
    pub config: Account<'info, Config>,

    #[account(
        seeds = [seed::FEE_BILLING_TOKEN_CONFIG, mint.as_ref()],
        bump,
        constraint = valid_version(billing_token_config.version, V2_MAX_BILLING_TOKEN_CONFIG_V) @ FeeQuoterError::InvalidVersion,
    )]
    pub billing_token_config: Account<'info, BillingTokenConfigWrapper>,
}
