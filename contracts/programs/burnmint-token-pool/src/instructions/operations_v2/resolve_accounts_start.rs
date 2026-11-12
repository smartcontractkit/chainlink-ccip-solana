use anchor_lang::prelude::*;

use base_token_pool::v2::{LockOrBurnInV2, ReleaseOrMintInV2, TokenPoolCpiResolutionInputs};
use base_token_pool::{constants::*, errors::*};
use ccip_common::ccvs::GetRequiredCCVsIn;
use ccip_common::resolution::*;

use crate::instructions::operations_v2::{burn_v2, get_required_ccvs, mint_v2};
use crate::state::*;
use crate::State;

/// The `cpi_resolution_inputs` this pool's `nested_resolve_accounts_start` decodes.
///
/// The router and the OffRamp start every resolution with a [`TokenPoolCpiResolutionInputs`], so the
/// first three variants must stay Borsh-identical to it: same order, same payloads. The exhaustive
/// `From` below stops compiling if that enum gains a variant, and the tests pin the encoding.
///
/// The variants after them are this pool's own later stages. Only this pool produces them, as
/// `next_cpi_resolution_inputs`, and the caller hands them back on the next round without decoding
/// them, so their shape is private to this program.
#[derive(AnchorSerialize, AnchorDeserialize, Clone, Debug)]
pub enum BurnMintCpiResolutionInputs {
    LockOrBurn(Box<LockOrBurnInV2>),
    ReleaseOrMint(Box<ReleaseOrMintInV2>),
    GetRequiredCcv(Box<GetRequiredCCVsIn>),
    // Additional rounds
    /// Stage two of `release_or_mint_tokens_v2`'s resolution. The mint that stage one asked for is
    /// now among the resolver's accounts, so its mint authority can be read.
    ReleaseOrMintWithMint,
}

impl From<TokenPoolCpiResolutionInputs> for BurnMintCpiResolutionInputs {
    fn from(inputs: TokenPoolCpiResolutionInputs) -> Self {
        match inputs {
            TokenPoolCpiResolutionInputs::LockOrBurn(inputs) => Self::LockOrBurn(inputs),
            TokenPoolCpiResolutionInputs::ReleaseOrMint(inputs) => Self::ReleaseOrMint(inputs),
            TokenPoolCpiResolutionInputs::GetRequiredCcv(inputs) => Self::GetRequiredCcv(inputs),
        }
    }
}

#[derive(Accounts)]
pub struct NestedResolveAccountsStart<'info> {
    #[account(
        seeds = [POOL_STATE_SEED, state.config.mint.as_ref()],
        bump,
        constraint = valid_version(state.version, MAX_POOL_STATE_V) @ CcipTokenPoolError::InvalidVersion,
    )]
    pub state: Account<'info, State>,
}

pub fn nested_resolve_accounts_start(
    ctx: Context<NestedResolveAccountsStart>,
    params: NestedResolveAccountsParams,
) -> Result<NestedResolveAccountsResponse> {
    let inputs = BurnMintCpiResolutionInputs::try_from_slice(&params.cpi_resolution_inputs)?;
    match inputs {
        BurnMintCpiResolutionInputs::LockOrBurn(lock_or_burn_in_v2) => {
            burn_v2::resolve_accounts::start(ctx, &lock_or_burn_in_v2)
        }
        BurnMintCpiResolutionInputs::ReleaseOrMint(release_or_mint_in_v2) => {
            mint_v2::resolve_accounts::start(ctx, &release_or_mint_in_v2)
        }
        BurnMintCpiResolutionInputs::GetRequiredCcv(get_required_ccvs_in) => {
            get_required_ccvs::resolve_accounts::start(ctx, &get_required_ccvs_in)
        }
        BurnMintCpiResolutionInputs::ReleaseOrMintWithMint => {
            mint_v2::resolve_accounts::with_mint(ctx)
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use base_token_pool::state::RemoteAddress;
    use ccip_common::messages::{FinalityConfig, SVMTokenReceiverUsage};
    use ccip_common::v2::MessageDirection;

    fn shared_inputs() -> Vec<TokenPoolCpiResolutionInputs> {
        vec![
            TokenPoolCpiResolutionInputs::LockOrBurn(Box::new(LockOrBurnInV2 {
                receiver: vec![1; 20],
                remote_chain_selector: 1,
                original_sender: Pubkey::new_unique(),
                amount: 2,
                local_token: Pubkey::new_unique(),
                requested_finality_config: FinalityConfig::default(),
                token_args: vec![3; 4],
            })),
            TokenPoolCpiResolutionInputs::ReleaseOrMint(Box::new(ReleaseOrMintInV2 {
                original_sender: RemoteAddress {
                    address: vec![4; 20],
                },
                remote_chain_selector: 5,
                receiver: Pubkey::new_unique(),
                amount: 6u64.into(),
                local_token: Pubkey::new_unique(),
                source_pool_address: RemoteAddress {
                    address: vec![7; 32],
                },
                source_pool_data: vec![8; 32],
                requested_finality_config: FinalityConfig::default(),
                receiver_usage: SVMTokenReceiverUsage::DeriveAtaAndCreate,
            })),
            TokenPoolCpiResolutionInputs::GetRequiredCcv(Box::new(GetRequiredCCVsIn {
                remote_chain_selector: 9,
                amount: 10u64.into(),
                local_token: Pubkey::new_unique(),
                direction: MessageDirection::Inbound,
                requested_finality_config: FinalityConfig::default(),
            })),
        ]
    }

    /// What the router and the OffRamp send must decode here as the same variant with the same
    /// payload, byte for byte.
    #[test]
    fn shared_inputs_decode_as_the_same_variants() {
        for shared in shared_inputs() {
            let bytes = serialize_metadata(&shared).unwrap();
            let decoded = BurnMintCpiResolutionInputs::try_from_slice(&bytes).unwrap();
            assert_eq!(serialize_metadata(&decoded).unwrap(), bytes);
            assert_eq!(
                serialize_metadata(&BurnMintCpiResolutionInputs::from(shared)).unwrap(),
                bytes
            );
        }
    }

    /// This pool's own stages must not be mistaken for anything the router or the OffRamp send.
    #[test]
    fn private_stages_are_not_shared_inputs() {
        let bytes =
            serialize_metadata(&BurnMintCpiResolutionInputs::ReleaseOrMintWithMint).unwrap();
        assert!(TokenPoolCpiResolutionInputs::try_from_slice(&bytes).is_err());
        assert!(matches!(
            BurnMintCpiResolutionInputs::try_from_slice(&bytes).unwrap(),
            BurnMintCpiResolutionInputs::ReleaseOrMintWithMint
        ));
    }
}
