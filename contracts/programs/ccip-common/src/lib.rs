//! Contains common logic used across ccip programs. Common logic is versioned independently:
//! v1 of the common logic may not necessarily be used by v1 of each particular program.
use anchor_lang::prelude::*;
pub mod account_management;
pub mod ccvs;
pub mod cpi;
pub mod extra_args;
pub mod messages;
pub mod multisig;
pub mod observe;
pub mod resolution;
pub mod seed;
pub mod state;
pub mod types;
pub mod v1;
pub mod v2;

#[error_code]
pub enum CommonCcipError {
    #[msg("The given sequence interval is invalid")]
    // offset error code so that they don't clash with other programs
    // (Anchor's base custom error code 6000 + offset 4000 = start at 10000)
    InvalidSequenceInterval = 4000,
    #[msg("Invalid pool accounts")]
    InvalidInputsPoolAccounts,
    #[msg("Invalid token accounts")]
    InvalidInputsTokenAccounts,
    #[msg("Invalid Token Admin Registry account")]
    InvalidInputsTokenAdminRegistryAccounts,
    #[msg("Invalid LookupTable account")]
    InvalidInputsLookupTableAccounts,
    #[msg("Invalid LookupTable account writable access")]
    InvalidInputsLookupTableAccountWritable,
    #[msg("Invalid pool signer account")]
    InvalidInputsPoolSignerAccounts,

    #[msg("Invalid chain family selector")]
    InvalidChainFamilySelector,
    #[msg("Invalid encoding")]
    InvalidEncoding,
    #[msg("Invalid EVM address")]
    InvalidEVMAddress,
    #[msg("Invalid SVM address")]
    InvalidSVMAddress,
    #[msg("Invalid TVM address")]
    InvalidTVMAddress,
    #[msg("Invalid Aptos address")]
    InvalidAptosAddress,
    #[msg("Invalid Sui address")]
    InvalidSuiAddress,

    #[msg("Message decoding error")]
    MessageDecodingError,

    #[msg("Invalid instruction data for account resolution")]
    InvalidResolutionInstructionData,
    #[msg("Invalid metadata for account resolution")]
    InvalidResolutionMetadata,
    #[msg("Unexpected signer in nested account resolution response")]
    InvalidNestedResolutionSigner,
    #[msg("Account list length overflow")]
    AccountListLengthOverflow,

    #[msg("Invalid code version")]
    InvalidCodeVersion,
    #[msg("Invalid pool interface version")]
    InvalidPoolInterfaceVersion,

    #[msg("Invalid ProtocolAmount")]
    InvalidProtocolAmount,
    #[msg("MissingCpiReturnData")]
    MissingCpiReturnData,
    #[msg("InvalidCpiReturnProgram")]
    InvalidCpiReturnProgram,
    #[msg("CPI target program does not match the expected program")]
    InvalidCpiTargetProgram,
    #[msg("InvalidAccountListLengths")]
    InvalidAccountListLengths,
    #[msg("Invalid Account version")]
    InvalidAccountVersion,
    #[msg("Invalid Account data")]
    InvalidAccountData,
    #[msg("Invalid receiver version")]
    InvalidReceiverVersion,
    #[msg("Invalid receiver registry account")]
    InvalidReceiverRegistry,

    #[msg("Invalid USD value conversion")]
    InvalidUsdConversion,

    #[msg("Account resolution response is too full to paginate")]
    ResolutionPageCapacityExhausted,

    #[msg("Message requires more CCVs than account resolution can describe")]
    TooManyCcvs,

    #[msg("Invalid Canton address")]
    InvalidCantonAddress,
}

// Sentinel value used for CCV lists that signals default CCVs must be included.
pub const INCLUDE_DEFAULT_CCVS: Pubkey = Pubkey::new_from_array([0; 32]);

/// Collapses a token pool's required-CCV response so that a pool specifying no CCVs still
/// enforces the lane defaults: an empty list becomes the [`INCLUDE_DEFAULT_CCVS`] sentinel,
/// which the CCV merge/aggregation later expands to the default CCVs. Mirrors EVM
/// `_getCCVsForPool`, where an empty pool response falls back to the destination defaults.
pub fn pool_ccvs_with_default_fallback(ccvs: Vec<Pubkey>) -> Vec<Pubkey> {
    if ccvs.is_empty() {
        vec![INCLUDE_DEFAULT_CCVS]
    } else {
        ccvs
    }
}

// bytes4(keccak256("CCIP ChainFamilySelector APTOS"));
pub const CHAIN_FAMILY_SELECTOR_APTOS: u32 = 0xac77ffec;

