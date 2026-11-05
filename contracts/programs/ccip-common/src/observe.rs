//! Shared conventions for the permissionless `observe_*` instructions.
//!
//! Every CCIP program exposes a small set of read-only `observe_*` instructions meant to be
//! executed through `simulateTransaction` by off-chain observability pipelines. They exist so
//! that consumers get a *stable* projection of on-chain state, decoupled from the account
//! layouts (which change across schema versions and migrations), bundled together with the
//! program's `type_version` so that a single simulation identifies both what the program is and
//! how it is configured.
//!
//! Conventions shared by all of them:
//!
//! * Permissionless: no signer, no writable account, no state change.
//! * Strict accounts: the PDAs are declared as typed `Account<'info, T>` with seed constraints,
//!   so observing an unconfigured lane fails the simulation rather than returning zeroed data.
//! * Every payload starts with `observation_version` ([`OBSERVATION_VERSION`]) followed by
//!   `type_version` (the program's `CCIP_BUILD_TYPE_VERSION`).
//! * Payloads must fit Solana's 1024-byte return data limit. Fields whose length is unbounded by
//!   configuration (allow lists, CCV allow lists) are therefore reported as a count, with the
//!   contents exposed by a separate paged instruction where consumers need them.
use anchor_lang::prelude::*;

/// Schema version of the `observe_*` payloads, shared by every CCIP program.
///
/// Bump this when a payload changes in a way that is not purely additive at the end of a struct,
/// so consumers can detect the change instead of misreading Borsh data.
pub const OBSERVATION_VERSION: u16 = 1;

/// Upper bound on how many items a paged `observe_*` instruction returns per call, chosen so
/// that a full page of `Pubkey`s stays well inside the 1024-byte return data limit.
pub const MAX_OBSERVE_PAGE_LEN: u32 = 24;

/// Parameters for the paged `observe_*` instructions.
#[derive(AnchorSerialize, AnchorDeserialize, Clone, Copy, Debug, Default)]
pub struct ObservePageParams {
    /// Index of the first item to return.
    pub offset: u32,
    /// Maximum number of items to return, capped at [`MAX_OBSERVE_PAGE_LEN`].
    pub limit: u32,
}

impl ObservePageParams {
    /// Resolves this page against a slice, returning the requested window. An `offset` past the
    /// end yields an empty slice (rather than an error) so that consumers can page until empty.
    pub fn apply<T>(&self, items: &[T]) -> Vec<T>
    where
        T: Clone,
    {
        let limit = self.limit.min(MAX_OBSERVE_PAGE_LEN) as usize;
        items
            .iter()
            .skip(self.offset as usize)
            .take(limit)
            .cloned()
            .collect()
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn page_returns_requested_window() {
        let items: Vec<u8> = (0..10).collect();

        let params = ObservePageParams {
            offset: 2,
            limit: 3,
        };
        assert_eq!(params.apply(&items), vec![2, 3, 4]);
    }

    #[test]
    fn page_caps_limit() {
        let items: Vec<u8> = (0..u8::MAX).collect();

        let params = ObservePageParams {
            offset: 0,
            limit: u32::MAX,
        };
        assert_eq!(params.apply(&items).len(), MAX_OBSERVE_PAGE_LEN as usize);
    }

    #[test]
    fn page_past_end_is_empty() {
        let items: Vec<u8> = (0..3).collect();

        let params = ObservePageParams {
            offset: 100,
            limit: 10,
        };
        assert!(params.apply(&items).is_empty());
    }

    #[test]
    fn page_clamps_to_remaining_items() {
        let items: Vec<u8> = (0..5).collect();

        let params = ObservePageParams {
            offset: 3,
            limit: 10,
        };
        assert_eq!(params.apply(&items), vec![3, 4]);
    }
}
