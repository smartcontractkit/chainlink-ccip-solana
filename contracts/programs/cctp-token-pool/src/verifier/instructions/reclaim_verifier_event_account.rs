use anchor_lang::prelude::instruction::Instruction;
use anchor_lang::prelude::program::invoke_signed;
use anchor_lang::prelude::*;

use anchor_discriminator::anchor_discriminator;

use crate::error::CctpPoolVerifierError;
use crate::verifier::cctp_constants::MESSAGE_TRANSMITTER_V2;
use crate::verifier::state::*;

#[derive(AnchorSerialize, AnchorDeserialize, Clone, Debug)]
pub struct ReclaimVerifierEventAccountParams {
    pub mint: Pubkey,
    pub original_sender: Pubkey,
    pub remote_chain_selector: u64,
    /// The nonce assigned by this verifier for that particular message
    /// (per-sender, per-token, per-remote-chain)
    pub ccip_cctp_nonce: u64,
    /// Attestation bytes as returned by Circle's iris endpoint
    pub attestation: Vec<u8>,
    /// Message bytes as returned by Circle's iris endpoint
    pub destination_message: Vec<u8>,
}

#[event_cpi]
#[derive(Accounts)]
#[instruction(params: ReclaimVerifierEventAccountParams)]
pub struct ReclaimVerifierEventAccount<'info> {
    #[account(
        seeds = [verifier_seed::CONFIG],
        bump = config.bump,
        constraint = config.is_latest_version() @ CctpPoolVerifierError::InvalidVersion,
    )]
    pub config: Account<'info, Config>,

    /// CHECK: CPI signer for CCTP operations, validated by seeds constraint
    #[account(
        mut,
        seeds = [verifier_seed::POOL_SIGNER_SEED, params.mint.as_ref()],
        bump,
    )]
    pub cpi_signer: UncheckedAccount<'info>,

    /// CHECK: MessageSent event account to reclaim rent from
    #[account(
        mut,
        seeds = [
            verifier_seed::CCTPV2_EVENT,
            &params.original_sender.to_bytes(),
            &params.remote_chain_selector.to_le_bytes(),
			&params.mint.as_ref(),
            &params.ccip_cctp_nonce.to_le_bytes(),
        ],
        bump,
    )]
    pub message_sent_event_account: UncheckedAccount<'info>,

    /// CHECK: CCTP Message Transmitter program, validated by seeds constraint
    #[account(
        mut,
        seeds = [b"message_transmitter"],
        bump,
        seeds::program = message_transmitter_program,
    )]
    pub message_transmitter_pda: UncheckedAccount<'info>,

    /// CHECK: CCTP Message Transmitter program account
    #[account(address = MESSAGE_TRANSMITTER_V2)]
    pub message_transmitter_program: UncheckedAccount<'info>,

    // For now, we restrict this to the verifier owner. Circle imposes a 5 day waiting period
    // for closing event accounts on CCTPv2, as nonce assignment is done offchain and therefore
    // closing too early could cause stuck messages. Because this operation is now more sensitive
    // than it was on CCTPv1, we restrict to the owner initially. In time, if we determine that it
    // is operationally safe, we may allow a "funds manager" to do this operation (similar to what we
    // had on CCTPv1).
    #[account(mut, address = config.owner @ CctpPoolVerifierError::Unauthorized)]
    pub authority: Signer<'info>,

    pub system_program: Program<'info, System>,
}

const RECLAIM_METHOD_DISCRIM: [u8; 8] = anchor_discriminator!("reclaim_event_account");

pub fn reclaim_verifier_event_account(
    ctx: Context<ReclaimVerifierEventAccount>,
    params: ReclaimVerifierEventAccountParams,
) -> Result<()> {
    let mut instruction_data = Vec::with_capacity(RECLAIM_METHOD_DISCRIM.len());
    instruction_data.extend_from_slice(&RECLAIM_METHOD_DISCRIM);
    borsh::to_writer(&mut instruction_data, &params.attestation)?;
    borsh::to_writer(&mut instruction_data, &params.destination_message)?;

    let acc_infos = vec![
        ctx.accounts.cpi_signer.to_account_info(),
        ctx.accounts.message_transmitter_pda.to_account_info(),
        ctx.accounts.message_sent_event_account.to_account_info(),
    ];

    let acc_metas: Vec<AccountMeta> = acc_infos
        .iter()
        .flat_map(|acc_info| {
            let is_signer = acc_info.key() == ctx.accounts.cpi_signer.key();
            acc_info.to_account_metas(Some(is_signer))
        })
        .collect();

    let instruction = Instruction {
        program_id: ctx.accounts.message_transmitter_program.key(),
        accounts: acc_metas,
        data: instruction_data,
    };

    let signer_seeds: &[&[u8]] = &[
        verifier_seed::POOL_SIGNER_SEED,
        &params.mint.to_bytes(),
        &[ctx.bumps.cpi_signer],
    ];

    invoke_signed(&instruction, &acc_infos, &[signer_seeds])?;

    emit_cpi!(CcipCctpV2MessageEventAccountClosed {
        original_sender: params.original_sender,
        remote_chain_selector: params.remote_chain_selector,
        mint: params.mint,
        ccip_cctp_nonce: params.ccip_cctp_nonce,
        address: ctx.accounts.message_sent_event_account.key()
    });

    Ok(())
}

#[event]
pub struct CcipCctpV2MessageEventAccountClosed {
    // Seeds for the CCTP message sent event account
    original_sender: Pubkey,
    remote_chain_selector: u64,
    mint: Pubkey,
    ccip_cctp_nonce: u64,
    // Actual event account address, derived from the seeds above
    address: Pubkey,
}
