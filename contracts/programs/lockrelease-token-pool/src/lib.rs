use anchor_lang::prelude::*;

pub mod security;
use base_token_pool::observe::{
    PoolChainConfigObservation, PoolChainConfigOverrideObservation, PoolChainConfigV2Observation,
    PoolStateObservation,
};
use base_token_pool::{rate_limiter::*, state::*, v1::*, v2::*};
use ccip_common::{resolution::*, state::PoolChainConfigOverrideType};

use ccip_common::ccvs::{GetRequiredCCVsIn, GetRequiredCCVsOut};
declare_id!("8eqh8wppT9c5rw4ERqNCffvU6cNFJWff9WmkcYtmGiqC");

pub mod state;
pub use state::*;

mod instructions;
use crate::instructions::*;

#[derive(AnchorSerialize, AnchorDeserialize, Clone)]
pub struct InitChainConfigOverrideParams {
    pub remote_chain_selector: u64,
    pub mint: Pubkey,
    pub override_type: PoolChainConfigOverrideType,
    pub inbound: RateLimitConfig,
    pub outbound: RateLimitConfig,
}

#[derive(AnchorSerialize, AnchorDeserialize, Clone)]
pub struct SetChainConfigOverrideRateLimitParams {
    pub remote_chain_selector: u64,
    pub mint: Pubkey,
    pub override_type: PoolChainConfigOverrideType,
    pub inbound: RateLimitConfig,
    pub outbound: RateLimitConfig,
}

#[derive(AnchorSerialize, AnchorDeserialize, Clone)]
pub struct DeleteChainConfigOverrideParams {
    pub remote_chain_selector: u64,
    pub mint: Pubkey,
    pub override_type: PoolChainConfigOverrideType,
}

#[derive(AnchorSerialize, AnchorDeserialize, Clone)]
pub struct SetTokenTransferFeeConfigParams {
    pub remote_chain_selector: u64,
    pub mint: Pubkey,
    pub config: TokenTransferFeeConfig,
}

#[derive(AnchorSerialize, AnchorDeserialize, Clone)]
pub struct DeleteTokenTransferFeeConfigParams {
    pub remote_chain_selector: u64,
    pub mint: Pubkey,
}

#[program]
pub mod lockrelease_token_pool {
    use super::*;

    // ==============================
    // Type & Version
    // ==============================

    /// Returns the program type (name) and version string encoded at build time.
    /// Offchain code calls this to confirm which program and version is deployed
    /// before sending any other instruction.
    pub fn type_version(_ctx: Context<Empty>) -> Result<String> {
        type_version::type_version(_ctx)
    }

    // ==============================
    // Observability
    // ==============================

    /// Returns a pool's per-mint configuration, for observability consumers.
    ///
    /// Permissionless and read-only: meant to be executed through `simulateTransaction`. The
    /// allow list is reported as a count only.
    pub fn observe_state(ctx: Context<ObserveState>, mint: Pubkey) -> Result<PoolStateObservation> {
        observe::observe_state(ctx, mint)
    }

    /// Returns a pool's configuration for a single remote chain, for observability consumers.
    /// Rate limiter buckets are reported as of now, i.e. including the refill accrued since the
    /// last transfer.
    ///
    /// Permissionless and read-only: meant to be executed through `simulateTransaction`. Fails if
    /// the lane has no chain config, which callers can read as "not configured".
    // Collapsing these into a `*Params` struct is an IDL-breaking change to a read-only
    // observability handler; deferred to the next IDL break.
    pub fn observe_chain_config(
        ctx: Context<ObserveChainConfig>,
        remote_chain_selector: u64,
        mint: Pubkey,
    ) -> Result<PoolChainConfigObservation> {
        observe::observe_chain_config(ctx, remote_chain_selector, mint)
    }