// bytes4(keccak256("CCIP ChainFamilySelector EVM"));
pub const CHAIN_FAMILY_SELECTOR_EVM: u32 = 0x2812d52c;

// bytes4(keccak256("CCIP ChainFamilySelector SUI"));
pub const CHAIN_FAMILY_SELECTOR_SUI: u32 = 0xc4e05953;

// bytes4(keccak256("CCIP ChainFamilySelector SVM"));
pub const CHAIN_FAMILY_SELECTOR_SVM: u32 = 0x1e10bdc4;

// byte4(keccak256("CCIP ChainFamilySelector TVM"));
pub const CHAIN_FAMILY_SELECTOR_TVM: u32 = 0x647e2ba9;

// bytes4(keccak256("CCIP ChainFamilySelector Canton"));
pub const CHAIN_FAMILY_SELECTOR_CANTON: u32 = 0xdfafaf4b;

// Duplicates the router ID to declare router accounts that must be visible from the common crate,
// avoiding a circular dependency. This means this crate may only declare accounts that belong
// to the router, and no other program.
declare_id!("Ccip842gzYHhvdDkSyi2YVCoAWPbYJoApMFzSxQroE9C");
// null contract required for IDL + gobinding generation
#[program]
pub mod ccip_common {
    use super::*;

    // Common interface shared across contracts. Defining it here allows us to reuse types when building the ixs for
    // each contract.
    pub fn resolve_accounts_start(
        _ctx: Context<ResolveAccountsStart>,
        _params: resolution::ResolveAccountsParams,
    ) -> Result<resolution::ResolveAccountsResponse> {
        Ok(resolution::ResolveAccountsResponse {
            ask_again_with: vec![],
            accounts_to_save: vec![],
            lookup_tables_to_save: vec![],
            next_ix_discriminator: None,
            metadata: vec![],
        })
    }
}

#[derive(Accounts)]
pub struct ResolveAccountsStart<'info> {
    /// CHECK: Only the config account of each contract is used in the common interface to begin account resolution
    /// This here is only used to generate common bindings, though each contract will define its own context.
    pub config: UncheckedAccount<'info>,
}

pub mod router_accounts {
    use super::*;

    // PDA seeds: [RECEIVER_REGISTRY, receiver_program]
    //
    // Router-owned record of a receiver's CCIP interface version. The OffRamp consults
    // it to pick the v1 vs v2 receiver flow (CCV resolution + delivery CPI) instead of trusting an
    // executor-supplied hint. An *absent* account means "v1" (see [`receiver_is_v2_from_account`]),
    // so only v2 receivers ever need to register.
    #[account]
    #[derive(InitSpace, PartialEq, Debug)]
    pub struct ReceiverRegistry {
        pub bump: u8,
        // Schema version of THIS account (not the receiver's interface). Currently always 1.
        pub version: u8,
        // The receiver PDA seeds, stored so the account is self-describing.
        pub receiver_program: Pubkey,
        // The receiver's CCIP interface version. The only valid values are 1 and 2.
        pub receiver_version: u8,
    }

    impl ReceiverRegistry {
        pub const LATEST_VERSION: u8 = 1; // schema version of this PDA
        pub const V1: u8 = 1;
        pub const V2: u8 = 2;

        pub fn validate_receiver_version(receiver_version: u8) -> Result<()> {
            require!(
                receiver_version == Self::V1 || receiver_version == Self::V2,
                CommonCcipError::InvalidReceiverVersion
            );
            Ok(())
        }

        pub fn is_v2(&self) -> bool {
            self.receiver_version == Self::V2
        }
    }

