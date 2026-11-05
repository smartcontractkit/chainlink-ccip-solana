use anchor_lang::prelude::*;

pub mod security;

declare_id!("Exex3nkLcxMFASH75gudot5ZSQXUMRc8BrYaYHTDJrt4");

mod event;
mod state;

mod instructions;
use crate::instructions::admin::*;
use crate::instructions::get_fee;
pub use crate::instructions::get_fee::GetFeeParams;
use crate::instructions::get_fee::*;
use crate::instructions::observe;
use crate::instructions::observe::*;
use crate::instructions::resolve_accounts_start;
use crate::instructions::resolve_accounts_start::*;
use crate::instructions::set_chain_config;
use crate::instructions::set_chain_config::*;

mod error;

use ccip_common::observe::ObservePageParams;
use ccip_common::resolution::{ResolveAccountsParams, ResolveAccountsResponse};
use ccip_common::seed;
use ccip_common::types::UsdCents;

#[program]
pub mod executor {

    use anchor_spl::token_interface;

    use crate::error::ExecutorError;

    use super::*;

    /// Initializes the Executor Config account.
    ///
    /// Restricted to the program upgrade authority.
    pub fn initialize(ctx: Context<InitializeConfig>, params: InitializeParams) -> Result<()> {
        instructions::admin::initialize(ctx, params)
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

    pub fn set_config(ctx: Context<UpdateConfig>, params: SetConfigParams) -> Result<()> {
        instructions::admin::set_config(ctx, params)
    }

    /// Sets the per-chain configuration for a remote destination chain.
    pub fn set_chain_config(
        ctx: Context<SetChainConfig>,
        params: SetChainConfigParams,
    ) -> Result<()> {
        set_chain_config::set_chain_config(ctx, params)
    }

    /// Replaces the CCV allowlist with `ccvs` and sets the allowlist flag, emitting one event per
    /// CCV that actually entered or left the list.
    pub fn set_allowed_ccvs(
        ctx: Context<UpdateConfig>,
        params: SetAllowedCcvsParams,
    ) -> Result<()> {
        instructions::admin::set_allowed_ccvs(ctx, params)
    }

    /// Delta-shaped counterpart to `set_allowed_ccvs`, matching EVM's
    /// `Executor.applyAllowedCCVUpdates(ccvsToRemove, ccvsToAdd, ccvAllowlistEnabled)` argument for
    /// argument: removals are applied first, then additions, then the allowlist flag. Both lists
    /// are idempotent.
    pub fn apply_allowed_ccv_updates(
        ctx: Context<UpdateConfig>,
        params: ApplyAllowedCcvUpdatesParams,
    ) -> Result<()> {
        instructions::admin::apply_allowed_ccv_updates(ctx, params)
    }

    pub fn get_fee(ctx: Context<GetFee>, params: GetFeeParams) -> Result<UsdCents> {
        get_fee::get_fee(ctx, params)
    }

    /// Returns this executor's global configuration, for observability consumers.
    ///
    /// Permissionless and read-only: meant to be executed through `simulateTransaction`. The CCV
    /// allow list is reported as a count only; page through it with `observe_allowed_ccvs`.
    pub fn observe_config(ctx: Context<ObserveConfig>) -> Result<ExecutorConfigObservation> {
        observe::observe_config(ctx)
    }

    /// Returns a page of this executor's CCV allow list, for observability consumers.
    ///
    /// Permissionless and read-only: meant to be executed through `simulateTransaction`.
    pub fn observe_allowed_ccvs(
        ctx: Context<ObserveConfig>,
        page: ObservePageParams,
    ) -> Result<ExecutorAllowedCcvsObservation> {
        observe::observe_allowed_ccvs(ctx, page)
    }

    /// Returns this executor's configuration for a single destination chain, for observability
    /// consumers.
    ///
    /// Permissionless and read-only: meant to be executed through `simulateTransaction`. Fails if
    /// the chain has no configuration, which callers can read as "not configured".
    pub fn observe_chain_config(
        ctx: Context<ObserveChainConfig>,
        remote_chain_selector: u64,
    ) -> Result<ExecutorChainConfigObservation> {
        observe::observe_chain_config(ctx, remote_chain_selector)
    }

    /// Sweeps every fee this executor accrued in `fee_token_mint` into the configured fee
    /// aggregator's ATA.
    ///
    /// Permissionless: the destination is pinned to `config.fee_aggregator`, so a caller can only
    /// trigger the sweep, never redirect it.
    pub fn withdraw_fee_tokens(ctx: Context<WithdrawFeeTokens>) -> Result<()> {
        require_keys_neq!(
            ctx.accounts.config.fee_aggregator,
            Pubkey::default(),
            ExecutorError::FeeAggregatorNotSet
        );

        let transfer = token_interface::TransferChecked {
            from: ctx.accounts.fee_vault_token_account.to_account_info(),
            to: ctx.accounts.fee_aggregator_token_account.to_account_info(),
            mint: ctx.accounts.fee_token_mint.to_account_info(),
            authority: ctx.accounts.fee_vault.to_account_info(),
        };

        require_gt!(
            ctx.accounts.fee_vault_token_account.amount,
            0,
            ExecutorError::InsufficientFunds
        );

        let token_program = *ctx.accounts.fee_token_program.key;
        let amount = ctx.accounts.fee_vault_token_account.amount;
        let decimals = ctx.accounts.fee_token_mint.decimals;

        let seeds = &[seed::FEE_VAULT, &[ctx.bumps.fee_vault]];
        let signer_seeds = &[&seeds[..]];
        let cpi_ctx = CpiContext::new_with_signer(token_program, transfer, signer_seeds);
        token_interface::transfer_checked(cpi_ctx, amount, decimals)
    }

    /// Entrypoint to resolve accounts for any supported instruction.
    pub fn resolve_accounts_start(
        ctx: Context<ResolveAccountsStart>,
        params: ResolveAccountsParams,
    ) -> Result<ResolveAccountsResponse> {
        resolve_accounts_start::resolve_accounts_start(ctx, params)
    }
}

#[derive(Accounts)]
pub struct Empty<'info> {
    // This is unused, but Anchor requires that there is at least one account in the context
    // Issue: https://github.com/solana-foundation/anchor/issues/2342 (might be patched in 2.0)
    pub clock: Sysvar<'info, Clock>,
}