    /// Returns a pool's rate limit override for a single remote chain and override type, for
    /// observability consumers. Rate limiter buckets are reported as of now, i.e. including the
    /// refill accrued since the last transfer.
    ///
    /// Permissionless and read-only: meant to be executed through `simulateTransaction`. Fails if
    /// the lane has no override of that type, which callers can read as "not overridden".
    pub fn observe_chain_config_override(
        ctx: Context<ObserveChainConfigOverride>,
        remote_chain_selector: u64,
        mint: Pubkey,
        override_type: PoolChainConfigOverrideType,
    ) -> Result<PoolChainConfigOverrideObservation> {
        observe::observe_chain_config_override(ctx, remote_chain_selector, mint, override_type)
    }

    /// Returns a pool's CCIP 2.0 CCV configuration for a single remote chain, for observability
    /// consumers.
    ///
    /// Permissionless and read-only: meant to be executed through `simulateTransaction`. Fails if
    /// the lane has no CCV config, which callers can read as "not configured".
    pub fn observe_chain_config_v2(
        ctx: Context<ObserveChainConfigV2>,
        remote_chain_selector: u64,
        mint: Pubkey,
    ) -> Result<PoolChainConfigV2Observation> {
        observe::observe_chain_config_v2(ctx, remote_chain_selector, mint)
    }

    // ==============================
    // Admin — Global Config
    // ==============================

    /// Initializes the program-level PoolConfig PDA.
    /// Must be called once by the program's upgrade authority before any
    /// token pool can be created.
    pub fn init_global_config(
        ctx: Context<InitGlobalConfig>,
        router_address: Pubkey,
        rmn_address: Pubkey,
    ) -> Result<()> {
        admin::init_global_config(ctx, router_address, rmn_address)
    }

    /// Enables or disables self-served pool initialization.
    /// When enabled, any mint authority may create a pool for their mint
    /// without requiring the program upgrade authority.
    pub fn update_self_served_allowed(
        ctx: Context<UpdateGlobalConfig>,
        self_served_allowed: bool,
    ) -> Result<()> {
        admin::update_self_served_allowed(ctx, self_served_allowed)
    }

    /// Updates the default router address stored in PoolConfig.
    /// New token pools created after this call will use the new router.
    pub fn update_default_router(
        ctx: Context<UpdateGlobalConfig>,
        router_address: Pubkey,
    ) -> Result<()> {
        admin::update_default_router(ctx, router_address)
    }

    /// Updates the default RMN Remote address stored in PoolConfig.
    /// New token pools created after this call will use the new RMN Remote.
    pub fn update_default_rmn(ctx: Context<UpdateGlobalConfig>, rmn_address: Pubkey) -> Result<()> {
        admin::update_default_rmn(ctx, rmn_address)
    }

    // ==============================
    // Token Pool Management
    // ==============================

    /// Creates a new per-mint State PDA for the given mint.
    /// The upgrade authority (or a mint authority when self-serve is enabled)
    /// becomes the pool owner.
    pub fn initialize(ctx: Context<InitializeTokenPool>) -> Result<()> {
        token_pool_management::initialize(ctx)
    }

    /// Creates a new per-mint State PDA for the given mint via a mint authority multisig signer.
    /// Self-serve must be enabled. Only 1-of-N multisigs are supported.
    pub fn initialize_by_multisig(ctx: Context<InitializeTokenPoolByMultisig>) -> Result<()> {
        token_pool_management::initialize_by_multisig(ctx)
    }

    /// Initiates an ownership transfer for this token pool to a new proposed owner.
    /// The proposed owner must call accept_ownership to complete the handover.
    pub fn transfer_ownership(ctx: Context<SetConfig>, proposed_owner: Pubkey) -> Result<()> {
        token_pool_management::transfer_ownership(ctx, proposed_owner)
    }

    /// Accepts a pending ownership transfer, making the caller the new pool owner.
    /// Shared function signature used by all CCIP token pool programs.
    pub fn accept_ownership(ctx: Context<AcceptOwnership>) -> Result<()> {
        token_pool_management::accept_ownership(ctx)
    }

