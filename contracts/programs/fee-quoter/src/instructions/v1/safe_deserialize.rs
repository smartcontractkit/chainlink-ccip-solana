/// Methods in this module are used to deserialize AccountInfo into the state structs
use anchor_lang::{prelude::*, system_program};
use ccip_common::seed;

use crate::context::{valid_version, MAX_BILLING_TOKEN_CONFIG_V, MAX_TOKEN_AND_CHAIN_CONFIG_V};
use crate::state::{BillingTokenConfig, BillingTokenConfigWrapper, PerChainPerTokenConfig};
use crate::FeeQuoterError;

/// Returns Ok(None) when there's no chain-token configuration pair. The user must still
/// specify the correct PDA, to ensure the configuration isn't ignored in the case it exists.
pub fn per_chain_per_token_config<'info>(
    acc_info: &'info AccountInfo<'info>,
    token: Pubkey,
    dest_chain_selector: u64,
) -> Result<Option<PerChainPerTokenConfig>> {
    let (expected, _) = Pubkey::find_program_address(
        &[
            seed::PER_CHAIN_PER_TOKEN_CONFIG,
            dest_chain_selector.to_le_bytes().as_ref(),
            token.key().as_ref(),
        ],
        &crate::ID,
    );
    require_keys_eq!(
        acc_info.key(),
        expected,
        FeeQuoterError::InvalidInputsPerChainPerTokenConfig
    );

    // An account still owned by the System Program has never been initialized by this
    // program, so the configuration is absent. Detecting absence from ownership is
    // lamport-agnostic: unlike the `AccountNotInitialized` error (which Anchor only
    // returns for a System-owned *and* zero-lamport account), it cannot be perturbed by
    // an unprivileged lamport transfer to the public-seed PDA. See INV-ACCT-INIT-2/3.
    if acc_info.owner == &system_program::ID {
        return Ok(None);
    }

    let config = Account::<PerChainPerTokenConfig>::try_from(acc_info)?.into_inner();
    require!(
        valid_version(config.version, MAX_TOKEN_AND_CHAIN_CONFIG_V),
        FeeQuoterError::InvalidVersion
    );

    Ok(Some(config))
}

/// Returns Ok(None) when there's no token specific billing config. The user must still
/// specify the correct PDA, to ensure the configuration isn't ignored in the case it exists.
pub fn billing_token_config<'info>(
    acc_info: &'info AccountInfo<'info>,
    token: Pubkey,
) -> Result<Option<BillingTokenConfig>> {
    let (expected, _) = Pubkey::find_program_address(
        &[seed::FEE_BILLING_TOKEN_CONFIG, token.as_ref()],
        &crate::ID,
    );
    require_keys_eq!(
        acc_info.key(),
        expected,
        FeeQuoterError::InvalidInputsBillingTokenConfig
    );

    // An account still owned by the System Program has never been initialized by this
    // program, so the configuration is absent. Detecting absence from ownership is
    // lamport-agnostic: unlike the `AccountNotInitialized` error (which Anchor only
    // returns for a System-owned *and* zero-lamport account), it cannot be perturbed by
    // an unprivileged lamport transfer to the public-seed PDA. See INV-ACCT-INIT-2/3.
    if acc_info.owner == &system_program::ID {
        return Ok(None);
    }

    let config_wrapper = Account::<BillingTokenConfigWrapper>::try_from(acc_info)?.into_inner();
    require!(
        valid_version(config_wrapper.version, MAX_BILLING_TOKEN_CONFIG_V),
        FeeQuoterError::InvalidVersion
    );

    Ok(Some(config_wrapper.config))
}
