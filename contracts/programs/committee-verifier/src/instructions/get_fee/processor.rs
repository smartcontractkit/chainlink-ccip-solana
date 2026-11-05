use anchor_lang::prelude::*;

use ccip_common::{
    messages::{FinalityConfig, SVM2AnyMessage},
    seed,
    types::{CrossChainGas, UsdCents},
};

use crate::{
    error::CommitteeVerifierError,
    state::{valid_version, Config, RemoteChainAdminConfig, RemoteChainConfig, MAX_CONFIG_V},
};

#[derive(AnchorSerialize, AnchorDeserialize, Debug, Clone)]
pub struct GetFeeParams {
    pub dest_chain_selector: u64,
    pub message: SVM2AnyMessage,
    pub extra_args: Vec<u8>,
    pub finality: FinalityConfig,
}

#[derive(AnchorSerialize, AnchorDeserialize, Debug, Clone, Default)]
pub struct GetFeeResult {
    pub fee: UsdCents,
    pub gas_for_verification: CrossChainGas,
    pub payload_size_bytes: u32,
}

impl From<&RemoteChainAdminConfig> for GetFeeResult {
    fn from(config: &RemoteChainAdminConfig) -> Self {
        Self {
            fee: config.fee,
            gas_for_verification: config.gas_for_verification,
            payload_size_bytes: config.payload_size_bytes,
        }
    }
}

/// Quotes the fee for a CCIP message verification.
///
/// This uses identical context as `start_verification_at_source`, as both instructions are
/// intended to be executed as CPIs from the same caller. Account resolution
/// for `start_verification_at_source` should suffice for both.
pub fn get_fee(ctx: Context<GetFee>, params: GetFeeParams) -> Result<GetFeeResult> {
    let finality_allowed = ctx
        .accounts
        .config
        .allowed_finality_config
        .ensure_requested_finality_allowed(params.finality);
    require!(finality_allowed, CommitteeVerifierError::InvalidFinality);

    Ok(GetFeeResult::from(
        &ctx.accounts.remote_chain_config.admin_config,
    ))
}

/// Presents the same account list as [`StartVerificationAtSource`], so one resolution CPI is valid
/// for both instructions: the router resolves only
/// `CommitteeVerifierCpiResolutionInputs::StartVerificationAtSource` (see the router's
/// `instructions/ccip_send/resolve_accounts/ccv.rs`) and reuses that list for the `get_verifier_fee` CPI.
/// Drift would silently break sending, so `account_lists_match_start_verification_at_source` in
/// `instructions/mod.rs` fails if these two ever diverge.
///
/// The duplication is deliberate rather than macro-generated:
/// - Anchor infers IDL seed metadata from the *syntactic form* of `seeds`, which must be a
///   field-access path rooted at an `#[instruction(...)]` arg. A shared accessor (trait method or
///   helper fn) compiles under `cargo check` but fails `anchor idl build` with E0425, and the two
///   params types keep the selector in different places — top-level here, under `message` there.
#[derive(Accounts)]
#[instruction(params: GetFeeParams)]
pub struct GetFee<'info> {
    #[account(
        seeds = [seed::CONFIG],
        bump = config.bump,
        constraint = valid_version(config.version, MAX_CONFIG_V) @ CommitteeVerifierError::InvalidVersion,
    )]
    pub config: Account<'info, Config>,

    #[account(
        address = remote_chain_config.admin_config.router_signer @ CommitteeVerifierError::Unauthorized,
    )]
    pub authority: Signer<'info>,

    #[account(
        seeds = [seed::REMOTE_CHAIN, &params.dest_chain_selector.to_le_bytes()],
        bump = remote_chain_config.bump,
        constraint = valid_version(remote_chain_config.version, MAX_CONFIG_V) @ CommitteeVerifierError::InvalidVersion,
    )]
    pub remote_chain_config: Account<'info, RemoteChainConfig>,
}
