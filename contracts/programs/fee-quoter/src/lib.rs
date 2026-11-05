use anchor_lang::error_code;
use anchor_lang::prelude::*;

pub mod security;

declare_id!("FeeQPGkKDeRV1MgoYfMH6L8o3KeuYjwUZrgn4LRKfjHi");

pub mod context;
use context::*;

pub mod messages;
use messages::*;

pub mod state;
use state::*;

pub mod state_v2;

pub mod event;
use event::*;

pub mod messages_v2;
use messages_v2::*;

mod instructions;
use instructions::router;

mod instructions_v2;
use instructions_v2::dest_chain::*;
use instructions_v2::fallback_get_token_transfer_fee::*;
use instructions_v2::observe;
use instructions_v2::observe::*;
use instructions_v2::quote_gas_for_exec_v2::*;
use instructions_v2::token_price::*;

// 1e18 Juels = 1 LINK natively (in EVM.) In SVM, the LINK mint likely has different decimals.
pub const LINK_JUEL_DECIMALS: u8 = 18;

#[program]
pub mod fee_quoter {

    use super::*;

    /// Initializes the Fee Quoter.
    ///
    /// The initialization is responsibility of Admin, nothing more than calling this method should be done first.
    ///
    /// # Arguments
    ///
    /// * `ctx` - The context containing the accounts required for initialization.
    /// * `max_fee_juels_per_msg` - The maximum fee in juels that can be charged per message.
    /// * `onramp` - The public key of the onramp.
    ///
    /// The function also uses the link_token_mint account from the context.
    pub fn initialize(
        ctx: Context<Initialize>,
        max_fee_juels_per_msg: u128,
        onramp: Pubkey,
    ) -> Result<()> {
        require_gte!(
            LINK_JUEL_DECIMALS,
            ctx.accounts.link_token_mint.decimals,
            FeeQuoterError::InvalidInputsMint
        );

        // trivial non-zero check, the value is provided in the expected 18-decimal format
        require_gt!(max_fee_juels_per_msg, 0, FeeQuoterError::InvalidInputs);

        ctx.accounts.config.set_inner(Config {
            version: MAX_CONFIG_V,
            owner: ctx.accounts.authority.key(),
            proposed_owner: Pubkey::default(),
            max_fee_juels_per_msg,
            link_token_mint: ctx.accounts.link_token_mint.key(),
            link_token_local_decimals: ctx.accounts.link_token_mint.decimals,
            onramp,
            default_code_version: CodeVersion::V1,
        });

        emit!(ConfigSet {
            max_fee_juels_per_msg,
            link_token_mint: ctx.accounts.link_token_mint.key(),
            link_token_local_decimals: ctx.accounts.link_token_mint.decimals,
            onramp,
            default_code_version: CodeVersion::V1,
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

    /// Returns the fee quoter's global configuration, for observability consumers.
    ///
    /// Permissionless and read-only: meant to be executed through `simulateTransaction`.
    ///
    /// # Arguments
    /// * `ctx` - The context containing the fee quoter config account
    pub fn observe_config(ctx: Context<ObserveConfig>) -> Result<FeeQuoterConfigObservation> {
        observe::observe_config(ctx)
    }

    /// Returns the CCIP 2.0 fee configuration and gas price state for a single destination chain,
    /// for observability consumers.
    ///
    /// Permissionless and read-only: meant to be executed through `simulateTransaction`. Fails if
    /// the lane has no `dest_chain_v2` account, which callers can read as "not configured".
    ///
    /// # Arguments
    /// * `ctx` - The context containing the dest chain account
    /// * `chain_selector` - The destination chain selector the lane belongs to
    pub fn observe_dest_chain_v2(
        ctx: Context<ObserveDestChainV2>,
        chain_selector: u64,
    ) -> Result<FeeQuoterDestChainV2Observation> {
        observe::observe_dest_chain_v2(ctx, chain_selector)
    }

    /// Transfers the ownership of the fee quoter to a new proposed owner.
    ///
    /// Shared func signature with other programs
    ///
    /// # Arguments
    ///
    /// * `ctx` - The context containing the accounts required for the transfer.
    /// * `proposed_owner` - The public key of the new proposed owner.
    pub fn transfer_ownership(ctx: Context<UpdateConfig>, new_owner: Pubkey) -> Result<()> {
        router::admin(ctx.accounts.config.default_code_version).transfer_ownership(ctx, new_owner)
    }

    /// Accepts the ownership of the fee quoter by the proposed owner.
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
        ctx: Context<UpdateConfig>,
        code_version: CodeVersion,
    ) -> Result<()> {
        router::admin(ctx.accounts.config.default_code_version)
            .set_default_code_version(ctx, code_version)
    }

    /// Sets the max_fee_juels_per_msg, which is an upper bound on how much can be billed for any message.
    /// (1 juels = 1e-18 LINK)
    ///
    /// Only the admin may set this.
    ///
    /// # Arguments
    ///
    /// * `ctx` - The context containing the accounts required for updating the configuration.
    /// * `max_fee_juels_per_msg` - The new value for the max_feel_juels_per_msg config.
    pub fn set_max_fee_juels_per_msg(
        ctx: Context<UpdateConfig>,
        max_fee_juels_per_msg: u128,
    ) -> Result<()> {
        router::admin(ctx.accounts.config.default_code_version)
            .set_max_fee_juels_per_msg(ctx, max_fee_juels_per_msg)
    }

    /// Sets the link_token_mint and updates the link_token_local_decimals.
    ///
    /// Only the admin may set this.
    ///
    /// # Arguments
    ///
    /// * `ctx` - The context containing the accounts required for updating the configuration.
    pub fn set_link_token_mint(ctx: Context<UpdateConfigLinkMint>) -> Result<()> {
        router::admin(ctx.accounts.config.default_code_version).set_link_token_mint(ctx)
    }

    /// Adds a billing token configuration.
    /// Only CCIP Admin can add a billing token configuration.
    ///
    /// # Arguments
    ///
    /// * `ctx` - The context containing the accounts required for adding the billing token configuration.
    /// * `billing_token_config_input` - The billing token configuration to be added.
    pub fn add_billing_token_config(
        ctx: Context<AddBillingTokenConfig>,
        billing_token_config_input: BillingTokenConfig,
    ) -> Result<()> {
        router::admin(ctx.accounts.config.default_code_version)
            .add_billing_token_config(ctx, billing_token_config_input)
    }

    /// Updates the billing token configuration.
    /// Only CCIP Admin can update a billing token configuration.
    ///
    /// # Arguments
    ///
    /// * `ctx` - The context containing the accounts required for updating the billing token configuration.
    /// * `billing_token_config_input` - The new billing token configuration.
    pub fn update_billing_token_config(
        ctx: Context<UpdateBillingTokenConfig>,
        billing_token_config_input: BillingTokenConfig,
    ) -> Result<()> {
        router::admin(ctx.accounts.config.default_code_version)
            .update_billing_token_config(ctx, billing_token_config_input)
    }

    /// Adds a new destination chain selector to the fee quoter.
    ///
    /// The Admin needs to add any new chain supported.
    /// When adding a new chain, the Admin needs to specify if it's enabled or not.
    ///
    /// # Arguments
    ///
    /// * `ctx` - The context containing the accounts required for adding the chain selector.
    /// * `chain_selector` - The new chain selector to be added.
    /// * `dest_chain_config` - The configuration for the chain as destination.
    pub fn add_dest_chain(
        ctx: Context<AddDestChain>,
        chain_selector: u64,
        dest_chain_config: DestChainConfig,
    ) -> Result<()> {
        router::admin(ctx.accounts.config.default_code_version).add_dest_chain(
            ctx,
            chain_selector,
            dest_chain_config,
        )
    }

    /// Disables the destination chain selector.
    ///
    /// The Admin is the only one able to disable the chain selector as destination. This method is thought of as an emergency kill-switch.
    ///
    /// # Arguments
    ///
    /// * `ctx` - The context containing the accounts required for disabling the chain selector.
    /// * `chain_selector` - The destination chain selector to be disabled.
    pub fn disable_dest_chain(
        ctx: Context<UpdateDestChainConfig>,
        chain_selector: u64,
    ) -> Result<()> {
        router::admin(ctx.accounts.config.default_code_version)
            .disable_dest_chain(ctx, chain_selector)
    }

    /// Updates the configuration of the destination chain selector.
    ///
    /// The Admin is the only one able to update the destination chain config.
    ///
    /// # Arguments
    ///
    /// * `ctx` - The context containing the accounts required for updating the chain selector.
    /// * `chain_selector` - The destination chain selector to be updated.
    /// * `dest_chain_config` - The new configuration for the destination chain.
    pub fn update_dest_chain_config(
        ctx: Context<UpdateDestChainConfig>,
        chain_selector: u64,
        dest_chain_config: DestChainConfig,
    ) -> Result<()> {
        router::admin(ctx.accounts.config.default_code_version).update_dest_chain_config(
            ctx,
            chain_selector,
            dest_chain_config,
        )
    }

    /// Sets the token transfer fee configuration for a particular token when it's transferred to a particular dest chain.
    /// It is an upsert, initializing the per-chain-per-token config account if it doesn't exist
    /// and overwriting it if it does.
    ///
    /// Only the Admin can perform this operation.
    ///
    /// # Arguments
    ///
    /// * `ctx` - The context containing the accounts required for setting the token billing configuration.
    /// * `chain_selector` - The chain selector.
    /// * `mint` - The public key of the token mint.
    /// * `cfg` - The token transfer fee configuration.
    pub fn set_token_transfer_fee_config(
        ctx: Context<SetTokenTransferFeeConfig>,
        chain_selector: u64,
        mint: Pubkey,
        cfg: TokenTransferFeeConfig,
    ) -> Result<()> {
        router::admin(ctx.accounts.config.default_code_version).set_token_transfer_fee_config(
            ctx,
            chain_selector,
            mint,
            cfg,
        )
    }

    /// Add a price updater address to the list of allowed price updaters.
    /// On price updates, the fee quoter will check the that caller is allowed.
    ///
    /// # Arguments
    ///
    /// * `ctx` - The context containing the accounts required for this operation.
    /// * `price_updater` - The price updater address.
    pub fn add_price_updater(ctx: Context<AddPriceUpdater>, price_updater: Pubkey) -> Result<()> {
        router::admin(ctx.accounts.config.default_code_version)
            .add_price_updater(ctx, price_updater)
    }

    /// Remove a price updater address from the list of allowed price updaters.
    ///
    /// # Arguments
    ///
    /// * `ctx` - The context containing the accounts required for this operation.
    /// * `price_updater` - The price updater address.
    pub fn remove_price_updater(
        ctx: Context<RemovePriceUpdater>,
        price_updater: Pubkey,
    ) -> Result<()> {
        router::admin(ctx.accounts.config.default_code_version)
            .remove_price_updater(ctx, price_updater)
    }

    /// Calculates the fee for sending a message to the destination chain.
    ///
    /// # Arguments
    ///
    /// * `ctx` - The context containing the accounts required for the fee calculation.
    /// * `dest_chain_selector` - The chain selector for the destination chain.
    /// * `message` - The message to be sent.
    ///
    /// # Additional accounts
    ///
    /// In addition to the fixed amount of accounts defined in the `GetFee` context,
    /// the following accounts must be provided:
    ///
    /// * First, the billing token config accounts for each token sent with the message, sequentially.
    ///   For each token with no billing config account (i.e. tokens that cannot be possibly used as fee
    ///   tokens, which also have no BPS fees enabled) the ZERO address must be provided instead.
    /// * Then, the per chain / per token config of every token sent with the message, sequentially
    ///   in the same order.
    ///
    /// # Returns
    ///
    /// GetFeeResult struct with:
    /// - the fee token mint address,
    /// - the fee amount of said token,
    /// - the fee value in juels,
    /// - additional data required when performing the cross-chain transfer of tokens in that message
    /// - deserialized and processed extra args
    pub fn get_fee<'info>(
        ctx: Context<'info, GetFee>,
        dest_chain_selector: u64,
        message: SVM2AnyMessage,
    ) -> Result<GetFeeResult> {
        let default_code_version = ctx.accounts.config.default_code_version;
        let lane_code_version = ctx.accounts.dest_chain.config.lane_code_version;
        router::public(lane_code_version, default_code_version).get_fee(
            ctx,
            dest_chain_selector,
            message,
        )
    }

    /// Updates prices for tokens and gas. This method may only be called by an allowed price updater.
    ///
    /// # Arguments
    ///
    /// * `ctx` - The context containing the accounts always required for the price updates
    /// * `token_updates` - Vector of token price updates
    /// * `gas_updates` - Vector of gas price updates
    ///
    /// # Additional accounts
    ///
    /// In addition to the fixed amount of accounts defined in the `UpdatePrices` context,
    /// the following accounts must be provided:
    ///
    /// * First, the billing token config accounts for each token whose price is being updated, in the same order
    /// as the token_updates vector.
    /// * Then, the dest chain accounts of every chain whose gas price is being updated, in the same order as the
    /// gas_updates vector.
    pub fn update_prices<'info>(
        ctx: Context<'info, UpdatePrices<'info>>,
        token_updates: Vec<TokenPriceUpdate>,
        gas_updates: Vec<GasPriceUpdate>,
    ) -> Result<()> {
        router::prices(ctx.accounts.config.default_code_version).update_prices(
            ctx,
            token_updates,
            gas_updates,
        )
    }

