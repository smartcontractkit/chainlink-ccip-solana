use std::collections::HashSet;

use anchor_lang::prelude::*;
use anchor_lang::solana_program::program::invoke_signed;
use anchor_spl::associated_token::get_associated_token_address_with_program_id;
use anchor_spl::token_2022::spl_token_2022::{self, instruction::transfer_checked};
use ccip_common::ccvs::GetRequiredCCVsIn;
use ccip_common::messages::{FinalityConfig, SVMTokenReceiverUsage};
use ccip_common::resolution::{find, CcipAccountMeta, ToMeta};
use ccip_common::seed;
use ccip_common::types::{CrossChainGas, ToTxData, UsdCents};

use crate::constants::{
    GET_POOL_FEE, LOCK_OR_BURN_V2, POOL_CHAINCONFIG_SEED, POOL_CHAINCONFIG_SEED_2_0,
    POOL_SIGNER_SEED, RELEASE_OR_MINT_V2,
};
use crate::errors::*;
use crate::helpers::local_protocol_amount_to_svm_amount;
use crate::rate_limiter::RateLimitTokenBucket;
use crate::state::{CCVConfig, RemoteAddress, TokenTransferFeeConfig, BPS_DIVIDER};

// Re-export shared v2 types from ccip_common so downstream crates can keep using
// `base_token_pool::v2::*`.
pub use ccip_common::v2::{CrossChainAmount, MessageDirection, ProtocolAmount, Receipt};

// V2 operation types — diverged from V1 with finality config and token args support.

#[derive(Clone, AnchorSerialize, AnchorDeserialize, Debug)]
pub struct LockOrBurnInV2 {
    pub receiver: Vec<u8>,
    pub remote_chain_selector: u64,
    pub original_sender: Pubkey,
    pub amount: u64,
    pub local_token: Pubkey,
    pub requested_finality_config: FinalityConfig,
    pub token_args: Vec<u8>,
}

impl ToTxData for LockOrBurnInV2 {
    fn to_tx_data(&self) -> Vec<u8> {
        let mut data = Vec::with_capacity(8);
        data.extend_from_slice(&LOCK_OR_BURN_V2);
        borsh::to_writer(&mut data, &self).unwrap();
        data
    }
}

/// Inputs to the first stage of nested account resolution for token pool instructions
/// invoked via CPI. All pools implementing the V2 resolution interface accept this type
/// (Borsh-encoded) as the `cpi_resolution_inputs` of `nested_resolve_accounts_start`, so
/// callers can encode it uniformly regardless of the pool program.
#[derive(AnchorSerialize, AnchorDeserialize, Clone, Debug)]
pub enum TokenPoolCpiResolutionInputs {
    /// Every field of the instruction's params is known before the onramp/offramp calls the pool, so the
    /// resolver sees what `lock_or_burn_tokens_v2` and `release_or_mint_in_v2` will.
    LockOrBurn(Box<LockOrBurnInV2>),
    ReleaseOrMint(Box<ReleaseOrMintInV2>),
    GetRequiredCcv(Box<GetRequiredCCVsIn>),
    // If any of the resolutions above required multiple steps, they could be tracked as
    // separate entries in this enum or, alternatively, marked by data fields inside the enum variant.
    //
    // For example, GetFee(Stage::One), GetFee(Stage::Two) and so on.
}

#[derive(Clone, AnchorSerialize, AnchorDeserialize)]
pub struct LockOrBurnOutV2 {
    pub dest_token_address: RemoteAddress,
    pub dest_pool_data: Vec<u8>,
    pub dest_token_amount: u64,
}

impl LockOrBurnOutV2 {
    pub const fn default_ret_bytes() -> u32 {
        32
    }
}

#[derive(Clone, AnchorSerialize, AnchorDeserialize, Debug)]
pub struct ReleaseOrMintInV2 {
    pub original_sender: RemoteAddress,
    pub remote_chain_selector: u64,
    pub receiver: Pubkey,
    pub amount: ProtocolAmount,
    pub local_token: Pubkey,
    pub source_pool_address: RemoteAddress,
    pub source_pool_data: Vec<u8>,
    pub requested_finality_config: FinalityConfig,
    pub receiver_usage: SVMTokenReceiverUsage,
}

