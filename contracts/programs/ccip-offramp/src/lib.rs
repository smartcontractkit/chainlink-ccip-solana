use anchor_lang::prelude::*;

pub mod security;

probing_bump_allocator::install_global_allocator!(
    anchor_lang::solana_program::entrypoint::HEAP_START_ADDRESS as usize
);

declare_id!("off43sBHBsmWTU6VbfwKYCNwyJ4rSDhunYvz3QL7bF9");

mod event;
use crate::event::BufferClosed;

mod buffering;
mod messages;

mod state;

mod instructions;
use crate::instructions::admin;
use crate::instructions::admin::*;
use crate::instructions::buffer_execution_inputs;
use crate::instructions::buffer_execution_inputs::*;
use crate::instructions::execute;
use crate::instructions::execute::*;
use crate::instructions::get_ccvs_for_msg;
use crate::instructions::get_ccvs_for_msg::*;
use crate::instructions::init;
use crate::instructions::init::*;
use crate::instructions::observe;
use crate::instructions::observe::*;
use crate::instructions::resolve_accounts_start;
use crate::instructions::resolve_accounts_start::*;

mod error;
use error::CcipOfframpError;

use ccip_common::resolution::{ResolveAccountsParams, ResolveAccountsResponse};

#[program]
pub mod ccip_offramp {

    use super::*;

    #[allow(clippy::empty_line_after_outer_attr)]
    // Initialization Flow //

    /// Initializes the CCIP Offramp, except for the config account (due to stack size limitations).
    ///
    /// The initialization of the Offramp is responsibility of Admin, nothing more than calling these
    /// initialization methods should be done first.
    ///
    /// # Arguments
    ///
    /// * `ctx` - The context containing the accounts required for initialization.
    pub fn initialize(ctx: Context<Initialize>) -> Result<()> {
        init::initialize(ctx)
    }

