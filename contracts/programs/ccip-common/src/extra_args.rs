use anchor_lang::prelude::*;

use crate::{
    messages::FinalityConfig, types::CrossChainGas, CommonCcipError, INCLUDE_DEFAULT_CCVS,
};

/// Upper bound on the CCVs a single message may carry, enforced wherever the lists are merged.
///
/// This is a structural limit, not a protocol policy: it is the longest list the onramp can
/// actually resolve. The first CCV round embeds the whole merged list in its metadata on top of
/// one fee-vault ATA per CCV, and `construct_final_ccv_list` emits those unpaginated, so a round
/// costs ~66 bytes per CCV against the 1kB return-data cap — 12 fit, 13 do not
/// (`first_round_fits_the_ccv_limit` pins both ends). A larger bound here would only admit
/// messages that fail during resolution. That byte budget binds well before the account page
/// does, so this stays below [`MAX_ACCOUNTS_PER_RESPONSE`](crate::resolution::MAX_ACCOUNTS_PER_RESPONSE)
/// rather than tracking it. The protocol's own limit is lower and lives in
/// the executor (`max_ccvs_per_msg`, mirroring EVM's `Executor.i_maxCCVsPerMsg`), which rejects an
/// over-large list during the fee CPI; this bound only stops a message that could not be resolved
/// at all from getting that far.
// In the future, we could remove this limit by adding pagination to the VaultATA resolution.
// We consider that it is set high enough for this to not be a priority.
pub const MAX_CCVS_PER_MESSAGE: usize = 12;

/// The necessary half of the relationship documented above: a CCV list that outgrows one page of
/// accounts could not be resolved at all, since `construct_final_ccv_list` emits its fee-vault
/// ATAs unpaginated. Shrinking the page below this bound has to come with a smaller CCV limit.
/// Fitting the page is not sufficient — the ~66 bytes per CCV are what actually bind — so the
/// router's `first_round_fits_the_ccv_limit` is what pins the value itself.
const _: () = assert!(MAX_CCVS_PER_MESSAGE <= crate::resolution::MAX_ACCOUNTS_PER_RESPONSE);

// NOTE: this file contains ExtraArgs for SVM -> remote chains
// these are onramp specific extraArgs for ccipSend only

/// Tag to indicate a gas limit (or dest chain equivalent processing units) and Out Of Order Execution. This tag is
/// available for multiple chain families. If there is no chain family specific tag, this is the default available
/// for a chain.
///
/// Note: not available for Solana VM based chains.
///
/// Fully compatible with the previously existing EVMExtraArgsV2, hence the string used to generate it.
/// bytes4(keccak256("CCIP EVMExtraArgsV2"));
pub const GENERIC_EXTRA_ARGS_V2_TAG: u32 = 0x181dcf10;
pub const GENERIC_EXTRA_ARGS_V3_TAG: u32 = 0xa69dd4aa;

/// bytes4(keccak256("CCIP SVMExtraArgsV1"));
pub const SVM_EXTRA_ARGS_V1_TAG: u32 = 0x1f3b3aba;

/// Extra args tag for chains that use the Sui VM.
pub const SUI_EXTRA_ARGS_V1_TAG: u32 = 0x21ea4ca9;

/// Sentinel executor value signalling that no automated execution is expected: when a message's
/// requested executor equals this tag, the executor fee is zero.
///
/// On EVM this is `address(bytes20(0xeba517d2))`, where `0xeba517d2 == bytes4(keccak256("NO_EXECUTION_TAG"))`.
/// The SVM equivalent is the 32-byte `Pubkey` holding those 4 tag bytes left-aligned (the remaining
/// 28 bytes zeroed), mirroring the left-alignment of EVM's `bytes4 -> bytes20` cast.
pub const NO_EXECUTION_TAG: Pubkey = Pubkey::new_from_array([
    0xeb, 0xa5, 0x17, 0xd2, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
    0, 0, 0, 0,
]);

