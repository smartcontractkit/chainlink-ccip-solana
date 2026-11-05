//! Combined `nested_resolve_accounts_start` dispatcher.
//!
//! This program serves two CPI-resolution protocols under the one Anchor instruction name
//! `nested_resolve_accounts_start` (mandated by the router/offramp, see plan §2.1):
//! - the pool-v2 protocol (`base_token_pool::v2::TokenPoolCpiResolutionInputs`), used by the
//!   router/onramp/offramp when resolving accounts for this program's *pool* role.
//! - the verifier/CCV protocol (`LombardVerifierCpiResolutionInputs`), used when resolving
//!   accounts for this program's *verifier* role. Only its `Verify` variant must stay
//!   Borsh-shape-identical to `committee_verifier::CommitteeVerifierCpiResolutionInputs::Verify`
//!   (see that variant's doc below) - round-1 input shared by every CCV. Later rounds are
//!   this program's own private continuation and carry no such constraint.
//!
//! Disambiguation is NOT done via `NestedResolveAccountsParams.caller` (that field's value is
//! only used later, to strip the signer bit off whichever resolved account matches it - see
//! `ccip_common::resolution::normalize_nested_accounts`). Instead, the single account this
//! instruction takes is decoded by Anchor discriminator: it is always either this program's own
//! per-mint `State` (pool-flavor calls) or its global `VerifierConfig` (verifier-flavor calls),
//! and those two account types have distinct, collision-free 8-byte discriminators. This is more
//! robust than branching on `caller`'s value, since it doesn't require recomputing/trusting any
//! router- or offramp-derived PDA.

use anchor_lang::prelude::*;
use base_token_pool::v2::TokenPoolCpiResolutionInputs;
use ccip_common::messages::MessageV1FieldsAtResolutionTime;
use ccip_common::resolution::{NestedResolveAccountsParams, NestedResolveAccountsResponse};

use crate::errors::LombardTokenPoolError;
use crate::instructions::{pool_v2_resolution, verifier::resolution as verifier_resolution};
use crate::state::{State, VerifierConfig};

/// The `Verify` variant must stay Borsh-shape-identical to
/// `committee_verifier::CommitteeVerifierCpiResolutionInputs::Verify` (field order, names, and
/// types), since `ccip-offramp`'s `ccv.rs` always serializes ONE `Verify` payload for round 1 of
/// ANY CCV's nested resolution, regardless of which CCV program it's about to call. `encoded_message`
/// and `verifier_results` were added to that shared round-1 payload specifically so Lombard's
/// `verify()` - which (unlike committee_verifier's) needs message- and payload-hash-dependent
/// accounts (mint, receiver, mailbox message PDAs) - has enough to resolve them; committee_verifier
/// itself ignores both fields.
///
/// `VerifyStage2` and the other variants below carry NO such cross-program constraint: they are
/// only ever produced by this program's own `verify_start` (as `next_cpi_resolution_inputs`) and
/// decoded by this same program on the next round, so their shape is entirely private to Lombard.
#[derive(AnchorSerialize, AnchorDeserialize, Clone, Debug)]
pub enum LombardVerifierCpiResolutionInputs {
    Verify {
        source_chain_selector: u64,
        encoded_message: Vec<u8>,
        verifier_results: Vec<u8>,
    },
    /// Round 1 of `start_verification_at_source`'s resolution.
    StartVerificationAtSource {
        fields: MessageV1FieldsAtResolutionTime,
        /// Mint the sender pays the fee in, WSOL-normalized.
        fee_token: Pubkey,
        /// This verifier's slice of the sender's `ccv_args`.
        verifier_args: Vec<u8>,
    },
    /// Round 2 (final) of `Verify`'s multi-round resolution (see module doc above). Carries what
    /// round 1 already extracted from the message/verifier_results, so round 2 doesn't need to
    /// re-decode them; round 2 additionally reads `remote_chain_config`'s, `mint`'s, and
    /// `state`'s on-chain data (the latter's `config.router` is needed to predict
    /// `allowed_offramp`'s PDA).
    VerifyStage2 {
        mint: Pubkey,
        receiver: Pubkey,
        receiver_usage: ccip_common::messages::SVMTokenReceiverUsage,
        payload_hash: [u8; 32],
        caller_program: Pubkey,
    },
    /// Round 2 of `start_verification_at_source`'s resolution (Lombard-private continuation).
    /// Reads `remote_chain_config`'s and `mint`'s on-chain data (via `ctx.remaining_accounts`,
    /// requested by round 1's `ask_again_with`) to derive the mint-/chain-dependent accounts.
    StartVerificationAtSourceStage2 { mint: Pubkey },
    /// Round 3 (Lombard-private continuation). Reads `mailbox_config`'s on-chain data (via
    /// `ctx.remaining_accounts`, requested by round 2's `ask_again_with`) to derive the accounts
    /// that depend on the mailbox's current outbound nonce/treasury.
    StartVerificationAtSourceStage3,
}

#[derive(Accounts)]
pub struct NestedResolveAccountsStart<'info> {
    /// CHECK: owned by this program, then manually discriminated in the handler as either its pool
    /// `State` or its `VerifierConfig`, per the module doc above. Whichever account resolution is
    /// being asked about (pool or verifier role) determines which one the caller passes in.
    #[account(owner = crate::ID @ LombardTokenPoolError::InvalidResolutionAccountOwner)]
    pub state_or_verifier_config: UncheckedAccount<'info>,
}

