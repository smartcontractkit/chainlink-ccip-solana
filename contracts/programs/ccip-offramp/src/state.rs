use anchor_lang::prelude::*;
use arrayvec::arrayvec;

use ccip_common::state::AccountSchemaVersion;

use crate::CcipOfframpError;

#[account]
#[derive(InitSpace)]
pub struct Config {
    pub version: u8,
    pub bump: u8,
    pub svm_chain_selector: u64,

    pub owner: Pubkey,
    pub proposed_owner: Pubkey,
}

impl AccountSchemaVersion for Config {
    const MAX_V: u8 = 1;

    fn version(&self) -> u8 {
        self.version
    }
}

#[account]
#[derive(InitSpace)]
pub struct ReferenceAddresses {
    pub version: u8,
    pub bump: u8,
    pub router: Pubkey,
    pub fee_quoter: Pubkey,
    pub offramp_lookup_table: Pubkey,
    pub rmn_remote: Pubkey,
}

impl AccountSchemaVersion for ReferenceAddresses {
    const MAX_V: u8 = 1;

    fn version(&self) -> u8 {
        self.version
    }
}

/// Maximum number of OnRamps that may be configured for a single source chain.
///
/// More than one is supported so a remote chain can upgrade its OnRamp without downtime: the new
/// OnRamp is added alongside the old one, and the old one is only removed once all of its inflight
/// messages have been executed (including manual executions).
pub const MAX_ON_RAMPS: usize = 10;

#[derive(Clone, AnchorSerialize, AnchorDeserialize, InitSpace, Debug)]
pub struct SourceChainConfig {
    pub is_enabled: bool, // Flag whether the source chain is enabled or not

    /// OnRamps allowed to send messages from this source chain. A message is accepted when it
    /// comes from any of them. Must be non-empty, hold at most [`MAX_ON_RAMPS`] entries, and
    /// contain no duplicate, empty or zero addresses.
    #[max_len(0)]
    // max_len = 0 for InitSpace calculation, the actual length is calculated using the length of the on_ramps list. `realloc` should be used to handle the increase in account size accordingly
    pub on_ramps: Vec<OnRampAddress>,

    /// Default CCVs for the lane. Must be non-empty.
    /// No Pubkey::default() or duplicates allowed.
    #[max_len(0)]
    // max_len = 0 for InitSpace calculation, the actual length is calculated using the length of the allow_list. `realloc` should be used to handle the increase in account size accordingly
    pub default_ccvs: Vec<Pubkey>,

    /// Lane-mandated CCVs that are always required for this lane.
    /// No Pubkey::default() or duplicates allowed (with default_ccvs or in lane_mandated_ccvs).
    #[max_len(0)]
    // max_len = 0 for InitSpace calculation, the actual length is calculated using the length of the allow_list. `realloc` should be used to handle the increase in account size accordingly
    pub lane_mandated_ccvs: Vec<Pubkey>,
}

impl SourceChainConfig {
    pub fn dynamic_space(
        on_ramps: &[OnRampAddress],
        default_ccvs: &[Pubkey],
        lane_mandated_ccvs: &[Pubkey],
    ) -> usize {
        on_ramps.len() * OnRampAddress::INIT_SPACE
            + (default_ccvs.len() + lane_mandated_ccvs.len()) * std::mem::size_of::<Pubkey>()
    }

    /// Whether `address` belongs to one of the OnRamps allowed to send messages from this source
    /// chain.
    pub fn allows_on_ramp(&self, address: &[u8]) -> bool {
        self.on_ramps
            .iter()
            .any(|on_ramp| on_ramp.bytes() == address)
    }
}

#[derive(Clone, AnchorSerialize, AnchorDeserialize, InitSpace, Debug)]
pub struct OnRampAddress {
    /// Private: only the first `len` bytes are meaningful, the rest is zero padding.
    /// Go through [`OnRampAddress::bytes`] to read the address.
    bytes: [u8; 64],
    len: u32,
}

// Field names are `bytes` / `len` rather than the default `xs` / `len` because they're part of the
// serialized account layout and the published IDL.
arrayvec!(OnRampAddress, u8, 64, u32, bytes, len);

#[account]
#[derive(InitSpace, Debug)]
pub struct MessageState {
    pub version: u8,
    pub bump: u8,
    pub execution_state: MessageExecutionState,
    pub message_id: [u8; 32],
}

impl AccountSchemaVersion for MessageState {
    const MAX_V: u8 = 1;

    fn version(&self) -> u8 {
        self.version
    }
}

impl OnRampAddress {
    /// The address bytes only, i.e. the `..len` prefix of the backing array.
    pub fn bytes(&self) -> &[u8] {
        self.as_slice()
    }

    pub fn is_zero(&self) -> bool {
        self.bytes().iter().all(|b| *b == 0)
    }

    pub const EMPTY: Self = Self {
        bytes: [0u8; 64],
        len: 0,
    };
}

impl TryInto<OnRampAddress> for Vec<u8> {
    type Error = CcipOfframpError;