/// The maximum number of accounts that can be passed in SVMExtraArgs.
pub const SVM_EXTRA_ARGS_MAX_ACCOUNTS: usize = 64;

/// The maximum number of receiver object ids that can be passed in SuiExtraArgs.
pub const SUI_EXTRA_ARGS_MAX_RECEIVER_OBJECT_IDS: usize = 64;

/// Splits the 4-byte big-endian version tag from the front of serialized extra args, returning the
/// tag and the remaining payload, or `None` when there are fewer than 4 bytes.
///
/// `None` does not distinguish between an empty slice (no extra args provided) and a non-empty but
/// too-short slice (malformed input): callers must inspect the original slice to tell them apart and
/// decide how to treat each case.
///
/// This mirrors the tag handling in the FeeQuoter's `process_extra_args` so that the on-ramp and
/// the quoter agree on how a serialized extra-args blob is split into (tag, payload).
pub fn split_extra_args_tag(extra_args: &[u8]) -> Option<(u32, &[u8])> {
    let tag = extra_args.get(..4)?;
    let tag = u32::from_be_bytes(
        tag.try_into()
            .expect(".get(..4) returns Some only for a 4-byte slice"),
    );
    Some((tag, &extra_args[4..]))
}

#[derive(Clone, AnchorSerialize, AnchorDeserialize, Default)]
pub struct GenericExtraArgsV2 {
    pub gas_limit: u128,                    // message gas limit
    pub allow_out_of_order_execution: bool, // user configurable OOO execution that must match with the DestChainConfig.EnforceOutOfOrderExecution
}

impl GenericExtraArgsV2 {
    pub fn serialize_with_tag(&self) -> Vec<u8> {
        let mut buffer = GENERIC_EXTRA_ARGS_V2_TAG.to_be_bytes().to_vec();
        borsh::to_writer(&mut buffer, &self).unwrap();
        buffer
    }
    pub fn default_config(default_gas_limit: u128) -> GenericExtraArgsV2 {
        GenericExtraArgsV2 {
            gas_limit: default_gas_limit,
            ..Default::default()
        }
    }
}

// Extra Args of message when SVM -> SVM
#[derive(Clone, AnchorSerialize, AnchorDeserialize, Default)]
pub struct SVMExtraArgsV1 {
    pub compute_units: u32, // compute units are used by the offchain to add computeUnitLimits to the execute stage
    pub account_is_writable_bitmap: u64,
    pub allow_out_of_order_execution: bool, // SVM chains require OOO, but users are required to explicitly state OOO is allowed in extraArgs
    pub token_receiver: [u8; 32],           // cannot be 0-address if tokens are sent in message
    pub accounts: Vec<[u8; 32]>,            // account list for messaging on remote SVM chain
}

impl SVMExtraArgsV1 {
    pub fn serialize_with_tag(&self) -> Vec<u8> {
        let mut buffer = SVM_EXTRA_ARGS_V1_TAG.to_be_bytes().to_vec();
        borsh::to_writer(&mut buffer, &self).unwrap();
        buffer
    }
    pub fn default_config(default_tx_gas_limit: u32) -> SVMExtraArgsV1 {
        SVMExtraArgsV1 {
            compute_units: default_tx_gas_limit,
            ..Default::default()
        }
    }
}

// Extra Args of message when SVM -> SUI
#[derive(Clone, AnchorSerialize, AnchorDeserialize, Default)]
pub struct SuiExtraArgsV1 {
    pub gas_limit: u128,                    // message gas limit
    pub allow_out_of_order_execution: bool, // user configurable OOO execution that must match with the DestChainConfig.EnforceOutOfOrderExecution
    pub token_receiver: [u8; 32],           // cannot be 0-address if tokens are sent in message
    pub receiver_object_ids: Vec<[u8; 32]>, // list of object ids for messaging on remote SUI chain
}

