use anchor_lang::prelude::*;
use anchor_lang::AnchorDeserialize;
use ccip_common::extra_args::{
    split_extra_args_tag, GenericExtraArgsV3, GENERIC_EXTRA_ARGS_V3_TAG,
};
use ccip_common::messages::SVM2AnyMessage;
use ccip_common::types::CrossChainGas;
use ccip_common::INCLUDE_DEFAULT_CCVS;

use crate::error::CcipRouterError;
use crate::state::DestChainConfigCcipV2;

impl DestChainConfigCcipV2 {
    // For CCIP v2.0 lanes, the logic required to parse V3 extra args lives entirely in the
    // onramp/router. v2.0 lanes only accept V3 extra args: there is no fallback to pre-V3 (V2 /
    // SVM / legacy) args, since senders are explicitly migrated to the new send method.
    //
    // The serialized extra args are prefixed with a 4-byte big-endian version tag (the same wire
    // format the FeeQuoter uses); we dispatch on that tag to decide how to parse the payload.
    //
    // The lane only defaults two things: the CCV list (`default_ccvs`, INV-SRC-5) and the executor
    // (`default_executor`, applied below). Every other V3 field is taken from the sender as-is. In
    // particular the gas limit is never defaulted, matching EVM, where `defaultTxGasLimit` is
    // applied only by `FeeQuoter.resolveLegacyArgs` for the pre-V3 args this function rejects.
    pub fn parse_extra_args_with_defaults(
        &self,
        message: &SVM2AnyMessage,
    ) -> Result<GenericExtraArgsV3> {
        let mut resolved = match split_extra_args_tag(&message.extra_args) {
            // No extra args provided at all: use the lane defaults.
            None if message.extra_args.is_empty() => GenericExtraArgsV3::default(),
            // Non-empty but too short to hold a 4-byte version tag: malformed user input.
            None => return Err(CcipRouterError::InvalidInstructionData.into()),
            // V3 tag but no payload: same as no extra args, use defaults.
            Some((GENERIC_EXTRA_ARGS_V3_TAG, [])) => GenericExtraArgsV3::default(),
            Some((GENERIC_EXTRA_ARGS_V3_TAG, mut data)) => {
                GenericExtraArgsV3::deserialize(&mut data)?
            }
            // Any non-V3 tag is rejected: v2.0 lanes do not support legacy extra args.
            Some(_) => return Err(CcipRouterError::InvalidInstructionData.into()),
        };

        if !resolved.token_receiver.is_empty() {
            require!(
                self.token_receiver_allowed,
                CcipRouterError::TokenReceiverNotAllowed
            );
        }

        // Conservative definition of pure-token-transfer:
        // no data, no gas, and no receiver (when there are separate token receivers)
        let is_pure_token_transfer = !message.token_amounts.is_empty()
            && resolved.gas_limit == CrossChainGas::new(0)
            && message.data.is_empty()
            && (!self.token_receiver_allowed || message.receiver.iter().all(|b| *b == 0));

        // A pure token transfer does not get default sender/receiver
        // CCVs applied.
        let (ccvs, ccv_args) = Self::resolve_user_or_default_ccvs(
            resolved.ccvs,
            resolved.ccv_args,
            &self.default_ccvs,
            is_pure_token_transfer,
        )?;
        resolved.ccvs = ccvs;
        resolved.ccv_args = ccv_args;

        // Fall back to the lane's default executor when the sender didn't request a specific one.
        // A zero executor means "unset" so it resolves to the configured default
        if resolved.executor == Pubkey::default() {
            resolved.executor = self.default_executor;
        }

        // Validate the wire shape of the requested finality. Note: the receiver's finality policy
        // is checked on delivery by the OffRamp, not here. A sender using a non-zero
        // requestedFinalityConfig targeting a receiver that only accepts WAIT_FOR_FINALITY
        // will succeed at send time but fail on execution. Senders should consult the receiver's
        // policy off-chain before choosing a requestedFinalityConfig.
        require!(
            resolved.finality.is_valid_finality(),
            CcipRouterError::InvalidFinality
        );

        Ok(resolved)
    }

    /// Resolves the final CCV list for a message by merging the user-requested CCVs with the
    /// lane defaults.
    ///
    /// Users can request the defaults by either:
    /// - providing an empty CCV list, or
    /// - including the default (zero) `Pubkey` as a placeholder, which is removed from the final
    ///   list.
    ///
    /// The resulting list is the user CCVs first (placeholder removed), followed by any default
    /// CCVs not already present. The CCV args stay parallel to the CCV list; defaults contribute
    /// empty args.
    ///
    /// Default CCVs are never applied for pure token transfers: the only entity at risk is the
    /// token issuer, who already defines their required CCVs (or falls back to defaults), so
    /// omitting sender/receiver defaults here does not increase their risk.
    fn resolve_user_or_default_ccvs(
        user_ccvs: Vec<Pubkey>,
        user_ccv_args: Vec<Vec<u8>>,
        default_ccvs: &[Pubkey],
        is_token_only_transfer: bool,
    ) -> Result<(Vec<Pubkey>, Vec<Vec<u8>>)> {
        // The CCV args are expected to be parallel to the CCV list.
        require_eq!(
            user_ccvs.len(),
            user_ccv_args.len(),
            CcipRouterError::InvalidInstructionData
        );

        // Fast path: no user CCVs provided. A pure token transfer skips defaults entirely;
        // otherwise we fall back to the lane defaults (with empty args).
        if user_ccvs.is_empty() {
            if is_token_only_transfer {
                return Ok((Vec::new(), Vec::new()));
            }
            let default_args = vec![Vec::new(); default_ccvs.len()];
            return Ok((default_ccvs.to_vec(), default_args));
        }

        // The user provided CCVs: ensure there are no duplicates. This also guarantees there is at
        // most one placeholder, so we only ever need to expand the defaults once.
        for (i, ccv) in user_ccvs.iter().enumerate() {
            require!(
                // SAFETY: `i` is a loop index over the slice being scanned, so `i + 1` stays within its length.
                !user_ccvs[i + 1..].contains(ccv),
                CcipRouterError::DuplicateCCVInExtraArgs
            );
        }

        // Without a placeholder, the user list is used verbatim.
        let Some(placeholder_idx) = user_ccvs
            .iter()
            .position(|ccv| *ccv == INCLUDE_DEFAULT_CCVS)
        else {
            return Ok((user_ccvs, user_ccv_args));
        };

        // Replace the placeholder with the default CCVs, skipping any default that the user already
        // requested explicitly.
        let capacity = user_ccvs.len() - 1 + default_ccvs.len();
        let mut ccvs = Vec::with_capacity(capacity);
        let mut ccv_args = Vec::with_capacity(capacity);

        for (j, (ccv, args)) in user_ccvs.into_iter().zip(user_ccv_args).enumerate() {
            if j == placeholder_idx {
                continue;
            }
            ccvs.push(ccv);
            ccv_args.push(args);
        }

        for default_ccv in default_ccvs {
            if ccvs.contains(default_ccv) {
                continue;
            }
            ccvs.push(*default_ccv);
            ccv_args.push(Vec::new());
        }

        Ok((ccvs, ccv_args))
    }
}
