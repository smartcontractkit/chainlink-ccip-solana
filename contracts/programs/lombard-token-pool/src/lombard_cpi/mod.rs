//! Hand-written, wire-compatible CPI client for the two Lombard programs this pool/verifier
//! actually calls (`bridge::deposit`, `mailbox::handle_message`), replacing a real dependency on
//! Lombard's own `bridge`/`mailbox` crates.
pub mod bridge;
pub mod mailbox;

use anchor_lang::prelude::*;

/// Mirrors anchor-lang's own generated `cpi::Return<T>`
pub struct CpiReturn<T> {
    program_id: Pubkey,
    _marker: std::marker::PhantomData<T>,
}

impl<T: AnchorDeserialize> CpiReturn<T> {
    fn snapshot(program_id: Pubkey) -> Self {
        Self {
            program_id,
            _marker: std::marker::PhantomData,
        }
    }

    pub fn get(&self) -> T {
        let (returned_program_id, data) =
            anchor_lang::solana_program::program::get_return_data().unwrap();
        assert_eq!(
            returned_program_id, self.program_id,
            "CPI return data program_id mismatch"
        );
        T::try_from_slice(&data).unwrap()
    }
}

/// Builds an `AccountMeta` reading `is_signer`/`is_writable` off the `AccountInfo` itself.
fn meta(info: &AccountInfo) -> anchor_lang::solana_program::instruction::AccountMeta {
    anchor_lang::solana_program::instruction::AccountMeta {
        pubkey: *info.key,
        is_signer: info.is_signer,
        is_writable: info.is_writable,
    }
}

/// Builds an `AccountMeta` with `is_signer` hardcoded `true`, matching how Anchor's own generated
/// CPI structs encode a `Signer<'info>` field as a compile-time-fixed property of the *callee's*
/// account layout.
fn signer_meta(info: &AccountInfo) -> anchor_lang::solana_program::instruction::AccountMeta {
    anchor_lang::solana_program::instruction::AccountMeta {
        pubkey: *info.key,
        is_signer: true,
        is_writable: info.is_writable,
    }
}
