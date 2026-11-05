use anchor_lang::prelude::*;

pub mod security;

mod context;
use crate::context::*;

mod token_context;
use crate::token_context::*;

mod state;
use crate::state::*;

pub mod event;
use crate::event::*;

pub mod messages;
use crate::messages::*;

pub mod rmn;

mod instructions_v1;
use crate::instructions_v1::router;

mod instructions;
use crate::instructions::admin;
use crate::instructions::admin::*;
use crate::instructions::ccip_send;
pub use crate::instructions::ccip_send::CcipSendV2Params;
use crate::instructions::ccip_send::*;
use crate::instructions::get_fee;
use crate::instructions::get_fee::*;
use crate::instructions::observe;
use crate::instructions::observe::*;
use crate::instructions::register_receiver;
use crate::instructions::register_receiver::*;
use crate::instructions::resolve_accounts_start;
use crate::instructions::resolve_accounts_start::*;
use crate::instructions::set_pool_interface_implementation;
use crate::instructions::set_pool_interface_implementation::*;

///////////////////////////////////////////////////////////////////////////////
// Specific exports required for backwards compatibility exposed to CCIP 2.0 //
///////////////////////////////////////////////////////////////////////////////
pub use instructions_v1::build_token_transfer_legacy;
pub use instructions_v1::interact_with_pool_legacy;
pub use instructions_v1::transfer_token_cpi_legacy;
pub use instructions_v1::LockOrBurnInLegacy;
pub use instructions_v1::LockOrBurnOutLegacy;

mod error;
use error::CcipRouterError;

use ccip_common::resolution::{ResolveAccountsParams, ResolveAccountsResponse};

// Anchor discriminators for CPI calls
const TOKENPOOL_LOCK_OR_BURN_DISCRIMINATOR: [u8; 8] =
    [0x72, 0xa1, 0x5e, 0x1d, 0x93, 0x19, 0xe8, 0xbf]; // lock_or_burn_tokens

probing_bump_allocator::install_global_allocator!(
    anchor_lang::solana_program::entrypoint::HEAP_START_ADDRESS as usize
);

declare_id!("Ccip842gzYHhvdDkSyi2YVCoAWPbYJoApMFzSxQroE9C");

#[program]
/// The `ccip_router` module contains the implementation of the Cross-Chain Interoperability Protocol (CCIP) Router.
///
/// This is the Collapsed Router Program for CCIP.
/// As it's upgradable persisting the same program id, there is no need to have an indirection of a Proxy Program.
/// This Router handles the OnRamp flow of the CCIP Messages.
///
/// NOTE to devs: This file however should contain *no logic*, only the entrypoints to the different versioned modules,
/// thus making it easier to ensure later on that logic can be changed during upgrades without affecting the interface.
pub mod ccip_router {
    #![warn(missing_docs)]

    use crate::instructions::admin::AddDestChainV2;

    use super::*;

    #[allow(clippy::empty_line_after_outer_attr)]
    //////////////////////////
    /// Initialization Flow //
    //////////////////////////

    /// Initializes the CCIP Router.
    ///
    /// The initialization of the Router is responsibility of Admin, nothing more than calling this method should be done first.
    ///
    /// # Arguments
    ///
    /// * `ctx` - The context containing the accounts required for initialization.
    /// * `svm_chain_selector` - The chain selector for SVM.
    /// * `fee_aggregator` - The public key of the fee aggregator.
    /// * `fee_quoter` - The public key of the fee quoter.
    /// * `link_token_mint` - The public key of the LINK token mint.
    /// * `rmn_remote` - The public key of the RMN remote.
    pub fn initialize(
        ctx: Context<InitializeCCIPRouter>,
        svm_chain_selector: u64,
        fee_aggregator: Pubkey,
        fee_quoter: Pubkey,
        link_token_mint: Pubkey,
        rmn_remote: Pubkey,
    ) -> Result<()> {
        ctx.accounts.config.set_inner(Config {
            version: MAX_CONFIG_V,
            default_code_version: CodeVersion::V1,
            owner: ctx.accounts.authority.key(),
            proposed_owner: Pubkey::default(),
            svm_chain_selector,
            fee_quoter,
            rmn_remote,
            link_token_mint,
            fee_aggregator,
        });

        emit!(events::admin::ConfigSet {
            svm_chain_selector: ctx.accounts.config.svm_chain_selector,
            fee_quoter: ctx.accounts.config.fee_quoter,
            rmn_remote: ctx.accounts.config.rmn_remote,
            link_token_mint: ctx.accounts.config.link_token_mint,
            fee_aggregator: ctx.accounts.config.fee_aggregator,
        });

        Ok(())
    }

