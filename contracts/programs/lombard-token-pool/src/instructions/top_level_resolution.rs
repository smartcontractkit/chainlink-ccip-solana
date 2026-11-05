//! Combined top-level `resolve_accounts_start` dispatcher
//!
//! Off-chain tooling convenience only - no on-chain protocol flow depends on this instruction
//! succeeding. Pool-v1 admin instructions use the older, separate `derive_accounts_*` staged
//! protocol (see `instructions/derive_accounts.rs`) and are intentionally not covered here.

use anchor_lang::prelude::*;
use ccip_common::resolution::*;

use crate::constants::{
    VERIFIER_CONFIG_SEED, VERIFIER_REMOTE_CHAIN_SEED, VERIFIER_STORAGE_LOCATIONS_SEED,
};
use crate::errors::LombardTokenPoolError;
use crate::instruction;
use crate::instructions::verifier::chain_config::{
    SetVerifierAllowlistParams, SetVerifierChainConfigParams,
};
use crate::state::VerifierConfig;

#[derive(Accounts)]
pub struct TopLevelResolveAccountsStart<'info> {
    #[account(seeds = [VERIFIER_CONFIG_SEED], bump = verifier_config.bump)]
    pub verifier_config: Account<'info, VerifierConfig>,
}

pub fn resolve_accounts_start(
    ctx: Context<TopLevelResolveAccountsStart>,
    params: ResolveAccountsParams,
) -> Result<ResolveAccountsResponse> {
    let (ix_discriminator, data) = validate_resolution_start(&params, false)?;

    match ix_discriminator {
        instruction::TransferVerifierOwnership::DISCRIMINATOR
        | instruction::AcceptVerifierOwnership::DISCRIMINATOR
        | instruction::TransferVerifierStorageLocationAdmin::DISCRIMINATOR
        | instruction::AcceptVerifierStorageLocationAdmin::DISCRIMINATOR
        | instruction::SetVerifierAllowlistAdmin::DISCRIMINATOR
        | instruction::SetVerifierFeeAggregator::DISCRIMINATOR
        | instruction::SetVerifierAllowedFinalityConfig::DISCRIMINATOR => {
            simple_admin_start(ctx, params)
        }
        instruction::InitVerifierChainConfig::DISCRIMINATOR => {
            let decoded = SetVerifierChainConfigParams::try_from_slice(data)
                .map_err(|_| LombardTokenPoolError::InvalidResolutionInputs)?;
            Ok(ResolveAccountsResponse {
                ask_again_with: vec![],
                accounts_to_save: vec![
                    ctx.accounts.verifier_config.key().readonly(),
                    params.caller.writable().signer(),
                    remote_chain_config_pda(decoded.remote_chain_selector).writable(),
                    anchor_lang::system_program::ID.readonly(),
                ],
                lookup_tables_to_save: vec![],
                next_ix_discriminator: None,
                metadata: vec![],
            })
        }
        instruction::EditVerifierChainConfig::DISCRIMINATOR => {
            let decoded = SetVerifierChainConfigParams::try_from_slice(data)
                .map_err(|_| LombardTokenPoolError::InvalidResolutionInputs)?;
            Ok(ResolveAccountsResponse {
                ask_again_with: vec![],
                accounts_to_save: vec![
                    ctx.accounts.verifier_config.key().readonly(),
                    params.caller.readonly().signer(),
                    remote_chain_config_pda(decoded.remote_chain_selector).writable(),
                ],
                lookup_tables_to_save: vec![],
                next_ix_discriminator: None,
                metadata: vec![],
            })
        }
        instruction::SetVerifierAllowlist::DISCRIMINATOR => {
            let decoded = SetVerifierAllowlistParams::try_from_slice(data)
                .map_err(|_| LombardTokenPoolError::InvalidResolutionInputs)?;
            Ok(ResolveAccountsResponse {
                ask_again_with: vec![],
                accounts_to_save: vec![
                    ctx.accounts.verifier_config.key().readonly(),
                    params.caller.writable().signer(),
                    remote_chain_config_pda(decoded.dest_chain_selector).writable(),
                    anchor_lang::system_program::ID.readonly(),
                ],
                lookup_tables_to_save: vec![],
                next_ix_discriminator: None,
                metadata: vec![],
            })
        }
        instruction::SetVerifierPath::DISCRIMINATOR
        | instruction::RemoveVerifierPath::DISCRIMINATOR => {
            // Both take `remote_chain_selector: u64` as their first plain (non-struct) argument,
            // Borsh-encoded directly (no wrapping params struct) - the first 8 bytes of `data`.
            let remote_chain_selector = data
                .get(..8)
                .and_then(|b| b.try_into().ok())
                .map(u64::from_le_bytes)
                .ok_or(LombardTokenPoolError::InvalidResolutionInputs)?;
            Ok(ResolveAccountsResponse {
                ask_again_with: vec![],
                accounts_to_save: vec![
                    ctx.accounts.verifier_config.key().readonly(),
                    params.caller.readonly().signer(),
                    remote_chain_config_pda(remote_chain_selector).writable(),
                ],
                lookup_tables_to_save: vec![],
                next_ix_discriminator: None,
                metadata: vec![],
            })
        }
        instruction::SetVerifierSupportedTokens::DISCRIMINATOR => {
            // `set_verifier_supported_tokens(tokens_to_remove, tokens_to_set)` takes plain args;
            // the account list doesn't depend on their contents, so no decode needed here.
            Ok(ResolveAccountsResponse {
                ask_again_with: vec![],
                accounts_to_save: vec![
                    ctx.accounts.verifier_config.key().writable(),
                    params.caller.writable().signer(),
                    anchor_lang::system_program::ID.readonly(),
                ],
                lookup_tables_to_save: vec![],
                next_ix_discriminator: None,
                metadata: vec![],
            })
        }
        instruction::SetVerifierRemoteAdapters::DISCRIMINATOR => {
            // `set_verifier_remote_adapters(remote_chain_selector: u64, remote_adapters: Vec<RemoteAdapter>)`
            // takes plain args, not a wrapping params struct - `remote_chain_selector` is the first 8 bytes of `data`.
            let remote_chain_selector = data
                .get(..8)
                .and_then(|b| b.try_into().ok())
                .map(u64::from_le_bytes)
                .ok_or(LombardTokenPoolError::InvalidResolutionInputs)?;
            Ok(ResolveAccountsResponse {
                ask_again_with: vec![],
                accounts_to_save: vec![
                    ctx.accounts.verifier_config.key().readonly(),
                    params.caller.writable().signer(),
                    remote_chain_config_pda(remote_chain_selector).writable(),
                    anchor_lang::system_program::ID.readonly(),
                ],
                lookup_tables_to_save: vec![],
                next_ix_discriminator: None,
                metadata: vec![],
            })
        }
        instruction::SetVerifierStorageLocations::DISCRIMINATOR => Ok(ResolveAccountsResponse {
            ask_again_with: vec![],
            accounts_to_save: vec![
                ctx.accounts.verifier_config.key().readonly(),
                params.caller.writable().signer(),
                find(&[VERIFIER_STORAGE_LOCATIONS_SEED], crate::ID).writable(),
                anchor_lang::system_program::ID.readonly(),
            ],
            lookup_tables_to_save: vec![],
            next_ix_discriminator: None,
            metadata: vec![],
        }),
        instruction::GetVerifierStorageLocations::DISCRIMINATOR => Ok(ResolveAccountsResponse {
            ask_again_with: vec![],
            accounts_to_save: vec![
                ctx.accounts.verifier_config.key().readonly(),
                find(&[VERIFIER_STORAGE_LOCATIONS_SEED], crate::ID).readonly(),
            ],
            lookup_tables_to_save: vec![],
            next_ix_discriminator: None,
            metadata: vec![],
        }),
        _ => {
            msg!(
                "Unknown discriminator to resolve accounts for: {}",
                BytesWrapper(ix_discriminator)
            );
            Err(LombardTokenPoolError::InvalidResolutionInputs.into())
        }
    }
}

fn remote_chain_config_pda(remote_chain_selector: u64) -> Pubkey {
    find(
        &[
            VERIFIER_REMOTE_CHAIN_SEED,
            &remote_chain_selector.to_le_bytes(),
        ],
        crate::ID,
    )
}

/// Shared single-stage resolution for the simple owner/admin-gated setters that only ever touch
/// `verifier_config` + the caller
/// (minus the CPI-event accounts, since this program uses plain `emit!`).
fn simple_admin_start(
    ctx: Context<TopLevelResolveAccountsStart>,
    params: ResolveAccountsParams,
) -> Result<ResolveAccountsResponse> {
    Ok(ResolveAccountsResponse {
        ask_again_with: vec![],
        accounts_to_save: vec![
            ctx.accounts.verifier_config.key().writable(),
            params.caller.writable().signer(),
        ],
        lookup_tables_to_save: vec![],
        next_ix_discriminator: None,
        metadata: vec![],
    })
}

// NOTE: `initialize_verifier`'s resolver is deliberately not covered here
