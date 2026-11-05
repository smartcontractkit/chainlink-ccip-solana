/*!
Common interface to perform account resolution for CCIP instructions.

Account resolution allows you to automatically construct the list of
accounts and LUT tables required to call an instruction with a
particular set of parameters. For a Go example of the flow, see
`ccip_account_resolutions.go`.

The entire process is read-only, so it can be performed entirely
through simulation. At a high level, you must perform the following
steps:

1. Call `<program>::resolve_accounts_start`. The discriminator is
   shared by all programs supporting account resolution, so the
   instruction can be built from
   `ccip_common::resolve_accounts_start`. As input, you must provide:
   - A `Context` struct containing the `Config` account for that
     program.
   - [`ResolveAccountsParams`] populated with the intended caller of
     the instruction of interest and its Borsh-encoded instruction
     data (including its discriminator prefix).
     `metadata` must be left empty for this first step.
   - Any additional accounts required by the instruction-specific
     resolver as extra accounts on the resolution instruction. These
     accounts are not serialized in [`ResolveAccountsParams`], because
     resolvers that need them usually need the actual `AccountInfo`
     to read account data. Each instruction-specific account
     resolution entrypoint documents whether it needs such accounts.
2. Read the [`ResolveAccountsResponse`] and, following the
   instructions on each field below, store all `_to_save` fields.
   Then, if `next_ix_discriminator` is nonempty, perform a call to
   the method identified by the discriminator,
   passing the list of accounts in `ask_again_with` and the metadata
   in `metadata`.
3. Repeat step 2 until `next_ix_discriminator` is returned empty.
   Concatenating all the `_to_save` fields from every stage yields
   the information needed to call the instruction of interest. In
   some cases, the `metadata` field may be an input to the final
   instruction.
*/
use anchor_lang::prelude::*;

use crate::CommonCcipError;

/// Discriminator for the shared `resolve_accounts_start` entrypoint that each
/// program implementing the resolution interface must expose. Sourced from the
/// common crate's own `ResolveAccountsStart` instruction so the value cannot
/// drift from the Anchor-generated sighash.
pub fn resolve_accounts_start_discriminator() -> [u8; 8] {
    crate::instruction::ResolveAccountsStart::DISCRIMINATOR
        .try_into()
        .expect("resolve_accounts_start discriminator must be 8 bytes")
}

#[derive(Debug, Default, PartialEq, Eq, Clone, AnchorDeserialize, AnchorSerialize)]
pub struct ResolveAccountsResponse {
    /// In the next resolution round, call the resolution method
    /// including these accounts (all read-only).
    pub ask_again_with: Vec<Pubkey>,
    /// Append these accounts to a separate accumulator list. Once
    /// `next_ix_discriminator` is empty, this accumulated list
    /// contains all accounts for the instruction of interest.
    pub accounts_to_save: Vec<CcipAccountMeta>,
    /// Append these lookup tables to a separate accumulator list.
    /// Once resolution is complete, the accumulated list contains all
    /// LUTs that the instruction of interest can use.
    pub lookup_tables_to_save: Vec<Pubkey>,
    /// Identifies the next resolution stage. If None, resolution is
    /// complete.
    pub next_ix_discriminator: Option<[u8; 8]>,
    /// Metadata required for the next resolution stage. In some cases, it
    /// may be an input to the final instruction as well.
    pub metadata: Vec<u8>,
}

impl ResolveAccountsResponse {
    /// Borsh size of a response with the given contents. Each [`CcipAccountMeta`] costs 34 bytes
    /// (32 pubkey + 2 flag bytes) and each pubkey 32; every vector pays a 4-byte length prefix, and
    /// `next_ix_discriminator` is a 1-byte `Option` tag plus its 8-byte payload.
    /// `response_size_matches_borsh` pins this against the real encoder.
    ///
    /// Charging for the discriminator payload unconditionally overstates a terminal response by 8
    /// bytes, which is the safe direction for every caller.
    pub const fn size(
        accounts: usize,
        ask_again_with: usize,
        lookup_tables: usize,
        metadata: usize,
    ) -> usize {
        (4 + 34 * accounts)
            + (4 + 32 * ask_again_with)
            + (4 + 32 * lookup_tables)
            + 9
            + (4 + metadata)
    }

    /// [`Self::size`] for this response's actual contents.
    pub fn encoded_size(&self) -> usize {
        Self::size(
            self.accounts_to_save.len(),
            self.ask_again_with.len(),
            self.lookup_tables_to_save.len(),
            self.metadata.len(),
        )
    }
}