    /// Updates the configuration of the destination chain selector.
    ///
    /// The Admin is the only one able to update the destination chain config.
    ///
    /// # Arguments
    ///
    /// * `ctx` - The context containing the accounts required for updating the chain selector.
    /// * `params` - The params including the chain selector and the new configs.
    pub fn upsert_dest_chain_config_v2(
        ctx: Context<UpsertDestChainConfigV2>,
        params: UpsertDestChainConfigV2Params,
    ) -> Result<()> {
        instructions_v2::dest_chain::upsert_dest_chain_config_v2(ctx, params)
    }

    /// Disables the destination chain selector (V2).
    ///
    /// The Admin is the only one able to disable the chain selector as destination.
    /// This method is thought of as an emergency kill-switch.
    ///
    /// # Arguments
    ///
    /// * `ctx` - The context containing the accounts required for disabling the chain selector.
    /// * `chain_selector` - The destination chain selector to be disabled.
    pub fn disable_dest_chain_v2(
        ctx: Context<DisableDestChainConfigV2>,
        chain_selector: u64,
    ) -> Result<()> {
        instructions_v2::dest_chain::disable_dest_chain_v2(ctx, chain_selector)
    }

    /// Returns the token's validated USD price (18 decimals) from its billing token config.
    ///
    /// Fails with `InvalidTokenPrice` if the price was never set (zero value or timestamp).
    ///
    /// # Arguments
    ///
    /// * `ctx` - The context containing the billing token config account for the mint.
    /// * `mint` - The token mint to get the price for.
    pub fn get_validated_token_price_v2(
        ctx: Context<GetValidatedTokenPriceV2>,
        mint: Pubkey,
    ) -> Result<ValidatedTokenPriceV2> {
        instructions_v2::token_price::get_validated_token_price_v2(ctx, mint)
    }

