use anchor_lang::prelude::*;

pub mod security;

use base_token_pool::observe::*;
use base_token_pool::rate_limiter::*;
use base_token_pool::state::*;
use base_token_pool::v1::*;
use base_token_pool::v2::*;

use ccip_common::ccvs::{GetRequiredCCVsIn, GetRequiredCCVsOut};
use ccip_common::resolution::{NestedResolveAccountsParams, NestedResolveAccountsResponse};
use ccip_common::state::PoolChainConfigOverrideType;
use ccip_common::v2::GetVerifierFeeParams;

declare_id!("CCiTPESGEevd7TBU8EGBKrcxuRq7jx3YtW6tPidnscaZ");

mod error;
mod nested_resolution;
mod utils;
use crate::nested_resolution::*;

mod type_version;
use crate::type_version::*;

mod pool;
use crate::pool::instructions::{
    observe::*, operations_v1::*, operations_v2::*, token_pool_chain_config::*,
    token_pool_management::*,
};
use crate::pool::state::CctpChain;

mod verifier;
use crate::verifier::instructions::*;

// Circle's CCTP domain ID for Solana is always 5, see https://developers.circle.com/stablecoins/supported-domains
pub(crate) const SOLANA_DOMAIN_ID: u32 = 5;

#[program]
pub mod cctp_token_pool {

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
        pool::instructions::observe::observe_state(ctx, mint)
    }

    /// Returns a pool's configuration for a single remote chain, for observability consumers.
    /// Rate limiter buckets are reported as of now, i.e. including the refill accrued since the
    /// last transfer.
    ///
    /// Permissionless and read-only: meant to be executed through `simulateTransaction`. Fails if
    /// the lane has no chain config, which callers can read as "not configured".
    pub fn observe_chain_config(
        ctx: Context<ObserveChainConfig>,
        remote_chain_selector: u64,
        mint: Pubkey,
    ) -> Result<PoolChainConfigObservation> {
        pool::instructions::observe::observe_chain_config(ctx, remote_chain_selector, mint)
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
        pool::instructions::observe::observe_chain_config_override(
            ctx,
            remote_chain_selector,
            mint,
            override_type,
        )
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
        pool::instructions::observe::observe_chain_config_v2(ctx, remote_chain_selector, mint)
    }

    // ==============================
    // Token Pool Management
    // ==============================

    /// Creates a new per-mint State PDA for the given mint.
    /// The upgrade authority (or a mint authority when self-serve is enabled)
    /// becomes the pool owner.
    pub fn initialize(
        ctx: Context<InitializeTokenPool>,
        router: Pubkey,
        rmn_remote: Pubkey,
    ) -> Result<()> {
        pool::instructions::token_pool_management::initialize(ctx, router, rmn_remote)
    }

    /// Initiates an ownership transfer for this token pool to a new proposed owner.
    /// The proposed owner must call accept_ownership to complete the handover.
    pub fn transfer_ownership(
        ctx: Context<TransferOwnership>,
        proposed_owner: Pubkey,
    ) -> Result<()> {
        pool::instructions::token_pool_management::transfer_ownership(ctx, proposed_owner)
    }

    /// Accepts a pending ownership transfer, making the caller the new pool owner.
    /// Shared function signature used by all CCIP token pool programs.
    pub fn accept_ownership(ctx: Context<AcceptOwnership>) -> Result<()> {
        pool::instructions::token_pool_management::accept_ownership(ctx)
    }

    /// Sets a new authority who can reclaim funds and close old event PDAs.
    pub fn set_fund_manager(ctx: Context<SetFundConfig>, fund_manager: Pubkey) -> Result<()> {
        pool::instructions::token_pool_management::set_fund_manager(ctx, fund_manager)
    }

    /// Sets a threshold for the minimum amount of SOL that must remain in the pool's signer account.
    /// When withdrawing SOL the pool signer account cannot drop below this threshold.
    pub fn set_minimum_signer_funds(ctx: Context<SetFundConfig>, amount: u64) -> Result<()> {
        pool::instructions::token_pool_management::set_minimum_signer_funds(ctx, amount)
    }

    /// Sets a new destination to which SOL can be transferred (withdrawn) from the pool's signer account.
    pub fn set_fund_reclaim_destination(
        ctx: Context<SetFundConfig>,
        fund_reclaim_destination: Pubkey,
    ) -> Result<()> {
        pool::instructions::token_pool_management::set_fund_reclaim_destination(
            ctx,
            fund_reclaim_destination,
        )
    }

    /// Updates the router address on an individual token pool's State.
    /// Changes the expected signer for lock/burn and release/mint calls.
    /// Only callable by the upgrade authority if they also own the pool.
    pub fn set_router(ctx: Context<AdminUpdateTokenPool>, new_router: Pubkey) -> Result<()> {
        pool::instructions::token_pool_management::set_router(ctx, new_router)
    }

    /// Updates the RMN Remote address on an individual token pool's State.
    /// Only callable by the upgrade authority if they also own the pool.
    pub fn set_rmn(ctx: Context<AdminUpdateTokenPool>, rmn_address: Pubkey) -> Result<()> {
        pool::instructions::token_pool_management::set_rmn(ctx, rmn_address)
    }

    /// Sets the rate-limit admin for a token pool.
    /// The rate-limit admin may call set_chain_rate_limit without being the pool owner.
    pub fn set_rate_limit_admin(
        ctx: Context<SetRateLimitAdmin>,
        mint: Pubkey,
        new_rate_limit_admin: Pubkey,
    ) -> Result<()> {
        pool::instructions::token_pool_management::set_rate_limit_admin(
            ctx,
            mint,
            new_rate_limit_admin,
        )
    }

    /// Adds addresses to the allow-list and/or enables the allow-list check.
    /// When enabled, only listed addresses may send tokens through the pool.
    pub fn configure_allow_list(
        ctx: Context<AddToAllowList>,
        add: Vec<Pubkey>,
        enabled: bool,
    ) -> Result<()> {
        pool::instructions::token_pool_management::configure_allow_list(ctx, add, enabled)
    }

    /// Removes addresses from the allow-list.
    pub fn remove_from_allow_list(
        ctx: Context<RemoveFromAllowlist>,
        remove: Vec<Pubkey>,
    ) -> Result<()> {
        pool::instructions::token_pool_management::remove_from_allow_list(ctx, remove)
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
        pool::instructions::token_pool_management::withdraw_pool_fee_tokens(ctx, amount)
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
        pool::instructions::token_pool_chain_config::init_chain_remote_config(
            ctx,
            remote_chain_selector,
            mint,
            cfg,
        )
    }

    /// Overwrites the remote config (token address and pool addresses) for an
    /// existing ChainConfig. Reallocates the PDA if the new config is larger.
    pub fn edit_chain_remote_config(
        ctx: Context<EditChainConfigDynamicSize>,
        remote_chain_selector: u64,
        mint: Pubkey,
        cfg: RemoteConfig,
    ) -> Result<()> {
        pool::instructions::token_pool_chain_config::edit_chain_remote_config(
            ctx,
            remote_chain_selector,
            mint,
            cfg,
        )
    }

    /// Edits remote config values that are specific to CCTP.
    pub fn edit_chain_remote_config_cctp(
        ctx: Context<EditChainConfig>,
        remote_chain_selector: u64,
        mint: Pubkey,
        cfg: CctpChain,
    ) -> Result<()> {
        pool::instructions::token_pool_chain_config::edit_chain_remote_config_cctp(
            ctx,
            remote_chain_selector,
            mint,
            cfg,
        )
    }

    /// Appends additional remote pool addresses to an existing ChainConfig.
    /// Reallocates the PDA to accommodate the extra entries.
    pub fn append_remote_pool_addresses(
        ctx: Context<AppendRemotePoolAddresses>,
        remote_chain_selector: u64,
        mint: Pubkey,
        addresses: Vec<RemoteAddress>,
    ) -> Result<()> {
        pool::instructions::token_pool_chain_config::append_remote_pool_addresses(
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
        pool::instructions::token_pool_chain_config::set_chain_rate_limit(
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
        pool::instructions::token_pool_chain_config::delete_chain_config(
            ctx,
            remote_chain_selector,
            mint,
        )
    }

    // Creates the PDA for the chain config V2
    pub fn init_chain_config_v2(
        ctx: Context<InitChainConfigV2>,
        params: SetChainConfigV2Params,
    ) -> Result<()> {
        pool::instructions::token_pool_chain_config::init_chain_config_v2(ctx, params)
    }

    // Modifies the PDA for the chain config V2, resizing it to fit the new CCV lists
    pub fn edit_chain_config_v2(
        ctx: Context<EditChainConfigV2>,
        params: SetChainConfigV2Params,
    ) -> Result<()> {
        pool::instructions::token_pool_chain_config::edit_chain_config_v2(ctx, params)
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
        pool::instructions::token_pool_chain_config::init_chain_config_override(ctx, params)
    }

    /// Updates the rate limits on a ChainConfigOverride PDA.
    /// Callable by the pool owner or the rate-limit admin.
    pub fn set_chain_config_override_rate_limit(
        ctx: Context<SetChainConfigOverrideRateLimit>,
        params: SetChainConfigOverrideRateLimitParams,
    ) -> Result<()> {
        pool::instructions::token_pool_chain_config::set_chain_config_override_rate_limit(
            ctx, params,
        )
    }

    /// Closes and reclaims rent from a ChainConfigOverride PDA.
    pub fn delete_chain_config_override(
        ctx: Context<DeleteChainConfigOverride>,
        params: DeleteChainConfigOverrideParams,
    ) -> Result<()> {
        pool::instructions::token_pool_chain_config::delete_chain_config_override(ctx, params)
    }

    // ===============================================
    // CCIP V1.6 Operations: Burn / Mint
    // ===============================================

    /// Mints tokens to the receiver's account as part of an inbound
    /// cross-chain transfer. Called by the offramp via CPI after the
    /// message has been verified and committed. Relies in CCTPv1 to do the actual minting.
    pub fn release_or_mint_tokens<'info>(
        ctx: Context<'info, TokenOfframp<'info>>,
        release_or_mint: ReleaseOrMintInV1,
    ) -> Result<ReleaseOrMintOutV1> {
        pool::instructions::operations_v1::mint_tokens(ctx, release_or_mint)
    }

    /// Burns tokens from the sender's account as part of an outbound
    /// cross-chain transfer. Called by the router via CPI after the onramp
    /// has validated the message. Relies on CCTPv1 to do the actual burning.
    pub fn lock_or_burn_tokens(
        ctx: Context<TokenOnramp>,
        lock_or_burn: LockOrBurnInV1,
    ) -> Result<LockOrBurnOutV1> {
        pool::instructions::operations_v1::burn_tokens(ctx, lock_or_burn)
    }

    /// Closes an old CCTPv1 event PDA and reclaims its rent.
    pub fn reclaim_event_account(
        ctx: Context<ReclaimEventAccount>,
        mint: Pubkey,
        original_sender: Pubkey,
        remote_chain_selector: u64,
        msg_nonce: u64,
        attestation: Vec<u8>,
    ) -> Result<()> {
        pool::instructions::token_pool_management::reclaim_event_account(
            ctx,
            mint,
            original_sender,
            remote_chain_selector,
            msg_nonce,
            attestation,
        )
    }

    /// Returns an amount of SOL from the pool signer account to the designated
    /// fund reclaimer. There are three entities involved:
    ///
    /// * `owner`: can configure the reclaimer and fund manager.
    /// * `fund_manager`: can execute this instruction.
    /// * `fund_reclaim_destination`: receives the funds.
    ///
    /// The resulting funds on the PDA cannot drop below `minimum_signer_funds`.
    pub fn reclaim_funds(ctx: Context<ReclaimFunds>, amount: u64) -> Result<()> {
        pool::instructions::token_pool_management::reclaim_funds(ctx, amount)
    }

    /// Called by the offramp via CPI during `DeriveAccountsExecute` to discover
    /// any extra accounts needed for the offramp token transfer, of which there
    /// are many over more than one round derivation.
    pub fn derive_accounts_release_or_mint_tokens(
        ctx: Context<Empty>,
        stage: String,
        release_or_mint: ReleaseOrMintInV1,
    ) -> Result<DeriveAccountsResponse> {
        pool::instructions::operations_v1::derive_accounts_release_or_mint_tokens(
            ctx,
            stage,
            release_or_mint,
        )
    }

    /// Called by the router via CPI during `DeriveAccountsCcipSend` to discover
    /// any extra accounts needed for the onramp token transfer, of which there
    /// are many over more than one round derivation.
    pub fn derive_accounts_lock_or_burn_tokens(
        ctx: Context<Empty>,
        stage: String,
        lock_or_burn: LockOrBurnInV1,
    ) -> Result<DeriveAccountsResponse> {
        pool::instructions::operations_v1::derive_accounts_lock_or_burn_tokens(
            ctx,
            stage,
            lock_or_burn,
        )
    }

    // ===============================================
    // CCIP V2.0 PoolV2 Operations: Burn / Mint
    // ===============================================

    /// Burns tokens from the pool as part of an outbound cross-chain transfer (V2).
    pub fn lock_or_burn_tokens_v2(
        ctx: Context<TokenOnrampV2>,
        lock_or_burn: LockOrBurnInV2,
    ) -> Result<LockOrBurnOutV2> {
        pool::instructions::operations_v2::burn_tokens_v2(ctx, lock_or_burn)
    }

    /// Mints tokens to the receiver as part of an inbound cross-chain transfer (V2).
    pub fn release_or_mint_tokens_v2<'info>(
        ctx: Context<'info, TokenOfframpV2<'info>>,
        release_or_mint: ReleaseOrMintInV2,
    ) -> Result<ReleaseOrMintOutV2> {
        pool::instructions::operations_v2::mint_tokens_v2(ctx, release_or_mint)
    }

    /// Returns the fee receipt for a token transfer.
    pub fn get_pool_fee(
        ctx: Context<GetFeeContext>,
        get_fee_in: GetFeeParams,
    ) -> Result<GetFeeResult> {
        pool::instructions::operations_v2::get_fee(ctx, get_fee_in)
    }

    /// This implementation returns the CctpVerifier only, for any message
    pub fn get_required_ccvs(
        ctx: Context<GetRequiredCCVsContext>,
        get_required_ccvs_in: GetRequiredCCVsIn,
    ) -> Result<GetRequiredCCVsOut> {
        pool::instructions::operations_v2::get_required_ccvs(ctx, get_required_ccvs_in)
    }

    // ===============================================
    // CCTP verifier (embedded CCV): admin & config
    // ===============================================

    pub fn initialize_verifier<'info>(
        ctx: Context<'info, InitializeVerifier<'info>>,
    ) -> Result<()> {
        verifier::instructions::initialize_verifier(ctx)
    }

    pub fn transfer_verifier_ownership(
        ctx: Context<UpdateVerifierConfig>,
        proposed_owner: Pubkey,
    ) -> Result<()> {
        verifier::instructions::admin::transfer_verifier_ownership(ctx, proposed_owner)
    }

    pub fn accept_verifier_ownership(ctx: Context<AcceptVerifierOwnership>) -> Result<()> {
        verifier::instructions::admin::accept_verifier_ownership(ctx)
    }

    pub fn transfer_verifier_storage_location_admin(
        ctx: Context<UpdateVerifierStorageLocationAdmin>,
        proposed_storage_location_admin: Pubkey,
    ) -> Result<()> {
        verifier::instructions::admin::transfer_verifier_storage_location_admin(
            ctx,
            proposed_storage_location_admin,
        )
    }

    pub fn accept_verifier_storage_location_admin(
        ctx: Context<AcceptVerifierStorageLocationAdmin>,
    ) -> Result<()> {
        verifier::instructions::admin::accept_verifier_storage_location_admin(ctx)
    }

    pub fn set_verifier_allowlist_admin(
        ctx: Context<UpdateVerifierConfig>,
        allowlist_admin: Pubkey,
    ) -> Result<()> {
        verifier::instructions::admin::set_verifier_allowlist_admin(ctx, allowlist_admin)
    }

    pub fn set_verifier_fee_aggregator(
        ctx: Context<UpdateVerifierConfig>,
        fee_aggregator: Pubkey,
    ) -> Result<()> {
        verifier::instructions::admin::set_verifier_fee_aggregator(ctx, fee_aggregator)
    }

    pub fn set_verifier_allowed_finality_config(
        ctx: Context<UpdateVerifierConfig>,
        finality: ccip_common::messages::FinalityConfig,
    ) -> Result<()> {
        verifier::instructions::admin::set_verifier_allowed_finality_config(ctx, finality)
    }

    pub fn set_verifier_fast_finality_bps(
        ctx: Context<UpdateVerifierConfig>,
        fast_finality_bps: u16,
    ) -> Result<()> {
        verifier::instructions::admin::set_verifier_fast_finality_bps(ctx, fast_finality_bps)
    }

    /// Creates the verifier configuration for a remote chain and sets its admin configuration.
    /// Fails if the remote chain is already configured; use `edit_verifier_chain_config` instead.
    pub fn init_verifier_chain_config(
        ctx: Context<InitVerifierChainConfig>,
        params: SetVerifierChainConfigParams,
    ) -> Result<()> {
        verifier::instructions::init_verifier_chain_config(ctx, params)
    }

    /// Overwrites the admin configuration of an already configured remote chain, leaving its
    /// allowlist and CCTP configuration untouched.
    pub fn edit_verifier_chain_config(
        ctx: Context<EditVerifierChainConfig>,
        params: SetVerifierChainConfigParams,
    ) -> Result<()> {
        verifier::instructions::edit_verifier_chain_config(ctx, params)
    }

    pub fn set_verifier_chain_config_cctp(
        ctx: Context<SetVerifierChainConfigCctp>,
        params: SetVerifierChainConfigCctpParams,
    ) -> Result<()> {
        verifier::instructions::set_verifier_chain_config_cctp(ctx, params)
    }

    pub fn set_verifier_allowlist(
        ctx: Context<SetVerifierAllowlist>,
        params: SetVerifierAllowlistParams,
    ) -> Result<()> {
        verifier::instructions::set_verifier_allowlist(ctx, params)
    }

    pub fn set_verifier_storage_locations(
        ctx: Context<SetVerifierStorageLocations>,
        params: SetVerifierStorageLocationsParams,
    ) -> Result<()> {
        verifier::instructions::set_verifier_storage_locations(ctx, params)
    }

    pub fn get_verifier_storage_locations(
        ctx: Context<GetVerifierStorageLocations>,
    ) -> Result<Vec<ccip_common::types::ByteString>> {
        verifier::instructions::get_verifier_storage_locations(ctx)
    }

    /// Closes an old CCTPv2 event PDA and reclaims its rent. Circle imposes a 5-day cooldown
    /// (since the message was originally sent) before it can be reclaimed.
    pub fn reclaim_verifier_event_account_v2(
        ctx: Context<ReclaimVerifierEventAccount>,
        params: ReclaimVerifierEventAccountParams,
    ) -> Result<()> {
        verifier::instructions::reclaim_verifier_event_account(ctx, params)
    }

    /// Sweeps this verifier's accrued fees for one fee token into the fee aggregator's ATA.
    ///
    /// Permissionless: the destination is pinned to `config.fee_aggregator`.
    pub fn withdraw_verifier_fee_tokens(ctx: Context<WithdrawVerifierFeeTokens>) -> Result<()> {
        verifier::instructions::admin::withdraw_verifier_fee_tokens(ctx)
    }

    // ===============================================
    // CCTP verifier (embedded CCV): core
    // ===============================================

    pub fn get_verifier_fee(
        ctx: Context<GetVerifierFee>,
        params: GetVerifierFeeParams,
    ) -> Result<GetVerifierFeeResult> {
        verifier::instructions::get_verifier_fee(ctx, params)
    }

    pub fn start_verification_at_source(
        ctx: Context<StartVerificationAtSource>,
        params: StartVerificationAtSourceParams,
    ) -> Result<Vec<u8>> {
        verifier::instructions::start_verification_at_source(ctx, params)
    }

    pub fn verify(ctx: Context<Verify>, params: VerifyParams) -> Result<()> {
        verifier::instructions::verify(ctx, params)
    }

    // ===============================================
    // Account resolution (PoolV2 & CCV)
    // ===============================================
    /// Resolves the accounts needed for nested CPI instructions based on the instruction discriminator.
    /// Includes verifier & poolV2 methods for onramp, offramp, and getting fees.
    /// To be called by the router & offramp programs as part of their account resolution methods.
    pub fn nested_resolve_accounts_start(
        ctx: Context<NestedResolveAccountsStart>,
        params: NestedResolveAccountsParams,
    ) -> Result<NestedResolveAccountsResponse> {
        nested_resolution::start(ctx, params)
    }
}
