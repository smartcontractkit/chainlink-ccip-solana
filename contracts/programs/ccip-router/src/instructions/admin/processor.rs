use std::collections::HashSet;

use anchor_lang::prelude::*;
use ccip_common::seed;

use crate::{
    context::{valid_version, ANCHOR_DISCRIMINATOR, MAX_CONFIG_V},
    error::CcipRouterError,
    event::events::{self},
    state::{
        Config, DestChainCcipV2, DestChainConfigCcipV2, DestChainState, ReferenceAddresses,
        RestoreOnAction,
    },
    MAX_CHAINSTATECCIPV2_V,
};

/// Validates the default + lane-mandated CCV lists of a dest chain config, mirroring EVM
/// `CCVConfigValidation._validateDefaultAndMandatedCCVs`:
/// - at least one CCV across the two lists (so every message has a verifier),
/// - no zero/default pubkey,
/// - no duplicates within or across the two lists.
///
/// Enforcing this at config time lets the rest of the code assume a well-formed CCV set when
/// building `ccv_and_executor_hash` and pricing.
fn validate_default_and_mandated_ccvs(
    default_ccvs: &[Pubkey],
    lane_mandated_ccvs: &[Pubkey],
) -> Result<()> {
    require!(
        !default_ccvs.is_empty() || !lane_mandated_ccvs.is_empty(),
        CcipRouterError::MustSpecifyDefaultOrMandatedCcvs
    );

    let mut seen = HashSet::new();
    for ccv in default_ccvs.iter().chain(lane_mandated_ccvs.iter()) {
        require!(seen.insert(*ccv), CcipRouterError::DuplicateCcvNotAllowed);
    }

    require!(
        !seen.contains(&Pubkey::default()),
        CcipRouterError::CcvZeroAddressNotAllowed
    );

    Ok(())
}

fn validate_dest_chain_config_v2(dest_chain_config: &DestChainConfigCcipV2) -> Result<()> {
    require_gt!(
        dest_chain_config.address_bytes_length,
        0,
        CcipRouterError::InvalidInputsAddressBytesLength
    );

    // INV-LCFG-5
    require_keys_neq!(
        dest_chain_config.default_executor,
        Pubkey::default(),
        CcipRouterError::InvalidExecutor
    );

    validate_default_and_mandated_ccvs(
        &dest_chain_config.default_ccvs,
        &dest_chain_config.lane_mandated_ccvs,
    )?;

    // The offramp is stored as raw destination-chain-sized bytes, so its width must agree with
    // the configured address width. Executor pricing relies on that equality.
    require_eq!(
        dest_chain_config.offramp.len(),
        dest_chain_config.address_bytes_length as usize,
        CcipRouterError::InvalidInputsAddressBytesLength
    );

    Ok(())
}

#[derive(AnchorSerialize, AnchorDeserialize, Clone)]
pub struct AddDestChainV2Params {
    pub new_chain_selector: u64,
    pub dest_chain_config: DestChainConfigCcipV2,
}

