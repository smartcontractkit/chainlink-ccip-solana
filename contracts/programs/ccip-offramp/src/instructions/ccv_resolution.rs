use anchor_lang::prelude::*;
use ccip_common::ccvs::{get_pool_required_ccvs_cpi, get_receiver_ccvs_cpi, GetReceiverCcvsOut};
use ccip_common::messages::{FinalityConfig, WAIT_FOR_FINALITY};
use ccip_common::v2::{MessageDirection, ProtocolAmount};
use ccip_common::{pool_ccvs_with_default_fallback, INCLUDE_DEFAULT_CCVS};

use crate::error::CcipOfframpError;
use crate::instructions::get_ccvs_for_msg::GetCcvsForMsgResponse;

/// Validates that `default_ccvs` and `lane_mandated_ccvs` form a valid CCV configuration.
///
/// Enforces:
/// - INV-CFG-1 / INV-CFG-5: at least one CCV across both sets combined
/// - INV-CFG-2 / INV-CFG-6: no Pubkey::default() (zero address) in either set
/// - INV-CFG-3 / INV-CFG-7: no duplicates within each set or across both sets
pub fn validate_default_and_mandated_ccvs(
    default_ccvs: &[Pubkey],
    lane_mandated_ccvs: &[Pubkey],
) -> Result<()> {
    require_neq!(
        default_ccvs.len() + lane_mandated_ccvs.len(),
        0,
        CcipOfframpError::MustSpecifyDefaultOrMandatedCCVs,
    );

    assert_no_duplicates(default_ccvs.iter().chain(lane_mandated_ccvs.iter()))?;

    for ccv in default_ccvs.iter().chain(lane_mandated_ccvs.iter()) {
        require_keys_neq!(
            *ccv,
            Pubkey::default(),
            CcipOfframpError::ZeroPubkeyNotAllowedInCCVConfig,
        );
    }

    Ok(())
}

/// Validates that a list of CCVs contains no duplicates.
pub fn assert_no_duplicates<'a, I>(ccvs: I) -> Result<()>
where
    I: IntoIterator<Item = &'a Pubkey>,
{
    let mut seen = Vec::new();

    for ccv in ccvs {
        require!(
            !seen.contains(ccv),
            CcipOfframpError::DuplicateCCVNotAllowed
        );
        seen.push(*ccv);
    }

    Ok(())
}

/// The receiver's resolved CCV contribution: `(optional_threshold, optional_ccvs, required_ccvs)`.
/// This is what a receiver contributes to a message's required-CCV set, already validated and with
/// the "empty ⇒ lane defaults" fallback applied (`required_ccvs` holds the `INCLUDE_DEFAULT_CCVS`
/// marker in that case). `get_ccvs_for_message` consumes this as plain data so the combination
/// logic stays pure and unit-testable; the actual receiver CPI happens in `resolve_receiver_ccvs`.
pub type ReceiverRequiredCcvs = (u8, Vec<Pubkey>, Vec<Pubkey>);

/// Validates a raw `GetReceiverCcvsOut` and applies the EVM-parity fallback, producing a
/// [`ReceiverRequiredCcvs`]. Pure (no CPI) so it is unit-testable. Mirrors the tail of EVM
/// `OffRamp._getCCVsFromReceiver`:
/// - no duplicates within required or optional,
/// - `optional_threshold <= optional_ccvs.len()`,
/// - the message's requested finality must be allowed by the receiver's `allowed_finality_config`,
/// - if the receiver specified nothing (empty required + zero threshold), fall back to the lane
///   defaults via the `INCLUDE_DEFAULT_CCVS` marker.
pub fn finalize_receiver_ccvs(
    out: GetReceiverCcvsOut,
    requested_finality: FinalityConfig,
) -> Result<ReceiverRequiredCcvs> {
    assert_no_duplicates(out.required_ccvs.iter())?;
    assert_no_duplicates(out.optional_ccvs.iter())?;
    // `INCLUDE_DEFAULT_CCVS` is the "use lane defaults" marker and is only meaningful in
    // `required_ccvs` (see `GetReceiverCcvsOut` docs). It is illegal in `optional_ccvs`: it can
    // never match a real CCV in the quorum scan (zero CCVs are rejected in lane config), so it
    // would silently consume an optional slot. Reject it here so a misbehaving receiver's CPI
    // response can't leak the marker into the optional set. Mirrors the example v2 receiver's own
    // `set_ccv_config` validation.
    for ccv in out.optional_ccvs.iter() {
        require_keys_neq!(
            *ccv,
            INCLUDE_DEFAULT_CCVS,
            CcipOfframpError::ZeroPubkeyNotAllowedInCCVConfig
        );
    }
    // Compare as usize (widening) rather than casting len to u8 (which would wrap for >255 CCVs).
    require_gte!(
        out.optional_ccvs.len(),
        out.optional_threshold as usize,
        CcipOfframpError::InvalidOptionalThreshold
    );
    require!(
        out.allowed_finality_config
            .ensure_requested_finality_allowed(requested_finality),
        CcipOfframpError::RequestedFinalityNotAllowed
    );

    let required_ccvs = if out.required_ccvs.is_empty() && out.optional_threshold == 0 {
        // Receiver specified nothing → pull in the lane defaults (EVM `new address[](1)` marker).
        vec![INCLUDE_DEFAULT_CCVS]
    } else {
        out.required_ccvs
    };

    Ok((out.optional_threshold, out.optional_ccvs, required_ccvs))
}