    /// Initializes the CCIP Offramp Config account.
    ///
    /// The initialization of the Offramp is responsibility of Admin, nothing more than calling these
    /// initialization methods should be done first.
    ///
    /// # Arguments
    ///
    /// * `ctx` - The context containing the accounts required for initialization of the config.
    /// * `svm_chain_selector` - The chain selector for SVM.
    pub fn initialize_config(
        ctx: Context<InitializeConfig>,
        svm_chain_selector: u64,
    ) -> Result<()> {
        init::initialize_config(ctx, svm_chain_selector)
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

    /// Returns the offramp's global configuration and reference addresses, for observability
    /// consumers.
    ///
    /// Permissionless and read-only: meant to be executed through `simulateTransaction`.
    ///
    /// # Arguments
    /// * `ctx` - The context containing the config and reference addresses accounts
    pub fn observe_config(ctx: Context<ObserveConfig>) -> Result<OffRampConfigObservation> {
        observe::observe_config(ctx)
    }

    /// Returns the offramp's configuration for a single source chain, for observability consumers.
    ///
    /// Permissionless and read-only: meant to be executed through `simulateTransaction`. Fails if
    /// the lane has no source chain account, which callers can read as "not configured".
    ///
    /// # Arguments
    /// * `ctx` - The context containing the source chain account
    /// * `source_chain_selector` - The source chain selector the lane belongs to
    pub fn observe_source_chain(
        ctx: Context<ObserveSourceChain>,
        source_chain_selector: u64,
    ) -> Result<OffRampSourceChainObservation> {
        observe::observe_source_chain(ctx, source_chain_selector)
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
        admin::transfer_ownership(ctx, proposed_owner)
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
        admin::accept_ownership(ctx)
    }

    #[allow(clippy::empty_line_after_outer_attr)]
    // Config //

    /// Updates reference addresses in the offramp contract, such as
    /// the CCIP router, Fee Quoter, and the Offramp Lookup Table.
    /// Only the Admin may update these addresses.
    ///
    /// # Arguments
    ///
    /// * `ctx` - The context containing the accounts required for updating the reference addresses.
    /// * `router` - The router address to be set.
    /// * `fee_quoter` - The fee_quoter address to be set.
    /// * `offramp_lookup_table` - The offramp_lookup_table address to be set.
    /// * `rmn_remote` - The rmn_remote address to be set.
    pub fn update_reference_addresses(
        ctx: Context<UpdateReferenceAddresses>,
        router: Pubkey,
        fee_quoter: Pubkey,
        offramp_lookup_table: Pubkey,
        rmn_remote: Pubkey,
    ) -> Result<()> {
        admin::update_reference_addresses(ctx, router, fee_quoter, offramp_lookup_table, rmn_remote)
    }

    /// Adds a new source chain selector with its config to the offramp.
    ///
    /// The Admin needs to add any new chain supported.
    /// When adding a new chain, the Admin needs to specify if it's enabled or not.
    ///
    /// # Arguments
    /// * `ctx` - The context containing the accounts required for adding the chain selector.
    /// * `params` - The parameters for updating the source chain, including the selector and new configuration
    pub fn add_source_chain(
        ctx: Context<AddSourceChain>,
        params: AddSourceChainParams,
    ) -> Result<()> {
        admin::add_source_chain(ctx, params)
    }

    /// Disables the source chain selector.
    ///
    /// The Admin is the only one able to disable the chain selector as source. This method is thought of as an emergency kill-switch.
    ///
    /// # Arguments
    ///
    /// * `ctx` - The context containing the accounts required for disabling the chain selector.
    /// * `source_chain_selector` - The source chain selector to be disabled.
    pub fn disable_source_chain_selector(
        ctx: Context<DisableSourceChain>,
        source_chain_selector: u64,
    ) -> Result<()> {
        admin::disable_source_chain_selector(ctx, source_chain_selector)
    }

    /// Deletes the source chain PDA.
    ///
    /// The Admin is the only one able to delete the chain as source.
    ///
    /// # Arguments
    ///
    /// * `ctx` - The context containing the accounts required for deleting the chain.
    /// * `source_chain_selector` - The source chain selector to be deleted.
    pub fn delete_source_chain(
        ctx: Context<DeleteSourceChain>,
        source_chain_selector: u64,
    ) -> Result<()> {
        admin::delete_source_chain(ctx, source_chain_selector)
    }

    /// Updates the configuration of the source chain selector.
    ///
    /// The Admin is the only one able to update the source chain config.
    ///
    /// The config is replaced in full, `on_ramps` included: the provided list becomes the complete
    /// set of OnRamps allowed to send from this source chain. When adding an OnRamp (e.g. because the
    /// source chain is upgrading its own), pass the OnRamps to keep alongside the new one — dropping
    /// an OnRamp that still has inflight messages leaves those messages unexecutable while it is
    /// missing. That is recoverable: re-adding the OnRamp makes them executable again, because the
    /// check runs at execution time and a rejected execution reverts without recording anything.
    ///
    /// # Arguments
    ///
    /// * `ctx` - The context containing the accounts required for updating the chain selector.
    /// * `params` - The parameters for updating the source chain, including the selector and new configuration.
    pub fn update_source_chain_config(
        ctx: Context<UpdateSourceChain>,
        params: UpdateSourceChainParams,
    ) -> Result<()> {
        admin::update_source_chain_config(ctx, params)
    }

    /// Updates the SVM chain selector in the offramp configuration.
    ///
    /// This method should only be used if there was an error with the initial configuration or if the solana chain selector changes.
    ///
    /// # Arguments
    ///
    /// * `ctx` - The context containing the accounts required for updating the configuration.
    /// * `new_chain_selector` - The new chain selector for SVM.
    pub fn update_svm_chain_selector(
        ctx: Context<UpdateConfig>,
        new_chain_selector: u64,
    ) -> Result<()> {
        admin::update_svm_chain_selector(ctx, new_chain_selector)
    }

    #[allow(clippy::empty_line_after_outer_attr)]
    // Off Ramp Flow //

    /// Entrypoint to resolve accounts of any of the supported operations in this program, to make it easier for
    /// offchain code to interact with the program without having to derive all accounts by itself, and future-proofing
    /// against changes in account structure.
    ///
    /// Some instructions may require additional accounts in the resolver context. Those accounts must be passed as
    /// extra accounts to the resolution instruction, after the accounts declared in the resolver context, and each
    /// instruction-specific resolver documents its own requirements.
    ///
    /// For `execute_v2`, if `ExecuteParams::exec_inputs` is `None`, account resolution is for buffered execution.
    /// In that case the first `resolve_accounts_start` call must include the execution inputs buffer PDA as an
    /// additional remaining account. The PDA is derived from
    /// `["execution_report_buffer", <buffer_id>, <execute_caller_pubkey>]`, where `execute_caller_pubkey` is the
    /// signer that will call `execute_v2`. The resolver reads that account to recover the message, then includes the
    /// buffer account in the returned `ask_again_with`/final account list as needed. Direct execution, where
    /// `exec_inputs` is present in the instruction data, does not require any additional resolver accounts.
    pub fn resolve_accounts_start(
        ctx: Context<ResolveAccountsStart>,
        params: ResolveAccountsParams,
    ) -> Result<ResolveAccountsResponse> {
        resolve_accounts_start::resolve_accounts_start(ctx, params)
    }

    /// Second account-resolution step for `get_ccvs_for_msg`.
    ///
    /// Callers should pass exactly the accounts returned by the previous
    /// `ask_again_with`. The resolver may ask for this same step once more with the receiver
    /// registry included so it can decide whether the receiver is v1 or v2 on-chain.
    pub fn resolve_accounts_get_ccvs_for_msg_stage(
        ctx: Context<ResolveAccountsGetCcvsForMsgStage>,
        params: ResolveAccountsParams,
    ) -> Result<ResolveAccountsResponse> {
        get_ccvs_for_msg::resolve_accounts::message_stage(ctx, params)
    }

    /// This is a second step on the account resolution process. Users should call `resolve_accounts_start` first,
    /// and by following that response and continuing with the process they will automatically invoke this method next,
    /// which will return more accounts needed for the operation they want to perform, and continue the process.
    ///
    /// For buffered `execute_v2` resolution, callers should pass exactly the accounts returned by
    /// `resolve_accounts_start.ask_again_with`. This includes the execution report buffer account as an additional
    /// remaining account after the declared `config` and `reference_addresses` accounts.
    pub fn resolve_accounts_execute_message_stage(
        ctx: Context<ResolveAccountsExecuteMessageStageContext>,
        params: ResolveAccountsParams,
    ) -> Result<ResolveAccountsResponse> {
        execute::resolve_accounts::message_stage(ctx, params)
    }

    /// Token-transfer account-resolution step that hands back the transferred token's lookup table.
    ///
    /// Reads the token's `TokenAdminRegistry` to learn its LUT, registers that LUT so the caller
    /// loads it, and asks again with the LUT included so the next stage can read the pool addresses
    /// out of it. Saves no accounts of its own.
    ///
    /// In buffered mode the execution-inputs buffer arrives as the first remaining account and is
    /// forwarded to the next stage, which is the first one that needs to read the message.
    pub fn resolve_accounts_execute_token_transfer_retrieve_lut_stage<'info>(
        ctx: Context<'info, ResolveAccountsExecuteTokenTransferRetrieveLutStage<'info>>,
        params: ResolveAccountsParams,
    ) -> Result<ResolveAccountsResponse> {
        execute::resolve_accounts::retrieve_token_lut_stage(ctx, params)
    }

