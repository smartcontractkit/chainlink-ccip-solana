use anchor_lang::prelude::*;
use ccip_common::messages::MessageV1;

use crate::error::CcipOfframpError;
use crate::instruction as offramp_instruction;
use crate::instructions::execute::parse_pubkey;
use crate::instructions::execute::resolve_accounts::common::parse_exec_inputs;
use crate::state::Config;
use ccip_common::resolution::*;
use ccip_common::seed;
use ccip_common::state::AccountSchemaVersion;
use ccip_common::v1::load_token_admin_registry_checked;

#[derive(Accounts)]
pub struct ResolveAccountsExecuteTokenTransferRetrieveLutStage<'info> {
    #[account(
        seeds = [seed::CONFIG],
        bump = config.bump,
        constraint = config.version == Config::MAX_V @ CcipOfframpError::InvalidVersion,
    )]
    pub config: Account<'info, Config>,
    pub admin_registry: UncheckedAccount<'info>,
}

pub fn retrieve_token_lut_stage<'info>(
    ctx: Context<'info, ResolveAccountsExecuteTokenTransferRetrieveLutStage<'info>>,
    params: ResolveAccountsParams,
) -> Result<ResolveAccountsResponse> {
    let exec_params = parse_exec_inputs(&params, ctx.remaining_accounts)?;
    let msg = MessageV1::from_bytes(&exec_params.encoded_message)
        .map_err(|_| CcipOfframpError::InvalidInstructionData)?;
    let token = parse_pubkey(
        &msg.token_transfer
            .ok_or(CcipOfframpError::InvalidInstructionData)?
            .dest_token_address,
    )?;
    let registry = load_token_admin_registry_checked(&ctx.accounts.admin_registry, token)?;
    let mut ask_again_with = vec![
        ctx.accounts.config.key(),
        find(&[seed::REFERENCE_ADDRESSES], crate::ID),
        ctx.accounts.admin_registry.key(),
        registry.lookup_table,
    ];

    // This stage does not read the execution inputs itself, but the next one
    // (`retrieve_token_static_accounts_stage`) does. In buffered mode the buffer arrives at
    // `remaining_accounts[0]` (this context consumes only `config` and `admin_registry`), so we
    // forward it after the four named accounts of the next stage to keep it at that stage's
    // `remaining_accounts[0]`, as `parse_exec_inputs` expects.
    if let Some(buffer) = ctx.remaining_accounts.first() {
        ask_again_with.push(*buffer.key);
    }

    Ok(ResolveAccountsResponse {
        ask_again_with,
        accounts_to_save: vec![],
        lookup_tables_to_save: vec![registry.lookup_table],
        // SAFETY: an Anchor `DISCRIMINATOR` is a const `[u8; 8]` slice, so the conversion to
        // `[u8; 8]` is infallible.
        next_ix_discriminator:
            Some(offramp_instruction::ResolveAccountsExecuteTokenTransferStaticAccountsStage::DISCRIMINATOR.try_into().unwrap()),
        metadata: params.metadata,
    })
}
