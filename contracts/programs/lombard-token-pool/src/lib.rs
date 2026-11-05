use anchor_lang::prelude::*;

use base_token_pool::observe::{
    PoolChainConfigObservation, PoolChainConfigOverrideObservation, PoolChainConfigV2Observation,
    PoolStateObservation,
};
use base_token_pool::rate_limiter::*;
use base_token_pool::state::{RemoteAddress, RemoteConfig};
use base_token_pool::v1::*;
use base_token_pool::v2::{
    GetFeeParams as GetPoolFeeParams, GetFeeResult as GetPoolFeeResult, LockOrBurnInV2,
    LockOrBurnOutV2, ReleaseOrMintInV2, ReleaseOrMintOutV2, SetChainConfigV2Params,
};
use ccip_common::messages::FinalityConfig;
use ccip_common::resolution::{
    NestedResolveAccountsParams, NestedResolveAccountsResponse, ResolveAccountsParams,
    ResolveAccountsResponse,
};
use ccip_common::state::PoolChainConfigOverrideType;
use ccip_common::types::ByteString;

pub mod constants;
pub mod context;
pub mod errors;
pub mod events;
pub mod instructions;
pub mod lombard_cpi;
pub mod security;
pub mod state;

pub use lombard_cpi::bridge;
pub use lombard_cpi::mailbox;

use instructions::*;

use crate::{
    context::*,
    state::{LombardChain, RemoteAdapter, SupportedToken},
};

#[cfg(feature = "mainnet")]
declare_id!("Lomb8TTCwJKEhZrJ1J8UbsCRjNSf7NNQUEr4qo3dkSk");
#[cfg(feature = "gastald")]
declare_id!("Lomi5fKsXdGrrW2M3JRbJx5DnT3zgmSEfYWMUPeNaAB");
#[cfg(feature = "staging")]
declare_id!("LomtioA14cDhme8bCCw5oc5a9FUDyT91z8ujtGnY5g9");
#[cfg(feature = "bft")]
declare_id!("LomdWAg9hHyz3VrvK5wXTap7o348Ku2QJ2j2H8Etj3C");
#[cfg(any(
    feature = "localnet",
    not(any(
        feature = "mainnet",
        feature = "gastald",
        feature = "staging",
        feature = "bft"
    ))
))]
declare_id!("51HDypJbcZ1bmqh4v16X2KaHcvc53fYi84rbb21VoN4t");

pub const RECEIVE_MESSAGE_DISCRIMINATOR: [u8; 8] = [38, 144, 127, 225, 31, 225, 238, 25]; // global:receive_message
pub const DEPOSIT_FOR_BURN_WITH_CALLER_DISCRIMINATOR: [u8; 8] =
    [167, 222, 19, 114, 85, 21, 14, 118]; // global:deposit_for_burn_with_caller
pub const RECLAIM_EVENT_ACCOUNT_DISCRIMINATOR: [u8; 8] = [94, 198, 180, 159, 131, 236, 15, 174]; // global:reclaim_event_account
pub const TOKEN_POOL_TYPE_AND_VERSION: &str = "lombard-token-pool 1.0.0";

#[program]
pub mod lombard_token_pool {
    use super::*;

    pub fn init_global_config(ctx: Context<InitGlobalConfig>) -> Result<()> {
        instructions::init_global_config(ctx)
    }

    pub fn initialize(
        ctx: Context<InitializeTokenPool>,
        router: Pubkey,
        rmn_remote: Pubkey,
        bridge: Pubkey,
    ) -> Result<()> {
        instructions::initialize(ctx, router, rmn_remote, bridge)
    }

    /// Returns the program type (name) and version.
    /// Used by offchain code to easily determine which program & version is being interacted with.
    ///
    /// # Arguments
    /// * `ctx` - The context
    pub fn type_version(_ctx: Context<Empty>) -> Result<String> {
        msg!("{}", TOKEN_POOL_TYPE_AND_VERSION.to_string());
        Ok(TOKEN_POOL_TYPE_AND_VERSION.to_string())
    }

    /// Returns a pool's per-mint configuration, for observability consumers.
    pub fn observe_state(ctx: Context<ObserveState>, mint: Pubkey) -> Result<PoolStateObservation> {
        instructions::observe_state(ctx, mint)
    }