    /// Returns the program type (name) and version.
    /// Used by offchain code to easily determine which program & version is being interacted with.
    ///
    /// # Arguments
    /// * `ctx` - The context
    pub fn type_version(_ctx: Context<Empty>) -> Result<String> {
        let response = env!("CCIP_BUILD_TYPE_VERSION").to_string();
        msg!("{}", response);
        Ok(response)
    }

    /// Returns the router's global configuration, for observability consumers.
    ///
    /// Permissionless and read-only: meant to be executed through `simulateTransaction`.
    ///
    /// # Arguments
    /// * `ctx` - The context containing the router config account
    pub fn observe_config(ctx: Context<ObserveConfig>) -> Result<RouterConfigObservation> {
        observe::observe_config(ctx)
    }

    /// Returns the CCIP 2.0 onramp state and configuration for a single destination chain, for
    /// observability consumers.
    ///
    /// Permissionless and read-only: meant to be executed through `simulateTransaction`. Fails if
    /// the lane has no `dest_chain_state_v2` account, which callers can read as "not configured".
    ///
    /// # Arguments
    /// * `ctx` - The context containing the dest chain state account
    /// * `dest_chain_selector` - The destination chain selector the lane belongs to
    pub fn observe_dest_chain_v2(
        ctx: Context<ObserveDestChainV2>,
        dest_chain_selector: u64,
    ) -> Result<RouterDestChainV2Observation> {
        observe::observe_dest_chain_v2(ctx, dest_chain_selector)
    }

    /// Transfers the ownership of the router to a new proposed owner.
    ///
    /// Shared func signature with other programs
    ///
    /// # Arguments
    ///
    /// * `ctx` - The context containing the accounts required for the transfer.
    /// * `proposed_owner` - The public key of the new proposed owner.
    pub fn transfer_ownership(
        ctx: Context<TransferOwnership>,
        proposed_owner: Pubkey,
    ) -> Result<()> {
        router::admin(ctx.accounts.config.default_code_version)
            .transfer_ownership(ctx, proposed_owner)
    }

    /// Accepts the ownership of the router by the proposed owner.
    ///
    /// Shared func signature with other programs
    ///
    /// # Arguments
    ///
    /// * `ctx` - The context containing the accounts required for accepting ownership.
    /// The new owner must be a signer of the transaction.
    pub fn accept_ownership(ctx: Context<AcceptOwnership>) -> Result<()> {
        router::admin(ctx.accounts.config.default_code_version).accept_ownership(ctx)
    }

    #[allow(clippy::empty_line_after_outer_attr)]
    /////////////
    /// Config //
    /////////////

    /// Sets the default code version to be used. This is then used by the slim routing layer to determine
    /// which version of the versioned business logic module (`instructions`) to use. Only the admin may set this.
    ///
    /// Shared func signature with other programs
    ///
    /// # Arguments
    ///
    /// * `ctx` - The context containing the accounts required for updating the configuration.
    /// * `code_version` - The new code version to be set as default.
    pub fn set_default_code_version(
        ctx: Context<UpdateConfigCCIPRouter>,
        code_version: CodeVersion,
    ) -> Result<()> {
        router::admin(ctx.accounts.config.default_code_version)
            .set_default_code_version(ctx, code_version)
    }

    /// Sets the address of the LINK token mint.
    /// The Admin is the only one able to set it.
    ///
    /// # Arguments
    ///
    /// * `ctx` - The context containing the accounts required for updating the configuration.
    /// * `link_token_mint` - The new address of the LINK token mint.
    pub fn set_link_token_mint(
        ctx: Context<UpdateConfigCCIPRouter>,
        link_token_mint: Pubkey,
    ) -> Result<()> {
        router::admin(ctx.accounts.config.default_code_version)
            .set_link_token_mint(ctx, link_token_mint)
    }

    /// Updates the fee aggregator in the router configuration.
    /// The Admin is the only one able to update the fee aggregator.
    ///
    /// # Arguments
    ///
    /// * `ctx` - The context containing the accounts required for updating the configuration.
    /// * `fee_aggregator` - The new fee aggregator address (ATAs will be derived for it for each token).
    pub fn update_fee_aggregator(
        ctx: Context<UpdateConfigCCIPRouter>,
        fee_aggregator: Pubkey,
    ) -> Result<()> {
        router::admin(ctx.accounts.config.default_code_version)
            .update_fee_aggregator(ctx, fee_aggregator)
    }

