use anchor_lang::prelude::*;
use base_token_pool::constants::valid_version;
use base_token_pool::errors::CcipTokenPoolError;
use ccip_common::types::ByteString;

use crate::constants::{VERIFIER_CONFIG_SEED, VERIFIER_STORAGE_LOCATIONS_SEED};
use crate::events::VerifierStorageLocationsSet;
use crate::state::{
    VerifierConfig, VerifierStorageLocations, MAX_VERIFIER_CONFIG_V,
    MAX_VERIFIER_STORAGE_LOCATIONS_V,
};

#[derive(AnchorSerialize, AnchorDeserialize, Clone, Debug)]
pub struct SetVerifierStorageLocationsParams {
    pub storage_locations: Vec<ByteString>,
}

pub fn set_verifier_storage_locations(
    ctx: Context<SetVerifierStorageLocations>,
    params: SetVerifierStorageLocationsParams,
) -> Result<()> {
    emit!(VerifierStorageLocationsSet {
        locations: params.storage_locations.clone(),
    });

    ctx.accounts.verifier_storage_locations.locations = params.storage_locations;
    Ok(())
}

#[derive(Accounts)]
#[instruction(params: SetVerifierStorageLocationsParams)]
pub struct SetVerifierStorageLocations<'info> {
    #[account(
        seeds = [VERIFIER_CONFIG_SEED],
        bump = verifier_config.bump,
        constraint = valid_version(verifier_config.version, MAX_VERIFIER_CONFIG_V) @ CcipTokenPoolError::InvalidVersion,
    )]
    pub verifier_config: Account<'info, VerifierConfig>,

    #[account(mut, address = verifier_config.storage_location_admin @ CcipTokenPoolError::Unauthorized)]
    pub authority: Signer<'info>,

    #[account(
        mut,
        seeds = [VERIFIER_STORAGE_LOCATIONS_SEED],
        bump = verifier_storage_locations.bump,
        constraint = valid_version(verifier_storage_locations.version, MAX_VERIFIER_STORAGE_LOCATIONS_V) @ CcipTokenPoolError::InvalidVersion,
        realloc = VerifierStorageLocations::DISCRIMINATOR.len() + VerifierStorageLocations::INIT_SPACE + VerifierStorageLocations::dynamic_space(&params.storage_locations),
        realloc::payer = authority,
        realloc::zero = false,
    )]
    pub verifier_storage_locations: Account<'info, VerifierStorageLocations>,

    pub system_program: Program<'info, System>,
}

pub fn get_verifier_storage_locations(
    ctx: Context<GetVerifierStorageLocations>,
) -> Result<Vec<ByteString>> {
    Ok(ctx.accounts.verifier_storage_locations.locations.clone())
}

#[derive(Accounts)]
pub struct GetVerifierStorageLocations<'info> {
    #[account(
        seeds = [VERIFIER_CONFIG_SEED],
        bump = verifier_config.bump,
        constraint = valid_version(verifier_config.version, MAX_VERIFIER_CONFIG_V) @ CcipTokenPoolError::InvalidVersion,
    )]
    pub verifier_config: Account<'info, VerifierConfig>,

    #[account(
        seeds = [VERIFIER_STORAGE_LOCATIONS_SEED],
        bump = verifier_storage_locations.bump,
        constraint = valid_version(verifier_storage_locations.version, MAX_VERIFIER_STORAGE_LOCATIONS_V) @ CcipTokenPoolError::InvalidVersion,
    )]
    pub verifier_storage_locations: Account<'info, VerifierStorageLocations>,
}
