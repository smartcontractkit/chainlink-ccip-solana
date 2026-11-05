use anchor_lang::prelude::*;

use ccip_common::resolution::*;
use ccip_common::seed;

use super::super::resolve_accounts_start::ResolveAccountsStart;
use super::SetChainConfigParams;

use crate::error::CommitteeVerifierError;

/// First (and only) stage of account resolution for the `init_chain_config` instruction.
pub fn init(
    ctx: Context<ResolveAccountsStart>,
    params: ResolveAccountsParams,
) -> Result<ResolveAccountsResponse> {
    let remote_chain_pda = remote_chain_pda(
        &params,
        crate::instruction::InitChainConfig::DISCRIMINATOR.len(),
    )?;

    Ok(ResolveAccountsResponse {
        ask_again_with: vec![],
        accounts_to_save: vec![
            ctx.accounts.config.key().readonly(),
            params.caller.writable().signer(),
            remote_chain_pda.writable(),
            system_program::ID.readonly(),
            find(&[seed::EVENT_AUTHORITY], crate::ID).readonly(),
            crate::ID.readonly(),
        ],
        lookup_tables_to_save: vec![],
        next_ix_discriminator: None,
        metadata: vec![],
    })
}

/// First (and only) stage of account resolution for the `edit_chain_config` instruction.
pub fn edit(
    ctx: Context<ResolveAccountsStart>,
    params: ResolveAccountsParams,
) -> Result<ResolveAccountsResponse> {
    let remote_chain_pda = remote_chain_pda(
        &params,
        crate::instruction::EditChainConfig::DISCRIMINATOR.len(),
    )?;

    Ok(ResolveAccountsResponse {
        ask_again_with: vec![],
        accounts_to_save: vec![
            ctx.accounts.config.key().readonly(),
            params.caller.readonly().signer(),
            remote_chain_pda.writable(),
            find(&[seed::EVENT_AUTHORITY], crate::ID).readonly(),
            crate::ID.readonly(),
        ],
        lookup_tables_to_save: vec![],
        next_ix_discriminator: None,
        metadata: vec![],
    })
}

fn remote_chain_pda(params: &ResolveAccountsParams, discriminator_len: usize) -> Result<Pubkey> {
    let (_, data) = params
        .ix_data
        .split_at_checked(discriminator_len)
        .ok_or(CommitteeVerifierError::InvalidInstructionData)?;

    let ix_params = SetChainConfigParams::try_from_slice(data)
        .map_err(|_| CommitteeVerifierError::InvalidInstructionData)?;

    Ok(find(
        &[
            seed::REMOTE_CHAIN,
            &ix_params.remote_chain_selector.to_le_bytes(),
        ],
        crate::ID,
    ))
}