/// Queries a v2 receiver for its CCV/finality requirements via CPI and finalizes the response.
/// `receiver_ccv_accounts` must be `[receiver_program, source_chain_ccv_config]` (the OffRamp's
/// dedicated `receiver_ccv` account segment); `expected_receiver` binds the CPI target to the
/// message's authoritative receiver (see `get_receiver_ccvs_cpi`).
pub fn resolve_receiver_ccvs<'info>(
    receiver_ccv_accounts: &[AccountInfo<'info>],
    expected_receiver: Pubkey,
    source_chain_selector: u64,
    sender: Vec<u8>,
    requested_finality: FinalityConfig,
) -> Result<ReceiverRequiredCcvs> {
    let out = get_receiver_ccvs_cpi(
        receiver_ccv_accounts,
        expected_receiver,
        source_chain_selector,
        sender,
    )?;
    finalize_receiver_ccvs(out, requested_finality)
}

/// Decides a message's receiver CCV contribution, shared by the authoritative `execute_v2` path and
/// the non-authoritative `get_ccvs_for_msg` view so they can never diverge. Returns `None` when the
/// receiver is not consulted (token-only transfers); otherwise:
/// - **v1 receiver** → the `INCLUDE_DEFAULT_CCVS` "use lane defaults" marker (no CCV interface).
/// - **v2 receiver** → the receiver is queried via CPI (`receiver_ccv_accounts` +
///   `expected_receiver`) and its response finalized (dedup/threshold/finality/fallback).
#[allow(clippy::too_many_arguments)]
pub fn resolve_receiver_required_ccvs<'info>(
    is_offramp_arbitrary_message: bool,
    receiver_is_v2: bool,
    receiver_ccv_accounts: &[AccountInfo<'info>],
    expected_receiver: Pubkey,
    source_chain_selector: u64,
    sender: Vec<u8>,
    requested_finality: FinalityConfig,
) -> Result<Option<ReceiverRequiredCcvs>> {
    if !is_offramp_arbitrary_message {
        return Ok(None);
    }
    if receiver_is_v2 {
        Ok(Some(resolve_receiver_ccvs(
            receiver_ccv_accounts,
            expected_receiver,
            source_chain_selector,
            sender,
            requested_finality,
        )?))
    } else {
        // INV-FIN-DST-2 / INV-FIN-OPTIN-2: a receiver without an explicit finality configuration is
        // treated as allowing only `WAIT_FOR_FINALITY`, so every FTF request is rejected. A v1
        // receiver has no CCV/finality interface and therefore no way to opt in.
        require!(
            WAIT_FOR_FINALITY.ensure_requested_finality_allowed(requested_finality),
            CcipOfframpError::RequestedFinalityNotAllowed
        );
        // v1 receiver: no CCV interface — pull in the lane defaults via the placeholder marker.
        Ok(Some((0, Vec::new(), vec![INCLUDE_DEFAULT_CCVS])))
    }
}