    /// Updates the router address on an individual token pool's State.
    /// Changes the expected signer for lock/burn and release/mint calls.
    /// Only callable by the upgrade authority if they also own the pool.
    pub fn set_router(ctx: Context<AdminUpdateTokenPool>, new_router: Pubkey) -> Result<()> {
        token_pool_management::set_router(ctx, new_router)
    }

    /// Updates the RMN Remote address on an individual token pool's State.
    /// Only callable by the upgrade authority if they also own the pool.
    pub fn set_rmn(ctx: Context<AdminUpdateTokenPool>, rmn_address: Pubkey) -> Result<()> {
        token_pool_management::set_rmn(ctx, rmn_address)
    }

    /// Sets the rate-limit admin for a token pool.
    /// The rate-limit admin may call set_chain_rate_limit without being the pool owner.
    pub fn set_rate_limit_admin(
        ctx: Context<SetRateLimitAdmin>,
        mint: Pubkey,
        new_rate_limit_admin: Pubkey,
    ) -> Result<()> {
        token_pool_management::set_rate_limit_admin(ctx, mint, new_rate_limit_admin)
    }

    /// Adds addresses to the allow-list and/or enables the allow-list check.
    /// When enabled, only listed addresses may send tokens through the pool.
    pub fn configure_allow_list(
        ctx: Context<AddToAllowList>,
        add: Vec<Pubkey>,
        enabled: bool,
    ) -> Result<()> {
        token_pool_management::configure_allow_list(ctx, add, enabled)
    }

    /// Removes addresses from the allow-list.
    pub fn remove_from_allow_list(
        ctx: Context<RemoveFromAllowlist>,
        remove: Vec<Pubkey>,
    ) -> Result<()> {
        token_pool_management::remove_from_allow_list(ctx, remove)
    }

    // ==============================
    // Token Pool Chain Config
    // ==============================

    /// Initializes a ChainConfig PDA for the given remote chain.
    /// Must be called before any cross-chain messages can be sent to or from
    /// that chain. Remote pool addresses are added separately via
    /// append_remote_pool_addresses.
    pub fn init_chain_remote_config(
        ctx: Context<InitializeChainConfig>,
        remote_chain_selector: u64,
        mint: Pubkey,
        cfg: RemoteConfig,
    ) -> Result<()> {
        token_pool_chain_config::init_chain_remote_config(ctx, remote_chain_selector, mint, cfg)
    }

    /// Overwrites the remote config (token address and pool addresses) for an
    /// existing ChainConfig. Reallocates the PDA if the new config is larger.
    pub fn edit_chain_remote_config(
        ctx: Context<EditChainConfigDynamicSize>,
        remote_chain_selector: u64,
        mint: Pubkey,
        cfg: RemoteConfig,
    ) -> Result<()> {
        token_pool_chain_config::edit_chain_remote_config(ctx, remote_chain_selector, mint, cfg)
    }

    /// Appends additional remote pool addresses to an existing ChainConfig.
    /// Reallocates the PDA to accommodate the extra entries.
    pub fn append_remote_pool_addresses(
        ctx: Context<AppendRemotePoolAddresses>,
        remote_chain_selector: u64,
        mint: Pubkey,
        addresses: Vec<RemoteAddress>,
    ) -> Result<()> {
        token_pool_chain_config::append_remote_pool_addresses(
            ctx,
            remote_chain_selector,
            mint,
            addresses,
        )
    }

    /// Updates the inbound and outbound rate limits for a specific remote chain.
    /// Callable by either the pool owner or the rate-limit admin.
    pub fn set_chain_rate_limit(
        ctx: Context<SetChainRateLimit>,
        remote_chain_selector: u64,
        mint: Pubkey,
        inbound: RateLimitConfig,
        outbound: RateLimitConfig,
    ) -> Result<()> {
        token_pool_chain_config::set_chain_rate_limit(
            ctx,
            remote_chain_selector,
            mint,
            inbound,
            outbound,
        )
    }