impl SuiExtraArgsV1 {
    pub fn serialize_with_tag(&self) -> Vec<u8> {
        let mut buffer = SUI_EXTRA_ARGS_V1_TAG.to_be_bytes().to_vec();
        borsh::to_writer(&mut buffer, &self).unwrap();
        buffer
    }
}

#[derive(Clone, AnchorSerialize, AnchorDeserialize, Default)]
pub struct GenericExtraArgsV3 {
    pub gas_limit: CrossChainGas, // message gas limit
    pub finality: FinalityConfig,
    pub ccvs: Vec<Pubkey>,
    pub ccv_args: Vec<Vec<u8>>,
    pub executor: Pubkey,
    pub executor_args: Vec<u8>,
    pub token_receiver: Vec<u8>,
    pub token_args: Vec<u8>,
}

impl GenericExtraArgsV3 {
    pub fn serialize_with_tag(&self) -> Vec<u8> {
        let mut buffer = GENERIC_EXTRA_ARGS_V3_TAG.to_be_bytes().to_vec();
        borsh::to_writer(&mut buffer, &self).unwrap();
        buffer
    }

    /// Merges lane-mandated and token-pool-required CCVs into this message's CCV list in place.
    ///
    /// The existing `ccvs`/`ccv_args` (user requested or default) are kept as-is, then extended
    /// with any lane-mandated and pool-required CCVs not already present. Mandated/required CCVs
    /// carry empty arguments. A zero CCV in the pool-required list expands to the lane defaults.
    ///
    /// There is no protocol-level requirement on the ordering of CCVs in the final list, but for
    /// determinism we keep user requested first, then lane-mandated second, pool-required last.
    ///
    /// Assumes there are no duplicates in the existing CCV list.
    ///
    /// Rejects a merged list longer than [`MAX_CCVS_PER_MESSAGE`]. Enforcing it here covers every
    /// path that builds the list — `ccip_send_v2`, `get_fee_v2` and account resolution — so the
    /// three cannot disagree about whether a message is resolvable.
    pub fn merge_ccv_lists(
        &mut self,
        pool_required: &[Pubkey],
        lane_mandated: &[Pubkey],
        default_ccvs: &[Pubkey],
    ) -> Result<()> {
        let additional = lane_mandated.len() + pool_required.len() + default_ccvs.len();
        self.ccvs.reserve(additional);
        self.ccv_args.reserve(additional);

        for ccv in lane_mandated.iter() {
            self.push_ccv_if_missing(*ccv);
        }

        for ccv in pool_required {
            if *ccv == INCLUDE_DEFAULT_CCVS {
                for default_ccv in default_ccvs {
                    self.push_ccv_if_missing(*default_ccv);
                }
            } else {
                self.push_ccv_if_missing(*ccv);
            }
        }

        require_gte!(
            MAX_CCVS_PER_MESSAGE,
            self.ccvs.len(),
            CommonCcipError::TooManyCcvs
        );

        Ok(())
    }