// We can't use anchor's `AccountMeta` since it doesn't implement
// AnchorSerialize/AnchorDeserialize, and it's too small to warrant wrapping.
#[derive(Debug, Default, PartialEq, Eq, Clone, AnchorDeserialize, AnchorSerialize)]
pub struct CcipAccountMeta {
    pub pubkey: Pubkey,
    pub is_signer: bool,
    pub is_writable: bool,
}

pub trait ToMeta {
    fn readonly(self) -> CcipAccountMeta;
    fn writable(self) -> CcipAccountMeta;
    fn with_writable(self, writable: bool) -> CcipAccountMeta;
    fn signer(self) -> CcipAccountMeta;
}

impl From<AccountMeta> for CcipAccountMeta {
    fn from(meta: AccountMeta) -> Self {
        Self {
            pubkey: meta.pubkey,
            is_signer: meta.is_signer,
            is_writable: meta.is_writable,
        }
    }
}

impl ToMeta for Pubkey {
    fn readonly(self) -> CcipAccountMeta {
        CcipAccountMeta {
            pubkey: self,
            is_signer: false,
            is_writable: false,
        }
    }

    fn writable(self) -> CcipAccountMeta {
        CcipAccountMeta {
            pubkey: self,
            is_signer: false,
            is_writable: true,
        }
    }

    fn with_writable(self, writable: bool) -> CcipAccountMeta {
        CcipAccountMeta {
            pubkey: self,
            is_signer: false,
            is_writable: writable,
        }
    }

    fn signer(self) -> CcipAccountMeta {
        CcipAccountMeta {
            pubkey: self,
            is_signer: true,
            is_writable: false,
        }
    }
}

impl ToMeta for CcipAccountMeta {
    fn readonly(self) -> CcipAccountMeta {
        Self {
            pubkey: self.pubkey,
            is_signer: self.is_signer,
            is_writable: false,
        }
    }

    fn writable(self) -> CcipAccountMeta {
        Self {
            pubkey: self.pubkey,
            is_signer: self.is_signer,
            is_writable: true,
        }
    }

    fn with_writable(self, writable: bool) -> CcipAccountMeta {
        Self {
            pubkey: self.pubkey,
            is_signer: self.is_signer,
            is_writable: writable,
        }
    }

    fn signer(self) -> CcipAccountMeta {
        Self {
            pubkey: self.pubkey,
            is_signer: true,
            is_writable: self.is_writable,
        }
    }
}

pub fn find(seeds: &[&[u8]], program_id: Pubkey) -> Pubkey {
    Pubkey::find_program_address(seeds, &program_id).0
}

#[derive(AnchorSerialize, AnchorDeserialize, Debug)]
pub struct ResolveAccountsParams {
    /// The account expected to sign the final instruction being resolved.
    pub caller: Pubkey,
    /// Borsh-encoded final instruction data, including its 8-byte
    /// Anchor discriminator prefix.
    ///
    /// IMPORTANT: For CPI instructions, it's possible the entire `ix_data` cannot
    /// be known at the point of account resolution. This is common in cases where
    /// part of the input to the CPI will be determined dynamically when executing the
    /// parent instruction. In those cases, `ix_data` must contain a best-effort
    /// approximation of the final instruction data, with those fields that are impossible
    /// to know ahead of time swapped with defaults, dummies or other context-appropriate
    /// values. In these cases, the resolver must take care not to rely on those dummy
    /// values to generate a valid list of accounts. An example of this pattern is
    /// CCVs receiving a `message_id` as part of their `ix_data`. The `message_id` cannot
    /// be known until execution time, so the value must be ignored by the `CCV` resolver.
    pub ix_data: Vec<u8>,
    /// Opaque Borsh-encoded metadata forwarded from the previous stage.
    /// Must be empty on the first call.
    pub metadata: Vec<u8>,
}

/// Resolution parameters used by nested resolvers. Rather than the full ix_data and a metadata
/// field, they have an opaque `cpi_resolution_inputs` field that doubles as both. The caller
/// of the nested resolution is responsible for correctly encoding it by consulting
/// the documentation.
#[derive(AnchorSerialize, AnchorDeserialize, Debug)]
pub struct NestedResolveAccountsParams {
    /// The account expected to sign the final instruction being resolved.
    pub caller: Pubkey,
    /// The program ID that derived `caller` and is driving this resolution (the router on the
    /// send side, the offramp on the execute side).
    pub caller_program: Pubkey,
    /// Will be deserialized into a type the nested resolver understands. For what to
    /// send in the first stage of nested resolution, consult the nested resolver's documentation.
    pub cpi_resolution_inputs: Vec<u8>,
}

