use anchor_lang::prelude::*;
use ccip_common::router_accounts::ReceiverRegistry;
use ccip_common::seed;

use crate::context::{valid_version, ANCHOR_DISCRIMINATOR, MAX_CONFIG_V};
use crate::error::CcipRouterError;
use crate::event::events::ReceiverRegistered;
use crate::state::Config;

/// Registers (or updates) a receiver's CCIP interface version in the router.
///
/// The version assertion is bound to the program it describes, so this instruction accepts exactly
/// two direct signers and rejects everything else — notably it never trusts the receiver's *own*
/// admin/upgrade authority as a direct caller (that authority acts only through the receiver
/// program's own instruction, which CPIs in here):
///
/// 1. **A PDA of the receiver program** — the self-registration path. The receiver program
///    `invoke_signed`s into this instruction, signing with its
///    `[seed::RECEIVER_REGISTRATION_SIGNER]` PDA. Only that program can sign for the PDA, so its
///    signature proves the receiver authorized registration of *itself*.
/// 2. **The router's CCIP Admin** — the override/repair path, gated on the router's
///    `config.owner`.
pub fn register_receiver(
    ctx: Context<RegisterReceiver>,
    register_receiver_params: RegisterReceiverParams,
) -> Result<()> {
    ReceiverRegistry::validate_receiver_version(register_receiver_params.receiver_version)?;

    ctx.accounts.receiver_registry.set_inner(ReceiverRegistry {
        bump: ctx.bumps.receiver_registry,
        version: ReceiverRegistry::LATEST_VERSION,
        receiver_program: register_receiver_params.receiver_program,
        receiver_version: register_receiver_params.receiver_version,
    });

    emit_cpi!(ReceiverRegistered {
        receiver_program: register_receiver_params.receiver_program,
        receiver_version: register_receiver_params.receiver_version,
    });

    Ok(())
}

#[derive(AnchorSerialize, AnchorDeserialize, Debug)]
pub struct RegisterReceiverParams {
    pub receiver_program: Pubkey,
    pub receiver_version: u8,
}

#[event_cpi]
#[derive(Accounts)]
#[instruction(register_receiver_params: RegisterReceiverParams)]
pub struct RegisterReceiver<'info> {
    #[account(
        init_if_needed,
        seeds = [seed::RECEIVER_REGISTRY, register_receiver_params.receiver_program.as_ref()],
        bump,
        payer = payer,
        space = ANCHOR_DISCRIMINATOR + ReceiverRegistry::INIT_SPACE,
    )]
    pub receiver_registry: Account<'info, ReceiverRegistry>,

    #[account(
        seeds = [seed::CONFIG],
        bump,
        constraint = valid_version(config.version, MAX_CONFIG_V) @ CcipRouterError::InvalidVersion,
    )]
    pub config: Account<'info, Config>,

    #[account(
        seeds = [seed::RECEIVER_REGISTRATION_SIGNER, &crate::ID.as_ref()],
        bump,
        seeds::program = register_receiver_params.receiver_program,
    )]
    pub receiver_registration_signer: UncheckedAccount<'info>,

    /// The authorizing signer: either the receiver program's `RECEIVER_REGISTRATION_SIGNER` PDA
    /// (self-registration via CPI) or the router's owner (override).
    #[account(constraint = authority.key() == receiver_registration_signer.key() || authority.key() == config.owner @ CcipRouterError::Unauthorized)]
    pub authority: Signer<'info>,

    /// Rent payer for the registry PDA on first registration. Kept separate from `authority` so the
    /// receiver-PDA path (whose signer is a non-fee-paying PDA) can still fund the account via the
    /// forwarded transaction payer.
    // PERMISSIONLESS: only funds rent; authorization is enforced on `authority` in the handler.
    #[account(mut)]
    pub payer: Signer<'info>,

    pub system_program: Program<'info, System>,
}
