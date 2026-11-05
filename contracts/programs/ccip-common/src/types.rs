use anchor_lang::prelude::*;
use ethnum::U256;
use std::{
    convert::Into,
    fmt,
    ops::{Add, AddAssign, Mul},
};

use crate::{messages::SVMTokenAmount, CommonCcipError};

#[derive(
    AnchorSerialize,
    AnchorDeserialize,
    InitSpace,
    PartialEq,
    Copy,
    Clone,
    Debug,
    Default,
    Eq,
    PartialOrd,
    Ord,
)]
pub struct UsdCents(u32);

impl UsdCents {
    pub fn new(raw: u32) -> Self {
        Self(raw)
    }

    pub const ZERO: Self = Self(0);

    pub fn convert<T: From<u32>>(self) -> T {
        self.0.into()
    }

    /// Converts an 18-decimals USD amount into whole cents, rounding up.
    ///
    /// Mirrors EVM's `(value + (1e16 - 1)) / 1e16`: rounding up is what guarantees that a
    /// non-zero USD amount can never be billed as zero cents. This is deliberately a named
    /// constructor rather than a `TryFrom` impl, because the rounding direction is a billing
    /// decision that call sites should state explicitly.
    pub fn round_up_from_usd18(value: Usd18Decimals) -> std::result::Result<Self, CommonCcipError> {
        let cent = 1u32.e(16);
        let rounded = value
            .get()
            .checked_add(cent - 1)
            .ok_or(CommonCcipError::InvalidUsdConversion)?
            / cent;
        Ok(Self(
            rounded
                .try_into()
                .map_err(|_| CommonCcipError::InvalidUsdConversion)?,
        ))
    }
}

impl fmt::Display for UsdCents {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        self.0.fmt(f)
    }
}

// Chain-agnostic definition of "gas", same as above
#[derive(
    AnchorSerialize,
    AnchorDeserialize,
    InitSpace,
    PartialEq,
    Copy,
    Clone,
    Debug,
    Default,
    Eq,
    PartialOrd,
    Ord,
)]
pub struct CrossChainGas(u32);

impl CrossChainGas {
    pub fn new(raw: u32) -> Self {
        Self(raw)
    }

    pub const ZERO: Self = Self(0);

    pub fn convert<T: From<u32>>(self) -> T {
        self.0.into()
    }
}

impl fmt::Display for CrossChainGas {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        self.0.fmt(f)
    }
}

/// Conventionally-text data stored on-chain as opaque bytes.
///
/// Borsh-compatible with `String`: both encode as a u32 length prefix followed by the raw bytes, so
/// a field can move from `String` to `ByteString` without migrating existing account data. What
/// changes is what the program pays for and can fail on — deserializing a `String` validates UTF-8
/// on every account load, a cost and a failure mode that a program which only stores and echoes the
/// value has no use for. `ByteString` keeps the bytes opaque on-chain while callers keep string
/// semantics off-chain, recovering the text with [`ByteString::to_utf8`].
///
/// Use it only where the content is text by convention but the program never interprets it. If
/// on-chain logic needs to *read* the text, `String` and its validation are the right choice.
///
/// `INIT_SPACE` covers the length prefix alone, since the payload is variable: an account holding
/// these must add the payload lengths itself (see committee-verifier's
/// `StorageLocations::dynamic_space`).
#[derive(AnchorSerialize, AnchorDeserialize, Clone, Debug, Default, PartialEq, Eq)]
pub struct ByteString(Vec<u8>);

impl anchor_lang::Space for ByteString {
    const INIT_SPACE: usize = 4;
}

impl ByteString {
    pub fn new(bytes: Vec<u8>) -> Self {
        Self(bytes)
    }

    pub fn as_bytes(&self) -> &[u8] {
        &self.0
    }

    pub fn into_bytes(self) -> Vec<u8> {
        self.0
    }

    /// Interprets the contents as UTF-8. Fallible by design: nothing on-chain enforces that the
    /// bytes are valid UTF-8, so off-chain readers must handle arbitrary input.
    pub fn to_utf8(&self) -> std::result::Result<&str, std::str::Utf8Error> {
        std::str::from_utf8(&self.0)
    }

