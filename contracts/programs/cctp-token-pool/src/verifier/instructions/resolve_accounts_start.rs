use anchor_lang::prelude::*;

use ccip_common::messages::MessageV1FieldsAtResolutionTime;
use ccip_common::resolution::*;

use super::{start_verification_at_source, verify};
use crate::verifier::state::*;

#[derive(AnchorSerialize, AnchorDeserialize, Clone, Debug)]
pub enum CctpCcvCpiResolutionInputs {
    /// The `Verify` variant must stay Borsh-shape-identical to
    /// `committee_verifier::CommitteeVerifierCpiResolutionInputs::Verify` (field order, names, and
    /// types), since `ccip-offramp`'s `ccv.rs` always serializes ONE `Verify` payload for round 1 of
    /// ANY CCV's nested resolution, regardless of which CCV program it's about to call. `encoded_message`
    /// and `verifier_results` were added to that shared round-1 payload specifically so CctpCcv's
    /// `verify()` - which (unlike committee_verifier's) needs message- and payload-hash-dependent
    /// accounts (mint, receiver, mailbox message PDAs) - has enough to resolve them; committee_verifier
    /// itself ignores both fields.
    Verify {
        source_chain_selector: u64,
        encoded_message: Vec<u8>,
        verifier_results: Vec<u8>,
    },
    /// Round 1 of `start_verification_at_source`'s resolution. `token` must stay
    /// Borsh-shape-identical to `committee_verifier::CommitteeVerifierCpiResolutionInputs::
    /// StartVerificationAtSource`'s own `token` field for the same reason as `Verify` above:
    /// `ccip-router` builds this payload generically for every CCV using its own copy of that
    /// type, and CctpCcv (unlike committee-verifier) needs the token to resolve mint-dependent
    /// accounts.
    StartVerificationAtSource {
        fields: MessageV1FieldsAtResolutionTime,
        /// Mint the sender pays the fee in, WSOL-normalized.
        fee_token: Pubkey,
        /// This verifier's slice of the sender's `ccv_args`.
        verifier_args: Vec<u8>,
    },
    // Additional rounds
    StartVerificationAtSourceWithNonce {
        sender: Pubkey,
    },
    VerifyWithRemoteChain {
        source_chain_selector: u64,
        mint: Pubkey,
        remote_burn_token: Pubkey,
        offramp: Pubkey,
    },
}

pub fn nested_resolve_accounts_start<'info>(
    config_key: Pubkey,
    config: Config,
    remaining_accounts: &'info [AccountInfo<'info>],
    params: NestedResolveAccountsParams,
) -> Result<NestedResolveAccountsResponse> {
    let inputs = CctpCcvCpiResolutionInputs::try_from_slice(&params.cpi_resolution_inputs)
        .inspect_err(|_| {
            msg!("Failed to parse inputs: {:?}", params.cpi_resolution_inputs);
        })?;

    msg!("Current stage: {:?}", inputs);
    match inputs {
        CctpCcvCpiResolutionInputs::StartVerificationAtSource { fields, .. } => {
            start_verification_at_source::resolve_accounts::start(
                config_key,
                config,
                params.caller,
                fields,
            )
        }
        CctpCcvCpiResolutionInputs::StartVerificationAtSourceWithNonce { sender } => {
            start_verification_at_source::resolve_accounts::with_nonce(remaining_accounts, sender)
        }
        CctpCcvCpiResolutionInputs::Verify {
            source_chain_selector,
            encoded_message,
            verifier_results,
        } => verify::resolve_accounts::start(
            config_key,
            config,
            source_chain_selector,
            encoded_message,
            verifier_results,
        ),
        CctpCcvCpiResolutionInputs::VerifyWithRemoteChain {
            source_chain_selector,
            mint,
            remote_burn_token,
            offramp,
        } => verify::resolve_accounts::with_remote_chain(
            remaining_accounts,
            source_chain_selector,
            mint,
            remote_burn_token,
            offramp,
        ),
    }
}
