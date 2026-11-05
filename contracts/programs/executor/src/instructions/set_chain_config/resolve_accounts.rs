use anchor_lang::prelude::*;

use ccip_common::resolution::*;
use ccip_common::seed;

use super::super::resolve_accounts_start::ResolveAccountsStart;
use super::SetChainConfigParams;

use crate::error::ExecutorError;

pub fn start(
    ctx: Context<ResolveAccountsStart>,
    params: ResolveAccountsParams,
) -> Result<ResolveAccountsResponse> {
    let (_, data) = params
        .ix_data
        .split_at_checked(crate::instruction::SetChainConfig::DISCRIMINATOR.len())
        .ok_or(ExecutorError::InvalidInstructionData)?;

    let ix_params = SetChainConfigParams::try_from_slice(data)
        .map_err(|_| ExecutorError::InvalidInstructionData)?;

    let remote_chain_pda = find(
        &[
            seed::REMOTE_CHAIN,
            &ix_params.remote_chain_selector.to_le_bytes(),
        ],
        crate::ID,
    );

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
        metadata: vec![],
        lookup_tables_to_save: vec![],
        next_ix_discriminator: None,
    })
}