    /// Borsh-serialized length of the payload, excluding the 4-byte length prefix already counted
    /// by `INIT_SPACE`.
    pub fn payload_len(&self) -> usize {
        self.0.len()
    }
}

impl From<String> for ByteString {
    fn from(value: String) -> Self {
        Self(value.into_bytes())
    }
}

impl From<&str> for ByteString {
    fn from(value: &str) -> Self {
        Self(value.as_bytes().to_vec())
    }
}

/// Lossy on purpose: this is for logs and error messages, where a panic or an empty string would be
/// less useful than the replacement character. Use [`ByteString::to_utf8`] when the distinction
/// between valid and invalid UTF-8 matters.
impl fmt::Display for ByteString {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        f.write_str(&String::from_utf8_lossy(&self.0))
    }
}

// ToTxData implements an interface that returns instruction data
pub trait ToTxData {
    fn to_tx_data(&self) -> Vec<u8>;
}

// USD with 18 decimals (i.e. $8 -> 8e18)
//
// Byte layout is identical to U256, we don't use U256 directly
// as the orphan rule makes it complicated to derive Anchor traits on it.
//
// Serialization: the wrapped array is U256's internal target-endian word representation, so
// the Borsh wire format is `[low_u128, high_u128]`, each little-endian — i.e. the full
// little-endian 256-bit byte layout. This relies on Solana targets (SBF and test hosts) being
// little-endian; off-chain readers of the Borsh encoding hard-code that interpretation.
// That's acceptable because this is an SVM-internal type (CPI return values, IDL exports),
// not a CCIP-level wire type: it never crosses chains, so it does not have to match EVM's
// big-endian uint256 encoding, unlike e.g. `ProtocolAmount`.
//
// Ord/PartialOrd are implemented manually: deriving them would compare the raw
// word array, whose order is target-endian dependent and numerically wrong on
// little-endian targets.
#[derive(Debug, Clone, Default, PartialEq, Eq, AnchorDeserialize, AnchorSerialize)]
pub struct Usd18Decimals([u128; 2]);

impl Usd18Decimals {
    pub const ZERO: Self = Self(U256::ZERO.0);

    pub fn new(value: u128) -> Self {
        Self((U256::new(value)).0)
    }

    pub fn from_usd_cents(cents: u32) -> Self {
        Self((U256::new(cents.into()) * 1u32.e(16)).0)
    }

    pub fn from_token_amount(sta: &SVMTokenAmount, price: &Usd18Decimals) -> Self {
        Usd18Decimals(
            (U256::new(sta.amount.into()) * Into::<U256>::into(price.clone()) / 1u32.e(18)).0,
        )
    }

    pub fn get(&self) -> U256 {
        self.clone().into()
    }
}

impl From<Usd18Decimals> for U256 {
    fn from(value: Usd18Decimals) -> Self {
        // The wrapped array is U256's internal (target-endian) word representation,
        // so the exact inverse of `From<U256>` is reconstructing from the raw words.
        // Note U256::from_words is NOT that inverse: it takes (hi, lo), while the
        // internal order on little-endian targets is [lo, hi].
        U256(value.0)
    }
}

impl PartialOrd for Usd18Decimals {
    fn partial_cmp(&self, other: &Self) -> Option<std::cmp::Ordering> {
        Some(self.cmp(other))
    }
}

impl Ord for Usd18Decimals {
    fn cmp(&self, other: &Self) -> std::cmp::Ordering {
        self.get().cmp(&other.get())
    }
}

impl From<UsdCents> for Usd18Decimals {
    fn from(cents: UsdCents) -> Self {
        Self::from_usd_cents(cents.0)
    }
}

impl From<U256> for Usd18Decimals {
    fn from(value: U256) -> Self {
        Self(value.0)
    }
}

impl Add for Usd18Decimals {
    type Output = Self;

    fn add(self, rhs: Self) -> Self::Output {
        (Into::<U256>::into(self) + Into::<U256>::into(rhs)).into()
    }
}

impl AddAssign for Usd18Decimals {
    fn add_assign(&mut self, rhs: Self) {
        *self = self.clone() + rhs
    }
}

impl<T: Into<U256>> Mul<T> for Usd18Decimals {
    type Output = Self;

