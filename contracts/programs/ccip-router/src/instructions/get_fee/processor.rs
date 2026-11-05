use anchor_lang::prelude::*;
use ccip_common::messages::NormalizeWsol;
use ccip_common::seed;
use fee_quoter::messages_v2::ValidatedTokenPriceV2;

use crate::instructions::ccip_send::resolve_accounts::common::GET_FEE_PLACEHOLDER_SENDER;
use crate::instructions::common::QuoteGasForExecV2CpiInputs;
use crate::messages::GetFeeResultV2;
use crate::rmn::verify_uncursed_cpi;
use crate::{
    context::{valid_version, MAX_CONFIG_V},
    instructions::common::{
        collect_receipts, get_token_price_from_fee_quoter_cpi, validate_token_transfer,
        FeeQuoterCpiAccounts, OnrampRemainingAccountsLayout, OnrampResolutionMetadata,
        PreflightValidatedTokenTransfer, TokenTransferContext,
    },
    messages::SVM2AnyMessage,
    state::{Config, DestChainCcipV2},
    CcipRouterError, MAX_CHAINSTATECCIPV2_V,
};

#[derive(AnchorSerialize, AnchorDeserialize, Debug)]
pub struct GetFeeParams {
    pub dest_chain_selector: u64,
    pub message: SVM2AnyMessage,
    /// Opaque account-resolution metadata produced by account resolution.
    pub resolution_metadata: Vec<u8>,
}

/// CCIPv2 message fee retrieval
pub fn get_fee_v2<'info>(
    ctx: Context<'info, GetFeeV2<'info>>,
    params: GetFeeParams,
) -> Result<GetFeeResultV2> {
    verify_uncursed_cpi(
        &ctx.accounts.rmn_remote,
        &ctx.accounts.rmn_remote_curses,
        &ctx.accounts.rmn_remote_config,
        params.dest_chain_selector,
    )?;
    let fee_token = params.message.fee_token.normalize_wsol();
    // Resolution metadata is shared with CcipSend, as the layout for additional
    // accounts is identical.
    let resolution_metadata = OnrampResolutionMetadata::try_from_slice(&params.resolution_metadata)
        .map_err(|_| CcipRouterError::InvalidAccountListLengths)?;

    let layout = OnrampRemainingAccountsLayout::new(
        ctx.remaining_accounts,
        &resolution_metadata.account_list_lengths,
    )?;
    let dest_chain_config_v2 = &ctx.accounts.dest_chain_state.config;
    let mut extra_args = dest_chain_config_v2.parse_extra_args_with_defaults(&params.message)?;

    let ValidatedTokenPriceV2 {
        price: fee_token_price,
        billing_enabled,
    } = get_token_price_from_fee_quoter_cpi(
        ctx.accounts.config.fee_quoter,
        ctx.accounts.fee_quoter_config.to_account_info(),
        ctx.accounts
            .fee_quoter_billing_token_config
            .to_account_info(),
        fee_token,
    )?;
    require!(billing_enabled, CcipRouterError::UnsupportedFeeToken);

    let fee_quoter_cpi_accounts = FeeQuoterCpiAccounts {
        fee_quoter: ctx.accounts.config.fee_quoter,
        config: ctx.accounts.fee_quoter_config.to_account_info(),
        dest_chain: ctx.accounts.fee_quoter_dest_chain.to_account_info(),
    };

    let mut token_transfer_context = TokenTransferContext::new(
        &ctx.accounts.token_admin_registry,
        &params.message.token_amounts,
    )?;

    let (token_pool_required_ccvs, token_pool_receipt) = match &token_transfer_context {
        TokenTransferContext::None => (vec![], None),
        TokenTransferContext::Legacy { .. } | TokenTransferContext::V2(_) => {
            let validated = validate_token_transfer(
                GET_FEE_PLACEHOLDER_SENDER,
                params.dest_chain_selector,
                fee_token,
                extra_args.finality,
                ctx.program_id.key(),
                &fee_quoter_cpi_accounts,
                &mut token_transfer_context,
                layout.token_transfer_accounts,
                &extra_args,
                false,
            )?;
            let pool_required_ccvs = validated.pool_required_ccvs();
            let receipt = match validated {
                PreflightValidatedTokenTransfer::Legacy { receipt, .. }
                | PreflightValidatedTokenTransfer::V2 { receipt, .. } => receipt,
            };
            (pool_required_ccvs, Some(receipt))
        }
    };

    extra_args.merge_ccv_lists(
        &token_pool_required_ccvs,
        &ctx.accounts.dest_chain_state.config.lane_mandated_ccvs,
        &ctx.accounts.dest_chain_state.config.default_ccvs,
    )?;

    let receipts = collect_receipts(
        &layout,
        &ctx.accounts.dest_chain_state.config,
        params.dest_chain_selector,
        &params.message,
        extra_args.finality,
        token_pool_receipt,
        &extra_args,
        &fee_token_price,
        &token_transfer_context,
        QuoteGasForExecV2CpiInputs::new(
            ctx.accounts.fee_quoter.key(),
            ctx.accounts.fee_quoter_config.to_account_info(),
            ctx.accounts
                .fee_quoter_billing_token_config
                .to_account_info(),
            ctx.accounts.fee_quoter_dest_chain.to_account_info(),
            ctx.accounts.fee_quoter_dest_chain_legacy.to_account_info(),
            params.message.fee_token,
            params.dest_chain_selector,
        ),
    )?;

    Ok(GetFeeResultV2 {
        amount: receipts.fee_token_amount_sum,
        token: fee_token,
    })
}