    /// Decides whether a receiver uses the CCIP v2 interface, from its (possibly absent)
    /// [`ReceiverRegistry`] account.
    ///
    /// Absence detection is lamport-agnostic per the account-initialization invariants
    /// (INV-ACCT-INIT-2/3/4): a System-owned account is treated as absent regardless of its lamport
    /// balance, so an unprivileged third party pre-funding the (public-seed) PDA cannot flip the
    /// result. When present, the account's key, owner (the router), discriminator, and schema
    /// `version` are all validated before its `receiver_version` is trusted.
    ///
    /// * `registry` — the account passed in at the registry PDA address (may be uninitialized).
    /// * `receiver` — the receiver program being checked.
    /// * `router_program_id` — the router program ID the registry PDA is derived under.
    pub fn receiver_is_v2_from_account(
        registry: &AccountInfo,
        receiver: Pubkey,
        router_program_id: Pubkey,
    ) -> Result<bool> {
        let (expected_pda, _) = Pubkey::find_program_address(
            &[seed::RECEIVER_REGISTRY, receiver.as_ref()],
            &router_program_id,
        );
        require_keys_eq!(
            registry.key(),
            expected_pda,
            CommonCcipError::InvalidReceiverRegistry
        );

        // If Absent -> v1. This is the only correct "absent" signal (a lamport
        // prefund leaves ownership untouched, so it cannot be used to grief this decision).
        if registry.owner == &anchor_lang::system_program::ID {
            return Ok(false);
        }

        let data = registry.try_borrow_data()?;
        // `try_deserialize` validates the 8-byte discriminator (identity of the type).
        let parsed = ReceiverRegistry::try_deserialize(&mut &data[..])
            .map_err(|_| CommonCcipError::InvalidReceiverRegistry)?;
        require_eq!(
            parsed.version,
            ReceiverRegistry::LATEST_VERSION,
            CommonCcipError::InvalidReceiverRegistry
        );

        Ok(parsed.is_v2())
    }

    #[derive(AnchorDeserialize, AnchorSerialize, Debug, PartialEq, Clone, InitSpace)]
    #[borsh(use_discriminant = false)]
    pub enum ResolutionType {
        // No dynamic resolution. Includes all entries of the LUT, without making CPIs.
        NoResolution = 0,
        // Includes all LUT entries + CPIs into the pool to resolve additional accounts.
        Standard = 1,
        // A middle ground: includes only the mandatory LUT entries, ignoring the additional ones for that pool,
        // and then CPIs into the pool to resolve additional accounts. Useful for cases like CCTP, where
        // the pool used to integrate with CCTPv1 (1.6 lanes) but now (2.0) no longer integrates with it
        // because the verifier takes care of integrating with CCTPv2 (and the CCTPv1 accounts are useless for that,
        // and they would bloat the effective list of accounts of the tx without being useful).
        WithoutExtendedLookupTable = 2,
    }

    impl ResolutionType {
        pub fn is_enabled(&self) -> bool {
            !matches!(self, ResolutionType::NoResolution)
        }

        pub fn use_lookup_table_additional_entries(&self) -> bool {
            !matches!(self, ResolutionType::WithoutExtendedLookupTable)
        }
    }

    #[account]
    #[derive(InitSpace, PartialEq, Debug)]
    pub struct TokenAdminRegistry {
        // --- V1 original fields ---
        pub version: u8,
        pub administrator: Pubkey,
        pub pending_administrator: Pubkey,
        pub lookup_table: Pubkey,
        // binary representation of indexes that are writable in token pool lookup table
        // lookup table can store 256 addresses
        pub writable_indexes: [u128; 2],
        pub mint: Pubkey,

        // --- V2 additional fields (renamed here for v3, narrowed back to bool in v4) ---
        // if true, the pool supports calling `derive_accounts_lock_or_burn` and
        // `derive_accounts_release_or_mint` to derive the list of accounts and LUTs
        // needed to interact with it on 1.6-compatible flows. If the pool implements the v2 interface
        // and is invoked on a 2.0+ lane, then it will invoke the newer `resolve_accounts_start` method.
        pub account_resolution_enabled: bool,

        // --- V3 additional fields ---
        pub bump: u8,
        // Pool interface version that the particular pool implements. It's just a coincidence that 1 & 2 line
        // up with protocol version 1.6 & 2.0, there could be other pool versions independently of protocol version.
        // 1 = CCIP v1.6
        // 2 = CCIP v2.0 and above
        pub interface_version: u8,

        // --- V4 additional fields ---
        // Only meaningful when `account_resolution_enabled`: if true, 2.0 flows include only the mandatory
        // entries of the pool's lookup table and skip the additional ones (see
        // `ResolutionType::WithoutExtendedLookupTable`). Negative sense on purpose, so that zero-filled
        // bytes (from realloc or from older layouts) mean "include all entries", the pre-v4 behavior.
        // Appended at the end, so 1.6 programs (which ignore trailing bytes) never see it.
        pub skip_lookup_table_additional_entries: bool,
    }

    impl TokenAdminRegistry {
        pub const LATEST_VERSION: u8 = 4; // this refers to the pda data layout version (the schema)

        // safe defaults for new values
        pub const DEFAULT_ACCOUNT_RESOLUTION_ENABLED: bool = false;
        pub const DEFAULT_SKIP_LOOKUP_TABLE_ADDITIONAL_ENTRIES: bool = false;
        pub const DEFAULT_INTERFACE_VERSION: u8 = 1; // refers to  interface implemented by the pool (v1 is compatible with CCIP 1.6)
        pub const DEFAULT_EMPTY_BUMP: u8 = 0; // this is just a placeholder, 0 means that it is not actually set