    fn mul(mut self, rhs: T) -> Self::Output {
        self = (Into::<U256>::into(self) * Into::<U256>::into(rhs)).into();
        self
    }
}

impl From<&TimestampedPackedU224> for Usd18Decimals {
    fn from(tpu: &TimestampedPackedU224) -> Self {
        let mut u256_buffer = [0u8; 32];
        u256_buffer[4..32].clone_from_slice(&tpu.value);
        U256::from_be_bytes(u256_buffer).into()
    }
}

pub trait Exponential {
    fn e(self, exponent: u8) -> U256;
}

impl<T: Into<u32>> Exponential for T {
    fn e(self, exponent: u8) -> U256 {
        // SAFETY: exponent is u8, widening to u32
        U256::from(self.into()) * U256::new(10).pow(exponent as u32)
    }
}

#[derive(InitSpace, Clone, AnchorSerialize, AnchorDeserialize, Debug, PartialEq)]
pub struct TimestampedPackedU224 {
    pub value: [u8; 28],
    pub timestamp: i64, // maintaining the type that SVM returns for the time (solana_program::clock::UnixTimestamp = i64)
}

#[cfg(test)]
mod tests {
    use super::*;

    /// `ByteString` is only a safe drop-in for a `String` field because the two share a Borsh
    /// encoding — that is what lets an existing account be reinterpreted without migration. If this
    /// ever fails, every account holding a migrated field is being misread.
    #[test]
    fn byte_string_is_borsh_compatible_with_string() {
        let strings = vec![
            String::new(),
            "postgres://default-aggregator-db:5432/aggregator".to_string(),
            "ünïcødé".to_string(),
        ];
        let byte_strings: Vec<ByteString> = strings.iter().cloned().map(ByteString::from).collect();

        let mut as_strings = Vec::new();
        strings.serialize(&mut as_strings).unwrap();
        let mut as_bytes = Vec::new();
        byte_strings.serialize(&mut as_bytes).unwrap();
        assert_eq!(as_strings, as_bytes);

        // And the reverse direction: bytes written as a Vec<String> decode as Vec<ByteString>.
        let decoded = Vec::<ByteString>::try_from_slice(&as_strings).unwrap();
        assert_eq!(decoded, byte_strings);
    }

    #[test]
    fn byte_string_survives_non_utf8() {
        // Nothing on-chain validates UTF-8, so decoding must report failure rather than panic.
        let raw = ByteString::new(vec![0xff, 0xfe]);
        assert!(raw.to_utf8().is_err());
        assert_eq!(raw.payload_len(), 2);
        // Display is lossy so logs stay usable.
        assert_eq!(raw.to_string(), "\u{fffd}\u{fffd}");
    }

    /// This is the difference the switch away from `String` actually buys, and the reason the
    /// committee-verifier storage-locations test round-trips an invalid sequence: Borsh `String`
    /// deserialization validates UTF-8, so a `Vec<String>` field rejects these bytes on every
    /// account load, while `Vec<ByteString>` carries them untouched.
    #[test]
    fn byte_string_accepts_payloads_string_would_reject() {
        let invalid = ByteString::new(vec![0xff, 0xfe, b'o', b'k']);
        let encoded = {
            let mut buf = Vec::new();
            vec![invalid.clone()].serialize(&mut buf).unwrap();
            buf
        };

        assert!(Vec::<String>::try_from_slice(&encoded).is_err());
        assert_eq!(
            Vec::<ByteString>::try_from_slice(&encoded).unwrap(),
            vec![invalid]
        );
    }

    #[test]
    fn u256_round_trip_is_lossless() {
        for value in [
            U256::ZERO,
            U256::new(1),
            U256::new(u128::MAX),
            U256::from_words(1, 0), // 2^128: crosses the word boundary
            U256::from_words(0xDEAD_BEEF, 0x1234_5678), // both words set
            U256::MAX,
        ] {
            assert_eq!(U256::from(Usd18Decimals::from(value)), value);
            assert_eq!(Usd18Decimals::from(value).get(), value);
        }
    }

