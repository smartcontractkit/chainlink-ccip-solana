use anchor_lang::prelude::*;
use ccip_common::seed;
use ccip_common::state::AccountSchemaVersion;

use crate::buffering::{Buffer, Buffering};
use crate::error::CcipOfframpError;
use crate::event::BufferInitialized;
use crate::state::Config;

/// Buffers execution inputs in chunks to support large messages that exceed transaction size limits.
/// Works with all ExecutionInputsV2 payloads regardless of message type:
/// - Token transfer only messages
/// - Arbitrary message only messages
/// - Combined token transfer + arbitrary message (largest, most common case for buffering)
///
/// # Parameters
///
/// * `buffer_id` - A 32-byte buffer id defined by the caller (typically the message_id).
/// * `total_length` - Total length in bytes of the serialized execution inputs.
/// * `chunk` - The specific chunk to add to the buffer. Chunk must have a consistent size, except
///   the last one in the buffer, which may be smaller.
/// * `chunk_index` - The index of this chunk.
/// * `num_chunks` - The total number of chunks in the report.
#[derive(AnchorSerialize, AnchorDeserialize, Debug)]
pub struct BufferExecutionInputsParams {
    buffer_id: [u8; 32],
    total_length: u32,
    chunk: Vec<u8>,
    chunk_index: u8,
    num_chunks: u8,
}

pub fn buffer_execution_inputs<'info>(
    ctx: Context<'info, BufferExecutionInputsContext<'info>>,
    params: BufferExecutionInputsParams,
) -> Result<()> {
    let buffer = &mut ctx.accounts.execution_inputs_buffer;
    let initializing = !buffer.is_initialized();

    buffer.add_chunk(
        params.total_length,
        &params.chunk,
        params.chunk_index,
        params.num_chunks,
    )?;

    if initializing {
        buffer.bump = ctx.bumps.execution_inputs_buffer;
        buffer.buffer_id = params.buffer_id;
        buffer.authority = ctx.accounts.authority.key();
        emit_cpi!(BufferInitialized {
            buffer_id: params.buffer_id,
            authority: ctx.accounts.authority.key(),
        });
    }

    Ok(())
}

#[event_cpi]
#[derive(Accounts)]
#[instruction(params: BufferExecutionInputsParams)]
pub struct BufferExecutionInputsContext<'info> {
    #[account(
        init_if_needed,
        payer = authority,
        seeds = [seed::EXECUTION_INPUTS_BUFFER, &params.buffer_id, authority.key().as_ref()],
        bump,
        space = Buffer::DISCRIMINATOR.len() + Buffer::INIT_SPACE + params.total_length as usize
    )]
    pub execution_inputs_buffer: Account<'info, Buffer>,

    #[account(
        seeds = [seed::CONFIG],
        bump = config.bump,
        constraint = config.version == Config::MAX_V @ CcipOfframpError::InvalidVersion,
    )]
    pub config: Account<'info, Config>,

    #[account(mut)]
    pub authority: Signer<'info>,
    pub system_program: Program<'info, System>,
}

#[derive(AnchorSerialize, AnchorDeserialize, Debug)]
pub struct CloseExecutionInputsBufferParams {
    buffer_id: [u8; 32],
}

#[event_cpi]
#[derive(Accounts)]
#[instruction(params: CloseExecutionInputsBufferParams)]
pub struct CloseExecutionInputsBufferContext<'info> {
    #[account(
        mut,
        seeds = [seed::EXECUTION_INPUTS_BUFFER, &params.buffer_id, authority.key().as_ref()],
        bump = execution_inputs_buffer.bump,
        close = authority
    )]
    pub execution_inputs_buffer: Account<'info, Buffer>,

    #[account(
        seeds = [seed::CONFIG],
        bump = config.bump,
        constraint = config.version == Config::MAX_V @ CcipOfframpError::InvalidVersion,
    )]
    pub config: Account<'info, Config>,

    #[account(mut)]
    pub authority: Signer<'info>,
    pub system_program: Program<'info, System>,
}