#[derive(Debug, Default, PartialEq, Eq, Clone, AnchorDeserialize, AnchorSerialize)]
pub struct NestedResolveAccountsResponse {
    /// In the next resolution round, call the resolution method
    /// including these accounts (all read-only).
    pub ask_again_with: Vec<Pubkey>,
    /// Append these accounts to a separate accumulator list. Once
    /// `next_ix_discriminator` is empty, this accumulated list
    /// contains all accounts for the instruction of interest.
    pub accounts_to_save: Vec<CcipAccountMeta>,
    /// Append these lookup tables to a separate accumulator list.
    /// Once resolution is complete, the accumulated list contains all
    /// LUTs that the instruction of interest can use.
    pub lookup_tables_to_save: Vec<Pubkey>,
    /// If nonempty, perform another resolution stage with these inputs.
    pub next_cpi_resolution_inputs: Vec<u8>,
}

#[derive(PartialEq)]
pub struct BytesWrapper<'a>(pub &'a [u8]); // just useful to print discriminators in a more readable way in logs
impl<'a> std::fmt::Display for BytesWrapper<'a> {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        write!(f, "{:?}", self.0)
    }
}

/// Validates and splits the instruction data in a resolve_accounts_start call.
/// Returns (ix_discriminator, remaining_data).
pub fn validate_resolution_start(
    params: &ResolveAccountsParams,
    instruction_has_data: bool,
) -> Result<(&[u8], &[u8])> {
    let (ix_discriminator, data) = params
        .ix_data
        .split_at_checked(8)
        .ok_or(CommonCcipError::InvalidResolutionInstructionData)?;
    if instruction_has_data {
        require!(
            !data.is_empty(),
            CommonCcipError::InvalidResolutionInstructionData
        );
    }
    require!(
        params.metadata.is_empty(), // it is actually the start
        CommonCcipError::InvalidResolutionInstructionData
    );
    Ok((ix_discriminator, data))
}

pub fn serialize_metadata<T: AnchorSerialize>(metadata: &T) -> Result<Vec<u8>> {
    let mut bytes = Vec::new();
    metadata
        .serialize(&mut bytes)
        .map_err(|_| CommonCcipError::InvalidResolutionMetadata)?;
    Ok(bytes)
}

pub fn deserialize_metadata<T: AnchorDeserialize>(metadata: &[u8]) -> Result<T> {
    T::try_from_slice(metadata).map_err(|_| CommonCcipError::InvalidResolutionMetadata.into())
}

/// When performing nested resolution, normalizes away the signer bit from resolved accounts,
/// since the final outer instruction will not receive these accounts as signers.
/// Returns the normalized accounts and whether the expected signer was found.
pub fn normalize_nested_accounts(
    mut accounts: Vec<CcipAccountMeta>,
    expected_signer: Pubkey,
) -> Result<(Vec<CcipAccountMeta>, bool)> {
    let mut signer_seen = false;
    for account in accounts.iter_mut() {
        if account.pubkey == expected_signer {
            signer_seen = true;
            account.is_signer = false;
        } else {
            require!(
                !account.is_signer,
                CommonCcipError::InvalidNestedResolutionSigner
            );
        }
    }

    Ok((accounts, signer_seen))
}

pub fn checked_add_u8(lhs: u8, rhs: usize) -> Result<u8> {
    let rhs = u8::try_from(rhs).map_err(|_| CommonCcipError::AccountListLengthOverflow)?;
    lhs.checked_add(rhs)
        .ok_or(CommonCcipError::AccountListLengthOverflow.into())
}

/// Narrows an account list length into the `u8` that `AccountListLengths` stores. Sections whose
/// size is driven by external input (e.g. the token pool lookup table, which is only bounded from
/// below at registration time) must go through this instead of `as u8`, so an oversized list fails
/// closed rather than silently wrapping into a bogus section length.
pub fn checked_len_u8(len: usize) -> Result<u8> {
    u8::try_from(len).map_err(|_| CommonCcipError::AccountListLengthOverflow.into())
}

/// Solana's cap on an instruction's return data, which is how a [`ResolveAccountsResponse`]
/// travels back to the resolver. Every stage's whole response must fit in it.
pub const MAX_RETURN_DATA_BYTES: usize = 1024;