    #[test]
    fn constructors_agree_with_u256() {
        assert_eq!(Usd18Decimals::ZERO.get(), U256::ZERO);
        assert_eq!(Usd18Decimals::new(5).get(), U256::new(5));
        assert_eq!(Usd18Decimals::new(u128::MAX).get(), U256::new(u128::MAX));
        // $8.00 -> 8e18
        assert_eq!(Usd18Decimals::from_usd_cents(800).get(), 8u32.e(18));
        assert_eq!(Usd18Decimals::from(UsdCents::new(800)).get(), 8u32.e(18));
    }

    #[test]
    fn cents_conversion_rounds_up() {
        let cents = |raw: U256| UsdCents::round_up_from_usd18(raw.into()).unwrap();

        assert_eq!(cents(U256::ZERO), UsdCents::ZERO);
        // Anything above zero bills at least one cent, however small.
        assert_eq!(cents(U256::new(1)), UsdCents::new(1));
        assert_eq!(cents(1u32.e(16) - U256::new(1)), UsdCents::new(1));
        // Exact multiples are not rounded up past themselves.
        assert_eq!(cents(1u32.e(16)), UsdCents::new(1));
        assert_eq!(cents(1u32.e(16) + U256::new(1)), UsdCents::new(2));
        // $8.00 -> 800 cents, round-tripping from_usd_cents.
        assert_eq!(cents(8u32.e(18)), UsdCents::new(800));

        // Values that don't fit in the u32 cent counter are rejected, not truncated.
        assert!(UsdCents::round_up_from_usd18(U256::MAX.into()).is_err());
    }

    #[test]
    fn ordering_is_numeric() {
        let small = Usd18Decimals::new(5);
        let large: Usd18Decimals = U256::from_words(1, 0).into(); // 2^128: only the high word set
                                                                  // A derived (word-array) ordering would compare the low words first and
                                                                  // consider `large` < `small` on little-endian targets.
        assert!(small < large);
        assert_eq!(
            small.clone().max(large.clone()),
            large,
            "max must pick the numerically larger value"
        );
        assert!(Usd18Decimals::ZERO < small);
        assert_eq!(small.cmp(&small.clone()), std::cmp::Ordering::Equal);
    }

    #[test]
    fn arithmetic_matches_u256_semantics() {
        let a = Usd18Decimals::new(300);
        let b = Usd18Decimals::new(700);
        assert_eq!((a.clone() + b).get(), U256::new(1000));

        let mut acc = Usd18Decimals::ZERO;
        acc += a.clone();
        acc += a.clone();
        assert_eq!(acc.get(), U256::new(600));

        assert_eq!((a * U256::new(4)).get(), U256::new(1200));

        // Chained ops that span both words: (2^100 * 2^100) + 1 == 2^200 + 1
        let wide = Usd18Decimals::new(1u128 << 100);
        let result = wide * U256::new(1u128 << 100) + Usd18Decimals::new(1);
        assert_eq!(result.get(), U256::from_words(1u128 << 72, 1));
    }

    #[test]
    fn from_token_amount_scales_by_price() {
        // 2e9 smallest units of a token priced at $3 per 1e18 smallest units -> $6e-9,
        // i.e. 6e9 in 18-decimals USD.
        let sta = SVMTokenAmount {
            token: Pubkey::new_unique(),
            amount: 2_000_000_000,
        };
        let price = Usd18Decimals::from(3u32.e(18));
        assert_eq!(
            Usd18Decimals::from_token_amount(&sta, &price).get(),
            U256::new(6_000_000_000)
        );
    }

    #[test]
    fn from_timestamped_packed_u224_reads_big_endian() {
        let mut packed = [0u8; 28];
        packed[27] = 0x2a; // 42 in the least significant (big-endian trailing) byte
        let tpu = TimestampedPackedU224 {
            value: packed,
            timestamp: 1,
        };
        assert_eq!(Usd18Decimals::from(&tpu).get(), U256::new(42));

        let mut wide = [0u8; 28];
        wide[0] = 1; // most significant byte of the 224-bit value
        let tpu_wide = TimestampedPackedU224 {
            value: wide,
            timestamp: 1,
        };
        // 2^216 == 2^88 << 128
        assert_eq!(
            Usd18Decimals::from(&tpu_wide).get(),
            U256::from_words(1u128 << 88, 0)
        );
    }
}
