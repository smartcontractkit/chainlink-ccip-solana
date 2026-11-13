use anchor_lang::prelude::*;
use anchor_lang::Discriminator;
use anchor_spl::associated_token;
use anchor_spl::associated_token::get_associated_token_address_with_program_id;
use anchor_spl::token_interface::{Mint, TokenAccount, TokenInterface};
use ethnum::U256;
use solana_address_lookup_table_interface as address_lookup_table;
use solana_address_lookup_table_interface::state::AddressLookupTable;

use crate::messages::SVMTokenReceiverUsage;
use crate::router_accounts::{
    OldTokenAdminRegistry, TokenAdminRegistry, TokenAdminRegistryV1, TokenAdminRegistryV2,
    TokenAdminRegistryV3,
};
use crate::seed;
use crate::CommonCcipError;

pub const MIN_TOKEN_POOL_ACCOUNTS: usize = 13; // see TokenAccounts struct for all required accounts
const U160_MAX: U256 = U256::from_words(u32::MAX as u128, u128::MAX);

pub const V1_TOKEN_ADMIN_REGISTRY_SIZE: usize =
    TokenAdminRegistry::DISCRIMINATOR.len() + TokenAdminRegistryV1::INIT_SPACE; // for migration v1->v4 of the TokenAdminRegistry
pub const V2_TOKEN_ADMIN_REGISTRY_SIZE: usize =
    TokenAdminRegistry::DISCRIMINATOR.len() + TokenAdminRegistryV2::INIT_SPACE; // for migration v2->v4 of the TokenAdminRegistry
pub const V3_TOKEN_ADMIN_REGISTRY_SIZE: usize =
    TokenAdminRegistry::DISCRIMINATOR.len() + TokenAdminRegistryV3::INIT_SPACE; // for migration v3->v4 of the TokenAdminRegistry

/// 32-byte big-endian uint256 representation of 11 (0x0b)
pub const APTOS_PRECOMPILE_SPACE: [u8; 32] = [
    0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
    0x0b,
];

/// 32-byte big-endian uint256 representation of 1024 (0x0400)
pub const EVM_PRECOMPILE_SPACE: [u8; 32] = [
    0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0x04,
    0x00,
];

/// 32-byte big-endian uint256 representation of 0xdee9
pub const SUI_PRECOMPILE_SPACE: [u8; 32] = [
    0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0xde,
    0xe9,
];

pub struct CcipV1OnrampTokenAccounts<'a> {
    pub common: CcipV1OfframpOnrampCommonTokenAccounts<'a>,
    pub ccip_router_pool_signer_bump: u8,
    pub remaining_accounts: &'a [AccountInfo<'a>],
}

pub struct CcipV1OfframpTokenAccounts<'a> {
    pub common: CcipV1OfframpOnrampCommonTokenAccounts<'a>,
    pub ccip_offramp_pool_signer: &'a AccountInfo<'a>,

    // as these ones are optional, they don't count for the MIN_TOKEN_POOL_ACCOUNTS
    pub user_receiver_wallet: Option<&'a AccountInfo<'a>>,
    pub ata_program: Option<&'a AccountInfo<'a>>,

    pub ccip_router_pool_signer_bump: u8,
    pub ccip_offramp_pool_signer_bump: u8,

    pub remaining_accounts: &'a [AccountInfo<'a>],
}

#[allow(clippy::too_many_arguments)]
pub fn validate_and_parse_onramp_token_accounts<'info>(
    token_sender_or_receiver: Pubkey,
    message_mint: Pubkey,
    chain_selector: u64,
    router: Pubkey,
    fee_quoter: Pubkey,
    accounts: &'info [AccountInfo<'info>],
    validate_sender_ata: bool,
) -> Result<CcipV1OnrampTokenAccounts<'info>> {
    let bumps = &mut <OnrampTokenAccountsValidationContext as anchor_lang::Bumps>::Bumps::default();
    let reallocs = &mut std::collections::BTreeSet::new();

    // `try_accounts` advances the slice it is given past every account it consumes, so validate
    // against a throwaway copy and keep `accounts` pointing at the start for extraction below.
    let mut validation_accounts = accounts;

    // leveraging Anchor's account context validation
    // Instead of manually checking each account (ownership, PDA derivation, constraints),
    // we're using Anchor's `try_accounts` to perform these validations based on the
    // constraints defined in the `OnrampTokenAccountsValidationContext` account context struct
    OnrampTokenAccountsValidationContext::try_accounts(
        &router,
        &mut validation_accounts,
        &[
            token_sender_or_receiver.as_ref(),
            message_mint.as_ref(),
            &chain_selector.to_le_bytes(),
            router.as_ref(),
            fee_quoter.as_ref(),
            &[u8::from(validate_sender_ata)],
        ]
        .concat(),
        bumps,
        reallocs,
    )?;

    let mut accounts_iter = accounts.iter();
    // We use the iterator here (rather than the context directly) to ensure we accurately get to
    // the point where additional accounts begin.
    let common_accounts = CcipV1OfframpOnrampCommonTokenAccounts::from_iter(&mut accounts_iter)?;
    let remaining_accounts = accounts_iter.as_slice();

    // Additional validations that can't be expressed in the account context
    common_accounts.additional_validations(remaining_accounts)?;

    Ok(CcipV1OnrampTokenAccounts {
        ccip_router_pool_signer_bump: bumps.ccip_router_pool_signer,
        remaining_accounts,
        common: common_accounts,
    })
}

#[allow(clippy::too_many_arguments)]
pub fn validate_and_parse_offramp_token_accounts<'info>(
    token_receiver: Pubkey,
    message_mint: Pubkey,
    chain_selector: u64,
    router: Pubkey,
    fee_quoter: Pubkey,
    offramp: Pubkey,
    token_receiver_usage: &SVMTokenReceiverUsage,
    accounts: &'info [AccountInfo<'info>],
) -> Result<CcipV1OfframpTokenAccounts<'info>> {
    let bumps =
        &mut <OfframpTokenAccountsValidationContext as anchor_lang::Bumps>::Bumps::default();
    let reallocs = &mut std::collections::BTreeSet::new();

    // `try_accounts` advances the slice it is given past every account it consumes, so validate
    // against a throwaway copy and keep `accounts` pointing at the start for extraction below.
    let mut validation_accounts = accounts;

    // leveraging Anchor's account context validation
    // Instead of manually checking each account (ownership, PDA derivation, constraints),
    // we're using Anchor's `try_accounts` to perform these validations based on the
    // constraints defined in the `OfframpTokenAccountsValidationContext` account context struct
    OfframpTokenAccountsValidationContext::try_accounts(
        &offramp,
        &mut validation_accounts,
        &[
            token_receiver.as_ref(),
            message_mint.as_ref(),
            &chain_selector.to_le_bytes(),
            router.as_ref(),
            fee_quoter.as_ref(),
            offramp.as_ref(),
            // SAFETY: `SVMTokenReceiverUsage` is a fieldless three-variant enum, so its
            // discriminant is 0..=2 and always fits in a `u8`.
            &[*token_receiver_usage as u8],
        ]
        .concat(),
        bumps,
        reallocs,
    )?;

    let mut accounts_iter = accounts.iter();
    // The offramp starts with the pool signer, then every element after that until ATA creation
    // is common with the onramp.
    let ccip_offramp_pool_signer = next_account_info(&mut accounts_iter)?;
    // We use the iterator here (rather than the context directly) to ensure we accurately get to
    // the point where additional accounts begin.
    let common_accounts = CcipV1OfframpOnrampCommonTokenAccounts::from_iter(&mut accounts_iter)?;

    // Only if creating the receiver ATA, then the wallet account is expected to come next. In this case,
    // the message's token_receiver is the wallet, and the user_token_account is the ATA. To create the ATA,
    // we also need the wallet account (as the ATA program requires it as input).
    let (user_receiver_wallet, ata_program) = if token_receiver_usage.should_create_ata() {
        let wallet = next_account_info(&mut accounts_iter)?;
        require_keys_eq!(
            wallet.key(),
            token_receiver,
            CommonCcipError::InvalidInputsTokenAccounts
        );
        let ata_program = next_account_info(&mut accounts_iter)?;
        require_keys_eq!(
            ata_program.key(),
            associated_token::ID,
            CommonCcipError::InvalidInputsTokenAccounts
        );
        (Some(wallet), Some(ata_program))
    } else {
        (None, None)
    };

    // collect remaining accounts
    let remaining_accounts = accounts_iter.as_slice();

    // Additional validations that can't be expressed in the account context
    common_accounts.additional_validations(remaining_accounts)?;

    Ok(CcipV1OfframpTokenAccounts {
        common: common_accounts,
        user_receiver_wallet,
        ata_program,
        ccip_router_pool_signer_bump: bumps.ccip_router_pool_signer,
        ccip_offramp_pool_signer_bump: bumps.ccip_offramp_pool_signer,
        remaining_accounts,
        ccip_offramp_pool_signer,
    })
}