    /// Returns a pool's configuration for a single remote chain, for observability consumers.
    /// Rate limiter buckets are reported as of now, i.e. including the refill accrued since the
    /// last transfer.
    pub fn observe_chain_config(
        ctx: Context<ObserveChainConfig>,
        remote_chain_selector: u64,
        mint: Pubkey,
    ) -> Result<PoolChainConfigObservation> {
        instructions::observe_chain_config(ctx, remote_chain_selector, mint)
    }

    /// Returns a pool's rate limit override for a single remote chain and override type, for
    /// observability consumers. Rate limiter buckets are reported as of now, i.e. including the
    /// refill accrued since the last transfer.
    pub fn observe_chain_config_override(
        ctx: Context<ObserveChainConfigOverride>,
        remote_chain_selector: u64,
        mint: Pubkey,
        override_type: PoolChainConfigOverrideType,
    ) -> Result<PoolChainConfigOverrideObservation> {
        instructions::observe_chain_config_override(ctx, remote_chain_selector, mint, override_type)
    }

    /// Returns a pool's CCIP 2.0 CCV configuration for a single remote chain, for observability
    /// consumers.
    pub fn observe_chain_config_v2(
        ctx: Context<ObserveChainConfigV2>,
        remote_chain_selector: u64,
        mint: Pubkey,
    ) -> Result<PoolChainConfigV2Observation> {
        instructions::observe_chain_config_v2(ctx, remote_chain_selector, mint)
    }

    pub fn transfer_ownership(ctx: Context<SetConfig>, proposed_owner: Pubkey) -> Result<()> {
        ctx.accounts.state.config.transfer_ownership(proposed_owner)
    }

    // shared func signature with other programs
    pub fn accept_ownership(ctx: Context<AcceptOwnership>) -> Result<()> {
        instructions::accept_ownership(ctx)
    }

    // set_router changes the expected signers for mint/release + burn/lock method calls
    // this is used to update the router address
    pub fn set_router(ctx: Context<AdminUpdateTokenPool>, new_router: Pubkey) -> Result<()> {
        instructions::set_router(ctx, new_router)
    }

    pub fn set_rmn(ctx: Context<AdminUpdateTokenPool>, rmn_address: Pubkey) -> Result<()> {
        instructions::set_rmn(ctx, rmn_address)
    }

    pub fn set_alt(ctx: Context<SetConfig>, alt: Option<Pubkey>) -> Result<()> {
        instructions::set_alt(ctx, alt)
    }

    // initialize remote config (with no remote pools as it must be zero sized)
    pub fn init_chain_remote_config(
        ctx: Context<InitializeChainConfig>,
        remote_chain_selector: u64,
        mint: Pubkey,
        cfg: RemoteConfig,
        dest_chain_id: [u8; 32],
        dest_caller: [u8; 32],
    ) -> Result<()> {
        instructions::init_chain_remote_config(
            ctx,
            remote_chain_selector,
            mint,
            cfg,
            dest_chain_id,
            dest_caller,
        )
    }

    // edit remote config's CCIP values
    pub fn edit_chain_remote_config(
        ctx: Context<EditChainConfigDynamicSize>,
        remote_chain_selector: u64,
        mint: Pubkey,
        cfg: RemoteConfig,
    ) -> Result<()> {
        instructions::edit_chain_remote_config(ctx, remote_chain_selector, mint, cfg)
    }

    // edit remote config's values that are specific to Lombard
    pub fn edit_chain_remote_config_lombard(
        ctx: Context<EditChainConfig>,
        remote_chain_selector: u64,
        mint: Pubkey,
        cfg: LombardChain,
    ) -> Result<()> {
        instructions::edit_chain_remote_config_lombard(ctx, remote_chain_selector, mint, cfg)
    }

    // Add remote pool addresses
    pub fn append_remote_pool_addresses(
        ctx: Context<AppendRemotePoolAddresses>,
        remote_chain_selector: u64,
        mint: Pubkey,
        addresses: Vec<RemoteAddress>,
    ) -> Result<()> {
        instructions::append_remote_pool_addresses(ctx, remote_chain_selector, mint, addresses)
    }

