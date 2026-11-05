use anchor_lang::prelude::*;

pub mod security;

declare_id!("CV2s8jsYhx9th8pyZ4pN7apLELAB42e5j4XqWhqTdWCm");

mod event;
mod state;
use state::CodeVersion;

mod instructions;
use crate::instructions::admin::*;
use crate::instructions::get_fee;
use crate::instructions::get_fee::*;
pub use crate::instructions::get_fee::{GetFeeParams, GetFeeResult};
use crate::instructions::get_storage_locations;
use crate::instructions::get_storage_locations::*;
use crate::instructions::observe;
use crate::instructions::observe::*;
use crate::instructions::remove_signer_config;
use crate::instructions::remove_signer_config::*;
use crate::instructions::resolve_accounts_start;
pub use crate::instructions::resolve_accounts_start::CommitteeVerifierCpiResolutionInputs;
use crate::instructions::resolve_accounts_start::*;
use crate::instructions::set_allowlist;
use crate::instructions::set_allowlist::*;
use crate::instructions::set_chain_config;
use crate::instructions::set_chain_config::*;
use crate::instructions::set_signer_config;
use crate::instructions::set_signer_config::*;
use crate::instructions::set_storage_locations;
use crate::instructions::set_storage_locations::*;
use crate::instructions::start_verification_at_source as start_verification_at_source_instruction;
pub use crate::instructions::start_verification_at_source::StartVerificationAtSourceParams;
use crate::instructions::start_verification_at_source::*;
use crate::instructions::verify;
pub use crate::instructions::verify::VerifyParams;
use crate::instructions::verify::*;
mod error;
use error::CommitteeVerifierError;

use ccip_common::messages::FinalityConfig;
use ccip_common::resolution::{
    NestedResolveAccountsParams, NestedResolveAccountsResponse, ResolveAccountsParams,
    ResolveAccountsResponse,
};
use ccip_common::types::ByteString;

#[program]
pub mod committee_verifier {

    use super::*;

    /// Initializes the Committee Verifier Config account.
    ///
    /// Restricted to the program upgrade authority.
    pub fn initialize(ctx: Context<InitializeConfig>) -> Result<()> {
        instructions::admin::initialize(ctx)
    }

    /// Returns the program type (name) and version.
    pub fn type_version(_ctx: Context<Empty>) -> Result<String> {
        let response = env!("CCIP_BUILD_TYPE_VERSION").to_string();
        msg!("{}", response);
        Ok(response)
    }

    /// Transfers ownership to a new proposed owner. The proposed owner must
    /// call `accept_ownership` to complete the transfer.
    pub fn transfer_ownership(ctx: Context<UpdateConfig>, proposed_owner: Pubkey) -> Result<()> {
        instructions::admin::transfer_ownership(ctx, proposed_owner)
    }

    /// Accepts a pending ownership transfer. Must be called by the proposed owner.
    pub fn accept_ownership(ctx: Context<AcceptOwnership>) -> Result<()> {
        instructions::admin::accept_ownership(ctx)
    }

    /// Proposes a new storage location admin. The proposed admin must call
    /// `accept_storage_location_admin` to complete the transfer.
    pub fn transfer_storage_location_admin(
        ctx: Context<UpdateStorageLocationAdmin>,
        proposed_storage_location_admin: Pubkey,
    ) -> Result<()> {
        instructions::admin::transfer_storage_location_admin(ctx, proposed_storage_location_admin)
    }

    /// Accepts a pending storage location admin transfer. Must be called by the
    /// proposed storage location admin.
    pub fn accept_storage_location_admin(ctx: Context<AcceptStorageLocationAdmin>) -> Result<()> {
        instructions::admin::accept_storage_location_admin(ctx)
    }

    /// Sets the allowlist admin. Can only be called by the owner.
    pub fn set_allowlist_admin(ctx: Context<UpdateConfig>, allowlist_admin: Pubkey) -> Result<()> {
        instructions::admin::set_allowlist_admin(ctx, allowlist_admin)
    }

