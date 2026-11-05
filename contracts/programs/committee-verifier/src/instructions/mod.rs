// bytes4(keccak256("CommitteeVerifier 2.0.0"))
pub const VERSION_TAG: [u8; 4] = [0xe9, 0xa0, 0x5a, 0x20];

pub mod admin;
pub mod start_verification_at_source;
pub use start_verification_at_source::*;
pub mod get_fee;
pub use get_fee::*;
pub mod get_storage_locations;
pub use get_storage_locations::*;
pub mod observe;
pub mod verify;
pub use verify::*;
pub mod resolve_accounts_start;
pub use resolve_accounts_start::*;
pub mod set_allowlist;
pub mod set_chain_config;
pub use set_allowlist::*;
pub mod set_signer_config;
pub use set_signer_config::*;
pub mod remove_signer_config;
pub use remove_signer_config::*;
pub mod set_storage_locations;
pub use set_storage_locations::*;

#[cfg(test)]
mod tests {
    use anchor_lang::prelude::Pubkey;
    use anchor_lang::ToAccountMetas;

    /// The router resolves accounts once, for `start_verification_at_source`, and reuses that list
    /// for its `get_verifier_fee` CPI on the same verifier (see the router's
    /// `instructions/ccip_send/resolve_accounts/ccv.rs`). So the two contexts must present an
    /// identical account list — same accounts, same order, same signer/writable flags.
    ///
    /// Adding or removing an account on one side breaks compilation here (missing/unknown field);
    /// reordering or changing a flag fails the assertion.
    #[test]
    fn account_lists_match_start_verification_at_source() {
        let config = Pubkey::new_unique();
        let authority = Pubkey::new_unique();
        let remote_chain_config = Pubkey::new_unique();

        let get_fee = crate::accounts::GetFee {
            config,
            authority,
            remote_chain_config,
        };
        let start_verification = crate::accounts::StartVerificationAtSource {
            config,
            authority,
            remote_chain_config,
        };

        assert_eq!(
            get_fee.to_account_metas(None),
            start_verification.to_account_metas(None),
        );
    }
}