impl ToTxData for ReleaseOrMintInV2 {
    fn to_tx_data(&self) -> Vec<u8> {
        let mut data = Vec::with_capacity(8);
        data.extend_from_slice(&RELEASE_OR_MINT_V2);
        borsh::to_writer(&mut data, &self).unwrap();
        data
    }
}

#[derive(Clone, AnchorSerialize, AnchorDeserialize)]
pub struct ReleaseOrMintOutV2 {
    pub destination_amount: u64,
}

impl ReleaseOrMintOutV2 {
    pub const fn ret_bytes() -> usize {
        // destination_amount
        8
    }
}

// Fee types
#[derive(Clone, AnchorSerialize, AnchorDeserialize, Debug)]
pub struct GetFeeParams {
    pub remote_chain_selector: u64,
    pub amount: u64,
    pub local_token: Pubkey,
    pub fee_token: Pubkey,
    pub requested_finality_config: FinalityConfig,
    pub token_args: Vec<u8>,
}

impl ToTxData for GetFeeParams {
    fn to_tx_data(&self) -> Vec<u8> {
        let mut data = Vec::with_capacity(8);
        data.extend_from_slice(&GET_POOL_FEE);
        borsh::to_writer(&mut data, &self).unwrap();
        data
    }
}

#[derive(Clone, AnchorSerialize, AnchorDeserialize, Debug, Default)]
pub struct GetFeeResult {
    pub dest_gas_limit: CrossChainGas,
    pub dest_bytes_overhead: u32,
    pub fee: UsdCents,
    pub token_fee_bps: u16,
    pub is_enabled: bool,
}

/// The BPS fee a pool charges on a transfer, in units of the transferred token. The pool keeps
/// this fee for itself and only locks or burns the remainder, mirroring EVM's
/// `TokenPool._getFee` (`amount * bps / BPS_DIVIDER`).
///
/// `fee_config` is `None` when no fee config PDA is initialized for the lane. A disabled config
/// charges nothing: `get_pool_fee` quotes no BPS fee in that case (the router falls back to the
/// fee quoter), and the amount charged must always match the amount quoted.
pub fn token_transfer_bps_fee(
    fee_config: Option<&TokenTransferFeeConfig>,
    amount: u64,
    requested_finality_config: FinalityConfig,
) -> u64 {
    let Some(fee_config) = fee_config.filter(|config| config.is_enabled) else {
        return 0;
    };

    let bps = if requested_finality_config.is_wait_for_finality() {
        fee_config.finality_bps_fee
    } else {
        fee_config.fast_finality_bps_fee
    };

    // Widened to u128: `amount * bps` may exceed u64. The result fits back into u64 as
    // `bps <= MAX_TOKEN_TRANSFER_FEE_BPS == BPS_DIVIDER`, i.e. the fee never exceeds `amount`.
    (u128::from(amount) * u128::from(bps) / u128::from(BPS_DIVIDER)) as u64
}

/// The local amount an outbound `get_required_ccvs` query compares against the CCV threshold.
///
/// The router queries with the gross amount the sender transfers, but the pool's in-token BPS fee
/// is not transferred value: only the post-fee remainder is locked or burned. This mirrors EVM's
/// `TokenPool.getRequiredCCVs`, which deducts the source fee before consulting its CCV hook. The
/// fee comes from [`token_transfer_bps_fee`], so the amount compared is exactly the amount
/// `lock_or_burn_tokens_v2` sends cross-chain.
pub fn outbound_required_ccvs_amount(
    amount: ProtocolAmount,
    fee_config: Option<&TokenTransferFeeConfig>,
    requested_finality_config: FinalityConfig,
) -> Result<u64> {
    let amount = local_protocol_amount_to_svm_amount(amount)?;
    // The fee never exceeds `amount`; saturating only as a defensive measure.
    Ok(amount.saturating_sub(token_transfer_bps_fee(
        fee_config,
        amount,
        requested_finality_config,
    )))
}