pub struct TokenPoolCcvCpiIn<'info> {
    /// Pool accounts in the order [`get_pool_required_ccvs_cpi`] expects:
    /// `[pool_program, pool_config, pool_chain_config, chain_config_v2]`.
    pub pool_accounts: Vec<AccountInfo<'info>>,
    pub source_denominated_amount: ProtocolAmount,
    pub local_token: Pubkey,
    pub remote_chain_selector: u64,
    pub requested_finality_config: FinalityConfig,
}

/// Where a message's token-pool-required CCVs come from, mirroring EVM `_getCCVsFromPool`, which is
/// invoked for *every* token transfer:
/// - **No token transfer** → the pool contributes nothing.
/// - **Legacy v1 pool** → the pool has no `get_required_ccvs` interface. EVM treats a non-`IPoolV2`
///   pool by returning the `address(0)` placeholder, which expands to the lane defaults; we mirror
///   that with the `INCLUDE_DEFAULT_CCVS` placeholder so a v1 token transfer is still verified by
///   the default CCVs (without this, a v1 token-only transfer on a default-only lane would require
///   zero CCVs and release/mint unverified).
/// - **v2 pool** → query `get_required_ccvs` via CPI (empty response also collapses to the
///   placeholder, matching an empty/uninitialized `ChainConfigV2`).
pub enum PoolCcvSource<'info> {
    NoTokenTransfer,
    LegacyV1,
    V2(TokenPoolCcvCpiIn<'info>),
}

/// Core CCV aggregation algorithm.
pub fn get_ccvs_for_message(
    source_chain_default_ccvs: &[Pubkey],
    source_chain_lane_mandated_ccvs: &[Pubkey],
    pool_ccv_source: PoolCcvSource,
    // The receiver's already-resolved CCV contribution (see [`ReceiverRequiredCcvs`]). `None` when
    // the receiver is not consulted (token-only transfers). Kept as plain data — not a CPI trigger —
    // so this aggregation stays pure and unit-testable; callers do the CPI via `resolve_receiver_ccvs`.
    receiver_required_ccvs: Option<ReceiverRequiredCcvs>,
) -> Result<GetCcvsForMsgResponse> {
    // EVM parity (`_getCCVsForMessage`): a token-only transfer that carries no token is a no-op, and
    // falls back to the lane defaults via the placeholder. This is the only fallback keyed on the
    // message shape; the others come from the pool or receiver contribution. In particular, an empty
    // required set is not a trigger on its own: a v2 receiver may legitimately require no CCVs and
    // rely on a positive optional threshold, which EVM honours without adding the defaults.
    let is_noop = receiver_required_ccvs.is_none()
        && matches!(pool_ccv_source, PoolCcvSource::NoTokenTransfer);
    let (optional_threshold, optional_ccvs, required_receiver_ccvs) = match receiver_required_ccvs {
        Some(receiver_ccvs) => receiver_ccvs,
        None if is_noop => (0, Vec::new(), vec![INCLUDE_DEFAULT_CCVS]),
        None => (0, Vec::new(), Vec::new()),
    };

    let pool_required_ccvs = match pool_ccv_source {
        PoolCcvSource::NoTokenTransfer => vec![],
        // Legacy v1 pool: no `get_required_ccvs` interface. EVM parity — a non-`IPoolV2` pool yields
        // the `address(0)` placeholder, so we emit `INCLUDE_DEFAULT_CCVS` here to pull in the lane
        // defaults (otherwise a v1 token-only transfer on a default-only lane would require no CCVs).
        PoolCcvSource::LegacyV1 => vec![INCLUDE_DEFAULT_CCVS],
        PoolCcvSource::V2(inputs) => {
            // The pool accounts have already been bound to the message's token (see `execute_v2`).
            // Because the pool is canonical, a CPI *error* here is a genuine fault and is propagated —
            // matching EVM, where a revert from `IPoolV2.getRequiredCCVs` propagates rather than being
            // swallowed.
            let ccvs = get_pool_required_ccvs_cpi(
                &inputs.pool_accounts,
                inputs.source_denominated_amount,
                inputs.local_token,
                inputs.remote_chain_selector,
                inputs.requested_finality_config,
                MessageDirection::Inbound,
            )?;
            // EVM parity (INV-DST-8/9/10): a pool that specifies no CCVs — an empty *response*, or a
            // pool with no/uninitialized `ChainConfigV2` (the pool returns an empty list) — falls
            // back to the lane defaults via the `INCLUDE_DEFAULT_CCVS` placeholder.
            pool_ccvs_with_default_fallback(ccvs)
        }
    };

    let max_capacity = source_chain_lane_mandated_ccvs.len()
        + source_chain_default_ccvs.len()
        + pool_required_ccvs.len()
        + required_receiver_ccvs.len();

    let mut required_ccvs = Vec::with_capacity(max_capacity);
    let mut should_include_default = false;

    for ccv in source_chain_lane_mandated_ccvs
        .iter()
        .chain(pool_required_ccvs.iter())
        .chain(required_receiver_ccvs.iter())
    {
        if *ccv == INCLUDE_DEFAULT_CCVS {
            should_include_default = true;
            continue;
        }

        if !required_ccvs.contains(ccv) {
            required_ccvs.push(*ccv);
        }
    }

    if should_include_default {
        for ccv in source_chain_default_ccvs {
            if !required_ccvs.contains(ccv) {
                required_ccvs.push(*ccv);
            }
        }
    }

    let (optional_ccvs, optional_threshold) =
        dedup_optional_against_required(optional_ccvs, &required_ccvs, optional_threshold);

    Ok(GetCcvsForMsgResponse {
        required_ccvs,
        optional_ccvs,
        optional_threshold,
    })
}