pub struct CcipV1OfframpOnrampCommonTokenAccounts<'a> {
    // accounts based on user or chain
    pub user_token_account: &'a AccountInfo<'a>,
    /// The fee quoter's PER_CHAIN_PER_TOKEN_CONFIG PDA for (dest chain, mint).
    pub per_chain_per_token_config: &'a AccountInfo<'a>,
    pub pool_chain_config: &'a AccountInfo<'a>,

    // constant accounts for any pool interaction
    pub lookup_table: &'a AccountInfo<'a>,
    pub token_admin_registry: &'a AccountInfo<'a>,
    pub pool_program: &'a AccountInfo<'a>,
    pub pool_config: &'a AccountInfo<'a>,
    pub pool_token_account: &'a AccountInfo<'a>,
    pub pool_signer: &'a AccountInfo<'a>,
    pub token_program: &'a AccountInfo<'a>,
    pub mint: &'a AccountInfo<'a>,
    pub fee_token_config: &'a AccountInfo<'a>,
    pub ccip_router_pool_signer: &'a AccountInfo<'a>,
}

impl<'a> CcipV1OfframpOnrampCommonTokenAccounts<'a> {
    // Extracted by re-iterating the original (non-advanced) input slice rather than from the
    // validation context. `try_accounts` only validates; downstream pool CPIs need references with
    // the full input lifetime, which Anchor's typed wrappers (InterfaceAccount/Interface) don't
    // expose -- their only accessor, `to_account_info()`, returns an owned clone.
    fn from_iter(iter: &mut impl Iterator<Item = &'a AccountInfo<'a>>) -> Result<Self> {
        Ok(Self {
            user_token_account: next_account_info(iter)?,
            per_chain_per_token_config: next_account_info(iter)?,
            pool_chain_config: next_account_info(iter)?,
            lookup_table: next_account_info(iter)?,
            token_admin_registry: next_account_info(iter)?,
            pool_program: next_account_info(iter)?,
            pool_config: next_account_info(iter)?,
            pool_token_account: next_account_info(iter)?,
            pool_signer: next_account_info(iter)?,
            token_program: next_account_info(iter)?,
            mint: next_account_info(iter)?,
            fee_token_config: next_account_info(iter)?,
            ccip_router_pool_signer: next_account_info(iter)?,
        })
    }

    fn required_entries(&self) -> impl Iterator<Item = &'a AccountInfo<'a>> {
        [
            self.lookup_table,
            self.token_admin_registry,
            self.pool_program,
            self.pool_config,
            self.pool_token_account,
            self.pool_signer,
            self.token_program,
            self.mint,
            self.fee_token_config,
            self.ccip_router_pool_signer,
        ]
        .into_iter()
    }

    // Validations that can't be performed in an Anchor context.
    fn additional_validations(&self, remaining_accounts: &'a [AccountInfo<'a>]) -> Result<()> {
        {
            // Check Lookup Table Address configured in TokenAdminRegistry
            let token_admin_registry_account =
                load_token_admin_registry_checked(self.token_admin_registry, self.mint.key())?;
            require_keys_eq!(
                token_admin_registry_account.lookup_table,
                self.lookup_table.key(),
                CommonCcipError::InvalidInputsLookupTableAccounts
            );

            // Check Lookup Table Entries
            let lookup_table_data = &mut &self.lookup_table.data.borrow()[..];
            let lookup_table_account: AddressLookupTable =
                AddressLookupTable::deserialize(lookup_table_data)
                    .map_err(|_| CommonCcipError::InvalidInputsLookupTableAccounts)?;

            // reconstruct + validate expected values in token pool lookup table
            // base set of constant accounts (10)
            // + additional constant accounts (remaining_accounts) that are not required but may be used for additional token pool functionality (like CPI)
            {
                // validate pool addresses
                let mut expected_keys: Vec<Pubkey> =
                    self.required_entries().map(|x| x.key()).collect();
                let mut remaining_keys: Vec<Pubkey> =
                    remaining_accounts.iter().map(|x| x.key()).collect();
                expected_keys.append(&mut remaining_keys);
                if token_admin_registry_account
                    .account_resolution()
                    .is_enabled()
                {
                    // When auto-derivation is supported, there may be more accounts than what the lookup table contains.
                    // On those extra accounts, given that the token pool supports derivation, we also trust that the pool
                    // performs the required checks.
                    let n = lookup_table_account.addresses.len();
                    // The LUT may not hold more entries than the required ones plus the
                    // caller-supplied remaining accounts, or the prefix comparison below
                    // would be out of bounds.
                    require!(
                        n <= expected_keys.len(),
                        CommonCcipError::InvalidInputsLookupTableAccounts
                    );
                    require!(
                        lookup_table_account.addresses.as_ref() == expected_keys[..n].to_vec(),
                        CommonCcipError::InvalidInputsLookupTableAccounts
                    );
                } else {
                    // When the pool does not support auto-derivation, the lookup table must contain all the expected accounts,
                    // and CCIP is expected to validate them.
                    require!(
                        lookup_table_account.addresses.as_ref() == expected_keys,
                        CommonCcipError::InvalidInputsLookupTableAccounts
                    );
                }
            }
            {
                // validate pool address writable
                // token admin registry contains an array (binary) of indexes that are writable
                // check that the writability of the passed accounts match the writable configuration (using indexes)
                let mut expected_is_writable: Vec<bool> =
                    self.required_entries().map(|x| x.is_writable).collect();

                let mut remaining_is_writable: Vec<bool> = if token_admin_registry_account
                    .account_resolution()
                    .is_enabled()
                {
                    // when auto-derivation is supported, we only validate the accounts present in the LUT
                    // as the pool is expected to validate the rest of the accounts. The LUT must contain
                    // the required entries, plus it may contain some additional ones that are static,
                    // and finally the auto-derivation may add more accounts at the end (static or message-dependant).
                    let lut_len = lookup_table_account.addresses.len();
                    let end = lut_len
                        .checked_sub(self.required_entries().count())
                        .ok_or(CommonCcipError::InvalidInputsLookupTableAccounts)?;

                    remaining_accounts[..end]
                        .iter()
                        .map(|x| x.is_writable)
                        .collect()
                } else {
                    // when auto-derivation is not supported, we validate all remaining accounts
                    // as they are all expected to be in the LUT
                    remaining_accounts.iter().map(|x| x.is_writable).collect()
                };

                expected_is_writable.append(&mut remaining_is_writable);
                for (i, is_writable) in expected_is_writable.iter().enumerate() {
                    let expected =
                    // SAFETY: i is bounded by LUT entries + remaining_accounts (max ~64)
                    token_admin_registry_writable::is(&token_admin_registry_account, i as u8);
                    let actual = *is_writable;
                    if expected != actual {
                        msg!(
                            "Expected account at index {} to be writable {}, but it is {}",
                            i,
                            expected,
                            actual
                        );
                        return Err(CommonCcipError::InvalidInputsLookupTableAccountWritable.into());
                    }
                }
            }

            Ok(())
        }
    }
}