    /// Updates the RMN remote program in the router configuration.
    /// The Admin is the only one able to update the RMN remote program.
    ///
    /// # Arguments
    ///
    /// * `ctx` - The context containing the accounts required for updating the configuration.
    /// * `rmn_remote,` - The new RMN remote address.
    pub fn update_rmn_remote(
        ctx: Context<UpdateConfigCCIPRouter>,
        rmn_remote: Pubkey,
    ) -> Result<()> {
        router::admin(ctx.accounts.config.default_code_version).update_rmn_remote(ctx, rmn_remote)
    }

    /// Adds a new chain selector to the router.
    ///
    /// The Admin needs to add any new chain supported (this means both OnRamp and OffRamp).
    /// When adding a new chain, the Admin needs to specify if it's enabled or not.
    /// They may enable only source, or only destination, or neither, or both.
    ///
    /// # Arguments
    ///
    /// * `ctx` - The context containing the accounts required for adding the chain selector.
    /// * `new_chain_selector` - The new chain selector to be added.
    /// * `source_chain_config` - The configuration for the chain as source.
    /// * `dest_chain_config` - The configuration for the chain as destination.
    pub fn add_chain_selector(
        ctx: Context<AddChainSelector>,
        new_chain_selector: u64,
        dest_chain_config: DestChainConfig,
    ) -> Result<()> {
        router::admin(ctx.accounts.config.default_code_version).add_chain_selector(
            ctx,
            new_chain_selector,
            dest_chain_config,
        )
    }

    /// Updates the configuration of the destination chain selector.
    ///
    /// The Admin is the only one able to update the destination chain config.
    ///
    /// # Arguments
    ///
    /// * `ctx` - The context containing the accounts required for updating the chain selector.
    /// * `dest_chain_selector` - The destination chain selector to be updated.
    /// * `dest_chain_config` - The new configuration for the destination chain.
    pub fn update_dest_chain_config(
        ctx: Context<UpdateDestChainSelectorConfig>,
        dest_chain_selector: u64,
        dest_chain_config: DestChainConfig,
    ) -> Result<()> {
        router::admin(ctx.accounts.config.default_code_version).update_dest_chain_config(
            ctx,
            dest_chain_selector,
            dest_chain_config,
        )
    }

    /// Adds a new destination chain to the router, for v2 lanes.
    ///
    /// Destination side only: this initializes the lane's `DestChainCcipV2` PDA, which is what
    /// `ccip_send_v2` reads. Source-chain configuration is the offramp's (`add_source_chain`), so
    /// enabling a lane in both directions means calling both programs.
    ///
    /// # Arguments
    ///
    /// * `ctx` - The context containing the accounts required for adding the destination chain.
    /// * `params` - Parameters required for this operation
    pub fn add_dest_chain_v2(
        ctx: Context<AddDestChainV2>,
        params: AddDestChainV2Params,
    ) -> Result<()> {
        admin::add_dest_chain_v2(ctx, params)
    }

    /// Updates the configuration of the destination chain selector, for v2
    /// lanes.
    ///
    /// The Admin is the only one able to update the destination chain config.
    ///
    /// # Arguments
    ///
    /// * `ctx` - The context containing the accounts required for updating the chain selector.
    /// * `params` - Parameters required for this operation
    pub fn update_dest_chain_config_v2(
        ctx: Context<UpdateDestChainSelectorConfigV2>,
        params: UpdateChainSelectorV2Params,
    ) -> Result<()> {
        admin::update_dest_chain_config_v2(ctx, params)
    }

    /// Updates the router's reference addresses, creating their PDA on first use.
    ///
    /// The Admin is the only one able to update them.
    ///
    /// # Arguments
    ///
    /// * `ctx` - The context containing the accounts required for this operation.
    /// * `onramp_lookup_table` - The address lookup table that account resolution returns for
    ///   `ccip_send_v2` and `get_fee_v2`, or `Pubkey::default()` to stop returning one. It is only
    ///   read during resolution, so it adds no accounts to those instructions.
    pub fn update_reference_addresses(
        ctx: Context<UpdateReferenceAddresses>,
        onramp_lookup_table: Pubkey,
    ) -> Result<()> {
        admin::update_reference_addresses(ctx, onramp_lookup_table)
    }