/// EVM parity (`_getCCVsForMessage`): any optional CCV that is also required is dropped from the
/// optional set, and the threshold is decremented by one *per dropped CCV*, floored at zero.
///
/// Note the floor: subtracting the count of *dropped* CCVs (not the count of remaining ones), with
/// `saturating_sub`, is what keeps the threshold sane. Subtracting the remaining count would
/// underflow whenever `optional_ccvs.len() > optional_threshold` (the normal case).
fn dedup_optional_against_required(
    mut optional_ccvs: Vec<Pubkey>,
    required_ccvs: &[Pubkey],
    optional_threshold: u8,
) -> (Vec<Pubkey>, u8) {
    let optional_count_before = optional_ccvs.len();
    optional_ccvs.retain(|ccv| !required_ccvs.contains(ccv));
    // Saturating rather than `as u8`: a receiver may return more than 255 optional CCVs (see the
    // `usize` comparison in `finalize_receiver_ccvs`), and wrapping the promoted count would leave
    // the threshold too high. Clamping to `u8::MAX` drives the `saturating_sub` below to zero, which
    // is the right answer once that many optional CCVs have turned out to be required.
    let promoted_to_required =
        u8::try_from(optional_count_before.saturating_sub(optional_ccvs.len())).unwrap_or(u8::MAX);
    (
        optional_ccvs,
        optional_threshold.saturating_sub(promoted_to_required),
    )
}

#[cfg(test)]
mod tests {
    use ccip_common::messages::MessageV1;

    use super::*;

    fn pk(seed: u8) -> Pubkey {
        let mut bytes = [0u8; 32];
        bytes[0] = seed;
        Pubkey::from(bytes)
    }

    #[test]
    fn test_validate_both_empty_fails() {
        let result = validate_default_and_mandated_ccvs(&[], &[]);
        assert_eq!(
            result.unwrap_err(),
            CcipOfframpError::MustSpecifyDefaultOrMandatedCCVs.into()
        );
    }

    #[test]
    fn test_validate_defaults_only_ok() {
        let defaults = vec![pk(1), pk(2)];
        validate_default_and_mandated_ccvs(&defaults, &[]).unwrap();
    }

    #[test]
    fn test_validate_mandated_only_ok() {
        let mandated = vec![pk(3)];
        validate_default_and_mandated_ccvs(&[], &mandated).unwrap();
    }

    #[test]
    fn test_validate_zero_in_defaults_fails() {
        let defaults = vec![Pubkey::default()];
        let result = validate_default_and_mandated_ccvs(&defaults, &[]);
        assert_eq!(
            result.unwrap_err(),
            CcipOfframpError::ZeroPubkeyNotAllowedInCCVConfig.into()
        );
    }

    #[test]
    fn test_validate_duplicate_across_sets_fails() {
        let defaults = vec![pk(1)];
        let mandated = vec![pk(1)];
        let result = validate_default_and_mandated_ccvs(&defaults, &mandated);
        assert_eq!(
            result.unwrap_err(),
            CcipOfframpError::DuplicateCCVNotAllowed.into()
        );
    }

