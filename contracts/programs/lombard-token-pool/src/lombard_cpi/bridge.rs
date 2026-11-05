//! Wire-compatible CPI client for the real Lombard `bridge` program.

use anchor_lang::prelude::*;
use anchor_lang::solana_program::instruction::Instruction;
use anchor_lang::solana_program::program::invoke_signed;

use super::{meta, signer_meta, CpiReturn};

pub mod constants {
    pub const OPTIONAL_MESSAGE_SIZE: usize = 36;
}

mod id {
    use anchor_lang::prelude::*;
    use anchor_lang::solana_program::pubkey::pubkey;

    #[cfg(feature = "mainnet")]
    pub const ID: Pubkey = pubkey!("Lomva5kTftuXE8992qRufkaeq3XrXV47qv3C1W9xW6Z");
    #[cfg(feature = "gastald")]
    pub const ID: Pubkey = pubkey!("LombUtstgyrZUhjvi12hUnm7HG7CxhtanUv6hakuCm4");
    #[cfg(feature = "staging")]
    pub const ID: Pubkey = pubkey!("LomS25cte2jkQoLbKembGB19gb2pMNKPFodwLHpMiWR");
    #[cfg(feature = "bft")]
    pub const ID: Pubkey = pubkey!("Lom9Em2WzV7gvtttdub9LZSR8gLgtbzFDhFm1zMQRp6");
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
    pub const ID: Pubkey = pubkey!("CAwQ43gQmFB6CD4zodoKt7ipPrHP7eQLxvGRY6tQ6zYx");
}
pub use id::ID;

pub mod program {
    use anchor_lang::prelude::*;

    #[derive(Clone)]
    pub struct Bridge;

    impl anchor_lang::Id for Bridge {
        fn id() -> Pubkey {
            super::ID
        }
    }
}

pub mod state {
    use anchor_lang::prelude::*;

    /// The explicit `"account"` namespace reproduces the exact same discriminator the default
    /// (no namespace) `#[account]` expansion would compute (`sighash("account",
    /// "RemoteBridgeConfig")` either way - see
    /// `anchor-syn::codegen::program::common::{sighash,gen_discriminator}`), but *skips* the
    /// auto-generated `Owner` impl, which anchor-lang always ties to `crate::ID` (the enclosing
    /// `lombard_token_pool` crate root, not this module's own `declare_id!()` - see
    /// `anchor-attribute-account::lib.rs`'s `owner_impl`, which only runs when the namespace arg
    /// is empty). Without this, `Account<'info, RemoteBridgeConfig>` would reject every real
    /// `RemoteBridgeConfig` account (owned by the real/mock `bridge` program) as an ownership
    /// mismatch.
    #[account("account")]
    pub struct RemoteBridgeConfig {
        pub bump: u8,
        pub chain_id: [u8; 32],
        pub bridge: [u8; 32],
    }

    impl anchor_lang::Owner for RemoteBridgeConfig {
        fn owner() -> Pubkey {
            super::ID
        }
    }

    #[account("account")]
    pub struct RemoteTokenConfig {
        pub bump: u8,
        pub chain_id: [u8; 32],
        pub token: [u8; 32],
        pub direction: u8,
        pub inbound_rate_limit: base_token_pool::rate_limiter::RateLimitTokenBucket,
    }

    impl anchor_lang::Owner for RemoteTokenConfig {
        fn owner() -> Pubkey {
            super::ID
        }
    }
}

pub mod utils {
    pub mod gmp_messages {
        use anchor_lang::prelude::*;

        #[derive(AnchorSerialize, AnchorDeserialize, Clone, Debug)]
        pub struct InboundResponse {
            pub amount: u64,
            pub message: Option<[u8; super::super::constants::OPTIONAL_MESSAGE_SIZE]>,
        }

        #[derive(AnchorSerialize, AnchorDeserialize, Clone, Debug)]
        pub struct OutboundResponse {
            pub nonce: u64,
            pub payload_hash: [u8; 32],
        }
    }

    pub mod token_actions {
        use anchor_lang::prelude::*;

        pub fn get_token_account(
            token_program: Pubkey,
            mint: Pubkey,
            owner: Pubkey,
        ) -> Result<Pubkey> {
            Ok(
                anchor_spl::associated_token::get_associated_token_address_with_program_id(
                    &owner,
                    &mint,
                    &token_program,
                ),
            )
        }
    }
}