        /// The resolution mode encoded by the two stored fields.
        pub fn account_resolution(&self) -> ResolutionType {
            match (
                self.account_resolution_enabled,
                self.skip_lookup_table_additional_entries,
            ) {
                (false, _) => ResolutionType::NoResolution,
                (true, false) => ResolutionType::Standard,
                (true, true) => ResolutionType::WithoutExtendedLookupTable,
            }
        }

        /// Stores `resolution` across the two fields, keeping `account_resolution_enabled` a bool
        /// that the 1.6 offramp can decode.
        pub fn set_account_resolution(&mut self, resolution: &ResolutionType) {
            self.account_resolution_enabled = resolution.is_enabled();
            self.skip_lookup_table_additional_entries =
                !resolution.use_lookup_table_additional_entries();
        }

        pub fn validate_interface_version(version: u8) -> Result<()> {
            // As of CCIP 2.0, there's only a pool's v1 interface (1.6-compatible) and v2 interface
            // (adds CCVs, fees, finality, and changes I/O structs...)
            require!(
                (1..=2).contains(&version),
                CommonCcipError::InvalidPoolInterfaceVersion
            );
            Ok(())
        }
    }

    /// Older `TokenAdminRegistry` layouts, kept deserializable so accounts that have not been
    /// migrated yet can still be read.
    ///
    /// `VERSION` is the schema version an account with this layout must have persisted, and
    /// `version()` exposes the version byte actually read from the account. Comparing the two
    /// detects an account whose length matches one schema version but whose stored version byte
    /// disagrees.
    pub(super) trait OldTokenAdminRegistry: AnchorDeserialize {
        const CANONICAL_VERSION: u8;

        fn stored_version(&self) -> u8;
    }

    #[derive(AnchorDeserialize, InitSpace)]
    pub(super) struct TokenAdminRegistryV3 {
        pub version: u8,
        pub administrator: Pubkey,
        pub pending_administrator: Pubkey,
        pub lookup_table: Pubkey,
        pub writable_indexes: [u128; 2],
        pub mint: Pubkey,
        // v3 stored the whole enum in this byte, so it could hold 2 (unreadable by the 1.6 offramp).
        pub account_resolution: ResolutionType,
        pub bump: u8,
        pub interface_version: u8,
    }

    impl OldTokenAdminRegistry for TokenAdminRegistryV3 {
        const CANONICAL_VERSION: u8 = 3;

        fn stored_version(&self) -> u8 {
            self.version
        }
    }

    impl From<TokenAdminRegistryV3> for TokenAdminRegistry {
        fn from(v3: TokenAdminRegistryV3) -> TokenAdminRegistry {
            // Translating every variant (not only 0/1) keeps a v3 registry that still holds 2 both readable
            // and migratable; otherwise it could neither be upgraded nor fixed through the (v4-only) setter.
            TokenAdminRegistry {
                version: 3,
                administrator: v3.administrator,
                pending_administrator: v3.pending_administrator,
                lookup_table: v3.lookup_table,
                writable_indexes: v3.writable_indexes,
                mint: v3.mint,
                account_resolution_enabled: v3.account_resolution.is_enabled(),
                interface_version: v3.interface_version,
                bump: v3.bump,
                skip_lookup_table_additional_entries: !v3
                    .account_resolution
                    .use_lookup_table_additional_entries(),
            }
        }
    }

    #[derive(AnchorDeserialize, InitSpace)]
    pub(super) struct TokenAdminRegistryV2 {
        pub version: u8,
        pub administrator: Pubkey,
        pub pending_administrator: Pubkey,
        pub lookup_table: Pubkey,
        // binary representation of indexes that are writable in token pool lookup table
        // lookup table can store 256 addresses
        pub writable_indexes: [u128; 2],
        pub mint: Pubkey,
        // if true, the pool supports calling `derive_accounts_lock_or_burn` and
        // `derive_accounts_release_or_mint` to derive the list of accounts and LUTs
        // needed to interact with it.
        pub supports_auto_derivation: bool,
    }

    impl OldTokenAdminRegistry for TokenAdminRegistryV2 {
        const CANONICAL_VERSION: u8 = 2;

        fn stored_version(&self) -> u8 {
            self.version
        }
    }