    /// Returns the resolved token transfer fee components for a token transfer.
    ///
    /// This function will check token-specific config first, then fall back to destination chain defaults.
    ///
    /// IMPORTANT: This is a fallback used when the pool doesn't support custom fees and/or the token
    /// pool is V1 in a V2 lane. For V2 token pools, always check it for a receipt first.
    ///
    /// # Arguments
    /// * `dest_chain_selector` - The destination chain selector.
    /// * `mint` - The transferred token mint
    pub fn get_fallback_token_transfer_fee_v2<'info>(
        ctx: Context<'info, GetFallbackTokenTransferFeeV2<'info>>,
        params: GetFallbackTokenTransferFeeV2Params,
    ) -> Result<FallbackTokenTransferFeeV2> {
        instructions_v2::fallback_get_token_transfer_fee::fallback_get_token_transfer_fee_v2(
            ctx, params,
        )
    }

    /// Returns the gas quote for execution. This involves making a correction
    /// of the total gas required, as well as returning the complete cost for
    /// all (corrected) gas.
    ///
    /// It also returns the premium multiplier to use for the particular fee
    /// token.
    ///
    /// # Arguments
    /// * `params` - Parameters describing the execution, including the total byte
    /// overhead, total gas before correction, and fee information.
    pub fn quote_gas_for_exec_v2<'info>(
        ctx: Context<'info, QuoteGasForExecV2<'info>>,
        params: QuoteGasForExecV2Params,
    ) -> Result<QuoteGasForExecV2Result> {
        instructions_v2::quote_gas_for_exec_v2::quote_gas_for_exec_v2(ctx, params)
    }
}