    /// Add an offramp address to the list of offramps allowed by the router, for a
    /// particular source chain. External users will check this list before accepting
    /// a `ccip_receive` CPI.
    ///
    /// # Arguments
    ///
    /// * `ctx` - The context containing the accounts required for this operation.
    /// * `source_chain_selector` - The source chain for the offramp's lane.
    /// * `offramp` - The offramp's address.
    pub fn add_offramp(
        ctx: Context<AddOfframp>,
        source_chain_selector: u64,
        offramp: Pubkey,
    ) -> Result<()> {
        router::admin(ctx.accounts.config.default_code_version).add_offramp(
            ctx,
            source_chain_selector,
            offramp,
        )
    }

    /// Remove an offramp address from the list of offramps allowed by the router, for a
    /// particular source chain. External users will check this list before accepting
    /// a `ccip_receive` CPI.
    ///
    /// # Arguments
    ///
    /// * `ctx` - The context containing the accounts required for this operation.
    /// * `source_chain_selector` - The source chain for the offramp's lane.
    /// * `offramp` - The offramp's address.
    pub fn remove_offramp(
        ctx: Context<RemoveOfframp>,
        source_chain_selector: u64,
        offramp: Pubkey,
    ) -> Result<()> {
        router::admin(ctx.accounts.config.default_code_version).remove_offramp(
            ctx,
            source_chain_selector,
            offramp,
        )
    }

    /// Updates the SVM chain selector in the router configuration.
    ///
    /// This method should only be used if there was an error with the initial configuration or if the solana chain selector changes.
    ///
    /// # Arguments
    ///
    /// * `ctx` - The context containing the accounts required for updating the configuration.
    /// * `new_chain_selector` - The new chain selector for SVM.
    pub fn update_svm_chain_selector(
        ctx: Context<UpdateConfigCCIPRouter>,
        new_chain_selector: u64,
    ) -> Result<()> {
        router::admin(ctx.accounts.config.default_code_version)
            .update_svm_chain_selector(ctx, new_chain_selector)
    }

    /// Bumps the CCIP version for a destination chain.
    /// This effectively just resets the message number of the destination chain state.
    /// If there had been a previous rollback, on re-upgrade the message number will resume from where it was
    /// prior to the rollback.
    ///
    /// # Arguments
    /// * `ctx` - The context containing the accounts required for the bump.
    /// * `dest_chain_selector` - The destination chain selector to bump version for.
    pub fn bump_ccip_version_for_dest_chain(
        ctx: Context<UpdateDestChainSelectorConfigNoRealloc>,
        dest_chain_selector: u64,
    ) -> Result<()> {
        router::admin(ctx.accounts.config.default_code_version)
            .bump_ccip_version_for_dest_chain(ctx, dest_chain_selector)
    }

    /// Rolls back the CCIP version for a destination chain.
    /// This effectively just restores the old version's message number of the destination chain state.
    /// We only support 1 consecutive rollback. If a rollback has occurred for that lane, the version can't
    /// be rolled back again without bumping the version first.
    ///
    /// # Arguments
    /// * `ctx` - The context containing the accounts required for the rollback.
    /// * `dest_chain_selector` - The destination chain selector to rollback the version for.
    pub fn rollback_ccip_version_for_dest_chain(
        ctx: Context<UpdateDestChainSelectorConfigNoRealloc>,
        dest_chain_selector: u64,
    ) -> Result<()> {
        router::admin(ctx.accounts.config.default_code_version)
            .rollback_ccip_version_for_dest_chain(ctx, dest_chain_selector)
    }

    #[allow(clippy::empty_line_after_outer_attr)]
    ///////////////////////////
    /// Token Admin Registry //
    ///////////////////////////

    /// Registers the Token Admin Registry via the CCIP Admin
    ///
    /// # Arguments
    ///
    /// * `ctx` - The context containing the accounts required for registration.
    /// * `token_admin_registry_admin` - The public key of the token admin registry admin to propose.
    pub fn ccip_admin_propose_administrator(
        ctx: Context<RegisterTokenAdminRegistryByCCIPAdmin>,
        token_admin_registry_admin: Pubkey,
    ) -> Result<()> {
        router::token_admin_registry(ctx.accounts.config.default_code_version)
            .ccip_admin_propose_administrator(ctx, token_admin_registry_admin)
    }