    /// Closes and reclaims rent from a ChainConfig PDA.
    /// After this call no cross-chain messages can be sent to or from that chain.
    pub fn delete_chain_config(
        ctx: Context<DeleteChainConfig>,
        remote_chain_selector: u64,
        mint: Pubkey,
    ) -> Result<()> {
        token_pool_chain_config::delete_chain_config(ctx, remote_chain_selector, mint)
    }

    // Creates the PDA for the chain config V2
    pub fn init_chain_config_v2(
        ctx: Context<InitChainConfigV2>,
        params: SetChainConfigV2Params,
    ) -> Result<()> {
        token_pool_chain_config::init_chain_config_v2(ctx, params)
    }

    // Modifies the PDA for the chain config V2, resizing it to fit the new CCV lists
    pub fn edit_chain_config_v2(
        ctx: Context<EditChainConfigV2>,
        params: SetChainConfigV2Params,
    ) -> Result<()> {
        token_pool_chain_config::edit_chain_config_v2(ctx, params)
    }

    // ==============================
    // Token Pool Chain Config Override
    // ==============================

    /// Initializes a ChainConfigOverride PDA for the given remote chain and override type.
    /// Used to set separate rate limit buckets (e.g. for faster-than-finality transfers).
    pub fn init_chain_config_override(
        ctx: Context<InitializeChainConfigOverride>,
        params: InitChainConfigOverrideParams,
    ) -> Result<()> {
        token_pool_chain_config::init_chain_config_override(ctx, params)
    }

    /// Updates the rate limits on a ChainConfigOverride PDA.
    /// Callable by the pool owner or the rate-limit admin.
    pub fn set_chain_config_override_rate_limit(
        ctx: Context<SetChainConfigOverrideRateLimit>,
        params: SetChainConfigOverrideRateLimitParams,
    ) -> Result<()> {
        token_pool_chain_config::set_chain_config_override_rate_limit(ctx, params)
    }

    /// Closes and reclaims rent from a ChainConfigOverride PDA.
    pub fn delete_chain_config_override(
        ctx: Context<DeleteChainConfigOverride>,
        params: DeleteChainConfigOverrideParams,
    ) -> Result<()> {
        token_pool_chain_config::delete_chain_config_override(ctx, params)
    }

    // ===============================================
    // CCIP V1.6 Operations: Lock / Release
    // ===============================================

