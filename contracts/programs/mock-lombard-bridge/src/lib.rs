//! Minimal mock of the real Lombard `bridge` program's on-chain interface.

use anchor_lang::prelude::*;
use anchor_spl::token_interface::{self, Burn, Mint, MintTo, TokenAccount, TokenInterface};

mod errors;
mod messages;
mod state;

pub use messages::{InboundResponse, OutboundResponse, OPTIONAL_MESSAGE_SIZE};
pub use state::{RemoteBridgeConfig, RemoteTokenConfig};

use errors::MockLombardBridgeError;

declare_id!("Lomva5kTftuXE8992qRufkaeq3XrXV47qv3C1W9xW6Z");

pub const REMOTE_BRIDGE_CONFIG_SEED: &[u8] = b"remote_bridge_config";
pub const REMOTE_TOKEN_CONFIG_SEED: &[u8] = b"remote_token_config";
/// PDA that owns minting for the mock's underlying SPL mint - the test harness must set the
/// real mint's `mint_authority` to this PDA when creating the mint.
pub const TOKEN_AUTHORITY_SEED: &[u8] = b"token_authority";

/// Emitted by `deposit` so a devenv test/off-chain harness can discover what to relay to
/// mock-lombard-mailbox, mirroring how a real relayer discovers deposits to bridge onward.
#[event]
pub struct DepositEvent {
    pub payload_hash: [u8; 32],
    pub sender: [u8; 32],
    pub recipient: [u8; 32],
    pub amount: u64,
    pub message: Option<[u8; OPTIONAL_MESSAGE_SIZE]>,
}

#[program]
pub mod mock_lombard_bridge {
    use super::*;

    /// Condensed test-setup call for `RemoteBridgeConfig`.
    pub fn test_setup_remote_bridge_config(
        ctx: Context<TestSetupRemoteBridgeConfig>,
        chain_id: [u8; 32],
        bridge: [u8; 32],
    ) -> Result<()> {
        let config = &mut ctx.accounts.remote_bridge_config;
        config.bump = ctx.bumps.remote_bridge_config;
        config.chain_id = chain_id;
        config.bridge = bridge;
        Ok(())
    }

    /// Condensed test-setup call for `RemoteTokenConfig`, consumed by
    /// `lombard_token_pool`'s `start_verification_at_source` (`RemoteTokenOrAdapterMismatch`
    /// guard).
    pub fn test_setup_remote_token_config(
        ctx: Context<TestSetupRemoteTokenConfig>,
        _mint: Pubkey,
        chain_id: [u8; 32],
        token: [u8; 32],
        direction: u8,
    ) -> Result<()> {
        let config = &mut ctx.accounts.remote_token_config;
        config.bump = ctx.bumps.remote_token_config;
        config.chain_id = chain_id;
        config.token = token;
        config.direction = direction;
        Ok(())
    }

    /// Wire-compatible with the real `bridge::deposit` instruction consumed via
    /// `lombard_cpi::bridge::cpi::deposit`: same instruction name.
    /// Burns `amount` from `sender_token_account` (authority = `sender`) and emits a `DepositEvent`
    /// carrying everything a devenv relayer needs to later call mock-lombard-mailbox's `mark_delivered`.
    pub fn deposit(
        ctx: Context<Deposit>,
        sender: [u8; 32],
        recipient: [u8; 32],
        _caller: Option<[u8; 32]>,
        amount: u64,
        message: Option<[u8; OPTIONAL_MESSAGE_SIZE]>,
    ) -> Result<OutboundResponse> {
        token_interface::burn(
            CpiContext::new(
                ctx.accounts.token_program.key(),
                Burn {
                    mint: ctx.accounts.mint.to_account_info(),
                    from: ctx.accounts.sender_token_account.to_account_info(),
                    authority: ctx.accounts.sender_authority.to_account_info(),
                },
            ),
            amount,
        )?;

        // Mock-internal only: doesn't need to match any real algorithm, just deterministic and
        // 32 bytes, so devenv tests can assert on it.
        let mut preimage = Vec::with_capacity(32 + 32 + 8 + OPTIONAL_MESSAGE_SIZE);
        preimage.extend_from_slice(&sender);
        preimage.extend_from_slice(&recipient);
        preimage.extend_from_slice(&amount.to_le_bytes());
        preimage.extend_from_slice(&message.unwrap_or([0u8; OPTIONAL_MESSAGE_SIZE]));
        let payload_hash = solana_sha256_hasher::hash(&preimage).to_bytes();

        emit!(DepositEvent {
            payload_hash,
            sender,
            recipient,
            amount,
            message,
        });

        Ok(OutboundResponse {
            nonce: 0,
            payload_hash,
        })
    }