/// Address of the ATA, for the pool's own `mint`, of that pool's `[FEE_VAULT, mint]` PDA — the
/// account [`collect_bps_fee`] moves in-token BPS fees into.
///
/// Both the vault PDA and its ATA are pure functions of the mint, so callers pass neither in:
/// `lock_or_burn_tokens_v2` derives the address to check the one account it does take, and its
/// resolver derives the same address to resolve it. Every account in that instruction costs 32
/// bytes of a `ccip_send` transaction that is already close to the size limit.
pub fn fee_vault_token_account_address(
    pool_program: &Pubkey,
    mint: &Pubkey,
    token_program: &Pubkey,
) -> Pubkey {
    let (fee_vault, _) =
        Pubkey::find_program_address(&[seed::FEE_VAULT, mint.as_ref()], pool_program);
    get_associated_token_address_with_program_id(&fee_vault, mint, token_program)
}

/// Accounts of a pool's `get_pool_fee` and `get_required_ccvs` queries, in the order both take
/// them: the pool state, and the lane's chain config and v2 chain config. The v2 chain config is
/// resolved even when it isn't initialized, as both queries take its address regardless.
pub fn pool_query_accounts(
    pool_program: Pubkey,
    state: Pubkey,
    mint: &Pubkey,
    remote_chain_selector: u64,
) -> Vec<CcipAccountMeta> {
    let remote_chain_selector = remote_chain_selector.to_le_bytes();
    vec![
        state.readonly(),
        find(
            &[
                POOL_CHAINCONFIG_SEED,
                remote_chain_selector.as_ref(),
                mint.as_ref(),
            ],
            pool_program,
        )
        .readonly(),
        find(
            &[
                POOL_CHAINCONFIG_SEED_2_0,
                remote_chain_selector.as_ref(),
                mint.as_ref(),
            ],
            pool_program,
        )
        .readonly(),
    ]
}

/// Moves a lane's in-token BPS fee out of the pool's token account and into the ATA of the pool's
/// `[FEE_VAULT, mint]` PDA, the same vault the router pays token-pool receipt fees into. Once
/// there, the fee can only leave through `withdraw_fee_tokens`: it is out of reach of the burn
/// path, and for lock-release pools out of the locked liquidity.
///
/// A zero `fee_amount` is a no-op, so lanes that charge no BPS fee pay no extra CUs and don't
/// need the vault ATA to exist. When a fee is charged the ATA must exist; `set_chain_config_v2`
/// creates it when the lane's fee config is written, so this only fails for a pool whose vault
/// ATA was closed after the fact.
#[allow(clippy::too_many_arguments)]
pub fn collect_bps_fee<'a>(
    token_program: Pubkey,
    pool_token_account: AccountInfo<'a>,
    fee_vault_token_account: AccountInfo<'a>,
    mint: AccountInfo<'a>,
    pool_signer: AccountInfo<'a>,
    pool_signer_bump: u8,
    fee_amount: u64,
    decimals: u8,
) -> Result<()> {
    if fee_amount == 0 {
        return Ok(());
    }

    require!(
        !fee_vault_token_account.data_is_empty(),
        CcipTokenPoolError::MissingFeeVaultTokenAccount
    );

    let mut ix = transfer_checked(
        &spl_token_2022::ID, // use spl-token-2022 to compile instruction - change program later
        &pool_token_account.key(),
        &mint.key(),
        &fee_vault_token_account.key(),
        &pool_signer.key(),
        &[],
        fee_amount,
        decimals,
    )?;
    ix.program_id = token_program; // set to user specified program

    let seeds = &[
        POOL_SIGNER_SEED,
        &mint.key().to_bytes(),
        &[pool_signer_bump],
    ];
    invoke_signed(
        &ix,
        &[
            pool_token_account,
            mint,
            fee_vault_token_account,
            pool_signer,
        ],
        &[&seeds[..]],
    )
    .map_err(Into::into)
}