    fn push_ccv_if_missing(&mut self, ccv: Pubkey) {
        if !self.ccvs.contains(&ccv) {
            self.ccvs.push(ccv);
            self.ccv_args.push(Vec::new());
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::pool_ccvs_with_default_fallback;

    fn pk(seed: u8) -> Pubkey {
        let mut bytes = [0u8; 32];
        bytes[0] = seed;
        Pubkey::from(bytes)
    }

    fn args_with_ccvs(ccvs: Vec<Pubkey>) -> GenericExtraArgsV3 {
        let ccv_args = vec![Vec::new(); ccvs.len()];
        GenericExtraArgsV3 {
            ccvs,
            ccv_args,
            ..Default::default()
        }
    }

    #[test]
    fn test_pool_ccvs_with_default_fallback() {
        // An empty pool response collapses to the sentinel; a non-empty one is kept verbatim.
        assert_eq!(
            pool_ccvs_with_default_fallback(vec![]),
            vec![INCLUDE_DEFAULT_CCVS]
        );
        assert_eq!(pool_ccvs_with_default_fallback(vec![pk(1)]), vec![pk(1)]);
    }

    #[test]
    fn test_merge_pool_sentinel_expands_defaults_after_mandated() {
        // The sentinel in the pool list expands to the lane defaults, after the lane-mandated CCVs.
        let mut args = args_with_ccvs(vec![]);
        args.merge_ccv_lists(&[INCLUDE_DEFAULT_CCVS], &[pk(9)], &[pk(1), pk(2)])
            .unwrap();
        assert_eq!(args.ccvs, vec![pk(9), pk(1), pk(2)]);
        assert_eq!(args.ccv_args.len(), args.ccvs.len());
    }

    #[test]
    fn test_merge_pool_sentinel_dedups_against_user_list() {
        // A default the user already requested is not duplicated by the sentinel expansion.
        let mut args = args_with_ccvs(vec![pk(1)]);
        args.merge_ccv_lists(&[INCLUDE_DEFAULT_CCVS], &[], &[pk(1), pk(2)])
            .unwrap();
        assert_eq!(args.ccvs, vec![pk(1), pk(2)]);
    }

    #[test]
    fn test_merge_sentinel_inside_non_empty_pool_list() {
        // A v2 pool may require its own CCVs *and* the lane defaults via an embedded sentinel.
        let mut args = args_with_ccvs(vec![]);
        args.merge_ccv_lists(&[pk(7), INCLUDE_DEFAULT_CCVS], &[], &[pk(1), pk(2)])
            .unwrap();
        assert_eq!(args.ccvs, vec![pk(7), pk(1), pk(2)]);
    }

    #[test]
    fn test_merge_without_sentinel_skips_defaults() {
        // A pool that specifies its own CCVs (no sentinel) does not pull in the lane defaults.
        let mut args = args_with_ccvs(vec![]);
        args.merge_ccv_lists(&[pk(7)], &[pk(9)], &[pk(1), pk(2)])
            .unwrap();
        assert_eq!(args.ccvs, vec![pk(9), pk(7)]);
    }

    #[test]
    fn test_merge_dedups_lane_mandated_and_pool_against_user() {
        // User-requested CCVs stay first and are not duplicated by mandated/pool contributions.
        let mut args = args_with_ccvs(vec![pk(7), pk(9)]);
        args.merge_ccv_lists(&[pk(7)], &[pk(9), pk(3)], &[pk(1)])
            .unwrap();
        assert_eq!(args.ccvs, vec![pk(7), pk(9), pk(3)]);
        assert_eq!(args.ccv_args.len(), args.ccvs.len());
    }

    /// The merged list feeds a single unpaginated vault-ATA response, so a list that cannot fit
    /// one page must be rejected where it is built rather than failing later as an opaque
    /// oversized response. Duplicates still collapse first, so only distinct CCVs count.
    #[test]
    fn test_merge_rejects_more_ccvs_than_one_response_can_describe() {
        let at_limit: Vec<Pubkey> = (0..MAX_CCVS_PER_MESSAGE as u8).map(pk).collect();
        let mut args = args_with_ccvs(at_limit.clone());
        assert!(args.merge_ccv_lists(&[], &[], &[]).is_ok());

        // One lane-mandated CCV beyond the limit.
        let mut args = args_with_ccvs(at_limit.clone());
        assert!(args
            .merge_ccv_lists(&[], &[pk(MAX_CCVS_PER_MESSAGE as u8)], &[])
            .is_err());

        // A CCV already present does not push the list over the limit.
        let mut args = args_with_ccvs(at_limit);
        assert!(args.merge_ccv_lists(&[], &[pk(0)], &[]).is_ok());
    }

    #[test]
    fn test_merge_empty_pool_list_contributes_nothing() {
        // No token transfer: an empty pool list (not the sentinel) leaves defaults out entirely.
        let mut args = args_with_ccvs(vec![pk(7)]);
        args.merge_ccv_lists(&[], &[], &[pk(1), pk(2)]).unwrap();
        assert_eq!(args.ccvs, vec![pk(7)]);
    }
}