    /// Sets the fee aggregator address. Can only be called by the owner.
    pub fn set_fee_aggregator(ctx: Context<UpdateConfig>, fee_aggregator: Pubkey) -> Result<()> {
        instructions::admin::set_fee_aggregator(ctx, fee_aggregator)
    }

    /// Sweeps this verifier's accrued fees for one fee token into the fee aggregator's ATA.
    ///
    /// Permissionless: the destination is pinned to `config.fee_aggregator`.
    pub fn withdraw_fee_tokens(ctx: Context<WithdrawFeeTokens>) -> Result<()> {
        instructions::admin::withdraw_fee_tokens(ctx)
    }

    /// Sets the allowed finality config. Can only be called by the owner.
    pub fn set_allowed_finality_config(
        ctx: Context<UpdateConfig>,
        finality: FinalityConfig,
    ) -> Result<()> {
        instructions::admin::set_allowed_finality_config(ctx, finality)
    }

    /// Sets the default code version used for instructions. This may affect
    /// account resolution for instructions that define different contexts on newer
    /// versions.
    pub fn set_default_code_version(
        ctx: Context<UpdateConfig>,
        code_version: CodeVersion,
    ) -> Result<()> {
        instructions::admin::set_default_code_version(ctx, code_version)
    }

    /// Verification of a cross-chain message. The verifier can implement this
    /// in any way it wants: signature verification, a quorum of signatures,
    /// native interop, or a ZK light client. Any proof required for
    /// verification is supplied through the `verifier_results` field in params.
    ///
    /// The `message_id` is a 32-byte hash of the entire message. It can be
    /// recomputed from the encoded message at the cost of a not-insignificant
    /// amount of compute. Any CCV MUST include the message ID or the entire
    /// message as part of its proof.
    pub fn verify(ctx: Context<Verify>, params: VerifyParams) -> Result<()> {
        verify::verify(ctx, params)
    }

    /// Quotes the fee, including gas and payload size, for verifying a CCIP
    /// message to a destination chain.
    ///
    /// Takes the user-facing [`SVM2AnyMessage`] rather than a wire-format
    /// message, since this is called on the sending (onramp) side before the
    /// message is finalized.
    ///
    /// This uses identical context as `start_verification_at_source`, as both instructions are
    /// intended to be executed as CPIs from the same caller. Account resolution
    /// for `start_verification_at_source` should suffice for both.
    pub fn get_verifier_fee(ctx: Context<GetFee>, params: GetFeeParams) -> Result<GetFeeResult> {
        get_fee::get_fee(ctx, params)
    }

    /// Source-side verification hook. Called on the onramp side when a message
    /// is being sent, giving the verifier an opportunity to inspect or
    /// transform the message and collect fees.
    ///
    /// Returns opaque verifier data that will be included in the cross-chain
    /// message metadata.
    pub fn start_verification_at_source(
        ctx: Context<StartVerificationAtSource>,
        params: StartVerificationAtSourceParams,
    ) -> Result<Vec<u8>> {
        start_verification_at_source_instruction::start_verification_at_source(ctx, params)
    }

    /// Returns the storage location identifiers for this CCV. Each entry
    /// uniquely identifies a storage location — this can be an address, a URL,
    /// or any other identifier that makes sense for the CCV.
    ///
    /// This is used by the executor(s) to know where to look for the proof
    /// data that the CCV has produced.
    ///
    /// Entries are returned as raw bytes: the program never interprets them, so it does not validate
    /// that they are UTF-8. Callers that expect text must decode them and handle invalid input.
    pub fn get_storage_locations(ctx: Context<GetStorageLocations>) -> Result<Vec<ByteString>> {
        get_storage_locations::get_storage_locations(ctx)
    }

    /// Returns this CCV's global configuration, for observability consumers.
    ///
    /// Permissionless and read-only: meant to be executed through `simulateTransaction`.
    pub fn observe_config(
        ctx: Context<ObserveConfig>,
    ) -> Result<CommitteeVerifierConfigObservation> {
        observe::observe_config(ctx)
    }