// validate_lock_or_burn_v2 checks for correctness on inputs (V2 variant)
// - token & pool is correct for chain
// - rate limiting (on the post-fee amount, as that is what crosses chains)
#[allow(clippy::too_many_arguments)]
pub fn validate_lock_or_burn_v2(
    lock_or_burn_in: &LockOrBurnInV2,
    fee_amount: u64,
    config_mint: Pubkey,
    allowed_finality_config: FinalityConfig,
    outbound_rate_limit: &mut RateLimitTokenBucket,
    ftf_outbound_rate_limit: Option<&mut RateLimitTokenBucket>,
    allow_list_enabled: bool,
    allow_list: &[Pubkey],
) -> Result<()> {
    require_keys_eq!(
        config_mint,
        lock_or_burn_in.local_token,
        CcipTokenPoolError::InvalidToken
    );
    require!(
        !allow_list_enabled || allow_list.contains(&lock_or_burn_in.original_sender),
        CcipTokenPoolError::InvalidSender
    );

    // EVM parity (`TokenPool._validateLockOrBurn`): the requested finality must be permitted by the
    // pool's configured allowed finality. Full finality (WAIT_FOR_FINALITY) is always allowed — that
    // case is handled inside `ensure_requested_finality_allowed`, so this call is unconditional. An
    // unconfigured pool (`allowed_finality_config == WAIT_FOR_FINALITY`) therefore permits only
    // full-finality transfers and rejects every fast-transfer request (safe-by-default), matching
    // EVM's zero-initialized `s_allowedFinalityConfig`.
    require!(
        allowed_finality_config
            .ensure_requested_finality_allowed(lock_or_burn_in.requested_finality_config),
        CcipTokenPoolError::InvalidRequestedFinality
    );

    // EVM parity (`TokenPool._validateLockOrBurn`): rate limits apply to the amount actually
    // locked or burned, i.e. after the pool's in-token BPS fee deduction.
    let post_fee_amount = lock_or_burn_in
        .amount
        .checked_sub(fee_amount)
        .ok_or(CcipTokenPoolError::InvalidInputs)?;

    consume_rate_limit(
        outbound_rate_limit,
        ftf_outbound_rate_limit,
        &lock_or_burn_in.requested_finality_config,
        post_fee_amount,
    )
}

// validate_release_or_mint_v2 checks for correctness on inputs (V2 variant)
// - token & pool is correct for chain
// - rate limiting
#[allow(clippy::too_many_arguments)]
pub fn validate_release_or_mint_v2(
    release_or_mint_in: &ReleaseOrMintInV2,
    parsed_amount: u64,
    config_mint: Pubkey,
    pool_addresses: &[RemoteAddress],
    inbound_rate_limit: &mut RateLimitTokenBucket,
    ftf_inbound_rate_limit: Option<&mut RateLimitTokenBucket>,
) -> Result<()> {
    // NOTE: inbound (release/mint) does NOT validate a finality *policy* here — and intentionally so,
    // to match EVM. EVM's base `TokenPool._validateReleaseOrMint`
    require_keys_eq!(
        config_mint,
        release_or_mint_in.local_token,
        CcipTokenPoolError::InvalidToken
    );
    require!(
        !pool_addresses.is_empty()
            && !release_or_mint_in.source_pool_address.is_empty()
            && pool_addresses.contains(&release_or_mint_in.source_pool_address),
        CcipTokenPoolError::InvalidSourcePoolAddress
    );

    consume_rate_limit(
        inbound_rate_limit,
        ftf_inbound_rate_limit,
        &release_or_mint_in.requested_finality_config,
        parsed_amount,
    )
}

/// Selects the appropriate rate limit bucket and consumes tokens.
/// If the message requests faster-than-finality and an override bucket
/// is provided and enabled, consume from the override bucket.
/// Otherwise, fall back to the default bucket.
fn consume_rate_limit(
    default_bucket: &mut RateLimitTokenBucket,
    ftf_bucket: Option<&mut RateLimitTokenBucket>,
    finality_config: &FinalityConfig,
    amount: u64,
) -> Result<()> {
    if !finality_config.is_wait_for_finality() {
        if let Some(bucket) = ftf_bucket {
            if bucket.is_enabled() {
                return bucket.consume::<Clock>(amount);
            }
        }
    }
    default_bucket.consume::<Clock>(amount)
}

/// Validates that a list of CCVs contains no duplicates.
pub fn require_no_nulls_nor_duplicates<'a, I>(ccvs: I) -> Result<()>
where
    I: IntoIterator<Item = &'a Pubkey>,
{
    let mut seen = HashSet::new();

    for ccv in ccvs {
        require!(
            seen.insert(*ccv),
            CcipTokenPoolError::DuplicateCCVNotAllowed,
        );
    }

    require!(
        !seen.contains(&Pubkey::default()),
        CcipTokenPoolError::CcvZeroAddressNotAllowed
    );

    Ok(())
}

