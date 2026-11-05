pub const MAX_POOL_STATE_V: u8 = 1;
pub const MAX_POOL_CONFIG_V: u8 = 1;
pub const MAX_POOL_CHAIN_CONFIG_V: u8 = 1;
pub const MAX_POOL_OVERRIDE_V: u8 = 1;

pub const SOLANA_DOMAIN_ID: u32 = 5;

// --- CCIP 2.0 additions ---

/// Seed for the per-(mint, remote chain) pool-v2 lane config PDA:
/// `[POOL_CHAINCONFIG_SEED_2_0, remote_chain_selector.to_le_bytes(), mint]`.
/// Matches `burnmint-token-pool`'s seed so tooling/clients recognize the same PDA shape.
pub const POOL_CHAINCONFIG_SEED_2_0: &[u8] = b"ccip_tokenpool_chainconfig_2_0";

/// Seed for the pool's global config PDA (`PoolConfig`): `[POOL_CONFIG_SEED]`. Deliberately NOT
/// `ccip_common::seed::CONFIG` ("config") - that slot is reserved for `VERIFIER_CONFIG_SEED`
/// below, since this program plays both the pool and verifier (CCV) roles and the offramp's
/// generic CCV account resolution (`ccip-offramp`'s `ccv.rs` `find(&[seed::CONFIG], ccv)`)
/// hardcodes "config" as the seed for a CCV's own config PDA under its own program ID.
pub const POOL_CONFIG_SEED: &[u8] = b"lombard_pool_config";

/// Seed for the verifier's global config PDA: `[VERIFIER_CONFIG_SEED]`. Must be
/// `ccip_common::seed::CONFIG` ("config") - see `POOL_CONFIG_SEED` doc above.
pub const VERIFIER_CONFIG_SEED: &[u8] = ccip_common::seed::CONFIG;
/// Seed for the verifier's per-remote-chain config PDA:
/// `[VERIFIER_REMOTE_CHAIN_SEED, remote_chain_selector.to_le_bytes()]`.
pub const VERIFIER_REMOTE_CHAIN_SEED: &[u8] = b"lombard_verifier_remote_chain";
/// Seed for the verifier's storage-locations PDA: `[VERIFIER_STORAGE_LOCATIONS_SEED]`. Trimmed to
/// "..._locs" (30 bytes) - Solana's `find_program_address` rejects seeds over 32 bytes, and the
/// untrimmed "...locations" spelling was 34.
pub const VERIFIER_STORAGE_LOCATIONS_SEED: &[u8] = b"lombard_verifier_storage_locs";

/// Size, in bytes, of the message-ID-binding payload packed into the Lombard bridge's
/// `optional_message` field: `version_tag (4) || message_id (32)`. The bridge's
/// `OPTIONAL_MESSAGE_SIZE` is 36 bytes, so the binding fills the field exactly.
pub const MESSAGE_BINDING_SIZE: usize = 4 + 32;
