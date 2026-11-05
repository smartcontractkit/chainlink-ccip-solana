use anchor_lang::prelude::*;
use ccip_common::messages::FinalityConfig;
use ccip_common::state::PoolChainConfigOverrideType;

use crate::rate_limiter::RateLimitConfig;

use crate::state::CCVConfig;
use crate::state::RemoteAddress;
use crate::state::TokenTransferFeeConfig;

#[event]
pub struct GlobalConfigUpdated {
    pub self_served_allowed: bool,
    pub router: Pubkey,
    pub rmn_remote: Pubkey,
}

// V1 operation events

#[event]
pub struct Burned {
    pub sender: Pubkey,
    pub amount: u64,
    pub mint: Pubkey,
}

#[event]
pub struct Minted {
    pub sender: Pubkey,
    pub recipient: Pubkey,
    pub amount: u64,
    pub mint: Pubkey,
}

#[event]
pub struct Locked {
    pub sender: Pubkey,
    pub amount: u64,
    pub mint: Pubkey,
}

#[event]
pub struct Released {
    pub sender: Pubkey,
    pub recipient: Pubkey,
    pub amount: u64,
    pub mint: Pubkey,
}

// V2 CPI events (unified, mostly EVM-parity)

#[event]
pub struct LockedOrBurned {
    pub mint: Pubkey,
    pub remote_chain_selector: u64,
    pub sender: Pubkey,
    pub amount: u64,
}

#[event]
pub struct ReleasedOrMinted {
    pub mint: Pubkey,
    pub remote_chain_selector: u64,
    pub sender: Pubkey,
    pub recipient: Pubkey, // the address specified in the ReleaseOrMintInV2 (either a wallet or a TokenAccount)
    pub user_token_account: Pubkey, // may be equal to the recipient (TokenAccount), or the ATA if the recipient is a wallet - this field is SVM-specific.
    pub amount: u64,
}

#[event]
pub struct RebalancerSet {
    pub mint: Pubkey,
    pub old_rebalancer: Pubkey,
    pub new_rebalancer: Pubkey,
}

// Configuration events
// note: configuration events are slightly different than EVM chains because configuration follows different steps

#[event]
pub struct RemoteChainConfigured {
    pub chain_selector: u64,
    pub token: RemoteAddress,
    pub previous_token: RemoteAddress,
    pub pool_addresses: Vec<RemoteAddress>,
    pub previous_pool_addresses: Vec<RemoteAddress>,
    pub mint: Pubkey,
}

#[event]
pub struct RateLimitConfigured {
    pub chain_selector: u64,
    pub outbound_rate_limit: RateLimitConfig,
    pub inbound_rate_limit: RateLimitConfig,
    pub mint: Pubkey,
}

#[event]
pub struct RemotePoolsAppended {
    pub chain_selector: u64,
    pub pool_addresses: Vec<RemoteAddress>,
    pub previous_pool_addresses: Vec<RemoteAddress>,
    pub mint: Pubkey,
}

#[event]
pub struct RemoteChainRemoved {
    pub chain_selector: u64,
    pub mint: Pubkey,
}

#[event]
pub struct RouterUpdated {
    pub old_router: Pubkey,
    pub new_router: Pubkey,
    pub mint: Pubkey,
}

#[event]
pub struct RmnRemoteUpdated {
    pub old_rmn_remote: Pubkey,
    pub new_rmn_remote: Pubkey,
    pub mint: Pubkey,
}

#[event]
pub struct RateLimitAdminSet {
    pub mint: Pubkey,
    pub old_rate_limit_admin: Pubkey,
    pub new_rate_limit_admin: Pubkey,
}

#[event]
pub struct OwnershipTransferRequested {
    pub from: Pubkey,
    pub to: Pubkey,
    pub mint: Pubkey,
}

#[event]
pub struct TokenPoolInitialized {
    pub mint: Pubkey,
    pub token_program: Pubkey,
    pub owner: Pubkey,
}

#[event]
pub struct OwnershipTransferred {
    pub from: Pubkey,
    pub to: Pubkey,
    pub mint: Pubkey,
}

#[event]
pub struct MintAuthorityTransferred {
    pub mint: Pubkey,
    pub old_mint_authority: Pubkey,
    pub new_mint_authority: Pubkey,
}

#[event]
pub struct LiquidityDeposited {
    pub provider: Pubkey,
    pub amount: u64,
    pub mint: Pubkey,
}

#[event]
pub struct LiquidityWithdrawn {
    pub provider: Pubkey,
    pub amount: u64,
    pub mint: Pubkey,
}

#[event]
pub struct ConfigV2Updated {
    pub remote_chain_selector: u64,
    pub mint: Pubkey,
    pub ccv: CCVConfig,
    pub allowed_finality: FinalityConfig,
    pub token_transfer_fee: TokenTransferFeeConfig,
}

// Override config events

#[event]
pub struct OverrideChainConfigConfigured {
    pub chain_selector: u64,
    pub mint: Pubkey,
    pub override_type: PoolChainConfigOverrideType,
    pub outbound_rate_limit: RateLimitConfig,
    pub inbound_rate_limit: RateLimitConfig,
}

#[event]
pub struct OverrideChainConfigRemoved {
    pub chain_selector: u64,
    pub override_type: PoolChainConfigOverrideType,
    pub mint: Pubkey,
}

// Token transfer fee config events

#[event]
pub struct TokenTransferFeeConfigUpdated {
    pub chain_selector: u64,
    pub mint: Pubkey,
    pub config: TokenTransferFeeConfig,
}

#[event]
pub struct TokenTransferFeeConfigRemoved {
    pub chain_selector: u64,
    pub mint: Pubkey,
}