#[derive(AnchorSerialize, AnchorDeserialize, Clone)]
pub struct SetChainConfigV2Params {
    pub mint: Pubkey,
    pub remote_chain_selector: u64,
    /// Allowed finality policy for outbound (lock_or_burn) transfers on this lane.
    /// `WAIT_FOR_FINALITY` (zero) permits only full-finality transfers.
    pub allowed_finality_config: FinalityConfig,
    pub token_transfer_fee_config: TokenTransferFeeConfig,
    pub ccv_config: CCVConfig,
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::state::MAX_TOKEN_TRANSFER_FEE_BPS;

    fn fee_config(finality_bps_fee: u16, fast_finality_bps_fee: u16) -> TokenTransferFeeConfig {
        TokenTransferFeeConfig {
            dest_gas_overhead: CrossChainGas::new(90_000),
            dest_bytes_overhead: 32,
            finality_fee: UsdCents::new(50),
            fast_finality_fee: UsdCents::new(100),
            finality_bps_fee,
            fast_finality_bps_fee,
            is_enabled: true,
        }
    }

    const FAST_FINALITY: FinalityConfig = FinalityConfig {
        flags: 0,
        block_depth: 5,
    };

    #[test]
    fn bps_fee_selects_rate_by_finality() {
        let config = fee_config(250, 100);
        // EVM `TokenPool._getFee` parity: amount * bps / BPS_DIVIDER, floored.
        assert_eq!(
            token_transfer_bps_fee(Some(&config), 1_000_000, FinalityConfig::default()),
            25_000
        );
        assert_eq!(
            token_transfer_bps_fee(Some(&config), 1_000_000, FAST_FINALITY),
            10_000
        );
        // Truncation: 99 * 250 / 10_000 = 2.475 -> 2.
        assert_eq!(
            token_transfer_bps_fee(Some(&config), 99, FinalityConfig::default()),
            2
        );
    }

    #[test]
    fn bps_fee_is_zero_without_an_enabled_config() {
        assert_eq!(
            token_transfer_bps_fee(None, 1_000_000, FinalityConfig::default()),
            0
        );

        let disabled = TokenTransferFeeConfig {
            is_enabled: false,
            ..fee_config(250, 100)
        };
        assert_eq!(
            token_transfer_bps_fee(Some(&disabled), 1_000_000, FinalityConfig::default()),
            0
        );
    }

    #[test]
    fn bps_fee_never_exceeds_the_amount() {
        // 100% fee (bps == BPS_DIVIDER) with the maximum amount: `amount * bps` overflows u64,
        // but the fee must still come out exactly as the full amount.
        let config = fee_config(MAX_TOKEN_TRANSFER_FEE_BPS, 0);
        assert_eq!(
            token_transfer_bps_fee(Some(&config), u64::MAX, FinalityConfig::default()),
            u64::MAX
        );
    }

    #[test]
    fn outbound_required_ccvs_amount_deducts_the_bps_fee() {
        let config = fee_config(100, 200);
        let gross = ProtocolAmount::from(1_000_000u64);

        // 1% finality fee: a gross transfer of 1,000,000 only sends 990,000 cross-chain.
        assert_eq!(
            outbound_required_ccvs_amount(gross, Some(&config), FinalityConfig::default()).unwrap(),
            990_000
        );
        // The fee rate follows the requested finality, as it does when the pool charges it.
        assert_eq!(
            outbound_required_ccvs_amount(gross, Some(&config), FAST_FINALITY).unwrap(),
            980_000
        );
    }

    #[test]
    fn outbound_required_ccvs_amount_is_gross_without_an_enabled_config() {
        let gross = ProtocolAmount::from(1_000_000u64);
        assert_eq!(
            outbound_required_ccvs_amount(gross, None, FinalityConfig::default()).unwrap(),
            1_000_000
        );

        let disabled = TokenTransferFeeConfig {
            is_enabled: false,
            ..fee_config(100, 200)
        };
        assert_eq!(
            outbound_required_ccvs_amount(gross, Some(&disabled), FinalityConfig::default())
                .unwrap(),
            1_000_000
        );
    }
}
