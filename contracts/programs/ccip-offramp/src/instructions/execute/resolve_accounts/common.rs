use anchor_lang::prelude::*;

use crate::error::CcipOfframpError;
use crate::instruction as offramp_instruction;
use crate::messages::OfframpAccountListLengths;
use ccip_common::resolution::*;

use super::super::{
    resolve_exec_inputs, ExecuteParams, ExecuteResolutionMetadata, ResolvedExecInputs,
};

/// Parses an [`ExecuteParams`] payload out of `params.ix_data` and resolves it via
/// [`resolve_exec_inputs`]. In buffered mode the buffer account is taken from
/// `remaining_accounts[0]` (callers are expected to keep the buffer at that index
/// across stages); in direct mode `remaining_accounts` is ignored.
pub(crate) fn parse_exec_inputs(
    params: &ResolveAccountsParams,
    remaining_accounts: &[AccountInfo],
) -> Result<ResolvedExecInputs> {
    let (_, data) = params
        .ix_data
        .split_at_checked(offramp_instruction::ExecuteV2::DISCRIMINATOR.len())
        .ok_or(CcipOfframpError::InvalidInstructionData)?;

    let exec_params = ExecuteParams::try_from_slice(data)
        .map_err(|_| CcipOfframpError::InvalidInstructionData)?;

    let buffer_info = if exec_params.exec_inputs.is_none() {
        Some(
            remaining_accounts
                .first()
                .ok_or(CcipOfframpError::InvalidInstructionData)?,
        )
    } else {
        None
    };

    resolve_exec_inputs(&exec_params, buffer_info, params.caller)
}

pub(crate) fn end_resolution(
    accounts_to_save: Vec<CcipAccountMeta>,
    account_list_lengths: OfframpAccountListLengths,
    lookup_tables_to_save: Vec<Pubkey>,
    message_id: [u8; 32],
) -> Result<ResolveAccountsResponse> {
    Ok(ResolveAccountsResponse {
        ask_again_with: vec![],
        accounts_to_save,
        lookup_tables_to_save,
        next_ix_discriminator: None,
        metadata: serialize_metadata(&ExecuteResolutionMetadata {
            account_list_lengths,
            message_id,
        })?,
    })
}

/// Threaded through the static token stage's own rounds, which page the token pool's lookup table.
/// That stage runs before any pool CPI, so it deliberately does not carry
/// [`ExecuteNestedTokenStageMetadata::inner_cpi_resolution_inputs`]: the two stages never need each
/// other's cursor, and keeping them apart also keeps each response's metadata small (see
/// `stage_metadata_fits_the_response_budget`).
#[derive(Clone, Debug, PartialEq, Eq, AnchorSerialize, AnchorDeserialize)]
pub(crate) struct ExecuteTokenStaticStageMetadata {
    pub account_list_lengths: OfframpAccountListLengths,
    /// How many token lookup-table entries the static token stage has already emitted. Non-zero
    /// only while that stage is paginating; see `retrieve_token_static_accounts_stage`.
    pub token_accounts_emitted: u8,
}

/// Threaded through the nested token stage's rounds, which drive the pool program's own resolver.
#[derive(Clone, Debug, PartialEq, Eq, AnchorSerialize, AnchorDeserialize)]
pub(crate) struct ExecuteNestedTokenStageMetadata {
    pub account_list_lengths: OfframpAccountListLengths,
    /// Inputs for the next round of nested pool resolution. Populated by the
    /// caller in the first round, populated from the previous
    /// round's `next_cpi_resolution_inputs` afterwards.
    pub inner_cpi_resolution_inputs: Vec<u8>,
}

#[cfg(test)]
mod tests {
    use super::*;

    /// Answers the sizing question [`MAX_ACCOUNTS_PER_RESPONSE`] is budgeted against: a stage that
    /// emits a full page of accounts has [`MAX_METADATA_PER_RESPONSE`] bytes left for its
    /// metadata. Both paginating stages carry the account-list lengths plus a one-byte cursor, so
    /// they sit far under it — this pins that as the account-list layout grows.
    ///
    /// `ccvs` is empty at both stages: the CCV segment is only appended later, by
    /// `begin_ccv_stages`, and those stages do not paginate.
    #[test]
    fn stage_metadata_fits_the_response_budget() {
        let account_list_lengths = OfframpAccountListLengths {
            buffer: 1,
            messaging: u8::MAX,
            token_transfer: u8::MAX,
            receiver_ccv: 2,
            receiver_registry: 1,
            ccvs: vec![],
        };

        let static_stage = serialize_metadata(&ExecuteTokenStaticStageMetadata {
            account_list_lengths: account_list_lengths.clone(),
            token_accounts_emitted: u8::MAX,
        })
        .unwrap();

        assert!(
            static_stage.len() <= MAX_METADATA_PER_RESPONSE,
            "static token stage metadata is {} bytes, budget is {MAX_METADATA_PER_RESPONSE}",
            static_stage.len()
        );

        // The nested stage does not paginate, but its metadata rides along with the accounts the
        // pool returns, so it is held to the same budget minus whatever the pool asks to carry.
        let nested_stage = serialize_metadata(&ExecuteNestedTokenStageMetadata {
            account_list_lengths,
            inner_cpi_resolution_inputs: vec![],
        })
        .unwrap();

        assert!(
            nested_stage.len() <= MAX_METADATA_PER_RESPONSE,
            "nested token stage metadata is {} bytes, budget is {MAX_METADATA_PER_RESPONSE}",
            nested_stage.len()
        );
    }
}
