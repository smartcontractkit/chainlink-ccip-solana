use anchor_lang::prelude::*;

use ccip_common::messages::MessageV1FieldsAtResolutionTime;
use ccip_common::resolution::*;
use ccip_common::seed;

use crate::error::CommitteeVerifierError;
use crate::instruction;
use crate::instructions::{
    admin, get_storage_locations, remove_signer_config, set_allowlist, set_chain_config,
    set_signer_config, set_storage_locations, start_verification_at_source, verify,
};
use crate::state::Config;
use crate::state::{valid_version, MAX_CONFIG_V};

/// Inputs to the first stage of nested account resolution for the committee verifier
/// instructions invoked via CPI. Callers Borsh-encode this type as the
/// `cpi_resolution_inputs` of `nested_resolve_accounts_start`.
#[derive(AnchorSerialize, AnchorDeserialize, Clone, Debug)]
#[allow(clippy::large_enum_variant)]
pub enum CommitteeVerifierCpiResolutionInputs {
    Verify {
        source_chain_selector: u64,
        encoded_message: Vec<u8>,
        verifier_results: Vec<u8>,
    },
    /// Everything `start_verification_at_source` receives that the onramp already knows while it
    /// resolves. `message_id` is not known: it hashes the fully built message, which does
    /// not exist yet. Neither is the fee token *amount*; only the mint.
    StartVerificationAtSource {
        fields: MessageV1FieldsAtResolutionTime,
        /// Mint the sender pays the fee in, WSOL-normalized.
        fee_token: Pubkey,
        /// This verifier's slice of the sender's `ccv_args`.
        verifier_args: Vec<u8>,
    },
    // If any of the resolutions above required multiple steps, they could be tracked as
    // separate entries in this enum or, alternatively, marked by data fields inside the enum variant.
    //
    // For example, GetFee(Stage::One), GetFee(Stage::Two) and so on.
}

#[derive(Accounts)]
pub struct NestedResolveAccountsStart<'info> {
    #[account(
        seeds = [seed::CONFIG],
        bump = config.bump,
        constraint = valid_version(config.version, MAX_CONFIG_V) @ CommitteeVerifierError::InvalidVersion,
    )]
    pub config: Account<'info, Config>,
}

pub fn nested_resolve_accounts_start(
    ctx: Context<NestedResolveAccountsStart>,
    params: NestedResolveAccountsParams,
) -> Result<NestedResolveAccountsResponse> {
    let inputs =
        CommitteeVerifierCpiResolutionInputs::try_from_slice(&params.cpi_resolution_inputs)?;
    match inputs {
        CommitteeVerifierCpiResolutionInputs::Verify {
            source_chain_selector,
            ..
        } => verify::resolve_accounts::start(ctx, source_chain_selector),
        CommitteeVerifierCpiResolutionInputs::StartVerificationAtSource {
            fields,
            fee_token,
            verifier_args,
        } => start_verification_at_source::resolve_accounts::start(
            ctx,
            params.caller,
            &fields,
            fee_token,
            &verifier_args,
        ),
    }
}

#[derive(Accounts)]
pub struct ResolveAccountsStart<'info> {
    #[account(
        seeds = [seed::CONFIG],
        bump = config.bump,
        constraint = valid_version(config.version, MAX_CONFIG_V) @ CommitteeVerifierError::InvalidVersion,
    )]
    pub config: Account<'info, Config>,
}

pub fn resolve_accounts_start(
    ctx: Context<ResolveAccountsStart>,
    params: ResolveAccountsParams,
) -> Result<ResolveAccountsResponse> {
    let (ix_discriminator, _data) = validate_resolution_start(&params, false)?;

    match ix_discriminator {
        instruction::GetStorageLocations::DISCRIMINATOR => {
            get_storage_locations::resolve_accounts::start(ctx, params)
        }
        instruction::InitChainConfig::DISCRIMINATOR => {
            set_chain_config::resolve_accounts::init(ctx, params)
        }
        instruction::EditChainConfig::DISCRIMINATOR => {
            set_chain_config::resolve_accounts::edit(ctx, params)
        }
        instruction::SetAllowlist::DISCRIMINATOR => {
            set_allowlist::resolve_accounts::start(ctx, params)
        }
        instruction::SetSignerConfig::DISCRIMINATOR => {
            set_signer_config::resolve_accounts::start(ctx, params)
        }
        instruction::RemoveSignerConfig::DISCRIMINATOR => {
            remove_signer_config::resolve_accounts::start(ctx, params)
        }
        instruction::SetStorageLocations::DISCRIMINATOR => {
            set_storage_locations::resolve_accounts::start(ctx, params)
        }
        instruction::TransferOwnership::DISCRIMINATOR
        | instruction::AcceptOwnership::DISCRIMINATOR
        | instruction::SetDefaultCodeVersion::DISCRIMINATOR
        | instruction::TransferStorageLocationAdmin::DISCRIMINATOR
        | instruction::AcceptStorageLocationAdmin::DISCRIMINATOR
        | instruction::SetAllowlistAdmin::DISCRIMINATOR
        | instruction::SetFeeAggregator::DISCRIMINATOR => {
            admin::resolve_accounts::start(ctx, params)
        }
        _ => {
            msg!(
                "Unknown discriminator to resolve accounts for: {}",
                BytesWrapper(ix_discriminator)
            );
            Err(CommitteeVerifierError::InvalidInstructionData.into())
        }
    }
}
