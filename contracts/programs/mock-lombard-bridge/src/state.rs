use anchor_lang::prelude::*;
use base_token_pool::rate_limiter::RateLimitTokenBucket;

#[account]
pub struct RemoteBridgeConfig {
    pub bump: u8,
    pub chain_id: [u8; 32],
    pub bridge: [u8; 32],
}

impl RemoteBridgeConfig {
    pub const SPACE: usize = 8 + 1 + 32 + 32;
}

/// Wire-compatible with the real bridge's `RemoteTokenConfig` (see
/// `lombard_cpi::bridge::state::RemoteTokenConfig`, which this must byte-match): only
/// `chain_id`/`token` are ever read by this mock or by `lombard_token_pool`'s
/// `RemoteTokenOrAdapterMismatch` guard. `inbound_rate_limit` is left zero-initialized by `init`
/// and never touched - `RateLimitTokenBucket`'s fields are all zero-valid, so this is a real,
/// well-formed (if inert) value, not a shortcut around its private constructor.
#[account]
#[derive(InitSpace)]
pub struct RemoteTokenConfig {
    pub bump: u8,
    pub chain_id: [u8; 32],
    pub token: [u8; 32],
    pub direction: u8,
    pub inbound_rate_limit: RateLimitTokenBucket,
}