    /// Token-transfer account-resolution step that derives the fixed set of pool accounts.
    ///
    /// Runs after the retrieve-LUT stage, which supplies the token's lookup table. Reads the pool
    /// program, mint and token program out of the LUT and the token transfer out of the message,
    /// then saves the accounts the `release_or_mint` CPI needs. Pools that resolve their own extra
    /// accounts continue into the nested-token stage from here.
    ///
    /// A lookup table too large for one 1kB response is emitted a page at a time: the stage then
    /// points back at itself, so callers must keep following `next_ix_discriminator` rather than
    /// assuming a single round.
    ///
    /// Callers should pass exactly the accounts returned by the previous `ask_again_with`; in
    /// buffered mode that includes the execution-inputs buffer as the first remaining account.
    pub fn resolve_accounts_execute_token_transfer_static_accounts_stage<'info>(
        ctx: Context<'info, ResolveAccountsExecuteTokenTransferStaticAccountsStage<'info>>,
        params: ResolveAccountsParams,
    ) -> Result<ResolveAccountsResponse> {
        execute::resolve_accounts::retrieve_token_static_accounts_stage(ctx, params)
    }

    /// Account-resolution step that delegates to the token pool's own resolver, for pools that implement account resolution (CCTP, Lombard).
    ///
    /// Repeats until the pool reports it has nothing left to resolve: each round CPIs into the
    /// pool's resolver as the offramp's pool signer and forwards the inputs the pool returned for
    /// the next round. The first round seeds it with `TokenPoolCpiResolutionInputs::ReleaseOrMint`.
    ///
    /// Callers should pass exactly the accounts returned by the previous `ask_again_with`; in
    /// buffered mode the execution-inputs buffer comes first and the pool's resolver accounts
    /// follow.
    pub fn resolve_accounts_execute_nested_token_stage<'info>(
        ctx: Context<'info, ResolveAccountsExecuteNestedTokenStage<'info>>,
        params: ResolveAccountsParams,
    ) -> Result<ResolveAccountsResponse> {
        execute::resolve_accounts::nested_token_stage(ctx, params)
    }

    /// This is a step on the account resolution process, which repeats at least once
    /// per ccv (potentially more as it's nested). Users should call `resolve_accounts_start` first,
    /// and by following that response and continuing with the process they will automatically invoke this method next,
    /// which will return more accounts needed for the operation they want to perform, and continue the process.
    pub fn resolve_accounts_execute_ccv_stage<'info>(
        ctx: Context<'info, ResolveAccountsExecuteCcvStageContext<'info>>,
        params: ResolveAccountsParams,
    ) -> Result<ResolveAccountsResponse> {
        execute::resolve_accounts::ccv_stage(ctx, params)
    }

    /// CCIPv2 message execution. Invoke this method to execute a message, either directly or buffered.
    /// This method is permissionless - anyone can execute a message on behalf of anyone else, as long as the message
    /// is valid and all the required verifier results are valid.
    pub fn execute_v2<'info>(
        ctx: Context<'info, ExecuteV2<'info>>,
        params: ExecuteParams,
    ) -> Result<()> {
        execute::execute_v2(ctx, params)
    }

    /// Get CCVs for a given message. To resolve the list, it takes into account the lane configurations, the
    /// pool's CCVs (when there is a token transfer), and the receiver contract's CCVs (when there is an arbitrary
    /// message), among other factors.
    pub fn get_ccvs_for_msg<'info>(
        ctx: Context<'info, GetCcvsForMsg<'info>>,
        params: GetCcvsForMsgParams,
    ) -> Result<GetCcvsForMsgResponse> {
        get_ccvs_for_msg::get_ccvs_for_msg(ctx, params)
    }

    /// Loads execution inputs into a buffer account for later execution. This is used for large messages that
    /// cannot be executed in a single transaction due to Solana's transaction size limits. Only the authority
    /// that created the buffer can later on use it to execute the message.
    pub fn buffer_execution_inputs<'info>(
        ctx: Context<'info, BufferExecutionInputsContext<'info>>,
        params: BufferExecutionInputsParams,
    ) -> Result<()> {
        buffer_execution_inputs::buffer_execution_inputs(ctx, params)
    }

    /// Closes the execution inputs buffer account, refunding its rent to the authority.
    /// It may only be invoked by the authority that originally created that buffer.
    pub fn close_execution_inputs_buffer(
        ctx: Context<CloseExecutionInputsBufferContext>,
        _params: CloseExecutionInputsBufferParams,
    ) -> Result<()> {
        emit_cpi!(BufferClosed {
            buffer_id: ctx.accounts.execution_inputs_buffer.buffer_id,
            authority: ctx.accounts.execution_inputs_buffer.authority,
        });
        Ok(())
    }
}

#[derive(Accounts)]
pub struct Empty {}