    // A consulted (non-token-only) receiver that specifies no CCVs yields the
    // `INCLUDE_DEFAULT_CCVS` placeholder, so the lane defaults are appended after the
    // lane-mandated CCVs. This mirrors the `finalize_receiver_ccvs` fallback for an empty
    // receiver response.
    fn consult_receiver(_msg: &MessageV1) -> Option<ReceiverRequiredCcvs> {
        Some((0, Vec::new(), vec![INCLUDE_DEFAULT_CCVS]))
    }

    #[test]
    fn test_get_ccvs_receiver_consulted_appends_defaults() {
        let defaults = vec![pk(1), pk(2)];
        let mandated = vec![pk(9)];
        let msg = MessageV1::default();
        let resp = get_ccvs_for_message(
            &defaults,
            &mandated,
            PoolCcvSource::NoTokenTransfer,
            consult_receiver(&msg),
        )
        .expect("Failed get_ccvs_for_message");
        // lane-mandated first, then defaults appended via the placeholder expansion.
        assert_eq!(resp.required_ccvs, vec![pk(9), pk(1), pk(2)]);
        assert!(resp.optional_ccvs.is_empty());
        assert_eq!(resp.optional_threshold, 0);
    }

    #[test]
    fn test_get_ccvs_defaults_only_when_receiver_consulted() {
        let defaults = vec![pk(1), pk(2)];
        let mandated: Vec<Pubkey> = vec![];
        let msg = MessageV1::default();
        let resp = get_ccvs_for_message(
            &defaults,
            &mandated,
            PoolCcvSource::NoTokenTransfer,
            consult_receiver(&msg),
        )
        .expect("Failed get_ccvs_for_message");
        // No mandated CCVs, but defaults are enforced via the placeholder.
        assert_eq!(resp.required_ccvs, vec![pk(1), pk(2)]);
        assert!(resp.optional_ccvs.is_empty());
        assert_eq!(resp.optional_threshold, 0);
    }

    #[test]
    fn test_get_ccvs_dedups_default_already_mandated() {
        // A CCV that is both lane-mandated and a default appears only once.
        let defaults = vec![pk(1), pk(2)];
        let mandated = vec![pk(1)];
        let msg = MessageV1::default();
        let resp = get_ccvs_for_message(
            &defaults,
            &mandated,
            PoolCcvSource::NoTokenTransfer,
            consult_receiver(&msg),
        )
        .expect("Failed get_ccvs_for_message");
        assert_eq!(resp.required_ccvs, vec![pk(1), pk(2)]);
    }

    #[test]
    fn test_dedup_optional_no_overlap_keeps_threshold() {
        // No optional CCV is required, so nothing is promoted and the threshold is unchanged.
        // (Regression: the previous formula `threshold - optional_ccvs.len()` underflowed here.)
        let optional = vec![pk(3), pk(4), pk(5)];
        let required = vec![pk(1), pk(2)];
        let (out, threshold) = dedup_optional_against_required(optional.clone(), &required, 2);
        assert_eq!(out, optional);
        assert_eq!(threshold, 2);
    }

    #[test]
    fn test_dedup_optional_promotes_and_decrements_threshold() {
        // pk(3) is both optional and required: dropped from optional, threshold decremented by 1.
        let optional = vec![pk(3), pk(4), pk(5)];
        let required = vec![pk(1), pk(3)];
        let (out, threshold) = dedup_optional_against_required(optional, &required, 2);
        assert_eq!(out, vec![pk(4), pk(5)]);
        assert_eq!(threshold, 1);
    }

    #[test]
    fn test_dedup_optional_threshold_floored_at_zero() {
        // Two optionals promoted to required with a starting threshold of 1: floored at 0, no underflow.
        let optional = vec![pk(1), pk(2), pk(5)];
        let required = vec![pk(1), pk(2)];
        let (out, threshold) = dedup_optional_against_required(optional, &required, 1);
        assert_eq!(out, vec![pk(5)]);
        assert_eq!(threshold, 0);
    }