pub fn load_token_admin_registry_checked(
    token_admin_registry: &AccountInfo<'_>,
    token: Pubkey,
) -> Result<TokenAdminRegistry> {
    let expected_registry = seed::token_admin_registry(&token, crate::ID); // ccip-common crate must have the same ID as ccip-router, which is the actual program that owns it

    require_keys_eq!(
        token_admin_registry.key(),
        expected_registry,
        CommonCcipError::InvalidInputsTokenAdminRegistryAccounts
    );

    Ok(match token_admin_registry.data_len() {
        V1_TOKEN_ADMIN_REGISTRY_SIZE => {
            load_old_token_admin_registry::<TokenAdminRegistryV1>(token_admin_registry)?
        }
        V2_TOKEN_ADMIN_REGISTRY_SIZE => {
            load_old_token_admin_registry::<TokenAdminRegistryV2>(token_admin_registry)?
        }
        V3_TOKEN_ADMIN_REGISTRY_SIZE => {
            load_old_token_admin_registry::<TokenAdminRegistryV3>(token_admin_registry)?
        }
        _ => load_current_token_admin_registry(token_admin_registry)?,
    })
}

// This replaces anchor's `try_from` deserialization as it had the wrong lifetime parameters, forcing
// an over-constraining of the reference lifetime. This performs the same checks Anchor does (code
// is identical except for tying the reference lifetime to `info`)
fn load_current_token_admin_registry(
    token_admin_registry: &AccountInfo<'_>,
) -> Result<TokenAdminRegistry> {
    if token_admin_registry.owner == &system_program::ID && token_admin_registry.lamports() == 0 {
        return Err(ErrorCode::AccountNotInitialized.into());
    }

    if token_admin_registry.owner != &TokenAdminRegistry::owner() {
        return Err(Error::from(ErrorCode::AccountOwnedByWrongProgram)
            .with_pubkeys((*token_admin_registry.owner, TokenAdminRegistry::owner())));
    }

    let data = token_admin_registry.try_borrow_data()?;
    TokenAdminRegistry::try_deserialize(&mut &data[..])
}

/// Loads a `TokenAdminRegistry` account that still holds an older schema layout `T`, which is
/// selected by the caller from the account's data length. The version byte persisted in the
/// account must agree with the layout it was dispatched to, so an account whose length and
/// version byte contradict each other is rejected instead of being silently reinterpreted.
fn load_old_token_admin_registry<T>(
    token_admin_registry: &AccountInfo<'_>,
) -> Result<TokenAdminRegistry>
where
    T: OldTokenAdminRegistry,
    TokenAdminRegistry: From<T>,
{
    let borrowed_data = token_admin_registry.try_borrow_data()?;
    let (discriminator, data) = borrowed_data.split_at(TokenAdminRegistry::DISCRIMINATOR.len());

    require_keys_eq!(
        token_admin_registry.owner.key(),
        crate::ID, // ccip-common crate must have the same ID as ccip-router, which is the actual program that owns it
        CommonCcipError::InvalidInputsTokenAdminRegistryAccounts
    );

    require!(
        TokenAdminRegistry::DISCRIMINATOR == discriminator,
        CommonCcipError::InvalidInputsTokenAdminRegistryAccounts
    );

    let old_token_admin_registry = T::deserialize(&mut &data[..])
        .map_err(|_| CommonCcipError::InvalidInputsTokenAdminRegistryAccounts)?;

    require_eq!(
        old_token_admin_registry.stored_version(), // the version byte actually persisted onchain
        T::CANONICAL_VERSION,                      // the version the dispatched-to layout must have
        CommonCcipError::InvalidInputsTokenAdminRegistryAccounts
    );

    Ok(TokenAdminRegistry::from(old_token_admin_registry))
}

pub mod token_admin_registry_writable {
    use crate::router_accounts::TokenAdminRegistry;

    // set writable inserts bits from left to right
    // index 0 is left-most bit
    pub fn set(tar: &mut TokenAdminRegistry, index: u8) {
        match index < 128 {
            true => {
                tar.writable_indexes[0] |= 1 << (127 - index);
            }
            false => {
                tar.writable_indexes[1] |= 1 << (255 - index);
            }
        }
    }

    pub fn is(tar: &TokenAdminRegistry, index: u8) -> bool {
        match index < 128 {
            true => tar.writable_indexes[0] & 1 << (127 - index) != 0,
            false => tar.writable_indexes[1] & 1 << (255 - index) != 0,
        }
    }

    pub fn reset(tar: &mut TokenAdminRegistry) {
        tar.writable_indexes = [0, 0];
    }

    #[cfg(test)]
    mod tests {
        use super::*;
        use anchor_lang::solana_program::pubkey::Pubkey;

        #[test]
        fn set_writable() {
            let state = &mut TokenAdminRegistry {
                version: 0,
                administrator: Pubkey::default(),
                pending_administrator: Pubkey::default(),
                lookup_table: Pubkey::default(),
                writable_indexes: [0, 0],
                mint: Pubkey::default(),
                account_resolution_enabled: false,
                skip_lookup_table_additional_entries: false,
                interface_version: 0,
                bump: 0,
            };

            set(state, 0);
            set(state, 128);
            assert_eq!(state.writable_indexes[0], 2u128.pow(127));
            assert_eq!(state.writable_indexes[1], 2u128.pow(127));

            reset(state);
            assert_eq!(state.writable_indexes[0], 0);
            assert_eq!(state.writable_indexes[1], 0);

            set(state, 0);
            set(state, 2);
            set(state, 127);
            set(state, 128);
            set(state, 2 + 128);
            set(state, 255);
            assert_eq!(
                state.writable_indexes[0],
                2u128.pow(127) + 2u128.pow(127 - 2) + 2u128.pow(0)
            );
            assert_eq!(
                state.writable_indexes[1],
                2u128.pow(127) + 2u128.pow(127 - 2) + 2u128.pow(0)
            );
        }

        #[test]
        fn check_writable() {
            let state = &TokenAdminRegistry {
                version: 0,
                administrator: Pubkey::default(),
                pending_administrator: Pubkey::default(),
                lookup_table: Pubkey::default(),
                writable_indexes: [
                    2u128.pow(127 - 7) + 2u128.pow(127 - 2) + 2u128.pow(127 - 4),
                    2u128.pow(127 - 8) + 2u128.pow(127 - 56) + 2u128.pow(127 - 100),
                ],
                mint: Pubkey::default(),
                account_resolution_enabled: false,
                skip_lookup_table_additional_entries: false,
                interface_version: 0,
                bump: 0,
            };

            assert!(!is(state, 0));
            assert!(!is(state, 128));
            assert!(!is(state, 255));

            assert_eq!(state.writable_indexes[0].count_ones(), 3);
            assert_eq!(state.writable_indexes[1].count_ones(), 3);

            assert!(is(state, 7));
            assert!(is(state, 2));
            assert!(is(state, 4));
            assert!(is(state, 128 + 8));
            assert!(is(state, 128 + 56));
            assert!(is(state, 128 + 100));
        }
    }
}

/// Returns true iff `a >= b` when both are interpreted as big-endian uint256.
fn ge_be_u256(a: &[u8; 32], b: &[u8; 32]) -> bool {
    // Big-endian uint256 compare == lexicographic compare
    for i in 0..32 {
        match a[i].cmp(&b[i]) {
            std::cmp::Ordering::Greater => return true,
            std::cmp::Ordering::Less => return false,
            std::cmp::Ordering::Equal => {} // continue
        }
    }
    true // equal
}