pub mod cpi {
    use super::*;

    pub mod accounts {
        use anchor_lang::prelude::*;

        use super::super::{meta, signer_meta};

        pub struct Deposit<'info> {
            pub fee_payer: AccountInfo<'info>,
            pub sender: AccountInfo<'info>,
            pub sender_token_account: AccountInfo<'info>,
            pub token_program: AccountInfo<'info>,
            pub config: AccountInfo<'info>,
            pub sender_config: AccountInfo<'info>,
            pub remote_bridge_config: AccountInfo<'info>,
            pub local_token_config: AccountInfo<'info>,
            pub remote_token_config: AccountInfo<'info>,
            pub mint: AccountInfo<'info>,
            pub mailbox: AccountInfo<'info>,
            pub mailbox_config: AccountInfo<'info>,
            pub outbound_message_path: AccountInfo<'info>,
            pub outbound_message: AccountInfo<'info>,
            pub mailbox_sender_config: AccountInfo<'info>,
            pub treasury: Option<AccountInfo<'info>>,
            pub system_program: AccountInfo<'info>,
        }

        impl<'info> anchor_lang::ToAccountMetas for Deposit<'info> {
            fn to_account_metas(&self, _is_signer: Option<bool>) -> Vec<AccountMeta> {
                let mut metas = vec![
                    signer_meta(&self.fee_payer),
                    signer_meta(&self.sender),
                    meta(&self.sender_token_account),
                    meta(&self.token_program),
                    meta(&self.config),
                    meta(&self.sender_config),
                    meta(&self.remote_bridge_config),
                    meta(&self.local_token_config),
                    meta(&self.remote_token_config),
                    meta(&self.mint),
                    meta(&self.mailbox),
                    meta(&self.mailbox_config),
                    meta(&self.outbound_message_path),
                    meta(&self.outbound_message),
                    meta(&self.mailbox_sender_config),
                ];
                // Matches Anchor's real generated CPI struct for an `Option<Account>` field: a
                // present account uses its own meta; an absent one is a readonly placeholder
                // pointing at the *callee's* program ID, so `Option<T>::try_accounts` on the
                // other side can distinguish "not provided" from a real account by position+key
                // equality without needing an explicit presence flag on the wire.
                match &self.treasury {
                    Some(treasury) => metas.push(meta(treasury)),
                    None => metas.push(AccountMeta::new_readonly(super::super::ID, false)),
                }
                metas.push(meta(&self.system_program));
                metas
            }
        }

        impl<'info> anchor_lang::ToAccountInfos<'info> for Deposit<'info> {
            fn to_account_infos(&self) -> Vec<AccountInfo<'info>> {
                let mut infos = vec![
                    self.fee_payer.clone(),
                    self.sender.clone(),
                    self.sender_token_account.clone(),
                    self.token_program.clone(),
                    self.config.clone(),
                    self.sender_config.clone(),
                    self.remote_bridge_config.clone(),
                    self.local_token_config.clone(),
                    self.remote_token_config.clone(),
                    self.mint.clone(),
                    self.mailbox.clone(),
                    self.mailbox_config.clone(),
                    self.outbound_message_path.clone(),
                    self.outbound_message.clone(),
                    self.mailbox_sender_config.clone(),
                ];
                if let Some(treasury) = &self.treasury {
                    infos.push(treasury.clone());
                }
                infos.push(self.system_program.clone());
                infos
            }
        }
    }

    pub fn deposit<'a, 'b, 'c, 'info>(
        ctx: CpiContext<'a, 'b, 'c, 'info, accounts::Deposit<'info>>,
        sender: [u8; 32],
        recipient: [u8; 32],
        caller: Option<[u8; 32]>,
        amount: u64,
        message: Option<[u8; constants::OPTIONAL_MESSAGE_SIZE]>,
    ) -> Result<CpiReturn<utils::gmp_messages::OutboundResponse>> {
        #[derive(AnchorSerialize)]
        struct Args {
            sender: [u8; 32],
            recipient: [u8; 32],
            caller: Option<[u8; 32]>,
            amount: u64,
            message: Option<[u8; constants::OPTIONAL_MESSAGE_SIZE]>,
        }

        let mut data = anchor_discriminator::anchor_discriminator!("deposit").to_vec();
        AnchorSerialize::serialize(
            &Args {
                sender,
                recipient,
                caller,
                amount,
                message,
            },
            &mut data,
        )
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
