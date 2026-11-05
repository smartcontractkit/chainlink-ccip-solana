use anchor_lang::prelude::*;
use ccip_common::v2::{CrossChainAmount, ProtocolAmount};
use ethnum::U256;

use crate::errors::*;

fn exp10(exponent: u8) -> Result<U256> {
    U256::new(10)
        .checked_pow(<u8 as Into<u32>>::into(exponent))
        .ok_or(CcipTokenPoolError::InvalidTokenAmountConversion.into())
}

/// Converts a local chain protocol amount to the local chain's token amount.
pub fn local_protocol_amount_to_svm_amount(amount: ProtocolAmount) -> Result<u64> {
    amount
        .to_u256()
        .try_into()
        .map_err(|_| CcipTokenPoolError::InvalidTokenAmountConversion.into())
}

/// Validates that `source_pool_data`, if present, encodes the same decimals as
/// `remote_decimals`. If `source_pool_data` is empty, the source token is assumed
/// to share the locally-configured token's remote decimals (similar to EVM's `_parseRemoteDecimals()`
/// fallback behavior), so no check is performed.
pub fn validate_source_pool_decimals(source_pool_data: &[u8], remote_decimals: u8) -> Result<()> {
    if source_pool_data.is_empty() {
        return Ok(());
    }

    let source_pool_decimals: u8 = ProtocolAmount::from_be_bytes(
        source_pool_data
            .try_into()
            .map_err(|_| CcipTokenPoolError::InvalidSourcePoolData)?,
    )
    .to_u256()
    .try_into()?;

    require_eq!(
        source_pool_decimals,
        remote_decimals,
        CcipTokenPoolError::ConfigAndRemotePoolDisagreeOnDecimals
    );

    Ok(())
}

pub fn protocol_amount_to_svm_token_amount(
    amount: ProtocolAmount,
    incoming_decimal: u8,
    local_decimal: u8,
) -> Result<u64> {
    u256_to_svm_token_amount(amount.to_u256(), incoming_decimal, local_decimal)
}

pub fn cross_chain_amount_to_svm_token_amount(
    amount: CrossChainAmount,
    incoming_decimal: u8,
    local_decimal: u8,
) -> Result<u64> {
    u256_to_svm_token_amount(
        U256::from_le_bytes(amount.to_le_bytes()),
        incoming_decimal,
        local_decimal,
    )
}

fn u256_to_svm_token_amount(
    mut incoming_amount: U256,
    incoming_decimal: u8,
    local_decimal: u8,
) -> Result<u64> {
    // handle differences in decimals by multiplying/dividing by 10^N
    match incoming_decimal.cmp(&local_decimal) {
        std::cmp::Ordering::Less => {
            incoming_amount = incoming_amount
                .checked_mul(exp10(local_decimal - incoming_decimal)?)
                .ok_or(CcipTokenPoolError::InvalidTokenAmountConversion)?;
        }
        std::cmp::Ordering::Equal => {
            // nothing needed if decimals are equal
        }
        std::cmp::Ordering::Greater => {
            incoming_amount = incoming_amount
                .checked_div(exp10(incoming_decimal - local_decimal)?)
                .ok_or(CcipTokenPoolError::InvalidTokenAmountConversion)?;
        }
    }

    // check to prevent panic
    require!(
        incoming_amount <= U256::from(u64::MAX),
        CcipTokenPoolError::InvalidTokenAmountConversion
    );
    Ok(incoming_amount.as_u64())
}

#[cfg(test)]
mod tests {
    use super::*;
    const BASE_VALUE: u64 = 1_000_000_000; // 1e9

    #[test]
    fn test_larger_incoming_decimal() {
        let amount = CrossChainAmount::from(BASE_VALUE * BASE_VALUE);
        let local_val = cross_chain_amount_to_svm_token_amount(amount, 18, 9).unwrap();
        assert!(local_val == BASE_VALUE);
    }

    #[test]
    fn test_smaller_incoming_decimal() {
        let amount = CrossChainAmount::from(BASE_VALUE);
        let local_val = cross_chain_amount_to_svm_token_amount(amount, 9, 18).unwrap();
        assert!(local_val == BASE_VALUE * BASE_VALUE);
    }

    #[test]
    fn test_equal_incoming_decimal() {
        let amount = CrossChainAmount::from(BASE_VALUE);
        let local_val = cross_chain_amount_to_svm_token_amount(amount, 9, 9).unwrap();
        assert!(local_val == BASE_VALUE);
    }

    #[test]
    fn test_u256_overflow() {
        let amount = CrossChainAmount::from(BASE_VALUE * BASE_VALUE);
        let res = cross_chain_amount_to_svm_token_amount(amount, 0, 18);
        assert!(res.is_err());
    }