    // set rate limit
    pub fn set_chain_rate_limit(
        ctx: Context<SetChainRateLimit>,
        remote_chain_selector: u64,
        mint: Pubkey,
        inbound: RateLimitConfig,
        outbound: RateLimitConfig,
    ) -> Result<()> {
        instructions::set_chain_rate_limit(ctx, remote_chain_selector, mint, inbound, outbound)
    }

    // set rate limit admin
    pub fn set_rate_limit_admin(
        ctx: Context<SetRateLimitAdmin>,
        mint: Pubkey,
        new_rate_limit_admin: Pubkey,
    ) -> Result<()> {
        instructions::set_rate_limit_admin(ctx, mint, new_rate_limit_admin)
    }

    // delete chain config
    pub fn delete_chain_config(
        ctx: Context<DeleteChainConfig>,
        remote_chain_selector: u64,
        mint: Pubkey,
    ) -> Result<()> {
        instructions::delete_chain_config(ctx, remote_chain_selector, mint)
    }

    pub fn configure_allow_list(
        ctx: Context<AddToAllowList>,
        add: Vec<Pubkey>,
        enabled: bool,
    ) -> Result<()> {
        instructions::configure_allow_list(ctx, add, enabled)
    }

    pub fn remove_from_allow_list(
        ctx: Context<RemoveFromAllowlist>,
        remove: Vec<Pubkey>,
    ) -> Result<()> {
        instructions::remove_from_allow_list(ctx, remove)
    }

    pub fn release_or_mint_tokens(
        ctx: Context<TokenOfframp>,
        release_or_mint: ReleaseOrMintInV1,
    ) -> Result<ReleaseOrMintOutV1> {
        instructions::release_or_mint_tokens(ctx, release_or_mint)
    }