    /// Internal-only convention with mock-lombard-mailbox: called exclusively via CPI from that
    /// program's `handle_message`, standing in for the real bridge's inbound GMP receive path.
    /// Mints `amount` to `recipient_token_account`, signed by the `token_authority` PDA.
    pub fn mock_gmp_receive(
        ctx: Context<MockGmpReceive>,
        _payload_hash: [u8; 32],
        _recipient: Pubkey,
        amount: u64,
        message: Option<[u8; OPTIONAL_MESSAGE_SIZE]>,
    ) -> Result<InboundResponse> {
        let signer_seeds: &[&[&[u8]]] = &[&[TOKEN_AUTHORITY_SEED, &[ctx.bumps.token_authority]]];
        token_interface::mint_to(
            CpiContext::new_with_signer(
                ctx.accounts.token_program.key(),
                MintTo {
                    mint: ctx.accounts.mint.to_account_info(),
                    to: ctx.accounts.recipient_token_account.to_account_info(),
                    authority: ctx.accounts.token_authority.to_account_info(),
                },
                signer_seeds,
            ),
            amount,
        )?;

        Ok(InboundResponse { amount, message })
    }
}

#[derive(Accounts)]
#[instruction(chain_id: [u8; 32])]
pub struct TestSetupRemoteBridgeConfig<'info> {
    #[account(mut)]
    pub payer: Signer<'info>,

    #[account(
        init,
        payer = payer,
        space = RemoteBridgeConfig::SPACE,
        seeds = [REMOTE_BRIDGE_CONFIG_SEED, chain_id.as_ref()],
        bump,
    )]
    pub remote_bridge_config: Account<'info, RemoteBridgeConfig>,

    pub system_program: Program<'info, System>,
}

#[derive(Accounts)]
#[instruction(mint: Pubkey, chain_id: [u8; 32])]
pub struct TestSetupRemoteTokenConfig<'info> {
    #[account(mut)]
    pub payer: Signer<'info>,

    #[account(
        init,
        payer = payer,
        space = RemoteTokenConfig::DISCRIMINATOR.len() + RemoteTokenConfig::INIT_SPACE,
        seeds = [REMOTE_TOKEN_CONFIG_SEED, mint.as_ref(), chain_id.as_ref()],
        bump,
    )]
    pub remote_token_config: Account<'info, RemoteTokenConfig>,

    pub system_program: Program<'info, System>,
}