    /// Overrides the pending admin of the Token Admin Registry
    ///
    /// # Arguments
    ///
    /// * `ctx` - The context containing the accounts required for registration.
    /// * `token_admin_registry_admin` - The public key of the token admin registry admin to propose.
    pub fn ccip_admin_override_pending_administrator(
        ctx: Context<OverridePendingTokenAdminRegistryByCCIPAdmin>,
        token_admin_registry_admin: Pubkey,
    ) -> Result<()> {
        router::token_admin_registry(ctx.accounts.config.default_code_version)
            .ccip_admin_override_pending_administrator(ctx, token_admin_registry_admin)
    }

    /// Registers the Token Admin Registry by the token owner.
    ///
    /// The Authority of the Mint Token can claim the registry of the token.
    ///
    /// # Arguments
    ///
    /// * `ctx` - The context containing the accounts required for registration.
    /// * `token_admin_registry_admin` - The public key of the token admin registry admin to propose.
    pub fn owner_propose_administrator(
        ctx: Context<RegisterTokenAdminRegistryByOwner>,
        token_admin_registry_admin: Pubkey,
    ) -> Result<()> {
        router::token_admin_registry(ctx.accounts.config.default_code_version)
            .owner_propose_administrator(ctx, token_admin_registry_admin)
    }

    /// Registers the Token Admin Registry by a signer of the mint authority multisig.
    ///
    /// # Arguments
    ///
    /// * `ctx` - The context containing the accounts required for registration.
    /// * `token_admin_registry_admin` - The public key of the token admin registry admin to propose.
    pub fn multisig_owner_propose_administrator(
        ctx: Context<RegisterTokenAdminRegistryByMultisigOwner>,
        token_admin_registry_admin: Pubkey,
    ) -> Result<()> {
        router::token_admin_registry(ctx.accounts.config.default_code_version)
            .multisig_owner_propose_administrator(ctx, token_admin_registry_admin)
    }

    /// Overrides the pending admin of the Token Admin Registry by the token owner
    ///
    /// # Arguments
    ///
    /// * `ctx` - The context containing the accounts required for registration.
    /// * `token_admin_registry_admin` - The public key of the token admin registry admin to propose.
    pub fn owner_override_pending_administrator(
        ctx: Context<OverridePendingTokenAdminRegistryByOwner>,
        token_admin_registry_admin: Pubkey,
    ) -> Result<()> {
        router::token_admin_registry(ctx.accounts.config.default_code_version)
            .owner_override_pending_administrator(ctx, token_admin_registry_admin)
    }

    /// Overrides the pending admin of the Token Admin Registry by a signer of the mint authority multisig
    ///
    /// # Arguments
    ///
    /// * `ctx` - The context containing the accounts required for registration.
    /// * `token_admin_registry_admin` - The public key of the token admin registry admin to propose.
    pub fn multisig_owner_override_pending_administrator(
        ctx: Context<OverridePendingTokenAdminRegistryByMultisigOwner>,
        token_admin_registry_admin: Pubkey,
    ) -> Result<()> {
        router::token_admin_registry(ctx.accounts.config.default_code_version)
            .multisig_owner_override_pending_administrator(ctx, token_admin_registry_admin)
    }

    /// Accepts the admin role of the token admin registry.
    ///
    /// The Pending Admin must call this function to accept the admin role of the Token Admin Registry.
    ///
    /// # Arguments
    ///
    /// * `ctx` - The context containing the accounts required for accepting the admin role.
    /// * `mint` - The public key of the token mint.
    pub fn accept_admin_role_token_admin_registry(
        ctx: Context<AcceptAdminRoleTokenAdminRegistry>,
    ) -> Result<()> {
        router::token_admin_registry(ctx.accounts.config.default_code_version)
            .accept_admin_role_token_admin_registry(ctx)
    }

    /// Transfers the admin role of the token admin registry to a new admin.
    ///
    /// Only the Admin can transfer the Admin Role of the Token Admin Registry, this setups the Pending Admin and then it's their responsibility to accept the role.
    ///
    /// # Arguments
    ///
    /// * `ctx` - The context containing the accounts required for the transfer.
    /// * `mint` - The public key of the token mint.
    /// * `new_admin` - The public key of the new admin.
    pub fn transfer_admin_role_token_admin_registry(
        ctx: Context<ModifyTokenAdminRegistry>,
        new_admin: Pubkey,
    ) -> Result<()> {
        router::token_admin_registry(ctx.accounts.config.default_code_version)
            .transfer_admin_role_token_admin_registry(ctx, new_admin)
    }