    #[test]
    fn test_get_ccvs_noop_with_lane_mandated_ccvs_enforces_defaults() {
        // No token and no receiver consulted is a no-op. EVM requires the lane defaults for it on
        // top of the lane-mandated CCVs, regardless of whether any are mandated.
        let defaults = vec![pk(1), pk(2)];
        let mandated = vec![pk(9)];
        let resp = get_ccvs_for_message(&defaults, &mandated, PoolCcvSource::NoTokenTransfer, None)
            .expect("Failed get_ccvs_for_message");
        assert_eq!(resp.required_ccvs, vec![pk(9), pk(1), pk(2)]);
    }

    #[test]
    fn test_get_ccvs_optional_only_receiver_does_not_enforce_defaults() {
        // A v2 receiver requiring no CCVs but a positive optional threshold is a real policy, not
        // an absence of one: EVM satisfies it with any optional CCV and does not add the defaults,
        // even though the merged required set ends up empty.
        let defaults = vec![pk(1)];
        let mandated: Vec<Pubkey> = vec![];
        let receiver = Some((1, vec![pk(3), pk(4)], Vec::new()));
        let resp = get_ccvs_for_message(
            &defaults,
            &mandated,
            PoolCcvSource::NoTokenTransfer,
            receiver,
        )
        .expect("Failed get_ccvs_for_message");
        assert!(resp.required_ccvs.is_empty());
        assert_eq!(resp.optional_ccvs, vec![pk(3), pk(4)]);
        assert_eq!(resp.optional_threshold, 1);
    }

    #[test]
    fn test_get_ccvs_legacy_v1_token_transfer_enforces_defaults() {
        // A legacy v1 token transfer has no pool `get_required_ccvs` interface. EVM enforces the lane
        // defaults for such pools via the `address(0)` placeholder; we mirror that, so the required
        // set is lane-mandated followed by the defaults (appended via placeholder expansion).
        let defaults = vec![pk(1), pk(2)];
        let mandated = vec![pk(9)];
        let resp = get_ccvs_for_message(&defaults, &mandated, PoolCcvSource::LegacyV1, None)
            .expect("Failed get_ccvs_for_message");
        assert_eq!(resp.required_ccvs, vec![pk(9), pk(1), pk(2)]);
    }

    #[test]
    fn test_get_ccvs_legacy_v1_default_only_lane_enforces_defaults() {
        // Even with no lane-mandated CCVs, a v1 token transfer must still require the lane defaults
        let defaults = vec![pk(1)];
        let mandated: Vec<Pubkey> = vec![];
        let resp = get_ccvs_for_message(&defaults, &mandated, PoolCcvSource::LegacyV1, None)
            .expect("Failed get_ccvs_for_message");
        assert_eq!(resp.required_ccvs, vec![pk(1)]);
    }

    #[test]
    fn test_get_ccvs_noop_with_no_lane_ccvs_enforces_defaults() {
        // Even with no lane-mandated CCVs, noop must still require the lane defaults
        let defaults = vec![pk(1)];
        let mandated: Vec<Pubkey> = vec![];
        let resp = get_ccvs_for_message(&defaults, &mandated, PoolCcvSource::NoTokenTransfer, None)
            .expect("Failed get_ccvs_for_message");
        assert_eq!(resp.required_ccvs, vec![pk(1)]);
    }

    // --- finalize_receiver_ccvs: regression coverage for the receiver-query result handling that
    // replaced the NONEVM-4970 receiver-query placeholder. The CPI itself can't run in a unit test, but the
    // validation + fallback that wraps it is pure and covered here. ---

    fn out(required: Vec<Pubkey>, optional: Vec<Pubkey>, threshold: u8) -> GetReceiverCcvsOut {
        GetReceiverCcvsOut {
            required_ccvs: required,
            optional_ccvs: optional,
            optional_threshold: threshold,
            allowed_finality_config: FinalityConfig::default(), // WAIT_FOR_FINALITY
        }
    }

    #[test]
    fn test_finalize_receiver_empty_falls_back_to_defaults_marker() {
        // Empty response with zero threshold → the `INCLUDE_DEFAULT_CCVS` marker (pull in lane defaults).
        let (threshold, optional, required) =
            finalize_receiver_ccvs(out(vec![], vec![], 0), FinalityConfig::default()).unwrap();
        assert_eq!(required, vec![INCLUDE_DEFAULT_CCVS]);
        assert!(optional.is_empty());
        assert_eq!(threshold, 0);
    }