// address validation helpers based on the chain family selector
pub fn validate_evm_address(address: &[u8]) -> Result<()> {
    require_eq!(address.len(), 32, CommonCcipError::InvalidEVMAddress);

    let addr_32: &[u8; 32] = address
        .try_into()
        .expect("slice length is guaranteed to be 32");

    require!(
        ge_be_u256(addr_32, &EVM_PRECOMPILE_SPACE),
        CommonCcipError::InvalidEVMAddress
    );

    let address: U256 = U256::from_be_bytes(
        address
            .try_into()
            .map_err(|_| CommonCcipError::InvalidEncoding)?,
    );
    require!(address <= U160_MAX, CommonCcipError::InvalidEVMAddress);

    Ok(())
}

pub fn validate_svm_address(address: &[u8], address_must_be_nonzero: bool) -> Result<()> {
    require_eq!(address.len(), 32, CommonCcipError::InvalidSVMAddress);
    require!(
        !address_must_be_nonzero || address.iter().any(|b| *b != 0),
        CommonCcipError::InvalidSVMAddress
    );

    Pubkey::try_from_slice(address)
        .map(|_| ())
        .map_err(|_| CommonCcipError::InvalidSVMAddress.into())
}

/// 36-byte TON address raw format:
/// - [0]   flags
/// - [1]   workchain_id
/// - [2..34) account_id (32 bytes)
/// - [34..36) crc16 (ignored here)
pub fn validate_tvm_address(address: &[u8]) -> Result<()> {
    require_eq!(address.len(), 36, CommonCcipError::InvalidTVMAddress);

    // account_id starts at offset 2 and is 32 bytes long
    let account_id: &[u8; 32] = address[2..34]
        .try_into()
        .expect("slice length is guaranteed to be 32");

    require!(
        account_id.iter().any(|b| *b != 0),
        CommonCcipError::InvalidTVMAddress
    );

    Ok(())
}

pub fn validate_aptos_address(address: &[u8]) -> Result<()> {
    require_eq!(address.len(), 32, CommonCcipError::InvalidAptosAddress);

    let addr_32: &[u8; 32] = address
        .try_into()
        .expect("slice length is guaranteed to be 32");

    require!(
        ge_be_u256(addr_32, &APTOS_PRECOMPILE_SPACE),
        CommonCcipError::InvalidAptosAddress
    );

    Ok(())
}

/// A Sui address is always 32 bytes. `address_must_be_outside_precompile_space` additionally
/// requires it to sit above the reserved precompile range: callers pass `false` when a zero
/// receiver is legitimate, i.e. a token-only transfer with no destination execution.
pub fn validate_sui_address(
    address: &[u8],
    address_must_be_outside_precompile_space: bool,
) -> Result<()> {
    require_eq!(address.len(), 32, CommonCcipError::InvalidSuiAddress);

    let addr_32: &[u8; 32] = address
        .try_into()
        .expect("slice length is guaranteed to be 32");

    require!(
        !address_must_be_outside_precompile_space || ge_be_u256(addr_32, &SUI_PRECOMPILE_SPACE),
        CommonCcipError::InvalidSuiAddress
    );

    Ok(())
}

/// A Canton address is a 32-byte hash (of a party ID for receivers, of an instance ID for
/// contracts) and has no precompile space, so it only needs to be non-zero.
pub fn validate_canton_address(address: &[u8]) -> Result<()> {
    require_eq!(address.len(), 32, CommonCcipError::InvalidCantonAddress);
    require!(
        address.iter().any(|b| *b != 0),
        CommonCcipError::InvalidCantonAddress
    );

    Ok(())
}

#[derive(Accounts)]
#[instruction(
    token_sender: Pubkey,
    message_mint: Pubkey,
    chain_selector: u64,
    router: Pubkey,
    fee_quoter: Pubkey,
    validate_sender_ata: bool,
)]
pub struct OnrampTokenAccountsValidationContext<'info> {
    #[account(
        address = get_associated_token_address_with_program_id(
            &token_sender.key(),
            &mint.key(),
            &token_program.key()
        ),
        // Ownership check ensures the ATA is initialized, which is only required when sending.
        constraint = !validate_sender_ata || *user_token_accounts.owner == token_program.key() @ CommonCcipError::InvalidInputsTokenAccounts,
    )]
    pub user_token_accounts: UncheckedAccount<'info>,

    /// CHECK: The fee quoter's per-chain-per-token config PDA
    // billing: configured via CCIP fee quoter
    // chain config: configured via pool
    #[account(
        seeds = [
            seed::PER_CHAIN_PER_TOKEN_CONFIG,
            chain_selector.to_le_bytes().as_ref(),
            mint.key().as_ref(),
        ],
        seeds::program = fee_quoter.key(),
        bump
    )]
    pub per_chain_per_token_config: UncheckedAccount<'info>,

    /// CHECK: Pool chain config PDA
    #[account(
        seeds = [
            seed::POOL_CHAINCONFIG_SEED,
            chain_selector.to_le_bytes().as_ref(),
            mint.key().as_ref(),
        ],
        seeds::program = pool_program.key(),
        owner = pool_program.key() @ CommonCcipError::InvalidInputsPoolAccounts,
        bump
    )]
    pub pool_chain_config: UncheckedAccount<'info>,

    /// CHECK: Lookup table
    #[account(owner = address_lookup_table::program::id() @ CommonCcipError::InvalidInputsLookupTableAccounts)]
    pub lookup_table: UncheckedAccount<'info>,

    /// CHECK: Token admin registry PDA
    #[account(
        seeds = [seed::TOKEN_ADMIN_REGISTRY, mint.key().as_ref()],
        seeds::program = router.key(),
        bump,
        owner = router.key() @ CommonCcipError::InvalidInputsTokenAdminRegistryAccounts,
    )]
    pub token_admin_registry: UncheckedAccount<'info>,

    /// CHECK: Pool program
    #[account(executable)]
    pub pool_program: UncheckedAccount<'info>,

    /// CHECK: Pool config PDA
    #[account(
        seeds = [seed::POOL_STATE_SEED, mint.key().as_ref()],
        seeds::program = pool_program.key(),
        bump,
        owner = pool_program.key() @ CommonCcipError::InvalidInputsPoolAccounts
    )]
    pub pool_config: UncheckedAccount<'info>,

    #[account(
        address = get_associated_token_address_with_program_id(
            &pool_signer.key(),
            &mint.key(),
            &token_program.key()
        ) @ CommonCcipError::InvalidInputsTokenAccounts
    )]
    pub pool_token_account: InterfaceAccount<'info, TokenAccount>,

    /// CHECK: Pool signer PDA
    #[account(
        seeds = [seed::POOL_SIGNER_SEED, mint.key().as_ref()],
        seeds::program = pool_program.key(),
        bump
    )]
    pub pool_signer: UncheckedAccount<'info>,

    pub token_program: Interface<'info, TokenInterface>,

    #[account(
        address = message_mint @ CommonCcipError::InvalidInputsTokenAccounts,
        owner = token_program.key() @ CommonCcipError::InvalidInputsTokenAccounts,
    )]
    pub mint: InterfaceAccount<'info, Mint>,

    /// CHECK: Fee token config PDA
    #[account(
        seeds = [
            seed::FEE_BILLING_TOKEN_CONFIG,
            mint.key().as_ref()
        ],
        seeds::program = fee_quoter.key(),
        bump
    )]
    pub fee_token_config: UncheckedAccount<'info>,

    /// CHECK: The signer to be used by the router program to invoke the pool program
    #[account(
        seeds = [seed::EXTERNAL_TOKEN_POOLS_SIGNER, pool_program.key().as_ref()],
        bump,
        seeds::program = router.key(),
    )]
    pub ccip_router_pool_signer: UncheckedAccount<'info>,
}