    /// Upgrades the Token Admin Registry from version 1 or 2 to the current version.
    ///
    /// Anyone may invoke this method, as the upgrade has safe defaults for any new value,
    /// and those can then be changed by the Token Admin Registry Admin via separate instructions.
    ///
    /// # Arguments
    ///
    /// * `ctx` - The context containing the accounts required for the upgrade.
    #[instruction(discriminator = [124, 26, 79, 44, 190, 150, 213, 49])] // backwards-compatible, to support renaming method
    pub fn upgrade_token_admin_registry(ctx: Context<UpgradeTokenAdminRegistry>) -> Result<()> {
        router::token_admin_registry(ctx.accounts.config.default_code_version)
            .upgrade_token_admin_registry(ctx)
    }

    /// Sets the pool interface implementation for a given token pool in the Token Admin Registry.
    /// Used to configure non-default values on new fields of the Token Admin Registry that refer to
    /// the pool's implemented interface, particularly which version of the pool interface is implemented
    /// and whether it implements account resolution or not.
    pub fn set_pool_interface_implementation(
        ctx: Context<EditPoolTokenAdminRegistryV3>,
        params: SetPoolInterfaceImplementationParams,
    ) -> Result<()> {
        set_pool_interface_implementation::set_pool_interface_implementation(ctx, params)
    }

    /// Registers (or updates) the CCIP interface version of a receiver program. The
    /// OffRamp consults the resulting `ReceiverRegistry` PDA to select the v1 vs v2 receiver flow;
    /// an unregistered receiver defaults to v1, so only v2 receivers need to register.
    ///
    /// Authorized either by a PDA of `receiver_program` (self-registration via CPI) or by the
    /// router's upgrade authority — never by the receiver's own admin as a direct caller. See
    /// `register_receiver::register_receiver` for the authorization check itself.
    ///
    /// # Arguments
    /// * `ctx` - The context containing the accounts required for registration.
    /// * `receiver_program` - The receiver program whose interface version is being registered.
    /// * `receiver_version` - The receiver's CCIP interface version; must be `1` or `2`.
    pub fn register_receiver(
        ctx: Context<RegisterReceiver>,
        params: RegisterReceiverParams,
    ) -> Result<()> {
        register_receiver::register_receiver(ctx, params)
    }

    /// Sets the pool lookup table for a given token mint.
    ///
    /// The administrator of the token admin registry can set the pool lookup table for a given token mint.
    ///
    /// # Arguments
    ///
    /// * `ctx` - The context containing the accounts required for setting the pool.
    /// * `writable_indexes` - a bit map of the indexes of the accounts in lookup table that are writable
    pub fn set_pool(
        ctx: Context<SetPoolTokenAdminRegistry>,
        writable_indexes: Vec<u8>,
    ) -> Result<()> {
        router::token_admin_registry(ctx.accounts.config.default_code_version)
            .set_pool(ctx, writable_indexes)
    }

    #[allow(clippy::empty_line_after_outer_attr)]
    //////////////
    /// Billing //
    //////////////

    /// Transfers the accumulated billed fees in a particular token to an arbitrary token account.
    /// Only the CCIP Admin can withdraw billed funds.
    ///
    /// # Arguments
    ///
    /// * `ctx` - The context containing the accounts required for the transfer of billed fees.
    /// * `transfer_all` - A flag indicating whether to transfer all the accumulated fees in that token or not.
    /// * `desired_amount` - The amount to transfer. If `transfer_all` is true, this value must be 0.
    pub fn withdraw_billed_funds(
        ctx: Context<WithdrawBilledFunds>,
        transfer_all: bool,
        desired_amount: u64, // if transfer_all is false, this value must be 0
    ) -> Result<()> {
        router::admin(ctx.accounts.config.default_code_version).withdraw_billed_funds(
            ctx,
            transfer_all,
            desired_amount,
        )
    }

    #[allow(clippy::empty_line_after_outer_attr)]
    ///////////////////
    /// On Ramp Flow //
    ///////////////////

