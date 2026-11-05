pub mod admin;
pub use admin::*;

pub mod initialize_verifier;
pub use initialize_verifier::*;

pub mod get_storage_locations;
pub use get_storage_locations::*;

pub mod reclaim_verifier_event_account;
pub use reclaim_verifier_event_account::*;

pub mod set_allowlist;
pub use set_allowlist::*;

pub mod set_chain_config;
pub use set_chain_config::*;

pub mod set_chain_config_cctp;
pub use set_chain_config_cctp::*;

pub mod set_storage_locations;
pub use set_storage_locations::*;

pub mod start_verification_at_source;
pub use start_verification_at_source::*;

pub mod get_fee;
pub use get_fee::*;

pub mod verify;
pub use verify::*;

pub mod resolve_accounts_start;

pub mod utils {
    use anchor_lang::prelude::*;

    use crate::error::CctpPoolVerifierError;

    pub fn left_padded_pubkey(address: &[u8]) -> Result<Pubkey> {
        let padded_bytes = padded::<32>(address)?;
        Ok(Pubkey::new_from_array(padded_bytes))
    }

    pub fn parse_pubkey(bytes: &[u8]) -> Result<Pubkey> {
        let bytes: [u8; 32] = bytes
            .try_into()
            .map_err(|_| CctpPoolVerifierError::InvalidPubkeyBytes)?;
        Ok(Pubkey::new_from_array(bytes))
    }

    fn padded<const N: usize>(bytes: &[u8]) -> Result<[u8; N]> {
        require_gte!(
            N,
            bytes.len(),
            CctpPoolVerifierError::InvalidInstructionData
        );

        let mut padded = [0u8; N];
        let start = N - bytes.len();
        padded[start..].copy_from_slice(bytes);
        Ok(padded)
    }
}

#[cfg(test)]
mod tests {
    use anchor_lang::prelude::Pubkey;
    use anchor_lang::ToAccountMetas;

    /// The router resolves accounts once, for `start_verification_at_source`, and reuses that list
    /// for its `get_verifier_fee` CPI on the same verifier (see the router's
    /// `instructions/ccip_send/resolve_accounts/ccv.rs`). So the two contexts for StartVerificationAtSource
    /// must include all the accounts of GetVerifierFee at the beginning (it can also include additional
    /// accounts afterwards that the get_verifier_fee handler will just ignore)
    ///
    /// Adding or removing an account on one side breaks compilation here (missing/unknown field);
    /// reordering or changing a flag fails the assertion.
    #[test]
    fn account_lists_match_start_verification_at_source() {
        let config = Pubkey::new_unique();
        let authority = Pubkey::new_unique();
        let remote_chain_config = Pubkey::new_unique();

        let get_fee = crate::accounts::GetVerifierFee {
            config,
            authority,
            remote_chain_config,
        };
        let start_verification = crate::accounts::StartVerificationAtSource {
            config,
            authority,
            remote_chain_config,
            mint: Pubkey::new_unique(),
            token_program: Pubkey::new_unique(),
            cpi_signer: Pubkey::new_unique(),
            nonce: Pubkey::new_unique(),
            cctp_message_sent_event: Pubkey::new_unique(),
            burn_token_account: Pubkey::new_unique(),
            system_program: Pubkey::new_unique(),
            sender_authority_pda: Pubkey::new_unique(),
            denylist_account: Pubkey::new_unique(),
            message_transmitter: Pubkey::new_unique(),
            token_messenger: Pubkey::new_unique(),
            remote_token_messenger: Pubkey::new_unique(),
            token_minter: Pubkey::new_unique(),
            local_token: Pubkey::new_unique(),
            token_messenger_minter_event_authority: Pubkey::new_unique(),
            message_transmitter_event_authority: Pubkey::new_unique(),
            message_transmitter_program: Pubkey::new_unique(),
            token_messenger_minter_program: Pubkey::new_unique(),
        };

        let fee_metas = get_fee.to_account_metas(None);
        let verification_metas = start_verification.to_account_metas(None);

        assert_eq!(fee_metas, verification_metas[..fee_metas.len()],);
    }
}