pub fn add_dest_chain_v2(ctx: Context<AddDestChainV2>, params: AddDestChainV2Params) -> Result<()> {
    require_neq!(
        params.new_chain_selector,
        0,
        CcipRouterError::InvalidInputsChainSelector
    );

    validate_dest_chain_config_v2(&params.dest_chain_config)?;

    let dest_chain_config_v2 = params.dest_chain_config.clone();

    ctx.accounts.dest_chain_state.set_inner(DestChainCcipV2 {
        version: MAX_CHAINSTATECCIPV2_V,
        bump: ctx.bumps.dest_chain_state,
        chain_selector: params.new_chain_selector,
        config: DestChainConfigCcipV2 {
            lane_code_version: dest_chain_config_v2.lane_code_version,
            address_bytes_length: dest_chain_config_v2.address_bytes_length,
            token_receiver_allowed: dest_chain_config_v2.token_receiver_allowed,
            allowed_senders: dest_chain_config_v2.allowed_senders,
            allow_list_enabled: dest_chain_config_v2.allow_list_enabled,
            default_executor: dest_chain_config_v2.default_executor,
            default_ccvs: dest_chain_config_v2.default_ccvs,
            lane_mandated_ccvs: dest_chain_config_v2.lane_mandated_ccvs,
            offramp: dest_chain_config_v2.offramp,
            message_network_fee: dest_chain_config_v2.message_network_fee,
            token_transfer_network_fee: dest_chain_config_v2.token_transfer_network_fee,
            base_execution_gas_cost: dest_chain_config_v2.base_execution_gas_cost,
            max_fee_per_message: dest_chain_config_v2.max_fee_per_message,
        },
        state: DestChainState {
            message_number: 0,
            restore_on_action: RestoreOnAction::None,
            message_number_to_restore: 0,
        },
    });

    emit_cpi!(events::admin::DestChainAddedV2 {
        dest_chain_selector: params.new_chain_selector,
        dest_chain_config: params.dest_chain_config,
    });

    Ok(())
}

/// Adds a destination lane. Initializes the `DestChainCcipV2` PDA that `ccip_send_v2` reads.
///
/// Destination side only: the router holds no source-chain state, so enabling a lane in both
/// directions also requires the offramp's `add_source_chain`.
#[event_cpi]
#[derive(Accounts)]
#[instruction(params: AddDestChainV2Params)]
pub struct AddDestChainV2<'info> {
    #[account(
        init,
        seeds = [seed::DEST_CHAIN_STATE_V2, params.new_chain_selector.to_le_bytes().as_ref()],
        bump,
        payer = authority,
        space = ANCHOR_DISCRIMINATOR + DestChainCcipV2::INIT_SPACE + params.dest_chain_config.dynamic_space(),
    )]
    pub dest_chain_state: Account<'info, DestChainCcipV2>,

    #[account(
        seeds = [seed::CONFIG],
        bump,
        constraint = valid_version(config.version, MAX_CONFIG_V) @ CcipRouterError::InvalidVersion,
    )]
    pub config: Account<'info, Config>,

    #[account(mut, address = config.owner @ CcipRouterError::Unauthorized)]
    pub authority: Signer<'info>,
    pub system_program: Program<'info, System>,
}

#[derive(AnchorSerialize, AnchorDeserialize, Clone)]
pub struct UpdateChainSelectorV2Params {
    pub dest_chain_selector: u64,
    pub dest_chain_config: DestChainConfigCcipV2,
}

pub fn update_dest_chain_config_v2(
    ctx: Context<UpdateDestChainSelectorConfigV2>,
    params: UpdateChainSelectorV2Params,
) -> Result<()> {
    validate_dest_chain_config_v2(&params.dest_chain_config)?;

    let dest_chain_config_v2 = params.dest_chain_config.clone();

    ctx.accounts.dest_chain_state.config = DestChainConfigCcipV2 {
        lane_code_version: dest_chain_config_v2.lane_code_version,
        address_bytes_length: dest_chain_config_v2.address_bytes_length,
        token_receiver_allowed: dest_chain_config_v2.token_receiver_allowed,
        allowed_senders: dest_chain_config_v2.allowed_senders,
        allow_list_enabled: dest_chain_config_v2.allow_list_enabled,
        default_executor: dest_chain_config_v2.default_executor,
        default_ccvs: dest_chain_config_v2.default_ccvs,
        lane_mandated_ccvs: dest_chain_config_v2.lane_mandated_ccvs,
        offramp: dest_chain_config_v2.offramp,
        message_network_fee: dest_chain_config_v2.message_network_fee,
        token_transfer_network_fee: dest_chain_config_v2.token_transfer_network_fee,
        base_execution_gas_cost: dest_chain_config_v2.base_execution_gas_cost,
        max_fee_per_message: dest_chain_config_v2.max_fee_per_message,
    };

    emit_cpi!(events::admin::DestChainConfigUpdatedV2 {
        dest_chain_selector: params.dest_chain_selector,
        dest_chain_config: params.dest_chain_config,
    });
    Ok(())
}

