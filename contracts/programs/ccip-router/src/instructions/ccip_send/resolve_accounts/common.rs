use anchor_lang::prelude::*;

use crate::{
    instruction::{CcipSendV2, GetFeeV2},
    instructions::{
        ccip_send::CcipSendV2Params,
        common::{OnrampRemainingAccountListLengths, OnrampResolutionMetadata},
        get_fee::GetFeeParams,
    },
    CcipRouterError,
};
use ccip_common::{resolution::*, seed};

/// Stand-in token sender for `get_fee_v2`. Fees don't depend on the sender, so both the
/// resolver and the handler derive the (possibly nonexistent) sender ATA from this key.
pub(crate) const GET_FEE_PLACEHOLDER_SENDER: Pubkey = Pubkey::new_from_array([0u8; 32]);

/// The onramp instruction under resolution, normalized to the ccip_send parameter shape all
/// shared stages consume.
pub(crate) struct OnrampResolutionInputs {
    pub params: CcipSendV2Params,
    /// The token sender whose accounts must be resolved: the resolution caller for
    /// `ccip_send_v2` (the send authority always signs it itself), or
    /// [`GET_FEE_PLACEHOLDER_SENDER`] for `get_fee_v2`.
    pub sender: Pubkey,
}

/// Parses the instruction being resolved into the ccip_send parameter shape all shared stages
/// consume. Every stage past `start` is shared between `ccip_send_v2` and `get_fee_v2` (see
/// `get_fee::resolve_accounts`), so the original `ix_data` may carry either instruction: both
/// hold the same dest chain selector and message, which is everything the stages rely on.
pub(crate) fn parse_onramp_resolution_inputs(
    params: &ResolveAccountsParams,
) -> Result<OnrampResolutionInputs> {
    let (discriminator, data) = params
        .ix_data
        .split_at_checked(CcipSendV2::DISCRIMINATOR.len())
        .ok_or(CcipRouterError::InvalidInstructionData)?;

    if discriminator == GetFeeV2::DISCRIMINATOR {
        let get_fee_params = GetFeeParams::try_from_slice(data)
            .map_err(|_| CcipRouterError::InvalidInstructionData)?;
        return Ok(OnrampResolutionInputs {
            params: CcipSendV2Params {
                dest_chain_selector: get_fee_params.dest_chain_selector,
                message: get_fee_params.message,
                resolution_metadata: get_fee_params.resolution_metadata,
            },
            sender: GET_FEE_PLACEHOLDER_SENDER,
        });
    }

    require!(
        discriminator == CcipSendV2::DISCRIMINATOR,
        CcipRouterError::InvalidInstructionData
    );
    let send_params = CcipSendV2Params::try_from_slice(data)
        .map_err(|_| CcipRouterError::InvalidInstructionData)?;

    Ok(OnrampResolutionInputs {
        params: send_params,
        sender: params.caller,
    })
}

pub(crate) fn end_resolution(
    accounts_to_save: Vec<CcipAccountMeta>,
    account_list_lengths: OnrampRemainingAccountListLengths,
) -> Result<ResolveAccountsResponse> {
    Ok(ResolveAccountsResponse {
        ask_again_with: vec![],
        accounts_to_save,
        lookup_tables_to_save: vec![],
        next_ix_discriminator: None,
        metadata: serialize_metadata(&OnrampResolutionMetadata {
            account_list_lengths,
        })?,
    })
}

/// Threaded through the static token stage's own rounds, which page the token pool's lookup table.
/// That stage runs before any pool CPI, so it deliberately does not carry
/// [`CcipSendTokenStagesMetadata::inner_cpi_resolution_inputs`]: the two stages never need each
/// other's cursor, and keeping them apart also keeps each response's metadata small (see
/// `stage_metadata_fits_the_response_budget`).
#[derive(Clone, Debug, PartialEq, Eq, AnchorSerialize, AnchorDeserialize, Default)]
pub(crate) struct CcipSendTokenStaticStageMetadata {
    pub account_list_lengths: OnrampRemainingAccountListLengths,
    /// How many token lookup-table entries the static token stage has already emitted. Non-zero
    /// only while that stage is paginating; see `retrieve_token_static_accounts_stage`.
    pub token_accounts_emitted: u8,
}

/// Threaded through the nested token stage's rounds, which drive the pool program's own resolver.
#[derive(Clone, Debug, PartialEq, Eq, AnchorSerialize, AnchorDeserialize, Default)]
pub(crate) struct CcipSendTokenStagesMetadata {
    pub account_list_lengths: OnrampRemainingAccountListLengths,
    /// Inputs for the next round of nested pool resolution. Populated by the caller
    /// on the first round, populated from the previous round's `next_cpi_resolution_inputs`
    /// afterwards.
    pub inner_cpi_resolution_inputs: Vec<u8>,
}

// Derives the pool accounts the construct-final-ccv-list stage needs in order to CPI into the
// pool's get_required_ccvs, in the order the stage expects them in its remaining accounts:
// [pool_program, pool_config, pool_chain_config].
pub(crate) fn pool_ccv_resolution_accounts(
    pool_program: Pubkey,
    token_mint: Pubkey,
    dest_chain_selector: u64,
) -> Vec<Pubkey> {
    let selector = dest_chain_selector.to_le_bytes();
    vec![
        pool_program,
        find(&[seed::POOL_STATE_SEED, token_mint.as_ref()], pool_program),
        find(
            &[seed::POOL_CHAINCONFIG_SEED, &selector, token_mint.as_ref()],
            pool_program,
        ),
        find(
            &[
                seed::POOL_CHAINCONFIG_SEED_2_0,
                &selector,
                token_mint.as_ref(),
            ],
            pool_program,
        ),
    ]
}

#[cfg(test)]
mod tests {
    use super::*;

    /// The onramp counterpart of the offramp's budget test: a stage that emits a full page of
    /// accounts has [`MAX_METADATA_PER_RESPONSE`] bytes left for its metadata, and the paginating
    /// stage's cursor metadata sits far under it.
    ///
    /// `ccvs` is empty at this stage: the CCV segment is only appended later, by the CCV stages,
    /// which do not paginate.
    #[test]
    fn stage_metadata_fits_the_response_budget() {
        let metadata = serialize_metadata(&CcipSendTokenStaticStageMetadata {
            account_list_lengths: OnrampRemainingAccountListLengths {
                token_transfer: u8::MAX,
                executor: u8::MAX,
                ccv_vault_atas: u8::MAX,
                ccvs: vec![],
            },
            token_accounts_emitted: u8::MAX,
        })
        .unwrap();

        assert!(
            metadata.len() <= MAX_METADATA_PER_RESPONSE,
            "static token stage metadata is {} bytes, budget is {MAX_METADATA_PER_RESPONSE}",
            metadata.len()
        );
    }
}