    #[test]
    fn test_u256_divide_to_zero() {
        let amount = CrossChainAmount::from(BASE_VALUE);
        let local_val = cross_chain_amount_to_svm_token_amount(amount, 18, 0).unwrap();
        assert!(local_val == 0);
    }

    #[test]
    fn test_u64_overflow() {
        let amount =
            CrossChainAmount::from(U256::from(u64::MAX).checked_add(U256::from(1u64)).unwrap());
        let res = cross_chain_amount_to_svm_token_amount(amount, 0, 0);
        assert!(res.is_err());
    }

    #[test]
    fn test_exp10_max_u256_decimal_scale() {
        assert_eq!(exp10(77).unwrap(), U256::new(10).checked_pow(77).unwrap());
    }

    #[test]
    fn test_exp10_overflow_errors() {
        assert!(exp10(78).is_err());
    }

    #[test]
    fn test_u256_decimal_divisor_overflow_errors() {
        let u256_bytes = U256::from(BASE_VALUE).to_le_bytes();
        let res = cross_chain_amount_to_svm_token_amount(
            CrossChainAmount::from_le_bytes(u256_bytes),
            78,
            0,
        );
        assert!(res.is_err());
    }

    #[test]
    fn test_u256_decimal_multiplier_overflow_errors() {
        let u256_bytes = U256::from(1u64).to_le_bytes();
        let res = cross_chain_amount_to_svm_token_amount(
            CrossChainAmount::from_le_bytes(u256_bytes),
            0,
            78,
        );
        assert!(res.is_err());
    }

    #[test]
    fn inbound_scales_amount_up_to_local_decimals() {
        // 1,000 tokens with 6 remote decimals -> 1,000 tokens with 9 local decimals.
        let amount = ProtocolAmount::from(1_000_000_000u64);

        let normalized = protocol_amount_to_svm_token_amount(amount, 6, 9).unwrap();

        assert_eq!(normalized, 1_000_000_000_000);
    }

    #[test]
    fn protocol_amount_scales_amount_up_to_local_decimals() {
        let amount = ProtocolAmount::from(1_000_000_000u64);

        let normalized = protocol_amount_to_svm_token_amount(amount, 6, 9).unwrap();

        assert_eq!(normalized, 1_000_000_000_000);
    }

    #[test]
    fn typed_amounts_encode_endianness_explicitly() {
        let value = U256::from(0x0102u16);
        let cross_chain_amount = CrossChainAmount::from(value);
        let protocol_amount = ProtocolAmount::from(value);

        assert_eq!(cross_chain_amount.to_le_bytes()[0..2], [0x02, 0x01]);
        assert_eq!(protocol_amount.to_be_bytes()[30..32], [0x01, 0x02]);
        assert_eq!(
            cross_chain_amount_to_svm_token_amount(cross_chain_amount, 9, 9).unwrap(),
            protocol_amount_to_svm_token_amount(protocol_amount, 9, 9).unwrap()
        );
    }

    #[test]
    fn inbound_scales_amount_down_before_u64_conversion() {
        // Regression: the source-denominated amount exceeds u64::MAX, but after
        // converting from 18 remote decimals to 9 local decimals it fits in u64.
        let amount = ProtocolAmount::from(19_000_000_000_000_000_000u128);

        let normalized = protocol_amount_to_svm_token_amount(amount, 18, 9).unwrap();

        assert_eq!(normalized, 19_000_000_000);
    }

    #[test]
    fn outbound_decodes_protocol_amount() {
        let amount = ProtocolAmount::from(123_456u64);

        let normalized = local_protocol_amount_to_svm_amount(amount).unwrap();

        assert_eq!(normalized, 123_456);
    }

    #[test]
    fn outbound_rejects_amount_above_u64_max() {
        let amount = ProtocolAmount::from(U256::from(u64::MAX) + U256::ONE);

        let err = local_protocol_amount_to_svm_amount(amount).unwrap_err();

        assert_eq!(err, CcipTokenPoolError::InvalidTokenAmountConversion.into());
    }

    #[test]
    fn inbound_rejects_amount_above_u64_max_after_conversion() {
        let amount = U256::from(u64::MAX)
            .checked_mul(U256::from(1_000_000_000u64))
            .unwrap()
            .checked_add(U256::from(1_000_000_000u64))
            .unwrap()
            .into();

        let err = protocol_amount_to_svm_token_amount(amount, 18, 9).unwrap_err();

        assert_eq!(err, CcipTokenPoolError::InvalidTokenAmountConversion.into());
    }
}
