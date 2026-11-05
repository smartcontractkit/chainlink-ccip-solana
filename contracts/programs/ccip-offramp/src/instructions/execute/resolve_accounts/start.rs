use anchor_lang::prelude::*;
use solana_program::sysvar::SysvarId as _;

use crate::error::CcipOfframpError;
use crate::instruction as offramp_instruction;
use ccip_common::{messages::MessageV1, resolution::*, seed};

use super::super::super::resolve_accounts_start::ResolveAccountsStart;
use super::common::parse_exec_inputs;

pub fn start(
    ctx: Context<ResolveAccountsStart>,
    params: ResolveAccountsParams,
) -> Result<ResolveAccountsResponse> {
    let caller = params.caller;
    let exec_params = parse_exec_inputs(&params, ctx.remaining_accounts)?;

    let msg = MessageV1::from_bytes(&exec_params.encoded_message)
        .map_err(|_| CcipOfframpError::InvalidInstructionData)?;
    let msg_id = msg.compute_message_id();
    let src_selector_seed = msg.source_chain_selector.to_le_bytes();

    let mut ask_again_with = vec![
        ctx.accounts.config.key(),
        find(&[seed::REFERENCE_ADDRESSES], crate::ID),
    ];

    if let Some(buffer_account) = exec_params.buffer_account {
        ask_again_with.push(buffer_account);
    }

    Ok(ResolveAccountsResponse {
        ask_again_with,
        accounts_to_save: vec![
            ctx.accounts.config.key().readonly(),
            find(&[seed::REFERENCE_ADDRESSES], crate::ID).readonly(),
            find(&[seed::SOURCE_CHAIN, src_selector_seed.as_ref()], crate::ID).readonly(),
            find(&[seed::MESSAGE_EXEC_STATE, msg_id.as_ref()], crate::ID).writable(),
            crate::ID.readonly(),
            caller.writable().signer(),
            system_program::ID.readonly(),
            Instructions::id().readonly(),
        ],
        lookup_tables_to_save: vec![],
        next_ix_discriminator: Some(
            offramp_instruction::ResolveAccountsExecuteMessageStage::DISCRIMINATOR
                .try_into()
                .unwrap(),
        ),
        metadata: vec![],
    })
}
