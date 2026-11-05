//! `get_required_ccvs`: mirrors EVM's `LombardTokenPool.getRequiredCCVs`, which hardcodes
//! `[lombardVerifierResolver, address(0)]`. Solana has no resolver indirection (this
//! program's address is upgraded in place), so the equivalent base is simply this program's own
//! address plus the `INCLUDE_DEFAULT_CCVS` sentinel. `ChainConfigV2.ccv_config`'s
//! threshold-escalation is layered on top of that fixed base (see `get_required_ccvs_for`),
//! mirroring `burnmint`/`lockrelease`'s own `get_required_ccvs`.

use anchor_lang::prelude::*;
use base_token_pool::constants::{valid_version, POOL_CHAINCONFIG_SEED, POOL_STATE_SEED};
use base_token_pool::errors::CcipTokenPoolError;
use base_token_pool::helpers::protocol_amount_to_svm_token_amount;
use base_token_pool::state::CCVConfig;
use base_token_pool::v2::outbound_required_ccvs_amount;
pub use ccip_common::ccvs::{GetRequiredCCVsIn, GetRequiredCCVsOut};
use ccip_common::v2::MessageDirection;
use ccip_common::INCLUDE_DEFAULT_CCVS;

use crate::constants::{MAX_POOL_CHAIN_CONFIG_V, MAX_POOL_STATE_V, POOL_CHAINCONFIG_SEED_2_0};
use crate::state::{ChainConfig, ChainConfigV2, State};

#[derive(Accounts)]
#[instruction(get_required_ccvs_in: GetRequiredCCVsIn)]
pub struct GetRequiredCCVsContext<'info> {
    #[account(
        seeds = [POOL_STATE_SEED, get_required_ccvs_in.local_token.as_ref()],
        bump,
        constraint = valid_version(state.version, MAX_POOL_STATE_V) @ CcipTokenPoolError::InvalidVersion,
        constraint = state.config.mint == get_required_ccvs_in.local_token @ CcipTokenPoolError::InvalidToken,
    )]
    pub state: Account<'info, State>,

    #[account(
        seeds = [POOL_CHAINCONFIG_SEED, &get_required_ccvs_in.remote_chain_selector.to_le_bytes(), state.config.mint.as_ref()],
        bump,
        constraint = valid_version(chain_config.version, MAX_POOL_CHAIN_CONFIG_V) @ CcipTokenPoolError::InvalidVersion,
    )]
    pub chain_config: Account<'info, ChainConfig>,

    /// CHECK: may not be initialized - loaded via `ChainConfigV2::try_load`, which returns `None`
    /// rather than erroring when the account doesn't exist yet.
    #[account(
        seeds = [POOL_CHAINCONFIG_SEED_2_0, &get_required_ccvs_in.remote_chain_selector.to_le_bytes(), state.config.mint.as_ref()],
        bump,
    )]
    pub chain_config_v2: UncheckedAccount<'info>,
}

pub fn get_required_ccvs(
    ctx: Context<GetRequiredCCVsContext>,
    get_required_ccvs_in: GetRequiredCCVsIn,
) -> Result<GetRequiredCCVsOut> {
    let Some(chain_config_v2) = ChainConfigV2::try_load(&ctx.accounts.chain_config_v2)? else {
        return Ok(GetRequiredCCVsOut {
            required_ccvs: vec![crate::ID, INCLUDE_DEFAULT_CCVS],
        });
    };

    let amount = match get_required_ccvs_in.direction {
        MessageDirection::Inbound => protocol_amount_to_svm_token_amount(
            get_required_ccvs_in.amount,
            ctx.accounts.chain_config.base.remote.decimals,
            ctx.accounts.state.config.decimals,
        )?,
        // The inbound amount is already post-fee, but the router queries outbound with the gross
        // amount: deduct the pool's BPS fee so the threshold sees only what is sent cross-chain.
        MessageDirection::Outbound => outbound_required_ccvs_amount(
            get_required_ccvs_in.amount,
            Some(&chain_config_v2.token_transfer_fee_config),
            get_required_ccvs_in.requested_finality_config,
        )?,
    };

    Ok(get_required_ccvs_for(
        &chain_config_v2.ccv_config,
        get_required_ccvs_in.direction,
        amount,
    ))
}

