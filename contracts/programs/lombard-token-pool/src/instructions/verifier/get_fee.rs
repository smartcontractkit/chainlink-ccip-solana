use anchor_lang::prelude::*;
use base_token_pool::constants::valid_version;
use base_token_pool::errors::CcipTokenPoolError;
use ccip_common::messages::{FinalityConfig, SVM2AnyMessage};
use ccip_common::types::{CrossChainGas, UsdCents};

use crate::constants::{VERIFIER_CONFIG_SEED, VERIFIER_REMOTE_CHAIN_SEED};
use crate::state::{
    VerifierConfig, VerifierRemoteChainConfig, MAX_VERIFIER_CONFIG_V,
    MAX_VERIFIER_REMOTE_CHAIN_CONFIG_V,
};

#[derive(Clone, AnchorSerialize, AnchorDeserialize, Debug)]
pub struct VerifierGetFeeParams {
    pub dest_chain_selector: u64,
    pub message: SVM2AnyMessage,
    pub extra_args: Vec<u8>,
    pub finality: FinalityConfig,
}

#[derive(AnchorSerialize, AnchorDeserialize, Debug, Clone, Default)]
pub struct VerifierGetFeeResult {
    pub fee: UsdCents,
    pub gas_for_verification: CrossChainGas,
    pub payload_size_bytes: u32,
}

/// Quotes the fee for verifying a CCIP message to a destination chain, mirroring
/// `committee-verifier`'s own `get_fee` processor.
pub fn get_verifier_fee(
    ctx: Context<GetVerifierFee>,
    params: VerifierGetFeeParams,
) -> Result<VerifierGetFeeResult> {
    require!(
        ctx.accounts
            .verifier_config
            .allowed_finality_config
            .ensure_requested_finality_allowed(params.finality),
        CcipTokenPoolError::InvalidInputs
    );

    let admin_config = &ctx.accounts.remote_chain_config.admin_config;
    Ok(VerifierGetFeeResult {
        fee: admin_config.fee,
        gas_for_verification: admin_config.gas_for_verification,
        payload_size_bytes: admin_config.payload_size_bytes,
    })
}

#[derive(Accounts)]
#[instruction(params: VerifierGetFeeParams)]
pub struct GetVerifierFee<'info> {
    #[account(
        seeds = [VERIFIER_CONFIG_SEED],
        bump = verifier_config.bump,
        constraint = valid_version(verifier_config.version, MAX_VERIFIER_CONFIG_V) @ CcipTokenPoolError::InvalidVersion,
    )]
    pub verifier_config: Account<'info, VerifierConfig>,

    /// The router-derived `ccv_signer` PDA the router signs this CPI with, matching
    /// `committee-verifier`'s own `authority: Signer` requirement on `GetFee`.
    #[account(
        address = remote_chain_config.admin_config.router_signer @ CcipTokenPoolError::Unauthorized,
    )]
    pub authority: Signer<'info>,

    #[account(
        seeds = [VERIFIER_REMOTE_CHAIN_SEED, &params.dest_chain_selector.to_le_bytes()],
        bump = remote_chain_config.bump,
        constraint = valid_version(remote_chain_config.version, MAX_VERIFIER_REMOTE_CHAIN_CONFIG_V) @ CcipTokenPoolError::InvalidVersion,
    )]
    pub remote_chain_config: Account<'info, VerifierRemoteChainConfig>,
}