#[derive(Accounts)]
#[instruction(
    token_receiver: Pubkey,
    message_mint: Pubkey,
    chain_selector: u64,
    router: Pubkey,
    fee_quoter: Pubkey,
    offramp: Pubkey,
    token_receiver_usage: SVMTokenReceiverUsage,
)]
pub struct OfframpTokenAccountsValidationContext<'info> {
    #[account(
        seeds = [
            seed::EXTERNAL_TOKEN_POOLS_SIGNER,
            pool_program.key().as_ref(),
        ],
        seeds::program = offramp.key(),
        bump,
    )]
    pub ccip_offramp_pool_signer: UncheckedAccount<'info>,

    #[account(
        address = get_associated_token_address_with_program_id(
            &token_receiver.key(),
            &mint.key(),
            &token_program.key()
        ),

         // validate owner
        constraint = {
            let owner = user_token_account.owner.key();
            if token_receiver_usage.should_create_ata() {
                // when (idempotent) creation is requested, the account can either be
                // uninitialized (owned by system program) or already be the ATA (owned by token program)
                owner == token_program.key() || owner == system_program::ID
            } else {
                // for every other case, we expect it to be a token account owned by the token program (initialized)
                owner == token_program.key()
            }
        } @ CommonCcipError::InvalidInputsTokenAccounts,
    )]
    pub user_token_account: UncheckedAccount<'info>,

    /// CHECK: The fee quoter's per-chain-per-token config PDA
    // billing: configured via CCIP fee quoter
    // chain config: configured via pool
    #[account(
        seeds = [
            seed::PER_CHAIN_PER_TOKEN_CONFIG,
            chain_selector.to_le_bytes().as_ref(),
            mint.key().as_ref(),
        ],
        seeds::program = fee_quoter.key(),
        bump
    )]
    pub per_chain_per_token_config: UncheckedAccount<'info>,

    /// CHECK: Pool chain config PDA
    #[account(
        seeds = [
            seed::POOL_CHAINCONFIG_SEED,
            chain_selector.to_le_bytes().as_ref(),
            mint.key().as_ref(),
        ],
        seeds::program = pool_program.key(),
        owner = pool_program.key() @ CommonCcipError::InvalidInputsPoolAccounts,
        bump
    )]
    pub pool_chain_config: UncheckedAccount<'info>,

    /// CHECK: Lookup table
    #[account(owner = address_lookup_table::program::id() @ CommonCcipError::InvalidInputsLookupTableAccounts)]
    pub lookup_table: UncheckedAccount<'info>,

    /// CHECK: Token admin registry PDA
    #[account(
        seeds = [seed::TOKEN_ADMIN_REGISTRY, mint.key().as_ref()],
        seeds::program = router.key(),
        bump,
        owner = router.key() @ CommonCcipError::InvalidInputsTokenAdminRegistryAccounts,
    )]
    pub token_admin_registry: UncheckedAccount<'info>,

    /// CHECK: Pool program
    #[account(executable)]
    pub pool_program: UncheckedAccount<'info>,

    /// CHECK: Pool config PDA
    #[account(
        seeds = [seed::POOL_STATE_SEED, mint.key().as_ref()],
        seeds::program = pool_program.key(),
        bump,
        owner = pool_program.key() @ CommonCcipError::InvalidInputsPoolAccounts
    )]
    pub pool_config: UncheckedAccount<'info>,

    #[account(
        address = get_associated_token_address_with_program_id(
            &pool_signer.key(),
            &mint.key(),
            &token_program.key()
        ) @ CommonCcipError::InvalidInputsTokenAccounts
    )]
    pub pool_token_account: InterfaceAccount<'info, TokenAccount>,

    /// CHECK: Pool signer PDA
    #[account(
        seeds = [seed::POOL_SIGNER_SEED, mint.key().as_ref()],
        seeds::program = pool_program.key(),
        bump
    )]
    pub pool_signer: UncheckedAccount<'info>,

    pub token_program: Interface<'info, TokenInterface>,

    #[account(
        address = message_mint @ CommonCcipError::InvalidInputsTokenAccounts,
        owner = token_program.key() @ CommonCcipError::InvalidInputsTokenAccounts,
    )]
    pub mint: InterfaceAccount<'info, Mint>,

    /// CHECK: Fee token config PDA
    #[account(
        seeds = [
            seed::FEE_BILLING_TOKEN_CONFIG,
            mint.key().as_ref()
        ],
        seeds::program = fee_quoter.key(),
        bump
    )]
    pub fee_token_config: UncheckedAccount<'info>,

    /// CHECK: The signer to be used by the router program to invoke the pool program
    #[account(
        seeds = [seed::EXTERNAL_TOKEN_POOLS_SIGNER, pool_program.key().as_ref()],
        bump,
        seeds::program = router.key(),
    )]
    pub ccip_router_pool_signer: UncheckedAccount<'info>,
}

#[cfg(test)]
mod tests {
    use super::*;

    mod loading_token_admin_registry {
        use crate::router_accounts::ResolutionType;

        use super::*;

        fn get_expected(version: u8) -> TokenAdminRegistry {
            TokenAdminRegistry {
                version,
                administrator: Pubkey::new_unique(),
                pending_administrator: Pubkey::new_unique(),
                lookup_table: Pubkey::new_unique(),
                writable_indexes: [1, 2],
                mint: Pubkey::new_unique(),
                // non-default value from v2 on, to make sure it's properly loaded from v2/v3 data
                account_resolution_enabled: version != 1,
                // v3 is the first layout storing these, so use non-defaults to make sure they're carried over
                interface_version: if version == 3 {
                    2
                } else {
                    TokenAdminRegistry::DEFAULT_INTERFACE_VERSION
                },
                bump: if version == 3 {
                    254
                } else {
                    TokenAdminRegistry::DEFAULT_EMPTY_BUMP
                },
                skip_lookup_table_additional_entries:
                    TokenAdminRegistry::DEFAULT_SKIP_LOOKUP_TABLE_ADDITIONAL_ENTRIES,
            }
        }

        fn to_v3_bytes(t: &TokenAdminRegistry) -> Vec<u8> {
            let mut bytes = borsh::to_vec(&t).unwrap();
            bytes.pop(); // remove the `skip_lookup_table_additional_entries` field, as this is not part of the v3 data
            bytes
        }

        fn to_v2_bytes(t: &TokenAdminRegistry) -> Vec<u8> {
            let mut bytes = to_v3_bytes(t);
            bytes.pop(); // remove the `interface_version` field, as this is not part of the v2 data
            bytes.pop(); // remove the `bump` field, as this is not part of the v2 data
            bytes
        }

        fn to_v1_bytes(t: &TokenAdminRegistry) -> Vec<u8> {
            let mut bytes = to_v2_bytes(t);
            bytes.pop(); // remove the `supports_auto_derivation` field, as this is not part of the v1 data
            bytes
        }