/// This program's own address plus `INCLUDE_DEFAULT_CCVS` is always required (see module doc);
/// `ccv_config`'s ordinary (direction-specific) CCVs are always appended on top of that, and its
/// threshold CCVs are appended further once `amount` reaches its threshold.
fn get_required_ccvs_for(
    ccv_config: &CCVConfig,
    direction: MessageDirection,
    amount: u64,
) -> GetRequiredCCVsOut {
    let (ordinary_ccvs, threshold_ccvs) = match direction {
        MessageDirection::Inbound => (&ccv_config.inbound_ccvs, &ccv_config.threshold_inbound_ccvs),
        MessageDirection::Outbound => (
            &ccv_config.outbound_ccvs,
            &ccv_config.threshold_outbound_ccvs,
        ),
    };

    let mut required_ccvs = vec![crate::ID, INCLUDE_DEFAULT_CCVS];
    required_ccvs.extend_from_slice(ordinary_ccvs);
    if ccv_config.threshold_amount > 0 && amount >= ccv_config.threshold_amount {
        required_ccvs.extend_from_slice(threshold_ccvs);
    }

    GetRequiredCCVsOut { required_ccvs }
}

#[cfg(test)]
mod tests {
    use super::*;

    const OUT_ORDINARY: Pubkey = Pubkey::new_from_array([1u8; 32]);
    const OUT_THRESH: Pubkey = Pubkey::new_from_array([2u8; 32]);
    const IN_ORDINARY: Pubkey = Pubkey::new_from_array([3u8; 32]);
    const IN_THRESH: Pubkey = Pubkey::new_from_array([4u8; 32]);

    fn cfg(threshold_amount: u64) -> CCVConfig {
        CCVConfig {
            threshold_amount,
            outbound_ccvs: vec![OUT_ORDINARY],
            threshold_outbound_ccvs: vec![OUT_THRESH],
            inbound_ccvs: vec![IN_ORDINARY],
            threshold_inbound_ccvs: vec![IN_THRESH],
        }
    }

    #[test]
    fn outbound_below_threshold_still_includes_ordinary_ccvs() {
        let out = get_required_ccvs_for(&cfg(100), MessageDirection::Outbound, 99);
        assert_eq!(
            out.required_ccvs,
            vec![crate::ID, INCLUDE_DEFAULT_CCVS, OUT_ORDINARY]
        );
    }

    #[test]
    fn outbound_at_or_above_threshold_adds_threshold_ccvs_on_top() {
        let out = get_required_ccvs_for(&cfg(100), MessageDirection::Outbound, 100);
        assert_eq!(
            out.required_ccvs,
            vec![crate::ID, INCLUDE_DEFAULT_CCVS, OUT_ORDINARY, OUT_THRESH]
        );
    }

    #[test]
    fn inbound_uses_inbound_lists() {
        let out = get_required_ccvs_for(&cfg(100), MessageDirection::Inbound, u64::MAX);
        assert_eq!(
            out.required_ccvs,
            vec![crate::ID, INCLUDE_DEFAULT_CCVS, IN_ORDINARY, IN_THRESH]
        );
    }

    #[test]
    fn zero_threshold_amount_never_applies_threshold_ccvs() {
        let out = get_required_ccvs_for(&cfg(0), MessageDirection::Outbound, u64::MAX);
        assert_eq!(
            out.required_ccvs,
            vec![crate::ID, INCLUDE_DEFAULT_CCVS, OUT_ORDINARY]
        );
    }
}