#[derive(Accounts)]
pub struct Deposit<'info> {
    pub fee_payer: Signer<'info>,
    /// Real SPL burn authority for `sender_token_account`. Named `sender_authority` (not
    /// `sender`, despite occupying the position the real bridge program calls `sender`) to
    /// avoid colliding with the `sender: [u8; 32]` instruction arg in the generated IDL/Go
    /// bindings - account order (position 2), not name, is what wire compatibility with
    /// `lombard_cpi::bridge::cpi::accounts::Deposit` actually depends on.
    pub sender_authority: Signer<'info>,
    #[account(mut)]
    pub sender_token_account: InterfaceAccount<'info, TokenAccount>,
    pub token_program: Interface<'info, TokenInterface>,
    /// CHECK: unused, present only for account-order parity with the real bridge program.
    pub config: UncheckedAccount<'info>,
    /// CHECK: unused, present only for account-order parity with the real bridge program.
    pub sender_config: UncheckedAccount<'info>,
    /// CHECK: unused, present only for account-order parity with the real bridge program.
    pub remote_bridge_config: UncheckedAccount<'info>,
    /// CHECK: unused, present only for account-order parity with the real bridge program.
    pub local_token_config: UncheckedAccount<'info>,
    /// CHECK: unused, present only for account-order parity with the real bridge program.
    pub remote_token_config: UncheckedAccount<'info>,
    #[account(mut)]
    pub mint: InterfaceAccount<'info, Mint>,
    /// CHECK: unused, present only for account-order parity with the real bridge program.
    pub mailbox: UncheckedAccount<'info>,
    /// CHECK: unused, present only for account-order parity with the real bridge program.
    pub mailbox_config: UncheckedAccount<'info>,
    /// CHECK: unused, present only for account-order parity with the real bridge program.
    pub outbound_message_path: UncheckedAccount<'info>,
    /// CHECK: unused, present only for account-order parity with the real bridge program.
    pub outbound_message: UncheckedAccount<'info>,
    /// CHECK: unused, present only for account-order parity with the real bridge program.
    pub mailbox_sender_config: UncheckedAccount<'info>,
    /// CHECK: unused, present only for account-order parity with the real bridge program.
    pub treasury: Option<UncheckedAccount<'info>>,
    /// CHECK: unused, present only for account-order parity with the real bridge program.
    pub system_program: UncheckedAccount<'info>,
}

#[derive(Accounts)]
#[instruction(_payload_hash: [u8; 32], recipient: Pubkey)]
pub struct MockGmpReceive<'info> {
    #[account(
        mut,
        constraint = recipient_token_account.owner == recipient @ MockLombardBridgeError::RecipientMismatch,
    )]
    pub recipient_token_account: InterfaceAccount<'info, TokenAccount>,
    #[account(mut)]
    pub mint: InterfaceAccount<'info, Mint>,
    /// CHECK: unused, kept only for shape-parity with a real mint-authority slot.
    pub mint_authority: UncheckedAccount<'info>,
    /// CHECK: PDA mint authority for the mock's underlying mint; signs the inner `mint_to` CPI.
    #[account(seeds = [TOKEN_AUTHORITY_SEED], bump)]
    pub token_authority: UncheckedAccount<'info>,
    pub token_program: Interface<'info, TokenInterface>,
}

#[cfg(test)]
mod tests {
    use super::*;

    fn preimage(
        sender: [u8; 32],
        recipient: [u8; 32],
        amount: u64,
        message: Option<[u8; OPTIONAL_MESSAGE_SIZE]>,
    ) -> [u8; 32] {
        let mut preimage = Vec::with_capacity(32 + 32 + 8 + OPTIONAL_MESSAGE_SIZE);
        preimage.extend_from_slice(&sender);
        preimage.extend_from_slice(&recipient);
        preimage.extend_from_slice(&amount.to_le_bytes());
        preimage.extend_from_slice(&message.unwrap_or([0u8; OPTIONAL_MESSAGE_SIZE]));
        solana_sha256_hasher::hash(&preimage).to_bytes()
    }

    #[test]
    fn payload_hash_is_deterministic() {
        let sender = [1u8; 32];
        let recipient = [2u8; 32];
        assert_eq!(
            preimage(sender, recipient, 100, None),
            preimage(sender, recipient, 100, None)
        );
    }

    #[test]
    fn payload_hash_changes_with_amount() {
        let sender = [1u8; 32];
        let recipient = [2u8; 32];
        assert_ne!(
            preimage(sender, recipient, 100, None),
            preimage(sender, recipient, 101, None)
        );
    }

    #[test]
    fn payload_hash_changes_with_message() {
        let sender = [1u8; 32];
        let recipient = [2u8; 32];
        let mut message = [0u8; OPTIONAL_MESSAGE_SIZE];
        message[0] = 7;
        assert_ne!(
            preimage(sender, recipient, 100, None),
            preimage(sender, recipient, 100, Some(message))
        );
    }
}