#[error_code]
pub enum FeeQuoterError {
    #[msg("The signer is unauthorized")]
    // offset error code so that they don't clash with other programs
    // (Anchor's base custom error code 6000 + offset 2000 = start at 8000)
    Unauthorized = 2000,
    #[msg("Invalid inputs")]
    InvalidInputs,
    #[msg("Gas limit is zero")]
    ZeroGasLimit,
    #[msg("Default gas limit exceeds the maximum")]
    DefaultGasLimitExceedsMaximum,
    #[msg("Invalid version of the onchain state")]
    InvalidVersion,
    #[msg("Proposed owner is the current owner")]
    RedundantOwnerProposal,
    #[msg("Account should be writable")]
    InvalidInputsMissingWritable,
    #[msg("Chain selector is invalid")]
    InvalidInputsChainSelector,
    #[msg("Mint account input is invalid")]
    InvalidInputsMint,
    #[msg("Mint account input has an invalid owner")]
    InvalidInputsMintOwner,
    #[msg("Token config account is invalid")]
    InvalidInputsTokenConfigAccount,
    #[msg("Missing extra args in message to SVM receiver")]
    InvalidInputsMissingExtraArgs,
    #[msg("Missing data after extra args tag")]
    InvalidInputsMissingDataAfterExtraArgs,
    #[msg("Destination chain state account is invalid")]
    InvalidInputsDestChainStateAccount,
    #[msg("Per chain per token config account is invalid")]
    InvalidInputsPerChainPerTokenConfig,
    #[msg("Billing token config account is invalid")]
    InvalidInputsBillingTokenConfig,
    #[msg("Number of accounts provided is incorrect")]
    InvalidInputsAccountCount,
    #[msg("No price or gas update provided")]
    InvalidInputsNoUpdates,
    #[msg("Invalid token accounts")]
    InvalidInputsTokenAccounts,
    #[msg("Destination chain disabled")]
    DestinationChainDisabled,
    #[msg("Fee token disabled")]
    FeeTokenDisabled,
    #[msg("Message exceeds maximum data size")]
    MessageTooLarge,
    #[msg("Message contains an unsupported number of tokens")]
    UnsupportedNumberOfTokens,
    #[msg("Invalid token price")]
    InvalidTokenPrice,
    #[msg("Stale gas price")]
    StaleGasPrice,
    #[msg("Inputs are missing token configuration")]
    InvalidInputsMissingTokenConfig,
    #[msg("Message fee is too high")]
    MessageFeeTooHigh,
    #[msg("Message gas limit too high")]
    MessageGasLimitTooHigh,
    #[msg("Extra arg out of order execution must be true")]
    ExtraArgOutOfOrderExecutionMustBeTrue,
    #[msg("Invalid extra args tag")]
    InvalidExtraArgsTag,
    #[msg("Invalid amount of accounts in extra args")]
    InvalidExtraArgsAccounts,
    #[msg("Invalid writability bitmap in extra args")]
    InvalidExtraArgsWritabilityBitmap,
    #[msg("Invalid token receiver")]
    InvalidTokenReceiver,
    #[msg("The caller is not an authorized price updater")]
    UnauthorizedPriceUpdater,
    #[msg("Minimum token transfer fee exceeds maximum")]
    InvalidTokenTransferFeeMaxMin,
    #[msg("Insufficient dest bytes overhead on transfer fee config")]
    InvalidTokenTransferFeeDestBytesOverhead,
    #[msg("Invalid code version")]
    InvalidCodeVersion,
    #[msg("Proposed owner is the default pubkey")]
    DefaultOwnerProposal,
    #[msg("Dest chain is disabled")]
    DestChainDisabled,
    #[msg("Price was never set")]
    PriceNeverSet,
    #[msg("Invalid Sui receiver object ids")]
    InvalidSuiReceiverObjectIds,
    #[msg("LINK fee multiplier percent must be greater than zero")]
    ZeroLinkFeeMultiplier,
    #[msg("Dest gas per payload byte base must not exceed 255")]
    DestGasPerPayloadByteBaseTooLarge,
    #[msg("Network fee must not exceed 65535 USD cents")]
    NetworkFeeTooLarge,
}