    pub fn derive_accounts_release_or_mint_tokens<'info>(
        ctx: Context<'info, Empty>,
        stage: String,
        release_or_mint: ReleaseOrMintInV1,
    ) -> Result<DeriveAccountsResponse> {
        instructions::derive_accounts_release_or_mint_tokens(ctx, stage, release_or_mint)
    }

    pub fn lock_or_burn_tokens(
        ctx: Context<TokenOnramp>,
        lock_or_burn: LockOrBurnInV1,
    ) -> Result<LockOrBurnOutV1> {
        instructions::lock_or_burn_tokens(ctx, lock_or_burn)
    }

    pub fn derive_accounts_lock_or_burn_tokens<'info>(
        ctx: Context<'info, Empty>,
        stage: String,
        lock_or_burn: LockOrBurnInV1,
    ) -> Result<DeriveAccountsResponse> {
        instructions::derive_accounts_lock_or_burn_tokens(ctx, stage, lock_or_burn)
    }

    // --- Lombard verifier (embedded CCV): admin & config ---

    pub fn initialize_verifier<'info>(
        ctx: Context<'info, InitializeVerifier<'info>>,
    ) -> Result<()> {
        instructions::initialize_verifier(ctx)
    }

    pub fn transfer_verifier_ownership(
        ctx: Context<UpdateVerifierConfig>,
        proposed_owner: Pubkey,
    ) -> Result<()> {
        instructions::transfer_verifier_ownership(ctx, proposed_owner)
    }

    pub fn accept_verifier_ownership(ctx: Context<AcceptVerifierOwnership>) -> Result<()> {
        instructions::accept_verifier_ownership(ctx)
    }

    pub fn transfer_verifier_storage_location_admin(
        ctx: Context<UpdateVerifierConfig>,
        proposed_storage_location_admin: Pubkey,
    ) -> Result<()> {
        instructions::transfer_verifier_storage_location_admin(ctx, proposed_storage_location_admin)
    }

    pub fn accept_verifier_storage_location_admin(
        ctx: Context<AcceptVerifierStorageLocationAdmin>,
    ) -> Result<()> {
        instructions::accept_verifier_storage_location_admin(ctx)
    }

    pub fn set_verifier_allowlist_admin(
        ctx: Context<UpdateVerifierConfig>,
        allowlist_admin: Pubkey,
    ) -> Result<()> {
        instructions::set_verifier_allowlist_admin(ctx, allowlist_admin)
    }

    pub fn set_verifier_fee_aggregator(
        ctx: Context<UpdateVerifierConfig>,
        fee_aggregator: Pubkey,
    ) -> Result<()> {
        instructions::set_verifier_fee_aggregator(ctx, fee_aggregator)
    }

    pub fn set_verifier_lookup_table(
        ctx: Context<SetVerifierLookupTable>,
        remote_chain_selector: u64,
        lookup_table: Option<Pubkey>,
    ) -> Result<()> {
        instructions::set_verifier_lookup_table(ctx, remote_chain_selector, lookup_table)
    }

    pub fn set_verifier_allowed_finality_config(
        ctx: Context<UpdateVerifierConfig>,
        finality: FinalityConfig,
    ) -> Result<()> {
        instructions::set_verifier_allowed_finality_config(ctx, finality)
    }

    /// Creates the verifier configuration for a remote chain and sets its admin configuration.
    /// Fails if the remote chain is already configured; use `edit_verifier_chain_config` instead.
    pub fn init_verifier_chain_config(
        ctx: Context<InitVerifierChainConfig>,
        params: SetVerifierChainConfigParams,
    ) -> Result<()> {
        instructions::init_verifier_chain_config(ctx, params)
    }

    /// Overwrites the admin configuration of an already configured remote chain, leaving the
    /// rest of its configuration untouched.
    pub fn edit_verifier_chain_config(
        ctx: Context<EditVerifierChainConfig>,
        params: SetVerifierChainConfigParams,
    ) -> Result<()> {
        instructions::edit_verifier_chain_config(ctx, params)
    }

    pub fn set_verifier_allowlist(
        ctx: Context<SetVerifierAllowlist>,
        params: SetVerifierAllowlistParams,
    ) -> Result<()> {
        instructions::set_verifier_allowlist(ctx, params)
    }

    pub fn set_verifier_storage_locations(
        ctx: Context<SetVerifierStorageLocations>,
        params: SetVerifierStorageLocationsParams,
    ) -> Result<()> {
        instructions::set_verifier_storage_locations(ctx, params)
    }

    pub fn get_verifier_storage_locations(
        ctx: Context<GetVerifierStorageLocations>,
    ) -> Result<Vec<ByteString>> {
        instructions::get_verifier_storage_locations(ctx)
    }

    pub fn set_verifier_path(
        ctx: Context<SetVerifierPath>,
        remote_chain_selector: u64,
        l_chain_id: [u8; 32],
        allowed_caller: [u8; 32],
        remote_bridge_sender: [u8; 32],
    ) -> Result<()> {
        instructions::set_verifier_path(
            ctx,
            remote_chain_selector,
            l_chain_id,
            allowed_caller,
            remote_bridge_sender,
        )
    }

    pub fn remove_verifier_path(
        ctx: Context<SetVerifierPath>,
        remote_chain_selector: u64,
    ) -> Result<()> {
        instructions::remove_verifier_path(ctx, remote_chain_selector)
    }

    pub fn set_verifier_supported_tokens(
        ctx: Context<SetVerifierSupportedTokens>,
        tokens_to_remove: Vec<Pubkey>,
        tokens_to_set: Vec<SupportedToken>,
    ) -> Result<()> {
        instructions::set_verifier_supported_tokens(ctx, tokens_to_remove, tokens_to_set)
    }

    pub fn set_verifier_remote_adapters(
        ctx: Context<SetVerifierRemoteAdapters>,
        remote_chain_selector: u64,
        remote_adapters: Vec<RemoteAdapter>,
    ) -> Result<()> {
        instructions::set_verifier_remote_adapters(ctx, remote_chain_selector, remote_adapters)
    }

    /// Sweeps this verifier's accrued fees for one fee token into the fee aggregator's ATA.
    ///
    /// Permissionless: the destination is pinned to `config.fee_aggregator`.
    pub fn withdraw_verifier_fee_tokens(ctx: Context<WithdrawVerifierFeeTokens>) -> Result<()> {
        instructions::withdraw_verifier_fee_tokens(ctx)
    }

    // --- Pool v2 ---

    pub fn lock_or_burn_tokens_v2(
        ctx: Context<TokenOnrampV2>,
        lock_or_burn: LockOrBurnInV2,
    ) -> Result<LockOrBurnOutV2> {
        instructions::lock_or_burn_tokens_v2(ctx, lock_or_burn)
    }

    pub fn release_or_mint_tokens_v2(
        ctx: Context<TokenOfframpV2>,
        release_or_mint: ReleaseOrMintInV2,
    ) -> Result<ReleaseOrMintOutV2> {
        instructions::release_or_mint_tokens_v2(ctx, release_or_mint)
    }

    // Creates the PDA for the chain config V2
    pub fn init_chain_config_v2(
        ctx: Context<InitChainConfigV2>,
        params: SetChainConfigV2Params,
    ) -> Result<()> {
        instructions::init_chain_config_v2(ctx, params)
    }

    // Modifies the PDA for the chain config V2, resizing it to fit the new CCV lists
    pub fn edit_chain_config_v2(
        ctx: Context<EditChainConfigV2>,
        params: SetChainConfigV2Params,
    ) -> Result<()> {
        instructions::edit_chain_config_v2(ctx, params)
    }

    pub fn init_chain_config_override(
        ctx: Context<InitializeChainConfigOverride>,
        params: InitChainConfigOverrideParams,
    ) -> Result<()> {
        instructions::init_chain_config_override(ctx, params)
    }

    pub fn set_chain_config_override_rate_limit(
        ctx: Context<SetChainConfigOverrideRateLimit>,
        params: SetChainConfigOverrideRateLimitParams,
    ) -> Result<()> {
        instructions::set_chain_config_override_rate_limit(ctx, params)
    }

    pub fn delete_chain_config_override(
        ctx: Context<DeleteChainConfigOverride>,
        params: DeleteChainConfigOverrideParams,
    ) -> Result<()> {
        instructions::delete_chain_config_override(ctx, params)
    }

    /// Sweeps the fees this pool accrued for `mint` in a single fee token into a recipient token
    /// account of the pool owner's choosing. Restricted to the pool owner.
    ///
    /// If `amount` is provided, only the requested amount is transferred. Otherwise (if None) the
    /// entire balance is transferred.
    pub fn withdraw_pool_fee_tokens(
        ctx: Context<WithdrawPoolFeeTokens>,
        amount: Option<u64>,
    ) -> Result<()> {
        instructions::withdraw_pool_fee_tokens(ctx, amount)
    }

    // --- get_pool_fee (pool role) / get_verifier_fee (verifier role) ---

    pub fn get_pool_fee(
        ctx: Context<GetPoolFeeContext>,
        get_fee_in: GetPoolFeeParams,
    ) -> Result<GetPoolFeeResult> {
        instructions::get_pool_fee(ctx, get_fee_in)
    }

    pub fn get_verifier_fee(
        ctx: Context<GetVerifierFee>,
        params: VerifierGetFeeParams,
    ) -> Result<VerifierGetFeeResult> {
        instructions::get_verifier_fee(ctx, params)
    }

    pub fn get_required_ccvs(
        ctx: Context<GetRequiredCCVsContext>,
        get_required_ccvs_in: GetRequiredCCVsIn,
    ) -> Result<GetRequiredCCVsOut> {
        instructions::get_required_ccvs(ctx, get_required_ccvs_in)
    }

    // --- Verifier core: message-ID binding ---

    pub fn start_verification_at_source(
        ctx: Context<StartVerificationAtSource>,
        params: StartVerificationAtSourceParams,
    ) -> Result<Vec<u8>> {
        instructions::start_verification_at_source(ctx, params)
    }

    pub fn verify(ctx: Context<Verify>, params: VerifyParams) -> Result<()> {
        instructions::verify(ctx, params)
    }

    // --- CCIP 2.0 account resolution ---

    pub fn resolve_accounts_start(
        ctx: Context<TopLevelResolveAccountsStart>,
        params: ResolveAccountsParams,
    ) -> Result<ResolveAccountsResponse> {
        instructions::top_level_resolution::resolve_accounts_start(ctx, params)
    }

    /// `instructions/nested_resolution.rs`.
    pub fn nested_resolve_accounts_start(
        ctx: Context<NestedResolveAccountsStart>,
        params: NestedResolveAccountsParams,
    ) -> Result<NestedResolveAccountsResponse> {
        instructions::nested_resolve_accounts_start(ctx, params)
    }
}
