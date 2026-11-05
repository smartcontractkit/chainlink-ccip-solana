use anchor_discriminator::anchor_discriminator;

pub const ANCHOR_DISCRIMINATOR: usize = 8; // 8-byte anchor discriminator length
pub const POOL_CHAINCONFIG_SEED: &[u8] = b"ccip_tokenpool_chainconfig"; // seed used by Token Pool Owner to set config per remote chain
pub const POOL_CHAINCONFIG_SEED_2_0: &[u8] = b"ccip_tokenpool_chainconfig_2_0"; // seed used by Token Pool Owner to set extra v2 configs for example the ccvs per remote chain
pub const POOL_OVERRIDE_CHAINCONFIG_SEED: &[u8] = b"chainconfig_override"; // custom override configs for ccip_tokenpool_chainconfig
pub const POOL_STATE_SEED: &[u8] = b"ccip_tokenpool_config"; // seed used by Token Pool Owner to set global config for the pool
pub const POOL_SIGNER_SEED: &[u8] = b"ccip_tokenpool_signer";

pub const EXTERNAL_TOKEN_POOLS_SIGNER: &[u8] = b"external_token_pools_signer";
pub const ALLOWED_OFFRAMP: &[u8] = b"allowed_offramp";

pub const CONFIG_SEED: &[u8] = b"config";

pub const fn valid_version(v: u8, max_version: u8) -> bool {
    !uninitialized(v) && v <= max_version
}

pub const fn uninitialized(v: u8) -> bool {
    v == 0
}

// discriminators
pub const RELEASE_MINT: [u8; 8] = anchor_discriminator!("release_or_mint_tokens");
pub const LOCK_BURN: [u8; 8] = anchor_discriminator!("lock_or_burn_tokens");
pub const LOCK_OR_BURN_V2: [u8; 8] = anchor_discriminator!("lock_or_burn_tokens_v2");
pub const RELEASE_OR_MINT_V2: [u8; 8] = anchor_discriminator!("release_or_mint_tokens_v2");
pub const GET_POOL_FEE: [u8; 8] = anchor_discriminator!("get_pool_fee");
