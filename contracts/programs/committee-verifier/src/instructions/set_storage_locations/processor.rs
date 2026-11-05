use anchor_lang::prelude::*;

use ccip_common::{seed, types::ByteString};

use crate::event::StorageLocationsSet;
use crate::state::valid_version;
use crate::state::{
    Config, StorageLocations, ANCHOR_DISCRIMINATOR, MAX_CONFIG_V, MAX_STORAGE_LOCATIONS_V,
};
use crate::CommitteeVerifierError;

#[derive(AnchorSerialize, AnchorDeserialize, Debug)]
pub struct SetStorageLocationsParams {
    pub storage_locations: Vec<ByteString>,
}

pub fn set_storage_locations(
    ctx: Context<SetStorageLocations>,
    params: SetStorageLocationsParams,
) -> Result<()> {
    emit_cpi!(StorageLocationsSet {
        locations: params.storage_locations.clone(),
    });

    ctx.accounts.storage_locations.locations = params.storage_locations;
    Ok(())
}

#[event_cpi]
#[derive(Accounts)]
#[instruction(params: SetStorageLocationsParams)]
pub struct SetStorageLocations<'info> {
    #[account(
        seeds = [seed::CONFIG],
        bump = config.bump,
        constraint = valid_version(config.version, MAX_CONFIG_V) @ CommitteeVerifierError::InvalidVersion,
    )]
    pub config: Account<'info, Config>,

    #[account(mut, address = config.storage_location_admin @ CommitteeVerifierError::Unauthorized)]
    pub authority: Signer<'info>,

    #[account(
        mut,
        seeds = [seed::STORAGE_LOCATIONS],
        bump = storage_locations.bump,
        constraint = valid_version(storage_locations.version, MAX_STORAGE_LOCATIONS_V) @ CommitteeVerifierError::InvalidVersion,
        realloc = ANCHOR_DISCRIMINATOR + StorageLocations::INIT_SPACE + StorageLocations::dynamic_space(&params.storage_locations),
        realloc::payer = authority,
        realloc::zero = false,
    )]
    pub storage_locations: Account<'info, StorageLocations>,

    pub system_program: Program<'info, System>,
}