    /// Called by the router via CPI during `DeriveAccountsCcipSend` to discover
    /// any extra accounts needed for the onramp token transfer.
    /// Lockrelease pools have no additional account requirements beyond the
    /// standard token lookup table, so this returns an empty set.
    pub fn derive_accounts_lock_or_burn_tokens<'info>(
        ctx: Context<'info, Empty>,
        stage: String,
        lock_or_burn: LockOrBurnInV1,
    ) -> Result<DeriveAccountsResponse> {
        operations_v1::derive_accounts_lock_or_burn_tokens(ctx, stage, lock_or_burn)
    }

    /// Locks tokens from the sender's account as part of an outbound
    /// cross-chain transfer. Called by the router via CPI after the onramp
    /// has validated the message.
    pub fn lock_or_burn_tokens(
        ctx: Context<TokenOnramp>,
        lock_or_burn: LockOrBurnInV1,
    ) -> Result<LockOrBurnOutV1> {
        operations_v1::lock_tokens_v1(ctx, lock_or_burn)
    }

    /// Called by the offramp via CPI during `DeriveAccountsExecute` to discover
    /// any extra accounts needed for the offramp token transfer.
    /// Lockrelease pools have no additional account requirements beyond the
    /// standard token lookup table, so this returns an empty set.
    pub fn derive_accounts_release_or_mint_tokens<'info>(
        ctx: Context<'info, Empty>,
        stage: String,
        release_or_mint: ReleaseOrMintInV1,
    ) -> Result<DeriveAccountsResponse> {
        operations_v1::derive_accounts_release_or_mint_tokens(ctx, stage, release_or_mint)
    }

    /// Releases tokens to the receiver's account as part of an inbound
    /// cross-chain transfer. Called by the offramp via CPI after the
    /// message has been verified and committed.
    pub fn release_or_mint_tokens(
        ctx: Context<TokenOfframp>,
        release_or_mint: ReleaseOrMintInV1,
    ) -> Result<ReleaseOrMintOutV1> {
        operations_v1::release_tokens_v1(ctx, release_or_mint)
    }

    // ===============================================
    // CCIP V2.0 Operations: Lock / Release
    // ===============================================

    /// Resolves the accounts needed for V2 lock_or_burn or release_or_mint
    /// instructions based on the instruction discriminator.
    pub fn nested_resolve_accounts_start(
        ctx: Context<NestedResolveAccountsStart>,
        params: NestedResolveAccountsParams,
    ) -> Result<NestedResolveAccountsResponse> {
        operations_v2::nested_resolve_accounts_start(ctx, params)
    }

    /// Locks tokens in the pool as part of an outbound cross-chain transfer (V2).
    pub fn lock_or_burn_tokens_v2(
        ctx: Context<TokenOnrampV2>,
        lock_or_burn: LockOrBurnInV2,
    ) -> Result<LockOrBurnOutV2> {
        operations_v2::lock_tokens_v2(ctx, lock_or_burn)
    }

    /// Releases tokens to the receiver as part of an inbound cross-chain transfer (V2).
    pub fn release_or_mint_tokens_v2(
        ctx: Context<TokenOfframpV2>,
        release_or_mint: ReleaseOrMintInV2,
    ) -> Result<ReleaseOrMintOutV2> {
        operations_v2::release_tokens_v2(ctx, release_or_mint)
    }

    /// Returns the fee receipt for a token transfer.
    pub fn get_pool_fee(
        ctx: Context<GetFeeContext>,
        get_fee_in: GetFeeParams,
    ) -> Result<GetFeeResult> {
        operations_v2::get_fee(ctx, get_fee_in)
    }

    /// Sweeps the fees this pool accrued for `mint` in a single fee token into a recipient token
    /// account of the pool owner's choosing. Restricted to the pool owner.
    ///
    /// If `amount` is provided, only the requested amount is transferred. Otherwise (if None) the
    /// entire balance is transferred.
    pub fn withdraw_fee_tokens(ctx: Context<WithdrawFeeTokens>, amount: Option<u64>) -> Result<()> {
        token_pool_management::fee_withdrawal::withdraw_fee_tokens(ctx, amount)
    }

    pub fn get_required_ccvs(
        ctx: Context<GetRequiredCCVsContext>,
        get_required_ccvs_in: GetRequiredCCVsIn,
    ) -> Result<GetRequiredCCVsOut> {
        operations_v2::get_required_ccvs(ctx, get_required_ccvs_in)
    }

    // ==============================
    // Liquidity Management
    // ==============================

    /// Sets the rebalancer address for the pool.
    /// If the rebalancer is the default address then no rebalance is allowed.
    pub fn set_rebalancer(ctx: Context<SetConfig>, rebalancer: Pubkey) -> Result<()> {
        liquidity::set_rebalancer(ctx, rebalancer)
    }

    /// Rebalancer adds liquidity to the pool's token account.
    pub fn provide_liquidity(ctx: Context<RebalancerTokenTransfer>, amount: u64) -> Result<()> {
        liquidity::provide_liquidity(ctx, amount)
    }

    /// Rebalancer withdraws liquidity from the pool's token account.
    /// Can be used to transfer liquidity from one pool to another by setting
    /// the `rebalancer` to the calling pool.
    pub fn withdraw_liquidity(ctx: Context<RebalancerTokenTransfer>, amount: u64) -> Result<()> {
        liquidity::withdraw_liquidity(ctx, amount)
    }
}
