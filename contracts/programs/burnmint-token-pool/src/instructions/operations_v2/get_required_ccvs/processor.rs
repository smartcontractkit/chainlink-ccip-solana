use anchor_lang::prelude::*;
use base_token_pool::helpers::protocol_amount_to_svm_token_amount;
use base_token_pool::state::CCVConfig;
use base_token_pool::{constants::*, errors::*, v2::*};
use ccip_common::ccvs::{GetRequiredCCVsIn, GetRequiredCCVsOut};

use crate::state::*;
use crate::{ChainConfig, State};

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
        seeds = [POOL_CHAINCONFIG_SEED, get_required_ccvs_in.remote_chain_selector.to_le_bytes().as_ref(), state.config.mint.as_ref()],
        bump,
    )]
    pub chain_config: Account<'info, ChainConfig>,
    /// CHECK: Chain Config V2 PDA. May or may not be initialized.
    /// When initialized (owner == this program)
    #[account(
        seeds = [POOL_CHAINCONFIG_SEED_2_0, get_required_ccvs_in.remote_chain_selector.to_le_bytes().as_ref(), state.config.mint.as_ref()],
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
            required_ccvs: Vec::new(),
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

fn get_required_ccvs_for(
    ccv_config: &CCVConfig,
    direction: MessageDirection,
    amount: u64,
) -> GetRequiredCCVsOut {
    let (base_ccvs, threshold_ccvs) = match direction {
        MessageDirection::Inbound => (&ccv_config.inbound_ccvs, &ccv_config.threshold_inbound_ccvs),
        MessageDirection::Outbound => (
            &ccv_config.outbound_ccvs,
            &ccv_config.threshold_outbound_ccvs,
        ),
    };

    GetRequiredCCVsOut {
        required_ccvs: if ccv_config.threshold_amount > 0 && amount >= ccv_config.threshold_amount {
            let mut ccvs = base_ccvs.clone();
            ccvs.extend_from_slice(threshold_ccvs);
            ccvs
        } else {
            base_ccvs.clone()
        },
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use base_token_pool::helpers::protocol_amount_to_svm_token_amount;
    use ccip_common::v2::ProtocolAmount;

    const OUT_BASE: Pubkey = Pubkey::new_from_array([1u8; 32]);
    const OUT_THRESH: Pubkey = Pubkey::new_from_array([2u8; 32]);
    const IN_BASE: Pubkey = Pubkey::new_from_array([3u8; 32]);
    const IN_THRESH: Pubkey = Pubkey::new_from_array([4u8; 32]);

    fn cfg(threshold_amount: u64) -> CCVConfig {
        CCVConfig {
            threshold_amount,
            outbound_ccvs: vec![OUT_BASE],
            threshold_outbound_ccvs: vec![OUT_THRESH],
            inbound_ccvs: vec![IN_BASE],
            threshold_inbound_ccvs: vec![IN_THRESH],
        }
    }

    #[test]
    fn outbound_below_threshold_returns_base_only() {
        let out = get_required_ccvs_for(&cfg(100), MessageDirection::Outbound, 99);
        assert_eq!(out.required_ccvs, vec![OUT_BASE]);
    }

    #[test]
    fn outbound_at_or_above_threshold_includes_threshold_ccvs() {
        let out = get_required_ccvs_for(&cfg(100), MessageDirection::Outbound, 100);
        assert_eq!(out.required_ccvs, vec![OUT_BASE, OUT_THRESH]);
    }

    #[test]
    fn inbound_uses_inbound_lists() {
        let out = get_required_ccvs_for(&cfg(100), MessageDirection::Inbound, u64::MAX);
        assert_eq!(out.required_ccvs, vec![IN_BASE, IN_THRESH]);
    }

    #[test]
    fn zero_threshold_amount_never_applies_threshold_ccvs() {
        // Mirrors the set_ccv_config guard: with threshold_amount == 0 the threshold CCVs would never
        // apply regardless of amount, so only the base set is returned.
        let out = get_required_ccvs_for(&cfg(0), MessageDirection::Outbound, u64::MAX);
        assert_eq!(out.required_ccvs, vec![OUT_BASE]);
    }

    #[test]
    fn inbound_uses_local_amount_for_threshold_ccvs() {
        // 1,000 tokens @ 6 remote decimals.
        let amount = ProtocolAmount::from(1_000_000_000u64);

        // Normalize to 9 local decimals.
        let normalized = protocol_amount_to_svm_token_amount(amount, 6, 9).unwrap();

        assert_eq!(normalized, 1_000_000_000_000);

        // Threshold is 500 tokens @ 9 local decimals.
        let out =
            get_required_ccvs_for(&cfg(500_000_000_000), MessageDirection::Inbound, normalized);

        assert_eq!(out.required_ccvs, vec![IN_BASE, IN_THRESH]);
    }
}