    #[test]
    fn test_finalize_receiver_specified_ccvs_pass_through() {
        let (threshold, optional, required) = finalize_receiver_ccvs(
            out(vec![pk(1)], vec![pk(2), pk(3)], 1),
            FinalityConfig::default(),
        )
        .unwrap();
        assert_eq!(required, vec![pk(1)]);
        assert_eq!(optional, vec![pk(2), pk(3)]);
        assert_eq!(threshold, 1);
    }

    #[test]
    fn test_finalize_receiver_only_optionals_no_defaults_fallback() {
        // A non-zero threshold means the receiver did specify requirements, so no defaults fallback.
        let (threshold, _optional, required) = finalize_receiver_ccvs(
            out(vec![], vec![pk(2), pk(3)], 1),
            FinalityConfig::default(),
        )
        .unwrap();
        assert!(required.is_empty());
        assert_eq!(threshold, 1);
    }

    #[test]
    fn test_finalize_receiver_duplicate_required_fails() {
        let err = finalize_receiver_ccvs(
            out(vec![pk(1), pk(1)], vec![], 0),
            FinalityConfig::default(),
        )
        .unwrap_err();
        assert_eq!(err, CcipOfframpError::DuplicateCCVNotAllowed.into());
    }

    #[test]
    fn test_finalize_receiver_default_in_optional_fails() {
        // `INCLUDE_DEFAULT_CCVS` is the lane-defaults marker and is only valid in `required_ccvs`;
        // it must be rejected in `optional_ccvs`.
        let err = finalize_receiver_ccvs(
            out(vec![], vec![pk(2), INCLUDE_DEFAULT_CCVS], 1),
            FinalityConfig::default(),
        )
        .unwrap_err();
        assert_eq!(
            err,
            CcipOfframpError::ZeroPubkeyNotAllowedInCCVConfig.into()
        );
    }

    #[test]
    fn test_finalize_receiver_threshold_gt_optional_len_fails() {
        let err = finalize_receiver_ccvs(out(vec![], vec![pk(2)], 2), FinalityConfig::default())
            .unwrap_err();
        assert_eq!(err, CcipOfframpError::InvalidOptionalThreshold.into());
    }

    #[test]
    fn test_finalize_receiver_disallowed_finality_fails() {
        // Receiver requires full finality (WAIT_FOR_FINALITY); a message requesting FTF (block_depth)
        // must be rejected.
        let requested = FinalityConfig {
            flags: 0,
            block_depth: 5,
        };
        let err = finalize_receiver_ccvs(out(vec![pk(1)], vec![], 0), requested).unwrap_err();
        assert_eq!(err, CcipOfframpError::RequestedFinalityNotAllowed.into());
    }

    // resolve_receiver_required_ccvs: the pure (non-CPI) branches shared by execute_v2 and the view.
    // The v2 branch does a CPI and is covered by the offramp integration tests instead.

    #[test]
    fn test_resolve_receiver_token_only_is_none() {
        // Not an arbitrary message (token-only): the receiver is not consulted.
        let r = resolve_receiver_required_ccvs(
            false,
            false,
            &[],
            Pubkey::default(),
            1,
            vec![],
            FinalityConfig::default(),
        )
        .unwrap();
        assert!(r.is_none());
    }

    #[test]
    fn test_resolve_receiver_v1_uses_lane_defaults_marker() {
        // Arbitrary message, v1 receiver: no CPI, contributes the `INCLUDE_DEFAULT_CCVS` marker.
        let r = resolve_receiver_required_ccvs(
            true,
            false,
            &[],
            pk(7),
            1,
            vec![1, 2, 3],
            FinalityConfig::default(),
        )
        .unwrap();
        assert_eq!(r, Some((0, vec![], vec![INCLUDE_DEFAULT_CCVS])));
    }

    #[test]
    fn test_resolve_receiver_v1_rejects_ftf() {
        // A v1 receiver has no finality interface, so it is treated as allowing only
        // `WAIT_FOR_FINALITY`: an FTF request (non-zero block depth) must be rejected rather than
        // silently falling through to the lane-defaults marker.
        let err = resolve_receiver_required_ccvs(
            true,
            false,
            &[],
            pk(7),
            1,
            vec![1, 2, 3],
            FinalityConfig {
                flags: 0,
                block_depth: 4,
            },
        )
        .unwrap_err();
        assert_eq!(err, CcipOfframpError::RequestedFinalityNotAllowed.into());
    }
}