/// The longest `ask_again_with` any stage hands back. The widest lists are the nested-token
/// handoffs — `[config, pool_program, pool_signer, buffer?, ..inner_accounts]` — and the pool's
/// inner accounts are what make it variable; six covers every stage in both ramps with room for
/// two inner accounts, and [`MAX_ACCOUNTS_PER_RESPONSE`] is budgeted against it.
// TODO: two slots for inner accounts is restrictive for deeper nested CPIs. Revisit when
// implementing the Lombard/CCTP verifiers, which will likely need a larger handoff (and so a
// smaller `MAX_ACCOUNTS_PER_RESPONSE`, or a paginated `ask_again_with`).
pub const MAX_ASK_AGAIN_WITH: usize = 6;

/// Upper bound on the accounts one resolution response may carry, so that a stage emitting a
/// variable-length segment knows when to paginate. Budgeted against the worst-case response: 18
/// accounts spend 616 bytes, a full `ask_again_with` 196, one lookup table 36 and the
/// discriminator 9, which leaves [`MAX_METADATA_PER_RESPONSE`] for the stage's own metadata.
pub const MAX_ACCOUNTS_PER_RESPONSE: usize = 18;

/// What a response carrying a full page of accounts still has left for its metadata blob. Each
/// paginating stage owns a test asserting its metadata fits (the stage metadata in both ramps is
/// an order of magnitude smaller — the account-list lengths plus a cursor).
pub const MAX_METADATA_PER_RESPONSE: usize = MAX_RETURN_DATA_BYTES
    - ResolveAccountsResponse::size(MAX_ACCOUNTS_PER_RESPONSE, MAX_ASK_AGAIN_WITH, 1, 0);

/// Decides how much of a variable-length account segment this round emits.
///
/// `pending` is how many entries are still unemitted, `already_in_response` how many accounts this
/// round has queued already, and `trailing` the entries that must share the response with the last
/// page (they follow the segment, so they cannot spill into a later round). Returns
/// `(take, is_final_page)`.
///
/// A non-final page always makes progress: if the caller has already filled the response, there is
/// no page size that would advance the cursor, and pagination would spin forever — so that is an
/// error rather than a `take` of 0. Every caller avoids it by keeping its fixed prefix well under
/// [`MAX_ACCOUNTS_PER_RESPONSE`].
pub fn plan_account_page(
    pending: usize,
    already_in_response: usize,
    trailing: usize,
) -> Result<(usize, bool)> {
    let capacity = MAX_ACCOUNTS_PER_RESPONSE.saturating_sub(already_in_response);
    // SAFETY: both operands are account counts bounded by the caller's segment limit (64 messaging
    // accounts, 256 lookup-table entries) plus a `trailing` of at most 2, so the sum cannot
    // overflow a usize.
    if pending + trailing <= capacity {
        return Ok((pending, true));
    }

    let take = capacity.min(pending);
    require_gt!(take, 0, CommonCcipError::ResolutionPageCapacityExhausted);
    Ok((take, false))
}

/// Returns the `take` lookup table entries starting at `cursor`.
///
/// The cursor arrives in caller-supplied stage metadata and [`plan_account_page`] only bounds
/// `take` against what is left past it, so the range is checked against the table here rather than
/// assumed to be in bounds.
pub fn lookup_table_page(addresses: &[Pubkey], cursor: usize, take: usize) -> Result<&[Pubkey]> {
    cursor
        .checked_add(take)
        .and_then(|end| addresses.get(cursor..end))
        .ok_or(CommonCcipError::InvalidInputsLookupTableAccounts.into())
}

/// Checks a fully assembled response against the return-data cap.
///
/// Stages that wrap a nested resolver cannot bound their own response up front: the accounts, the
/// `ask_again_with` and part of the metadata (a CCV's `inner_cpi_resolution_inputs`) all come from
/// the inner program. Left unchecked, an oversized response fails when the runtime writes the
/// return data, with nothing to say which side overspent. Checking here turns that into a clear
/// error at the stage that assembled it.
///
/// Conservative by 8 bytes for a terminal response, since [`ResolveAccountsResponse::size`] always
/// charges for a `next_ix_discriminator` payload.
pub fn require_response_fits(response: &ResolveAccountsResponse) -> Result<()> {
    require_gte!(
        MAX_RETURN_DATA_BYTES,
        response.encoded_size(),
        CommonCcipError::ResolutionPageCapacityExhausted
    );
    Ok(())
}

#[cfg(test)]
mod pagination_tests {
    use super::*;