pub fn nested_resolve_accounts_start(
    ctx: Context<NestedResolveAccountsStart>,
    params: NestedResolveAccountsParams,
) -> Result<NestedResolveAccountsResponse> {
    let account_info = ctx.accounts.state_or_verifier_config.to_account_info();
    let account_key = account_info.key();
    let data = account_info.try_borrow_data()?;

    if let Ok(state) = State::try_deserialize(&mut &data[..]) {
        drop(data);
        let inputs = TokenPoolCpiResolutionInputs::try_from_slice(&params.cpi_resolution_inputs)
            .map_err(|_| LombardTokenPoolError::InvalidResolutionInputs)?;
        return match inputs {
            TokenPoolCpiResolutionInputs::LockOrBurn(lock_or_burn_in) => {
                pool_v2_resolution::lock_or_burn_start(&lock_or_burn_in)
            }
            TokenPoolCpiResolutionInputs::ReleaseOrMint(_) => {
                pool_v2_resolution::release_or_mint_start(account_key)
            }
            TokenPoolCpiResolutionInputs::GetRequiredCcv(get_required_ccvs_in) => {
                pool_v2_resolution::get_required_ccvs_start(
                    account_key,
                    &state,
                    get_required_ccvs_in.remote_chain_selector,
                )
            }
        };
    }

    VerifierConfig::try_deserialize(&mut &data[..])
        .map_err(|_| LombardTokenPoolError::InvalidResolutionInputs)?;
    drop(data);

    let inputs = LombardVerifierCpiResolutionInputs::try_from_slice(&params.cpi_resolution_inputs)
        .map_err(|_| LombardTokenPoolError::InvalidResolutionInputs)?;
    match inputs {
        LombardVerifierCpiResolutionInputs::Verify {
            source_chain_selector,
            encoded_message,
            verifier_results,
        } => verifier_resolution::verify_start(
            account_key,
            params.caller,
            params.caller_program,
            source_chain_selector,
            &encoded_message,
            &verifier_results,
        ),
        LombardVerifierCpiResolutionInputs::StartVerificationAtSource { fields, .. } => {
            let token = fields
                .token_transfer
                .map(|tt| tt.source_token_address)
                .unwrap_or_default();

            verifier_resolution::start_verification_at_source_start(
                account_key,
                params.caller,
                fields.dest_chain_selector,
                token,
            )
        }
        LombardVerifierCpiResolutionInputs::VerifyStage2 {
            mint,
            receiver,
            receiver_usage,
            payload_hash,
            caller_program,
        } => verifier_resolution::verify_stage2(
            ctx.remaining_accounts,
            mint,
            receiver,
            receiver_usage,
            payload_hash,
            caller_program,
        ),
        LombardVerifierCpiResolutionInputs::StartVerificationAtSourceStage2 { mint } => {
            verifier_resolution::start_verification_at_source_stage2(
                account_key,
                ctx.remaining_accounts,
                mint,
            )
        }
        LombardVerifierCpiResolutionInputs::StartVerificationAtSourceStage3 => {
            verifier_resolution::start_verification_at_source_stage3(ctx.remaining_accounts)
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use base_token_pool::state::BaseConfig;
    use ccip_common::messages::FinalityConfig;

    fn dummy_state() -> State {
        State {
            version: 1,
            config: BaseConfig {
                token_program: Pubkey::default(),
                mint: Pubkey::new_unique(),
                decimals: 9,
                pool_signer: Pubkey::new_unique(),
                pool_token_account: Pubkey::new_unique(),
                owner: Pubkey::new_unique(),
                proposed_owner: Pubkey::default(),
                rate_limit_admin: Pubkey::new_unique(),
                router_onramp_authority: Pubkey::new_unique(),
                router: Pubkey::new_unique(),
                rebalancer: Pubkey::default(),
                can_accept_liquidity: false,
                list_enabled: false,
                allow_list: vec![],
                rmn_remote: Pubkey::new_unique(),
            },
            bridge: Pubkey::new_unique(),
            alt: None,
        }
    }

    fn dummy_verifier_config() -> VerifierConfig {
        VerifierConfig {
            version: 1,
            bump: 0,
            owner: Pubkey::new_unique(),
            proposed_owner: Pubkey::default(),
            storage_location_admin: Pubkey::new_unique(),
            proposed_storage_location_admin: Pubkey::default(),
            allowlist_admin: Pubkey::new_unique(),
            fee_aggregator: Pubkey::default(),
            allowed_finality_config: FinalityConfig::default(),
            version_tag: [1, 2, 3, 4],
            supported_tokens: vec![],
        }
    }

    /// This is the safety property the whole dispatcher depends on: a serialized `State`
    /// (pool-flavor) can never be mistaken for a `VerifierConfig` (verifier-flavor), and vice
    /// versa, because Anchor's 8-byte account discriminators (derived from the type name) don't
    /// collide - `nested_resolve_accounts_start` relies on exactly this to route by account type
    /// rather than by `NestedResolveAccountsParams.caller`.
    #[test]
    fn state_and_verifier_config_discriminators_never_collide() {
        let mut state_bytes = Vec::new();
        dummy_state().try_serialize(&mut state_bytes).unwrap();

        let mut verifier_bytes = Vec::new();
        dummy_verifier_config()
            .try_serialize(&mut verifier_bytes)
            .unwrap();

        assert!(State::try_deserialize(&mut &state_bytes[..]).is_ok());
        assert!(VerifierConfig::try_deserialize(&mut &state_bytes[..]).is_err());

        assert!(VerifierConfig::try_deserialize(&mut &verifier_bytes[..]).is_ok());
        assert!(State::try_deserialize(&mut &verifier_bytes[..]).is_err());
    }
}
