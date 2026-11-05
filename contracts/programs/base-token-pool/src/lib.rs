use anchor_lang::prelude::*;

pub mod constants;
pub mod errors;
pub mod events;
pub mod helpers;
pub mod observe;
pub mod state;
pub mod v1;
pub mod v2;

pub mod rate_limiter;

// null contract required for IDL + gobinding generation
declare_id!("DtHUwbNFN8RSeEBje4eH5gpqmheA9S86KaSWoqpQKzZu");
#[program]
pub mod base_token_pool {}
