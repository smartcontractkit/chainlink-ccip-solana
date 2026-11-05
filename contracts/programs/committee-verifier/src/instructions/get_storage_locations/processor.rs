use anchor_lang::prelude::*;

use ccip_common::{seed, types::ByteString};

use crate::state::{
    valid_version, Config, StorageLocations, MAX_CONFIG_V, MAX_STORAGE_LOCATIONS_V,
};
use crate::CommitteeVerifierError;

/// Returns storage location identifiers for this CCV.
pub fn get_storage_locations(ctx: Context<GetStorageLocations>) -> Result<Vec<ByteString>> {
    Ok(ctx.accounts.storage_locations.locations.clone())
}

#[derive(Accounts)]
pub struct GetStorageLocations<'info> {
    #[account(
        seeds = [seed::CONFIG],
        bump = config.bump,
        constraint = valid_version(config.version, MAX_CONFIG_V) @ CommitteeVerifierError::InvalidVersion,
    )]
    pub config: Account<'info, Config>,

    #[account(
        seeds = [seed::STORAGE_LOCATIONS],
        bump = storage_locations.bump,
        constraint = valid_version(storage_locations.version, MAX_STORAGE_LOCATIONS_V) @ CommitteeVerifierError::InvalidVersion,
    )]
    pub storage_locations: Account<'info, StorageLocations>,
}