    /// Returns this CCV's configuration for a single remote (source) chain, for observability
    /// consumers.
    ///
    /// Permissionless and read-only: meant to be executed through `simulateTransaction`. Fails if
    /// the lane has no configuration, which callers can read as "not configured".
    pub fn observe_remote_chain(
        ctx: Context<ObserveRemoteChain>,
        remote_chain_selector: u64,
    ) -> Result<CommitteeVerifierRemoteChainObservation> {
        observe::observe_remote_chain(ctx, remote_chain_selector)
    }

    /// Sets the storage location identifiers for this CCV.
    ///
    /// Can only be called by the storage_location_admin.
    pub fn set_storage_locations(
        ctx: Context<SetStorageLocations>,
        params: SetStorageLocationsParams,
    ) -> Result<()> {
        set_storage_locations::set_storage_locations(ctx, params)
    }

    /// Sets the allowlist configuration. Overwrites the entire allowlist
    /// with the provided senders and enabled flag.
    ///
    /// Can only be called by the allowlist admin.
    pub fn set_allowlist(ctx: Context<SetAllowlist>, params: SetAllowlistParams) -> Result<()> {
        set_allowlist::set_allowlist(ctx, params)
    }

    /// Sets the signer configuration for a remote source chain.
    /// Overwrites the entire signer set and threshold. Signers are sorted
    /// on-chain for efficient binary search during signature verification.
    ///
    /// Can only be called by the owner.
    pub fn set_signer_config(
        ctx: Context<SetSignerConfig>,
        params: SetSignerConfigParams,
    ) -> Result<()> {
        set_signer_config::set_signer_config(ctx, params)
    }

    /// Clears the signer configuration for a remote source chain by setting
    /// the threshold to 0 and emptying the signer set.
    ///
    /// Can only be called by the owner.
    pub fn remove_signer_config(
        ctx: Context<RemoveSignerConfig>,
        params: RemoveSignerConfigParams,
    ) -> Result<()> {
        remove_signer_config::remove_signer_config(ctx, params)
    }

    /// Creates the configuration for a remote chain and sets its admin configuration.
    /// Fails if the remote chain is already configured; use `edit_chain_config` instead.
    ///
    /// Can only be called by the owner.
    pub fn init_chain_config(
        ctx: Context<InitChainConfig>,
        params: SetChainConfigParams,
    ) -> Result<()> {
        set_chain_config::init_chain_config(ctx, params)
    }

    /// Overwrites the admin configuration of an already configured remote chain, leaving its
    /// signer and allowlist configuration untouched.
    ///
    /// Can only be called by the owner.
    pub fn edit_chain_config(
        ctx: Context<EditChainConfig>,
        params: SetChainConfigParams,
    ) -> Result<()> {
        set_chain_config::edit_chain_config(ctx, params)
    }

    /// Entrypoint to resolve accounts for any supported top-level instruction.
    pub fn resolve_accounts_start(
        ctx: Context<ResolveAccountsStart>,
        params: ResolveAccountsParams,
    ) -> Result<ResolveAccountsResponse> {
        resolve_accounts_start::resolve_accounts_start(ctx, params)
    }

    /// Entrypoint to resolve accounts for the instructions invoked via CPI
    /// (`verify`, `get_verifier_fee` and `start_verification_at_source`).
    pub fn nested_resolve_accounts_start(
        ctx: Context<NestedResolveAccountsStart>,
        params: NestedResolveAccountsParams,
    ) -> Result<NestedResolveAccountsResponse> {
        resolve_accounts_start::nested_resolve_accounts_start(ctx, params)
    }
}

#[derive(Accounts)]
pub struct Empty<'info> {
    // This is unused, but Anchor requires that there is at least one account in the context
    // Issue: https://github.com/solana-foundation/anchor/issues/2342 (might be patched in 2.0)
    pub clock: Sysvar<'info, Clock>,
}