    /// Sends a message to the destination chain.
    ///
    /// Request a message to be sent to the destination chain.
    /// The method name needs to be ccip_send with Anchor encoding.
    /// This function is called by the CCIP Sender Contract (or final user) to send a message to the CCIP Router.
    /// The message will be sent to the receiver on the destination chain selector.
    /// This message emits the event CCIPMessageSent with all the necessary data to be retrieved by the OffChain Code
    ///
    /// # Arguments
    ///
    /// * `ctx` - The context containing the accounts required for sending the message.
    /// * `dest_chain_selector` - The chain selector for the destination chain.
    /// * `message` - The message to be sent. The size limit of data is 256 bytes.
    /// * `token_indexes` - Indices into the remaining accounts vector where the subslice for a token begins.
    pub fn ccip_send<'info>(
        ctx: Context<'info, CcipSend<'info>>,
        dest_chain_selector: u64,
        message: SVM2AnyMessage,
        token_indexes: Vec<u8>,
    ) -> Result<[u8; 32]> {
        router::onramp(
            ctx.accounts.dest_chain_state.config.lane_code_version,
            ctx.accounts.config.default_code_version,
        )
        .ccip_send(ctx, dest_chain_selector, message, token_indexes)
    }

    /// Sends a message to the destination chain through the CCIP v2.0 Onramp
    ///
    /// Request a message to be sent to the destination chain.
    /// The method name needs to be ccip_send_v2 with Anchor encoding.
    /// This function is called by the CCIP Sender Contract (or final user) to send a message to the CCIP Router.
    /// The message will be sent to the receiver on the destination chain selector.
    /// This message emits the event CCIPMessageSentV2 with all the necessary data to be retrieved by the OffChain Code
    ///
    /// # Arguments
    ///
    /// * `ctx` - The context containing the accounts required for sending the message.
    /// * `params` - Parameters required for sending the message, including:
    ///    * `dest_chain_selector` - The chain selector for the destination chain.
    ///    * `message` - The message to be sent.
    ///    * `resolution_metadata` - An opaque value obtained from calling `resolve_accounts_start`, which must be done
    ///       before invoking this instruction.
    pub fn ccip_send_v2<'info>(
        ctx: Context<'info, CcipSendV2<'info>>,
        params: CcipSendV2Params,
    ) -> Result<[u8; 32]> {
        ccip_send::ccip_send_v2(ctx, params)
    }

    /// Queries the onramp for the fee required to send a message.
    ///
    /// This call is permissionless. Note it does not verify whether there's a curse active
    /// in order to avoid the RMN CPI overhead.
    ///
    /// # Arguments
    ///
    /// * `ctx` - The context containing the accounts required for obtaining the message fee.
    /// * `dest_chain_selector` - The chain selector for the destination chain.
    /// * `message` - The message to be sent. The size limit of data is 256 bytes.
    pub fn get_fee<'info>(
        ctx: Context<'info, GetFee<'info>>,
        dest_chain_selector: u64,
        message: SVM2AnyMessage,
    ) -> Result<GetFeeResult> {
        router::onramp(
            ctx.accounts.dest_chain_state.config.lane_code_version,
            ctx.accounts.config.default_code_version,
        )
        .get_fee(ctx, dest_chain_selector, message)
    }

    /// Automatically derives all accounts required to call `ccip_send`.
    ///
    /// This method receives the bare minimum amount of information needed to construct
    /// the entire account list to send a transaction, and builds it iteratively
    /// over the course of multiple calls.
    ///
    /// The return type contains:
    ///
    /// * `accounts_to_save`: The caller must append these accounts to a list they maintain.
    ///   When complete, this list will contain all accounts needed to call `ccip_send`.
    /// * `ask_again_with`: When `next_stage` is not empty, the caller must call `derive_accounts_ccip_send`
    ///   again, including exactly these accounts as the `remaining_accounts`.
    /// * `lookup_tables_to_save`: The caller must save those LUTs. They can be used for `ccip_send`.
    /// * `current_stage`: A string describing the current stage of the derivation process. When the stage
    ///   is `TokenTransferStaticAccounts/<N>/0`, it means the `accounts_to_save` block in this response contains
    ///   all accounts relating to the Nth token being transferred. Use this information to construct
    ///   the `token_indexes` vector that `ccip_send` requires.
    /// * `next_stage`: If nonempty, this means the instruction must get called again with this value
    ///   as the `stage` argument.
    ///
    /// Therefore, and starting with an empty `remaining_accounts` list, the caller must repeatedly
    /// call `derive_accounts_ccip_send` until `next_stage` is returned empty.
    ///
    /// # Arguments
    ///
    /// * `ctx`: Context containing only the config.
    /// * `stage`: Requested derivation stage. Pass "Start" the first time, then for each subsequent
    ///   call, pass the value returned in `response.next_stage` until empty.
    /// * `params`:
    ///    * `ccip_send_caller`: Public key of the account that will sign the call to `ccip_send`.
    ///    * `dest_chain_selector`: CCIP chain selector for the dest chain.
    ///    * `fee_token_mint`: The mint address for the token used for fees. Pubkey::default() if native SOL.
    ///    * `mints_of_transferred_token`: List of all token mints for tokens being transferred.
    pub fn derive_accounts_ccip_send<'info>(
        ctx: Context<'info, ViewConfigOnly<'info>>,
        params: DeriveAccountsCcipSendParams,
        stage: String,
    ) -> Result<DeriveAccountsResponse> {
        let default_code_version: CodeVersion = ctx.accounts.config.default_code_version;

        router::onramp(default_code_version, default_code_version)
            .derive_accounts_ccip_send(ctx, params, stage)
    }

    /// CCIPv2 message fee retrieval
    pub fn get_fee_v2<'info>(
        ctx: Context<'info, GetFeeV2<'info>>,
        params: GetFeeParams,
    ) -> Result<GetFeeResultV2> {
        get_fee::get_fee_v2(ctx, params)
    }

    /// Entrypoint to resolve accounts of any of the supported operations in this program, to make it easier for
    /// offchain code to interact with the program without having to derive all accounts by itself, and future-proofing
    /// against changes in account structure.
    pub fn resolve_accounts_start(
        ctx: Context<ResolveAccountsStart>,
        params: ResolveAccountsParams,
    ) -> Result<ResolveAccountsResponse> {
        resolve_accounts_start::resolve_accounts_start(ctx, params)
    }

    /// Continues nested account resolution for the fee token billing components used by `ccip_send_v2`.
    pub fn resolve_accounts_fee_token_billing_stage(
        ctx: Context<ResolveAccountsFeeTokenBillingStage>,
        params: ResolveAccountsParams,
    ) -> Result<ResolveAccountsResponse> {
        ccip_send::resolve_accounts::fee_token_billing_stage(ctx, params)
    }

    /// Continues nested account resolution for the executor fee query used by `ccip_send_v2`.
    pub fn resolve_accounts_executor_stage<'info>(
        ctx: Context<'info, ResolveAccountsCcipSendExecutorStage<'info>>,
        params: ResolveAccountsParams,
    ) -> Result<ResolveAccountsResponse> {
        ccip_send::resolve_accounts::executor_stage(ctx, params)
    }

    /// Continues nested account resolution for `ccip_send_v2`.
    pub fn resolve_accounts_construct_final_ccv_list_stage<'info>(
        ctx: Context<'info, ResolveAccountsCcipSendConstructFinalCcvListStage<'info>>,
        params: ResolveAccountsParams,
    ) -> Result<ResolveAccountsResponse> {
        ccip_send::resolve_accounts::construct_final_ccv_list_stage(ctx, params)
    }

    /// Continues nested account resolution for CCV source hooks used by `ccip_send_v2`.
    pub fn resolve_accounts_ccv_stage<'info>(
        ctx: Context<'info, ResolveAccountsCcvStage<'info>>,
        params: ResolveAccountsParams,
    ) -> Result<ResolveAccountsResponse> {
        ccip_send::resolve_accounts::ccv_stage(ctx, params)
    }

    /// Continues nested account resolution for token transfer used `ccip_send_v2`.
    pub fn resolve_accounts_token_transfer_retrieve_lut_stage<'info>(
        ctx: Context<'info, ResolveAccountsCcipSendTokenTransferRetrieveLutStage<'info>>,
        params: ResolveAccountsParams,
    ) -> Result<ResolveAccountsResponse> {
        ccip_send::resolve_accounts::retrieve_token_lut_stage(ctx, params)
    }

    /// Continues nested account resolution for token transfer static accounts used by `ccip_send_v2`.
    ///
    /// A lookup table too large for one 1kB response is emitted a page at a time: the stage then
    /// points back at itself, so callers must keep following `next_ix_discriminator` rather than
    /// assuming a single round.
    pub fn resolve_accounts_token_transfer_static_accounts_stage<'info>(
        ctx: Context<'info, ResolveAccountsCcipSendTokenTransferStaticAccountsStage<'info>>,
        params: ResolveAccountsParams,
    ) -> Result<ResolveAccountsResponse> {
        ccip_send::resolve_accounts::retrieve_token_static_accounts_stage(ctx, params)
    }

    /// Continues nested account resolution for token pool hooks used by `ccip_send_v2`.
    pub fn resolve_accounts_nested_token_stage<'info>(
        ctx: Context<'info, ResolveAccountsNestedTokenStage<'info>>,
        params: ResolveAccountsParams,
    ) -> Result<ResolveAccountsResponse> {
        ccip_send::resolve_accounts::nested_token_stage(ctx, params)
    }
}