    fn try_into(self) -> std::result::Result<OnRampAddress, Self::Error> {
        let mut address = OnRampAddress::EMPTY;
        if self.is_empty() {
            return Err(CcipOfframpError::InvalidOnrampAddress);
        }

        // The capacity check lives in `try_extend`, so it can't drift from the write it guards.
        address
            .try_extend(&self)
            .map_err(|_| CcipOfframpError::InvalidOnrampAddress)?;

        Ok(address)
    }
}

impl<const N: usize> From<[u8; N]> for OnRampAddress {
    fn from(val: [u8; N]) -> Self {
        let mut address = Self::EMPTY;
        assert!(N > 0, "an OnRamp address cannot be empty");

        // `From` is infallible, so an oversized N can only panic. Callers with runtime-sized input
        // should go through `TryInto` instead.
        address.extend(&val);
        address
    }
}

#[account]
#[derive(InitSpace, Debug)]
pub struct SourceChain {
    pub version: u8,
    pub bump: u8,
    pub chain_selector: u64,       // Chain selector used for the seed
    pub config: SourceChainConfig, // values configured by an admin
}

impl AccountSchemaVersion for SourceChain {
    const MAX_V: u8 = 1;

    fn version(&self) -> u8 {
        self.version
    }
}

impl SourceChain {
    pub fn dynamic_space(
        on_ramps: &[OnRampAddress],
        default_ccvs: &[Pubkey],
        lane_mandated_ccvs: &[Pubkey],
    ) -> usize {
        SourceChainConfig::dynamic_space(on_ramps, default_ccvs, lane_mandated_ccvs)
    }
}

#[derive(Clone, Copy, AnchorSerialize, AnchorDeserialize, Debug, PartialEq, InitSpace)]
#[borsh(use_discriminant = true)]
// used in the commit report execution_states field
pub enum MessageExecutionState {
    Untouched = 0,
    InProgress = 1, // Not used in SVM, but used in EVM
    Success = 2,
    Failure = 3, // Not used in SVM, but used in EVM
}

impl TryFrom<u128> for MessageExecutionState {
    type Error = &'static str;

    fn try_from(value: u128) -> std::result::Result<MessageExecutionState, &'static str> {
        match value {
            0 => Ok(MessageExecutionState::Untouched),
            1 => Ok(MessageExecutionState::InProgress),
            2 => Ok(MessageExecutionState::Success),
            3 => Ok(MessageExecutionState::Failure),
            _ => Err("Invalid ExecutionState"),
        }
    }
}

#[cfg(test)]
mod tests {

    use super::*;
    use std::convert::TryFrom;

    #[test]
    fn test_execution_state_try_from() {
        assert_eq!(
            MessageExecutionState::try_from(0).unwrap(),
            MessageExecutionState::Untouched
        );
        assert_eq!(
            MessageExecutionState::try_from(1).unwrap(),
            MessageExecutionState::InProgress
        );
        assert_eq!(
            MessageExecutionState::try_from(2).unwrap(),
            MessageExecutionState::Success
        );
        assert_eq!(
            MessageExecutionState::try_from(3).unwrap(),
            MessageExecutionState::Failure
        );
        assert!(MessageExecutionState::try_from(4).is_err());
    }

    #[test]
    fn allows_any_configured_on_ramp() {
        let first = OnRampAddress::from([0xAB; 32]);
        let second = OnRampAddress::from([0xCD; 20]);
        let config = SourceChainConfig {
            is_enabled: true,
            on_ramps: vec![first.clone(), second.clone()],
            default_ccvs: vec![Pubkey::new_unique()],
            lane_mandated_ccvs: vec![],
        };

        assert!(config.allows_on_ramp(first.bytes()));
        assert!(config.allows_on_ramp(second.bytes()));
        assert!(!config.allows_on_ramp(OnRampAddress::from([0xEF; 32]).bytes()));
        // The whole address is compared, so a prefix of a configured one does not match.
        assert!(!config.allows_on_ramp(OnRampAddress::from([0xAB; 20]).bytes()));
        assert!(!config.allows_on_ramp(&[]));
    }

    #[test]
    fn dynamic_space_matches_serialized_size() {
        // `space` / `realloc` are computed as INIT_SPACE + dynamic_space, so the two must add up to
        // the real serialized size or configs with several OnRamps would not fit in their account.
        let config = SourceChainConfig {
            is_enabled: true,
            on_ramps: vec![
                OnRampAddress::from([0xAB; 64]),
                OnRampAddress::from([0xCD; 32]),
            ],
            default_ccvs: vec![Pubkey::new_unique()],
            lane_mandated_ccvs: vec![Pubkey::new_unique()],
        };
        let source_chain = SourceChain {
            version: SourceChain::MAX_V,
            bump: 255,
            chain_selector: 42,
            config: config.clone(),
        };

        let mut serialized = Vec::new();
        source_chain.serialize(&mut serialized).unwrap();

        assert_eq!(
            serialized.len(),
            SourceChain::INIT_SPACE
                + SourceChain::dynamic_space(
                    &config.on_ramps,
                    &config.default_ccvs,
                    &config.lane_mandated_ccvs
                )
        );
    }
}