    impl From<TokenAdminRegistryV2> for TokenAdminRegistry {
        fn from(v2: TokenAdminRegistryV2) -> TokenAdminRegistry {
            TokenAdminRegistry {
                version: 2,
                administrator: v2.administrator,
                pending_administrator: v2.pending_administrator,
                lookup_table: v2.lookup_table,
                writable_indexes: v2.writable_indexes,
                mint: v2.mint,
                account_resolution_enabled: v2.supports_auto_derivation, // same byte, renamed
                interface_version: TokenAdminRegistry::DEFAULT_INTERFACE_VERSION, // v2 didn't have this field, so use the safe default
                bump: TokenAdminRegistry::DEFAULT_EMPTY_BUMP, // v2 didn't have this field, so use the safe default
                skip_lookup_table_additional_entries:
                    TokenAdminRegistry::DEFAULT_SKIP_LOOKUP_TABLE_ADDITIONAL_ENTRIES, // added in v4
            }
        }
    }

    #[derive(AnchorDeserialize, InitSpace)]
    pub(super) struct TokenAdminRegistryV1 {
        pub version: u8,
        pub administrator: Pubkey,
        pub pending_administrator: Pubkey,
        pub lookup_table: Pubkey,
        pub writable_indexes: [u128; 2],
        pub mint: Pubkey,
    }

    impl OldTokenAdminRegistry for TokenAdminRegistryV1 {
        const CANONICAL_VERSION: u8 = 1;

        fn stored_version(&self) -> u8 {
            self.version
        }
    }

    impl From<TokenAdminRegistryV1> for TokenAdminRegistry {
        fn from(v1: TokenAdminRegistryV1) -> TokenAdminRegistry {
            TokenAdminRegistry {
                version: 1,
                administrator: v1.administrator,
                pending_administrator: v1.pending_administrator,
                lookup_table: v1.lookup_table,
                writable_indexes: v1.writable_indexes,
                mint: v1.mint,
                account_resolution_enabled: TokenAdminRegistry::DEFAULT_ACCOUNT_RESOLUTION_ENABLED,
                interface_version: TokenAdminRegistry::DEFAULT_INTERFACE_VERSION,
                bump: TokenAdminRegistry::DEFAULT_EMPTY_BUMP,
                skip_lookup_table_additional_entries:
                    TokenAdminRegistry::DEFAULT_SKIP_LOOKUP_TABLE_ADDITIONAL_ENTRIES,
            }
        }
    }

    #[cfg(test)]
    mod tests {
        use super::*;

        #[test]
        fn validate_receiver_version_accepts_only_v1_and_v2() {
            ReceiverRegistry::validate_receiver_version(1).unwrap();
            ReceiverRegistry::validate_receiver_version(2).unwrap();

            for bad in [0, 3, 20] {
                let err = ReceiverRegistry::validate_receiver_version(bad).unwrap_err();
                assert_eq!(err, CommonCcipError::InvalidReceiverVersion.into());
            }
        }

        #[test]
        fn is_v2_reflects_receiver_version() {
            let mut reg = ReceiverRegistry {
                bump: 0,
                version: ReceiverRegistry::LATEST_VERSION,
                receiver_program: Pubkey::default(),
                receiver_version: ReceiverRegistry::V2,
            };
            assert!(reg.is_v2());
            reg.receiver_version = ReceiverRegistry::V1;
            assert!(!reg.is_v2());
        }
    }
}

/// Anchor 0.32 only includes types in the IDL when they are referenced by an instruction
/// argument, an #[account] context, or an #[event] struct. Types that are deserialized from
/// raw bytes at runtime (not part of any instruction signature) would be missing from the IDL
/// and Go bindings. This cfg-guarded event forces Anchor to include those types. It is never
/// emitted at runtime — it only exists during `anchor idl build`.
#[cfg(feature = "idl-build")]
#[event]
pub struct IdlBuildTypeExport {
    pub token_admin_registry: router_accounts::TokenAdminRegistry,
    pub receiver_registry: router_accounts::ReceiverRegistry,
    pub ccip_account_meta: resolution::CcipAccountMeta,
    pub resolve_accounts_params: resolution::ResolveAccountsParams,
    pub resolve_accounts_response: resolution::ResolveAccountsResponse,
    pub svm_dest_blob: messages::SVMDestBlob,
    pub generic_extra_args_v3: crate::extra_args::GenericExtraArgsV3,
    pub generic_extra_args_v2: crate::extra_args::GenericExtraArgsV2,
    pub svm_extra_args_v1: crate::extra_args::SVMExtraArgsV1,
    pub sui_extra_args_v1: crate::extra_args::SuiExtraArgsV1,
    pub usd_18_decimals: types::Usd18Decimals,
}