#[event_cpi]
#[derive(Accounts)]
#[instruction(params: UpdateChainSelectorV2Params)]
pub struct UpdateDestChainSelectorConfigV2<'info> {
    #[account(
        mut,
        seeds = [seed::DEST_CHAIN_STATE_V2, params.dest_chain_selector.to_le_bytes().as_ref()],
        bump = dest_chain_state.bump,
        constraint = valid_version(dest_chain_state.version, MAX_CHAINSTATECCIPV2_V) @ CcipRouterError::InvalidVersion,
        realloc = ANCHOR_DISCRIMINATOR + DestChainCcipV2::INIT_SPACE + params.dest_chain_config.dynamic_space(),
        realloc::payer = authority,
        // `realloc::zero = false` is used here because we are only reallocating to a larger size.
        // The memory for the new allocation will be zeroed automatically, so we don't need to manually zero it.
        realloc::zero = false
    )]
    pub dest_chain_state: Account<'info, DestChainCcipV2>,

    #[account(
        seeds = [seed::CONFIG],
        bump,
        constraint = valid_version(config.version, MAX_CONFIG_V) @ CcipRouterError::InvalidVersion,
    )]
    pub config: Account<'info, Config>,

    #[account(mut, address = config.owner @ CcipRouterError::Unauthorized)]
    pub authority: Signer<'info>,
    pub system_program: Program<'info, System>,
}

#[event_cpi]
#[derive(Accounts)]
#[instruction(dest_chain_selector: u64)]
pub struct MigrateDestChainSelectorConfigV2<'info> {
    #[account(
        mut,
        seeds = [seed::DEST_CHAIN_STATE_V2, dest_chain_selector.to_le_bytes().as_ref()],
        bump,
        owner = crate::ID, // Initialized
    )]
    pub dest_chain: UncheckedAccount<'info>,

    #[account(
        seeds = [seed::CONFIG],
        bump,
        constraint = valid_version(config.version, MAX_CONFIG_V) @ CcipRouterError::InvalidVersion,
    )]
    pub config: Account<'info, Config>,

    #[account(mut, address = config.owner @ CcipRouterError::Unauthorized)]
    pub authority: Signer<'info>,
    pub system_program: Program<'info, System>,
}

pub fn update_reference_addresses(
    ctx: Context<UpdateReferenceAddresses>,
    onramp_lookup_table: Pubkey,
) -> Result<()> {
    ctx.accounts
        .reference_addresses
        .set_inner(ReferenceAddresses {
            version: 1,
            bump: ctx.bumps.reference_addresses,
            onramp_lookup_table,
        });

    emit_cpi!(events::admin::ReferenceAddressesSet {
        onramp_lookup_table
    });
    Ok(())
}

#[event_cpi]
#[derive(Accounts)]
pub struct UpdateReferenceAddresses<'info> {
    // Unlike the offramp's, created on first use rather than in `initialize`, so routers
    // initialized before it existed can still set it.
    #[account(
        init_if_needed,
        seeds = [seed::REFERENCE_ADDRESSES],
        bump,
        payer = authority,
        space = ANCHOR_DISCRIMINATOR + ReferenceAddresses::INIT_SPACE,
    )]
    pub reference_addresses: Account<'info, ReferenceAddresses>,

    #[account(
        seeds = [seed::CONFIG],
        bump,
        constraint = valid_version(config.version, MAX_CONFIG_V) @ CcipRouterError::InvalidVersion,
    )]
    pub config: Account<'info, Config>,

    #[account(mut, address = config.owner @ CcipRouterError::Unauthorized)]
    pub authority: Signer<'info>,
    pub system_program: Program<'info, System>,
}