#[event_cpi]
#[derive(Accounts)]
#[instruction(params: GetFeeParams)]
pub struct GetFeeV2<'info> {
    #[account(
        seeds = [seed::CONFIG],
        bump,
        constraint = valid_version(config.version, MAX_CONFIG_V) @ CcipRouterError::InvalidVersion,
    )]
    pub config: Account<'info, Config>,

    #[account(
        seeds = [seed::DEST_CHAIN_STATE_V2, params.dest_chain_selector.to_le_bytes().as_ref()],
        bump = dest_chain_state.bump,
        constraint = valid_version(dest_chain_state.version, MAX_CHAINSTATECCIPV2_V) @ CcipRouterError::InvalidVersion,
    )]
    pub dest_chain_state: Account<'info, DestChainCcipV2>,

    /// CHECK: When there is a token transfer, this contains the token admin registry entry for the transferred
    /// token. When there isn't, it's the zero pubkey. This is stored separately to the token accounts as it's
    /// necessary to determine the schema for the token account layout. The handler validates it against the
    /// registry PDA derived from the transferred token before using it.
    pub token_admin_registry: UncheckedAccount<'info>,

    ////////////////////
    // fee quoter CPI //
    ////////////////////
    ///
    /// CHECK: This is the account for the Fee Quoter program
    #[account(
        address = config.fee_quoter @ CcipRouterError::InvalidVersion,
    )]
    pub fee_quoter: UncheckedAccount<'info>,

    /// CHECK: This account is just used in the CPI to the Fee Quoter program
    #[account(
        seeds = [seed::CONFIG],
        bump,
        seeds::program = config.fee_quoter,
    )]
    pub fee_quoter_config: UncheckedAccount<'info>,

    /// CHECK: This account is just used in the CPI to the Fee Quoter program
    #[account(
        seeds = [seed::DEST_CHAIN_V2, params.dest_chain_selector.to_le_bytes().as_ref()],
        bump,
        seeds::program = config.fee_quoter,
    )]
    pub fee_quoter_dest_chain: UncheckedAccount<'info>,

    /// CHECK: This account is just used in the CPI to the Fee Quoter program
    #[account(
        seeds = [seed::DEST_CHAIN, params.dest_chain_selector.to_le_bytes().as_ref()],
        bump,
        seeds::program = config.fee_quoter,
    )]
    pub fee_quoter_dest_chain_legacy: UncheckedAccount<'info>,

    /// CHECK: This account is just used in the CPI to the Fee Quoter program
    #[account(
        seeds = [seed::FEE_BILLING_TOKEN_CONFIG, params.message.fee_token.normalize_wsol().as_ref()],
        bump,
        seeds::program = config.fee_quoter,
    )]
    pub fee_quoter_billing_token_config: UncheckedAccount<'info>,

    /// CHECK: This account is just used in the CPI to the Fee Quoter program
    #[account(
        seeds = [seed::FEE_BILLING_TOKEN_CONFIG,
            config.link_token_mint.key().as_ref(),
        ],
        bump,
        seeds::program = config.fee_quoter,
    )]
    pub fee_quoter_link_token_config: UncheckedAccount<'info>,

    ////////////////////
    // RMN Remote CPI //
    ////////////////////
    /// CHECK: This is the account for the RMN Remote program
    #[account(
        address = config.rmn_remote @ CcipRouterError::InvalidRMNRemoteAddress,
    )]
    pub rmn_remote: UncheckedAccount<'info>,

    /// CHECK: This account is just used in the CPI to the RMN Remote program
    #[account(
        seeds = [seed::CURSES],
        bump,
        seeds::program = config.rmn_remote,
    )]
    pub rmn_remote_curses: UncheckedAccount<'info>,

    /// CHECK: This account is just used in the CPI to the RMN Remote program
    #[account(
        seeds = [seed::CONFIG],
        bump,
        seeds::program = config.rmn_remote,
    )]
    pub rmn_remote_config: UncheckedAccount<'info>,
    // Remaining accounts (CCIP 2.0 fees, dynamic length)
    // CCV accounts from MSG
    // CCV accounts mandated by pool
    // CCV accounts mandated by lane
    // Token Pool accounts for custom fees
}
