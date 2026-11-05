use anchor_lang::prelude::*;

use ccip_common::types::ByteString;

use crate::error::CctpPoolVerifierError;
use crate::verifier::state::{verifier_seed, AccountSchemaVersion, Config, StorageLocations};

#[derive(AnchorSerialize, AnchorDeserialize, Debug)]
pub struct SetVerifierStorageLocationsParams {
    pub storage_locations: Vec<ByteString>,
}

pub fn set_verifier_storage_locations(
    ctx: Context<SetVerifierStorageLocations>,
    params: SetVerifierStorageLocationsParams,
) -> Result<()> {
    emit_cpi!(StorageLocationsSet {
        locations: params.storage_locations.clone(),
    });

    ctx.accounts.storage_locations.locations = params.storage_locations;
    Ok(())
}

#[event_cpi]
#[derive(Accounts)]
#[instruction(params: SetVerifierStorageLocationsParams)]
pub struct SetVerifierStorageLocations<'info> {
    #[account(
        seeds = [verifier_seed::CONFIG],
        bump = config.bump,
        constraint = config.is_latest_version() @ CctpPoolVerifierError::InvalidVersion,
    )]
    pub config: Account<'info, Config>,

    #[account(mut, address = config.storage_location_admin @ CctpPoolVerifierError::Unauthorized)]
    pub authority: Signer<'info>,

    #[account(
        mut,
        seeds = [verifier_seed::STORAGE_LOCATIONS],
        bump = storage_locations.bump,
        constraint = storage_locations.is_latest_version() @ CctpPoolVerifierError::InvalidVersion,
        realloc = StorageLocations::DISCRIMINATOR.len() + StorageLocations::INIT_SPACE + StorageLocations::dynamic_space(&params.storage_locations),
        realloc::payer = authority,
        realloc::zero = false,
    )]
    pub storage_locations: Account<'info, StorageLocations>,

    pub system_program: Program<'info, System>,
}

#[event]
pub struct StorageLocationsSet {
    pub locations: Vec<ByteString>,
}
