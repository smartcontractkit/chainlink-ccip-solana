use anchor_lang::prelude::*;

use ccip_common::state::AccountSchemaVersion;
use ccip_common::types::ByteString;

use crate::error::CctpPoolVerifierError;
use crate::verifier::state::{verifier_seed, Config, StorageLocations};

/// Returns storage location identifiers for this CCV.
pub fn get_verifier_storage_locations(
    ctx: Context<GetVerifierStorageLocations>,
) -> Result<Vec<ByteString>> {
    Ok(ctx.accounts.storage_locations.locations.clone())
}

#[derive(Accounts)]
pub struct GetVerifierStorageLocations<'info> {
    #[account(
        seeds = [verifier_seed::CONFIG],
        bump = config.bump,
        constraint = config.is_latest_version() @ CctpPoolVerifierError::InvalidVersion,
    )]
    pub config: Account<'info, Config>,

    #[account(
        seeds = [verifier_seed::STORAGE_LOCATIONS],
        bump = storage_locations.bump,
        constraint = storage_locations.is_latest_version() @ CctpPoolVerifierError::InvalidVersion,
    )]
    pub storage_locations: Account<'info, StorageLocations>,
}
