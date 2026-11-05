//! Wire-compatible CPI client for the real Lombard `mailbox` program. See `lombard_cpi` module
//! docs; only what this pool/verifier actually touches is reproduced here.

use anchor_lang::prelude::*;
use anchor_lang::solana_program::instruction::Instruction;
use anchor_lang::solana_program::program::invoke_signed;

use super::{meta, signer_meta, CpiReturn};

// See the identical note on lombard_cpi::bridge: devenv force-loads mock-lombard-mailbox at
// whatever address `blockchains.solana_programs` configures, so mock-lombard-mailbox simply
// declare_id!s as this same mainnet address - no devenv-specific feature/ID needed here.
//
// Built with the plain `pubkey!` macro rather than `declare_id!` for the same reason as
// lombard_cpi::bridge::id: `declare_id!` generates an IDL-address-detection test that `anchor
// build` can't disambiguate across multiple `declare_id!` invocations in one crate - see that
// module's comment for the full explanation.
mod id {
    use anchor_lang::prelude::*;
    use anchor_lang::solana_program::pubkey::pubkey;

    #[cfg(feature = "mainnet")]
    pub const ID: Pubkey = pubkey!("Lomsq4ZNkZQGysC1pQc2NqNiAbXQm6C8nLmKcUoESEA");
    #[cfg(feature = "gastald")]
    pub const ID: Pubkey = pubkey!("Lomu595CAtJGF6mpnfeAJ7daZfVdHeRkAdKyfqzXqom");
    #[cfg(feature = "staging")]
    pub const ID: Pubkey = pubkey!("Lom5doBNAny5AaPS9J7SRPLghVqwEQLrEmvQMhNEUqa");
    #[cfg(feature = "bft")]
    pub const ID: Pubkey = pubkey!("LomJw912MoUd7iiAesTQAgz1paLcTqi6ndG3w3pnKH9");
    #[cfg(any(
        feature = "localnet",
        feature = "devnet",
        not(any(
            feature = "mainnet",
            feature = "gastald",
            feature = "staging",
            feature = "bft"
        ))
    ))]
    pub const ID: Pubkey = pubkey!("3TfSFMuw31Je57m5Wcd9ZopGzjrHLHkjh292aEwXvm3h");
}
pub use id::ID;

pub mod program {
    use anchor_lang::prelude::*;

    #[derive(Clone)]
    pub struct Mailbox;

    impl anchor_lang::Id for Mailbox {
        fn id() -> Pubkey {
            super::ID
        }
    }
}

pub mod state {
    use anchor_lang::prelude::*;

    /// Mirrors `mailbox::state::Config` exactly
    /// (`_vendor/lombard-contracts/programs/mailbox/src/state.rs:9-28`) - `derive_accounts.rs`
    /// reads `global_nonce` off this directly to derive the `outbound_message` PDA seed.
    ///
    /// See the identical note on `bridge::state::RemoteBridgeConfig`: the explicit `"account"`
    /// namespace keeps the discriminator unchanged but skips anchor-lang's auto-generated
    /// `Owner` impl (which would otherwise tie ownership to `crate::ID`, i.e. our own pool
    /// program, instead of the real/mock `mailbox` program that actually owns this account).
    #[account("account")]
    pub struct Config {
        pub admin: Pubkey,
        pub pending_admin: Pubkey,
        pub treasury: Pubkey,
        pub paused: bool,
        pub consortium: Pubkey,
        pub global_nonce: u64,
        pub default_max_payload_size: u32,
        pub fee_per_byte: u64,
    }

    impl anchor_lang::Owner for Config {
        fn owner() -> Pubkey {
            super::ID
        }
    }
}

pub mod cpi {
    use super::*;

    pub mod accounts {
        use anchor_lang::prelude::*;

        use super::super::{meta, signer_meta};

        /// Field order matches `mailbox::instructions::handle_message::HandleMessage` exactly
        /// (`_vendor/lombard-contracts/programs/mailbox/src/instructions/handle_message.rs:12-27`).
        pub struct HandleMessage<'info> {
            pub handler: AccountInfo<'info>,
            pub config: AccountInfo<'info>,
            pub message_info: AccountInfo<'info>,
            pub recipient_program: AccountInfo<'info>,
        }

        impl<'info> anchor_lang::ToAccountMetas for HandleMessage<'info> {
            fn to_account_metas(&self, _is_signer: Option<bool>) -> Vec<AccountMeta> {
                vec![
                    signer_meta(&self.handler),
                    meta(&self.config),
                    meta(&self.message_info),
                    meta(&self.recipient_program),
                ]
            }
        }

        impl<'info> anchor_lang::ToAccountInfos<'info> for HandleMessage<'info> {
            fn to_account_infos(&self) -> Vec<AccountInfo<'info>> {
                vec![
                    self.handler.clone(),
                    self.config.clone(),
                    self.message_info.clone(),
                    self.recipient_program.clone(),
                ]
            }
        }
    }

    /// Wire-compatible with the real `mailbox::handle_message` instruction. Callers pass the
    /// accounts `handle_message` needs to forward to the recipient program via
    /// `CpiContext::with_remaining_accounts` - those are appended after this struct's own four
    /// accounts, matching the real handler's `ctx.remaining_accounts` forwarding
    /// (`_vendor/lombard-contracts/programs/mailbox/src/instructions/handle_message.rs:56-79`).
    pub fn handle_message<'a, 'b, 'c, 'info>(
        ctx: CpiContext<'a, 'b, 'c, 'info, accounts::HandleMessage<'info>>,
        payload_hash: [u8; 32],
    ) -> Result<CpiReturn<Option<Vec<u8>>>> {
        #[derive(AnchorSerialize)]
        struct Args {
            payload_hash: [u8; 32],
        }

        let mut data = anchor_discriminator::anchor_discriminator!("handle_message").to_vec();
        AnchorSerialize::serialize(&Args { payload_hash }, &mut data)
            .map_err(|_| anchor_lang::error::ErrorCode::InstructionDidNotSerialize)?;

        let instruction = Instruction {
            program_id: ctx.program_id,
            accounts: ctx.to_account_metas(None),
            data,
        };
        invoke_signed(&instruction, &ctx.to_account_infos(), ctx.signer_seeds)?;
        Ok(CpiReturn::snapshot(ctx.program_id))
    }
}