        fn to_acc_info<'a>(
            key: &'a Pubkey,
            lamports: &'a mut u64,
            data: &'a mut [u8],
        ) -> AccountInfo<'a> {
            AccountInfo::new(key, false, true, lamports, data, &crate::ID, false)
        }

        struct TestVersionParams {
            version: u8,
            to_bytes: fn(&TokenAdminRegistry) -> Vec<u8>,
            size: usize,
        }
        const V1: TestVersionParams = TestVersionParams {
            version: 1,
            to_bytes: to_v1_bytes,
            size: V1_TOKEN_ADMIN_REGISTRY_SIZE,
        };
        const V2: TestVersionParams = TestVersionParams {
            version: 2,
            to_bytes: to_v2_bytes,
            size: V2_TOKEN_ADMIN_REGISTRY_SIZE,
        };
        const V3: TestVersionParams = TestVersionParams {
            version: 3,
            to_bytes: to_v3_bytes,
            size: V3_TOKEN_ADMIN_REGISTRY_SIZE,
        };

        fn check_valid_load<T>(params: TestVersionParams)
        where
            T: OldTokenAdminRegistry,
            TokenAdminRegistry: From<T>,
        {
            let expected = get_expected(params.version);
            let bytes = (params.to_bytes)(&expected);

            let mut data = vec![0u8; params.size];
            data[0..8].copy_from_slice(TokenAdminRegistry::DISCRIMINATOR);
            data[8..].copy_from_slice(bytes.as_slice());

            let (mut lamports, key) = (0u64, Pubkey::new_unique());
            let account_info = to_acc_info(&key, &mut lamports, &mut data);
            let result = load_old_token_admin_registry::<T>(&account_info).unwrap();
            assert_eq!(result, expected);
        }

        #[test]
        fn test_valid_load_v1() {
            check_valid_load::<TokenAdminRegistryV1>(V1);
        }

        #[test]
        fn test_valid_load_v2() {
            check_valid_load::<TokenAdminRegistryV2>(V2);
        }

        #[test]
        fn test_valid_load_v3() {
            check_valid_load::<TokenAdminRegistryV3>(V3);
        }

        /// A v3 registry may still hold `WithoutExtendedLookupTable` (2) in the resolution byte. It
        /// must load as that mode (split across the two v4 fields) rather than fail, so the account
        /// stays usable on 2.0 lanes and can still be migrated to v4.
        #[test]
        fn test_v3_without_extended_lookup_table_loads() {
            let mut data = vec![0u8; V3_TOKEN_ADMIN_REGISTRY_SIZE];
            data[0..8].copy_from_slice(TokenAdminRegistry::DISCRIMINATOR);
            data[8..].copy_from_slice(to_v3_bytes(&get_expected(3)).as_slice());
            // the resolution byte sits right before `bump` and `interface_version`
            let resolution_offset = V3_TOKEN_ADMIN_REGISTRY_SIZE - 3;
            assert_eq!(data[resolution_offset], 1);
            data[resolution_offset] = 2;

            let (mut lamports, key) = (0u64, Pubkey::new_unique());
            let account_info = to_acc_info(&key, &mut lamports, &mut data);
            let result =
                load_old_token_admin_registry::<TokenAdminRegistryV3>(&account_info).unwrap();
            assert_eq!(
                result.account_resolution(),
                ResolutionType::WithoutExtendedLookupTable
            );
            assert!(result.account_resolution_enabled);
            assert!(result.skip_lookup_table_additional_entries);
        }

        #[test]
        fn test_account_resolution_round_trip() {
            for resolution in [
                ResolutionType::NoResolution,
                ResolutionType::Standard,
                ResolutionType::WithoutExtendedLookupTable,
            ] {
                let mut registry = get_expected(TokenAdminRegistry::LATEST_VERSION);
                registry.set_account_resolution(&resolution);
                assert_eq!(registry.account_resolution(), resolution);
            }
        }

        /// Frozen copy of `router_accounts::TokenAdminRegistry` as of chainlink-ccip v1.6
        /// (1f9fb0b2). It keeps its original name so `#[account]` derives the same discriminator,
        /// and the crate ID is the Router's, as in 1.6, so the owner check matches too. Left out of
        /// IDL builds so it cannot collide with the live account type.
        #[cfg(not(feature = "idl-build"))]
        mod frozen_1_6 {
            use anchor_lang::prelude::*;

            #[account]
            pub struct TokenAdminRegistry {
                pub version: u8,
                pub administrator: Pubkey,
                pub pending_administrator: Pubkey,
                pub lookup_table: Pubkey,
                pub writable_indexes: [u128; 2],
                pub mint: Pubkey,
                pub supports_auto_derivation: bool,
            }
        }

        /// The 1.6 offramp is not upgraded and keeps loading every registry it executes against
        /// with its own (v2) layout: a `bool` at the position of `account_resolution_enabled`,
        /// with any trailing bytes ignored. Every registry the current code can write must stay
        /// loadable that way, or 1.6 execution (including manual execution) breaks for the token.
        ///
        /// This goes through the same path as the 1.6 `load_token_admin_registry_checked`: any
        /// account that is not exactly the v1 size is loaded with `Account::try_from`, which checks
        /// the owner and the discriminator before decoding. 1.6 was built on Anchor 0.29, whose
        /// `#[account]` codegen performs the same three checks as the version used here.
        #[cfg(not(feature = "idl-build"))]
        #[test]
        fn test_latest_layout_is_loadable_by_1_6_offramp() {
            assert_eq!(
                frozen_1_6::TokenAdminRegistry::DISCRIMINATOR,
                TokenAdminRegistry::DISCRIMINATOR
            );

            for resolution in [
                ResolutionType::NoResolution,
                ResolutionType::Standard,
                ResolutionType::WithoutExtendedLookupTable,
            ] {
                let mut registry = get_expected(TokenAdminRegistry::LATEST_VERSION);
                registry.interface_version = 2;
                registry.bump = 254;
                registry.set_account_resolution(&resolution);
                let mut data = TokenAdminRegistry::DISCRIMINATOR.to_vec();
                data.extend_from_slice(&borsh::to_vec(&registry).unwrap());
                // 1.6 only takes its hand-rolled v1 path for accounts of exactly the v1 size.
                assert_ne!(data.len(), V1_TOKEN_ADMIN_REGISTRY_SIZE);

                let (mut lamports, key) = (1u64, Pubkey::new_unique());
                let account_info = to_acc_info(&key, &mut lamports, &mut data);
                let legacy = Account::<frozen_1_6::TokenAdminRegistry>::try_from(&account_info)
                    .unwrap_or_else(|e| panic!("1.6 cannot load {resolution:?}: {e}"))
                    .into_inner();
                assert_eq!(legacy.supports_auto_derivation, resolution.is_enabled());
                assert_eq!(legacy.version, registry.version);
                assert_eq!(legacy.administrator, registry.administrator);
                assert_eq!(legacy.pending_administrator, registry.pending_administrator);
                assert_eq!(legacy.lookup_table, registry.lookup_table);
                assert_eq!(legacy.writable_indexes, registry.writable_indexes);
                assert_eq!(legacy.mint, registry.mint);
            }
        }

        #[test]
        fn test_invalid_discriminator() {
            let source = get_expected(1);
            let bytes = to_v1_bytes(&source);

            let mut data = vec![0u8; V1_TOKEN_ADMIN_REGISTRY_SIZE];
            data[0..8].copy_from_slice(TokenAdminRegistry::DISCRIMINATOR);
            data[2] = 2; // change the discriminator to an invalid one
            data[8..].copy_from_slice(bytes.as_slice());

            let (mut lamports, key) = (0u64, Pubkey::new_unique());
            let account_info = to_acc_info(&key, &mut lamports, &mut data);

            let result = load_old_token_admin_registry::<TokenAdminRegistryV1>(&account_info);
            assert!(result.is_err());
        }

        fn check_invalid_version_load<T>(params: TestVersionParams)
        where
            T: OldTokenAdminRegistry,
            TokenAdminRegistry: From<T>,
        {
            for v in [0, 3, 4, 255].into_iter().filter(|v| *v != params.version) {
                let source = get_expected(v);

                let bytes = (params.to_bytes)(&source);

                let mut data = vec![0u8; params.size];
                data[0..8].copy_from_slice(TokenAdminRegistry::DISCRIMINATOR);
                data[8..].copy_from_slice(bytes.as_slice());

                let (mut lamports, key) = (0u64, Pubkey::new_unique());
                let account_info = to_acc_info(&key, &mut lamports, &mut data);

                let result = load_old_token_admin_registry::<T>(&account_info);
                assert!(result.is_err(), "Version {} should not be valid", v);
            }
        }

        #[test]
        fn test_invalid_version_v1() {
            check_invalid_version_load::<TokenAdminRegistryV1>(V1);
        }

        #[test]
        fn test_invalid_version_v2() {
            check_invalid_version_load::<TokenAdminRegistryV2>(V2);
        }

        #[test]
        fn test_invalid_version_v3() {
            check_invalid_version_load::<TokenAdminRegistryV3>(V3);
        }

        /// An account whose length matches one schema version but whose persisted version byte
        /// claims a different one is contradictory, and must be rejected rather than read as the
        /// layout its length happens to match.
        fn check_mismatched_version_load<T>(params: TestVersionParams, other_version: u8)
        where
            T: OldTokenAdminRegistry,
            TokenAdminRegistry: From<T>,
        {
            let source = get_expected(other_version);
            let bytes = (params.to_bytes)(&source);

            let mut data = vec![0u8; params.size];
            data[0..8].copy_from_slice(TokenAdminRegistry::DISCRIMINATOR);
            data[8..].copy_from_slice(bytes.as_slice());
            assert_eq!(data[8], other_version); // the persisted version byte disagrees with the length

            let (mut lamports, key) = (0u64, Pubkey::new_unique());
            let account_info = to_acc_info(&key, &mut lamports, &mut data);

            let result = load_old_token_admin_registry::<T>(&account_info);
            assert!(
                result.is_err(),
                "v{} layout holding version byte {} should not load",
                params.version,
                other_version
            );
        }

        #[test]
        fn test_mismatched_version_v1_layout_claiming_v2() {
            check_mismatched_version_load::<TokenAdminRegistryV1>(V1, 2);
        }

        #[test]
        fn test_mismatched_version_v2_layout_claiming_v1() {
            check_mismatched_version_load::<TokenAdminRegistryV2>(V2, 1);
        }

        #[test]
        fn test_mismatched_version_v3_layout_claiming_v4() {
            check_mismatched_version_load::<TokenAdminRegistryV3>(V3, 4);
        }

        #[test]
        fn test_invalid_owner() {
            let source = get_expected(1);
            let bytes = to_v1_bytes(&source);

            let mut data = vec![0u8; V1_TOKEN_ADMIN_REGISTRY_SIZE];
            data[0..8].copy_from_slice(TokenAdminRegistry::DISCRIMINATOR);
            data[8..].copy_from_slice(bytes.as_slice());

            let (mut lamports, key) = (0u64, Pubkey::new_unique());
            let mut account_info = to_acc_info(&key, &mut lamports, &mut data);
            let invalid_owner = Pubkey::new_unique();
            account_info.owner = &invalid_owner; // change the owner to an invalid one

            let result = load_old_token_admin_registry::<TokenAdminRegistryV1>(&account_info);
            assert!(result.is_err());
        }

        // -------------------------------------------------------------------------------------
        // Mint binding: `load_token_admin_registry_checked` must only ever load the *canonical*
        // registry for the token it is asked about. Callers use `interface_version` to pick the
        // token transfer schema (legacy v1 accounts vs v2 accounts + `get_required_ccvs` query),
        // so accepting some other token's registry would let a caller pick the schema at will.
        // -------------------------------------------------------------------------------------

        /// The canonical registry PDA for `mint`, as derived by the router (and, identically, by
        /// `load_token_admin_registry_checked`).
        fn canonical_registry(mint: &Pubkey) -> Pubkey {
            seed::token_admin_registry(mint, crate::ID)
        }

        /// Account data for a legacy (v1-schema) registry. It loads with `interface_version == 1`,
        /// which is what selects the legacy token transfer path.
        fn legacy_registry_data(registry: &TokenAdminRegistry) -> Vec<u8> {
            let bytes = to_v1_bytes(registry);
            let mut data = vec![0u8; V1_TOKEN_ADMIN_REGISTRY_SIZE];
            data[0..8].copy_from_slice(TokenAdminRegistry::DISCRIMINATOR);
            data[8..].copy_from_slice(bytes.as_slice());
            data
        }

        /// Account data for a current-schema registry, whose `interface_version` is whatever the
        /// router stored (i.e. `2` for a pool implementing the v2 interface).
        fn current_registry_data(registry: &TokenAdminRegistry) -> Vec<u8> {
            let mut data = TokenAdminRegistry::DISCRIMINATOR.to_vec();
            data.extend_from_slice(&borsh::to_vec(registry).unwrap());
            // Otherwise the loader would take one of the legacy migration paths instead.
            assert_ne!(data.len(), V1_TOKEN_ADMIN_REGISTRY_SIZE);
            assert_ne!(data.len(), V2_TOKEN_ADMIN_REGISTRY_SIZE);
            assert_ne!(data.len(), V3_TOKEN_ADMIN_REGISTRY_SIZE);
            data
        }

        fn error_code(error: &Error) -> u32 {
            match error {
                Error::AnchorError(e) => e.error_code_number,
                Error::ProgramError(e) => panic!("unexpected program error: {e:?}"),
            }
        }

        #[test]
        fn test_checked_load_accepts_canonical_legacy_registry() {
            let expected = get_expected(1);
            let mint = expected.mint;

            let mut data = legacy_registry_data(&expected);
            let (mut lamports, key) = (0u64, canonical_registry(&mint));
            let account_info = to_acc_info(&key, &mut lamports, &mut data);

            let loaded = load_token_admin_registry_checked(&account_info, mint).unwrap();
            assert_eq!(loaded, expected);
            assert_eq!(loaded.interface_version, 1); // i.e. the legacy transfer path
        }

        #[test]
        fn test_checked_load_accepts_canonical_current_registry() {
            let mint = Pubkey::new_unique();
            let expected = TokenAdminRegistry {
                version: TokenAdminRegistry::LATEST_VERSION,
                interface_version: 2,
                mint,
                ..get_expected(TokenAdminRegistry::LATEST_VERSION)
            };
            let mut data = current_registry_data(&expected);
            let (mut lamports, key) = (0u64, canonical_registry(&mint));
            let account_info = to_acc_info(&key, &mut lamports, &mut data);

            let loaded = load_token_admin_registry_checked(&account_info, mint).unwrap();
            assert_eq!(loaded, expected);
            assert_eq!(loaded.interface_version, 2); // i.e. the v2 transfer path
        }

        // The attack this guards against: a token served by a v2 pool, plus any *other* token on the
        // same router that is still on a v1 pool. Passing the second token's (perfectly valid, legacy)
        // registry alongside the first token's accounts would force the legacy path, skipping the
        // canonical pool's required CCVs. The registry must be bound to the token being transferred.
        #[test]
        fn test_checked_load_rejects_registry_of_another_mint() {
            let expected = get_expected(1);
            let mint = expected.mint;
            let other_mint = Pubkey::new_unique();

            let mut data = legacy_registry_data(&expected);
            let (mut lamports, key) = (0u64, canonical_registry(&other_mint)); // wrong mint's tar
            let account_info = to_acc_info(&key, &mut lamports, &mut data);

            // The account is a genuine, loadable registry — just not this token's.
            load_token_admin_registry_checked(&account_info, other_mint).unwrap();

            let err = load_token_admin_registry_checked(&account_info, mint).unwrap_err();
            assert_eq!(
                error_code(&err),
                u32::from(CommonCcipError::InvalidInputsTokenAdminRegistryAccounts)
            );
        }

        #[test]
        fn test_checked_load_rejects_non_pda_registry() {
            let registry = get_expected(1);
            let mint = registry.mint;

            let mut data = legacy_registry_data(&registry);
            let (mut lamports, key) = (0u64, Pubkey::new_unique()); // wrong key here
            let account_info = to_acc_info(&key, &mut lamports, &mut data);

            let err = load_token_admin_registry_checked(&account_info, mint).unwrap_err();
            assert_eq!(
                error_code(&err),
                u32::from(CommonCcipError::InvalidInputsTokenAdminRegistryAccounts)
            );
        }
    }

    // -----------------------------
    // Helpers for address tests
    // -----------------------------

    fn u256_to_32_be(x: U256) -> [u8; 32] {
        x.to_be_bytes()
    }

    fn be32_with_last_byte(v: u8) -> [u8; 32] {
        let mut a = [0u8; 32];
        a[31] = v;
        a
    }

    // -----------------------------
    // TON / TVM address tests
    // -----------------------------

    #[test]
    fn validate_tvm_address_accepts_len_36_nonzero_account_id() {
        // 36-byte TON address raw format:
        // [0] flags, [1] workchain_id, [2..34) account_id (32 bytes), [34..36) crc16
        let mut addr = [0u8; 36];
        addr[0] = 1;
        addr[1] = 2;
        addr[2] = 1; // make account_id non-zero
        validate_tvm_address(&addr).unwrap();
    }

    #[test]
    fn validate_tvm_address_rejects_wrong_len() {
        let addr = [1u8; 35];
        assert_eq!(
            validate_tvm_address(&addr).unwrap_err(),
            CommonCcipError::InvalidTVMAddress.into()
        );
    }

    #[test]
    fn validate_tvm_address_rejects_zero_account_id() {
        let mut addr = [0u8; 36];
        addr[0] = 1;
        addr[1] = 2;
        // account_id is all-zero
        assert_eq!(
            validate_tvm_address(&addr).unwrap_err(),
            CommonCcipError::InvalidTVMAddress.into()
        );
    }

    // -----------------------------
    // Aptos address tests
    // -----------------------------

    #[test]
    fn validate_aptos_address_accepts_valid_ge_min() {
        // exactly min
        validate_aptos_address(&APTOS_PRECOMPILE_SPACE).unwrap();

        // > min
        let mut addr = APTOS_PRECOMPILE_SPACE;
        addr[31] = 0x0c;
        validate_aptos_address(&addr).unwrap();
    }

    #[test]
    fn validate_aptos_address_rejects_wrong_len() {
        let addr = [1u8; 31];
        assert_eq!(
            validate_aptos_address(&addr).unwrap_err(),
            CommonCcipError::InvalidAptosAddress.into()
        );
    }

    #[test]
    fn validate_aptos_address_rejects_zero_address() {
        let addr = [0u8; 32];
        assert_eq!(
            validate_aptos_address(&addr).unwrap_err(),
            CommonCcipError::InvalidAptosAddress.into()
        );
    }

    #[test]
    fn validate_aptos_address_rejects_lower_than_min() {
        // 0x0a < 0x0b
        let addr = be32_with_last_byte(0x0a);
        assert_eq!(
            validate_aptos_address(&addr).unwrap_err(),
            CommonCcipError::InvalidAptosAddress.into()
        );
    }

    // -----------------------------
    // Sui address tests
    // -----------------------------

    #[test]
    fn validate_sui_address_accepts_valid_ge_min() {
        // exactly min
        validate_sui_address(&SUI_PRECOMPILE_SPACE, true).unwrap();

        // > min (increase last byte, staying >=)
        let mut addr = SUI_PRECOMPILE_SPACE;
        addr[31] = 0xea;
        validate_sui_address(&addr, true).unwrap();
    }

    #[test]
    fn validate_sui_address_rejects_wrong_len() {
        let addr = [1u8; 31];
        assert_eq!(
            validate_sui_address(&addr, true).unwrap_err(),
            CommonCcipError::InvalidSuiAddress.into()
        );
    }

    #[test]
    fn validate_sui_address_rejects_zero_address() {
        let addr = [0u8; 32];
        assert_eq!(
            validate_sui_address(&addr, true).unwrap_err(),
            CommonCcipError::InvalidSuiAddress.into()
        );
    }

    #[test]
    fn validate_sui_address_accepts_precompile_space_when_gate_is_off() {
        // A token-only transfer has no destination execution, so a zero receiver is legitimate
        // and the precompile-space floor is not applied.
        validate_sui_address(&[0u8; 32], false).unwrap();

        // The length check still applies regardless of the gate.
        assert_eq!(
            validate_sui_address(&[0u8; 31], false).unwrap_err(),
            CommonCcipError::InvalidSuiAddress.into()
        );
    }

    #[test]
    fn validate_sui_address_rejects_lower_than_min() {
        // Make something just below 0xdee9: 0xdee8
        let mut addr = SUI_PRECOMPILE_SPACE;
        addr[31] = 0xe8;
        assert_eq!(
            validate_sui_address(&addr, true).unwrap_err(),
            CommonCcipError::InvalidSuiAddress.into()
        );
    }

    // -----------------------------
    // Canton address tests
    // -----------------------------

    #[test]
    fn validate_canton_address_accepts_nonzero_32_bytes() {
        let mut addr = [0u8; 32];
        addr[31] = 1;
        validate_canton_address(&addr).unwrap();
    }

    #[test]
    fn validate_canton_address_rejects_zero_address() {
        assert_eq!(
            validate_canton_address(&[0u8; 32]).unwrap_err(),
            CommonCcipError::InvalidCantonAddress.into()
        );
    }

    #[test]
    fn validate_canton_address_rejects_wrong_len() {
        assert_eq!(
            validate_canton_address(&[1u8; 20]).unwrap_err(),
            CommonCcipError::InvalidCantonAddress.into()
        );
    }

    // -----------------------------
    // EVM address tests
    // -----------------------------

    #[test]
    fn validate_evm_address_accepts_valid_ge_min_and_le_u160max() {
        // exactly min (1024)
        validate_evm_address(&EVM_PRECOMPILE_SPACE).unwrap();

        // > min but still small
        let mut addr = EVM_PRECOMPILE_SPACE;
        addr[31] = 0x01; // 0x0401
        validate_evm_address(&addr).unwrap();

        // a high-but-valid 160-bit address: U160_MAX itself
        let high = u256_to_32_be(U160_MAX);
        validate_evm_address(&high).unwrap();
    }

    #[test]
    fn validate_evm_address_rejects_wrong_len() {
        let addr = [1u8; 31];
        assert_eq!(
            validate_evm_address(&addr).unwrap_err(),
            CommonCcipError::InvalidEVMAddress.into()
        );
    }

    #[test]
    fn validate_evm_address_rejects_zero_address() {
        let addr = [0u8; 32];
        assert_eq!(
            validate_evm_address(&addr).unwrap_err(),
            CommonCcipError::InvalidEVMAddress.into()
        );
    }

    #[test]
    fn validate_evm_address_rejects_lower_than_min() {
        // 1023 (0x03ff) is below 1024
        let addr = [
            0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
            0, 0x03, 0xff,
        ];
        assert_eq!(
            validate_evm_address(&addr).unwrap_err(),
            CommonCcipError::InvalidEVMAddress.into()
        );

        // also check something very small like 1
        let addr = be32_with_last_byte(1);
        assert_eq!(
            validate_evm_address(&addr).unwrap_err(),
            CommonCcipError::InvalidEVMAddress.into()
        );
    }

    #[test]
    fn validate_evm_address_rejects_greater_than_u160max() {
        // U160_MAX + 1 should fail the <= U160_MAX check
        let too_big = u256_to_32_be(U160_MAX + U256::new(1));
        assert_eq!(
            validate_evm_address(&too_big).unwrap_err(),
            CommonCcipError::InvalidEVMAddress.into()
        );

        // also fail for something with non-zero in the top 96 bits (definitely > u160 max)
        let mut definitely_too_big = [0u8; 32];
        definitely_too_big[0] = 1; // sets a high-order byte
        definitely_too_big[31] = 0x10; // keep non-zero
        assert_eq!(
            validate_evm_address(&definitely_too_big).unwrap_err(),
            CommonCcipError::InvalidEVMAddress.into()
        );
    }

    // -----------------------------
    // SVM address tests (Solana)
    // -----------------------------

    #[test]
    fn validate_svm_address_accepts_valid_pubkey_bytes() {
        let pk = Pubkey::new_unique();
        validate_svm_address(pk.as_ref(), true).unwrap();
    }

    #[test]
    fn validate_svm_address_rejects_wrong_len() {
        let addr = [1u8; 31];
        assert_eq!(
            validate_svm_address(&addr, true).unwrap_err(),
            CommonCcipError::InvalidSVMAddress.into()
        );
    }

    #[test]
    fn validate_svm_address_rejects_all_zero() {
        let addr = [0u8; 32];
        assert_eq!(
            validate_svm_address(&addr, true).unwrap_err(),
            CommonCcipError::InvalidSVMAddress.into()
        );
    }
}