    /// The byte accounting behind [`MAX_ACCOUNTS_PER_RESPONSE`] is hand-written, so check it
    /// against the encoder that actually produces the response.
    #[test]
    fn response_size_matches_borsh() {
        let response = ResolveAccountsResponse {
            ask_again_with: vec![Pubkey::new_unique(); MAX_ASK_AGAIN_WITH],
            accounts_to_save: vec![CcipAccountMeta::default(); MAX_ACCOUNTS_PER_RESPONSE],
            lookup_tables_to_save: vec![Pubkey::new_unique()],
            next_ix_discriminator: Some([0u8; 8]),
            metadata: vec![0u8; MAX_METADATA_PER_RESPONSE],
        };

        let mut encoded = Vec::new();
        response.serialize(&mut encoded).unwrap();

        assert_eq!(encoded.len(), response.encoded_size());
        assert_eq!(
            encoded.len(),
            ResolveAccountsResponse::size(
                MAX_ACCOUNTS_PER_RESPONSE,
                MAX_ASK_AGAIN_WITH,
                1,
                MAX_METADATA_PER_RESPONSE
            )
        );
        // The worst case is exactly the budget: a full page of accounts, the widest handoff, a
        // lookup table and a metadata blob spending every remaining byte.
        assert_eq!(encoded.len(), MAX_RETURN_DATA_BYTES);
    }

    /// A stage wrapping a nested resolver cannot bound its response up front, so the assembled
    /// result is checked. Metadata is the axis that matters here: a response with a modest account
    /// page still busts the cap once the inner resolver's state grows.
    #[test]
    fn an_oversized_response_is_rejected() {
        let mut response = ResolveAccountsResponse {
            ask_again_with: vec![Pubkey::new_unique(); 4],
            accounts_to_save: vec![CcipAccountMeta::default(); 4],
            lookup_tables_to_save: vec![],
            next_ix_discriminator: Some([0u8; 8]),
            metadata: vec![0u8; 64],
        };
        assert!(require_response_fits(&response).is_ok());

        response.metadata = vec![0u8; MAX_RETURN_DATA_BYTES];
        assert!(require_response_fits(&response).is_err());

        // Accounts alone can bust it too, with no metadata at all.
        response.metadata = vec![];
        response.accounts_to_save = vec![CcipAccountMeta::default(); MAX_ACCOUNTS_PER_RESPONSE * 2];
        assert!(require_response_fits(&response).is_err());
    }

    #[test]
    fn a_page_that_fits_alongside_its_trailing_accounts_is_final() {
        assert_eq!(plan_account_page(0, 0, 0).unwrap(), (0, true));
        assert_eq!(plan_account_page(5, 10, 3).unwrap(), (5, true));

        // One more than fits alongside the trailing accounts: this page still empties the segment,
        // but it is not final, so the trailing accounts get a round of their own.
        assert_eq!(plan_account_page(6, 10, 3).unwrap(), (6, false));

        // With more pending than capacity, the page is capped at the free slots.
        assert_eq!(plan_account_page(20, 10, 3).unwrap(), (8, false));
    }

    /// A caller that fills the response leaves no room to advance the cursor. Returning `(0,
    /// false)` there would make the resolver re-enter the same stage forever, so it must fail.
    #[test]
    fn a_full_response_cannot_paginate() {
        assert!(plan_account_page(1, MAX_ACCOUNTS_PER_RESPONSE, 0).is_err());
        assert!(plan_account_page(1, MAX_ACCOUNTS_PER_RESPONSE + 1, 0).is_err());

        // With nothing pending there is nothing to advance, so a full response is still final.
        assert_eq!(
            plan_account_page(0, MAX_ACCOUNTS_PER_RESPONSE, 0).unwrap(),
            (0, true)
        );
    }

    /// The cursor is caller-supplied, so a page reaching past the table must fail rather than
    /// panic on the slice.
    #[test]
    fn a_lookup_table_page_must_lie_within_the_table() {
        let addresses = vec![Pubkey::new_unique(); 10];

        assert_eq!(lookup_table_page(&addresses, 0, 10).unwrap().len(), 10);
        assert_eq!(lookup_table_page(&addresses, 4, 6).unwrap().len(), 6);
        assert!(lookup_table_page(&addresses, 10, 0).unwrap().is_empty());

        assert!(lookup_table_page(&addresses, 0, 11).is_err());
        assert!(lookup_table_page(&addresses, 5, 6).is_err());
        assert!(lookup_table_page(&addresses, 11, 0).is_err());
        assert!(lookup_table_page(&addresses, usize::MAX, 1).is_err());
    }
}
