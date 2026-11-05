use anchor_lang::prelude::*;
use anchor_spl::associated_token::{self, get_associated_token_address_with_program_id};
use anchor_spl::token_interface::{Mint, TokenAccount, TokenInterface};
use solana_address_lookup_table_interface as address_lookup_table;
use solana_address_lookup_table_interface::state::AddressLookupTable;

use crate::messages::{FinalityConfig, SVM2AnyMessage, SVMTokenReceiverUsage};
use crate::types::CrossChainGas;
use crate::v1::{load_token_admin_registry_checked, token_admin_registry_writable};
use crate::{seed, CommonCcipError};

/// Legacy Cross-chain amount representation (u256 in little-endian bytes).
/// Kept for backwards compatibility with V1 pools that still expect little-endian.
/// Note that CCIP 2.0 code uses ProtocolAmount instead, which is BE in accordance
/// to the CCIP cross-chain spec.
#[derive(AnchorSerialize, AnchorDeserialize, Clone, Copy, Default, Debug, PartialEq, Eq)]
pub struct CrossChainAmount {
    le_bytes: [u8; 32],
}

impl CrossChainAmount {
    pub const fn to_le_bytes(self) -> [u8; 32] {
        self.le_bytes
    }

    pub const fn from_le_bytes(le_bytes: [u8; 32]) -> Self {
        Self { le_bytes }
    }
}

impl<T: Into<ethnum::U256>> From<T> for CrossChainAmount {
    fn from(value: T) -> Self {
        Self {
            le_bytes: value.into().to_le_bytes(),
        }
    }
}

/// CCIP 2.0 compatible cross-chain amount representation (u256 in big-endian bytes, based
/// on abi.encodePacked)
#[derive(AnchorSerialize, AnchorDeserialize, Clone, Copy, Default, Debug, PartialEq, Eq)]
pub struct ProtocolAmount {
    be_bytes: [u8; 32],
}

impl ProtocolAmount {
    pub const ZERO: Self = ProtocolAmount {
        be_bytes: [0u8; 32],
    };

    pub const fn to_be_bytes(self) -> [u8; 32] {
        self.be_bytes
    }

    pub const fn from_be_bytes(be_bytes: [u8; 32]) -> Self {
        Self { be_bytes }
    }

    pub fn to_le_bytes(mut self) -> [u8; 32] {
        self.be_bytes.reverse();
        self.be_bytes
    }

    pub fn to_u256(self) -> ethnum::U256 {
        ethnum::U256::from_be_bytes(self.be_bytes)
    }
}

impl TryFrom<ProtocolAmount> for u64 {
    type Error = CommonCcipError;

    fn try_from(value: ProtocolAmount) -> std::result::Result<Self, Self::Error> {
        value
            .to_u256()
            .try_into()
            .map_err(|_| CommonCcipError::InvalidProtocolAmount)
    }
}

impl<T: Into<ethnum::U256>> From<T> for ProtocolAmount {
    fn from(value: T) -> Self {
        Self {
            be_bytes: value.into().to_be_bytes(),
        }
    }
}

impl From<ProtocolAmount> for CrossChainAmount {
    fn from(mut value: ProtocolAmount) -> Self {
        value.be_bytes.reverse();
        Self {
            le_bytes: value.be_bytes,
        }
    }
}

impl From<CrossChainAmount> for ProtocolAmount {
    fn from(mut value: CrossChainAmount) -> Self {
        value.le_bytes.reverse();
        Self {
            be_bytes: value.le_bytes,
        }
    }
}

/// CCIP 2.0 fee/gas receipt for a single entity (verifier, token pool, executor, network).
#[derive(Clone, AnchorSerialize, AnchorDeserialize, Debug, Default)]
pub struct Receipt {
    pub issuer: Pubkey,
    pub dest_gas_limit: CrossChainGas,
    pub dest_bytes_overhead: u32,
    pub fee_token_amount: u64,
    pub extra_args: Vec<u8>,
}

/// Direction of a cross-chain message, used for CCV requirements.
#[derive(Clone, AnchorSerialize, AnchorDeserialize, Debug, PartialEq)]
pub enum MessageDirection {
    Outbound,
    Inbound,
}

pub struct CcipV2OnrampTokenAccounts<'a> {
    pub common: CcipV2OfframpOnrampCommonTokenAccounts<'a>,
    // Onramp only as non-BPS fees are always paid on source
    pub vault_ata: &'a AccountInfo<'a>,
    pub ccip_router_pool_signer_bump: u8,

    // Resolved by the token pool
    pub extra_cpi_accounts: &'a [AccountInfo<'a>],
}

pub struct CcipV2OfframpTokenAccounts<'a> {
    pub common: CcipV2OfframpOnrampCommonTokenAccounts<'a>,
    pub ccip_offramp_pool_signer: &'a AccountInfo<'a>,

    // as these ones are optional, they don't count for the MIN_TOKEN_POOL_ACCOUNTS
    pub user_receiver_wallet: Option<&'a AccountInfo<'a>>,
    pub ata_program: Option<&'a AccountInfo<'a>>,

    pub ccip_router_pool_signer_bump: u8,
    pub ccip_offramp_pool_signer_bump: u8,

    // Resolved by the token pool
    pub extra_cpi_accounts: &'a [AccountInfo<'a>],
}

#[allow(clippy::too_many_arguments)]
pub fn validate_and_parse_onramp_token_accounts_v2<'info>(
    token_sender: Pubkey,
    message_mint: Pubkey,
    chain_selector: u64,
    router: Pubkey,
    fee_quoter: Pubkey,
    accounts: &'info [AccountInfo<'info>],
    validate_sender_ata: bool,
) -> Result<CcipV2OnrampTokenAccounts<'info>> {
    let bumps = &mut <OnrampTokenAccountsValidationContext as anchor_lang::Bumps>::Bumps::default();
    let reallocs = &mut std::collections::BTreeSet::new();

    // `try_accounts` advances the slice it is given past every account it consumes, so validate
    // against a throwaway copy and keep `accounts` pointing at the start for extraction below.
    let mut accounts_to_validate = accounts;

    // leveraging Anchor's account context validation
    // Instead of manually checking each account (ownership, PDA derivation, constraints),
    // we're using Anchor's `try_accounts` to perform these validations based on the
    // constraints defined in the `OnrampTokenAccountsValidationContext` account context struct
    OnrampTokenAccountsValidationContext::try_accounts(
        &router,
        &mut accounts_to_validate,
        &[
            token_sender.as_ref(),
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

    let vault_ata = next_account_info(&mut accounts_iter)?;
    let common_accounts = CcipV2OfframpOnrampCommonTokenAccounts::from_iter(&mut accounts_iter)?;
    let remaining_accounts = accounts_iter.as_slice();

    // Additional validations that can't be expressed in the account context
    common_accounts.additional_validations(remaining_accounts)?;

    Ok(CcipV2OnrampTokenAccounts {
        ccip_router_pool_signer_bump: bumps.ccip_router_pool_signer,
        extra_cpi_accounts: remaining_accounts,
        common: common_accounts,
        vault_ata,
    })
}

#[allow(clippy::too_many_arguments)]
pub fn validate_and_parse_offramp_token_accounts_v2<'info>(
    token_receiver: Pubkey,
    message_mint: Pubkey,
    chain_selector: u64,
    router: Pubkey,
    fee_quoter: Pubkey,
    offramp: Pubkey,
    token_receiver_usage: SVMTokenReceiverUsage,
    accounts: &'info [AccountInfo<'info>],
) -> Result<CcipV2OfframpTokenAccounts<'info>> {
    let bumps =
        &mut <OfframpTokenAccountsValidationContext as anchor_lang::Bumps>::Bumps::default();
    let reallocs = &mut std::collections::BTreeSet::new();

    // `try_accounts` advances the slice it is given past every account it consumes, so validate
    // against a throwaway copy and keep `accounts` pointing at the start for extraction below.
    let mut accounts_for_validation = accounts;

    // leveraging Anchor's account context validation
    // Instead of manually checking each account (ownership, PDA derivation, constraints),
    // we're using Anchor's `try_accounts` to perform these validations based on the
    // constraints defined in the `OfframpTokenAccountsValidationContext` account context struct
    OfframpTokenAccountsValidationContext::try_accounts(
        &offramp,
        &mut accounts_for_validation,
        &[
            token_receiver.as_ref(),
            message_mint.as_ref(),
            &chain_selector.to_le_bytes(),
            router.as_ref(),
            fee_quoter.as_ref(),
            // SAFETY: `SVMTokenReceiverUsage` is a fieldless three-variant enum, so its
            // discriminant is 0..=2 and always fits in a `u8`.
            &[token_receiver_usage as u8],
        ]
        .concat(),
        bumps,
        reallocs,
    )?;

    let mut accounts_iter = accounts.iter();
    // The offramp starts with the pool signer, then every element after that until ATA creation
    // is common with the onramp.
    let ccip_offramp_pool_signer = next_account_info(&mut accounts_iter)?;
    let common_accounts = CcipV2OfframpOnrampCommonTokenAccounts::from_iter(&mut accounts_iter)?;

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

    Ok(CcipV2OfframpTokenAccounts {
        common: common_accounts,
        user_receiver_wallet,
        ata_program,
        ccip_router_pool_signer_bump: bumps.ccip_router_pool_signer,
        ccip_offramp_pool_signer_bump: bumps.ccip_offramp_pool_signer,
        extra_cpi_accounts: remaining_accounts,
        ccip_offramp_pool_signer,
    })
}

/// Number of constant token pool accounts every pool lookup table starts with. The LUT may
/// contain additional static entries after them. Must match
/// [`CcipV2OfframpOnrampCommonTokenAccounts::required_entries`].
pub const TOKEN_POOL_LUT_REQUIRED_ENTRIES: usize = 10;

pub struct CcipV2OfframpOnrampCommonTokenAccounts<'a> {
    // accounts based on user or chain
    pub user_token_account: &'a AccountInfo<'a>,
    /// The fee quoter's PER_CHAIN_PER_TOKEN_CONFIG PDA for (dest chain, mint).
    pub per_chain_per_token_config: &'a AccountInfo<'a>,
    pub pool_chain_config: &'a AccountInfo<'a>,
    pub pool_chain_config_v2: &'a AccountInfo<'a>,
    pub pool_chain_config_override: &'a AccountInfo<'a>,

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

impl<'a> CcipV2OfframpOnrampCommonTokenAccounts<'a> {
    /// Pool accounts in the order [`get_pool_required_ccvs_cpi`] expects:
    /// `[pool_program, pool_config, pool_chain_config, chain_config_v2]`.
    pub fn get_pool_required_ccvs_accounts(&self) -> Vec<AccountInfo<'a>> {
        vec![
            self.pool_program.clone(),
            self.pool_config.clone(),
            self.pool_chain_config.clone(),
            self.pool_chain_config_v2.clone(),
        ]
    }

    // Extracted by re-iterating the original (non-advanced) input slice rather than from the
    // validation context. `try_accounts` only validates; downstream pool CPIs need references with
    // the full input lifetime, which Anchor's typed wrappers (InterfaceAccount/Interface) don't
    // expose -- their only accessor, `to_account_info()`, returns an owned clone.
    fn from_iter(iter: &mut impl Iterator<Item = &'a AccountInfo<'a>>) -> Result<Self> {
        Ok(Self {
            user_token_account: next_account_info(iter)?,
            per_chain_per_token_config: next_account_info(iter)?,
            pool_chain_config: next_account_info(iter)?,
            pool_chain_config_v2: next_account_info(iter)?,
            pool_chain_config_override: next_account_info(iter)?,
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
        let entries: [_; TOKEN_POOL_LUT_REQUIRED_ENTRIES] = [
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
        ];
        entries.into_iter()
    }

    // Validations that can't be performed in an Anchor context.
    fn additional_validations(&self, remaining_accounts: &'a [AccountInfo<'a>]) -> Result<()> {
        // Check Lookup Table Address configured in TokenAdminRegistry
        let token_admin_registry_account =
            load_token_admin_registry_checked(self.token_admin_registry, self.mint.key())?;
        require_keys_eq!(
            token_admin_registry_account.lookup_table,
            self.lookup_table.key(),
            CommonCcipError::InvalidInputsLookupTableAccounts
        );

        require_gt!(
            token_admin_registry_account.interface_version,
            1,
            CommonCcipError::InvalidInputsTokenAccounts
        );

        // Check Lookup Table Entries
        let lookup_table_data = &mut &self.lookup_table.data.borrow()[..];
        let lookup_table_account: AddressLookupTable =
            AddressLookupTable::deserialize(lookup_table_data)
                .map_err(|_| CommonCcipError::InvalidInputsLookupTableAccounts)?;

        // reconstruct + validate expected values in token pool lookup table
        // base set of constant accounts (10)
        // + additional constant accounts (remaining_accounts) that are not required but may be used for additional token pool functionality (like CPI)
        let is_resolution_enabled = token_admin_registry_account.account_resolution.is_enabled();

        let use_additional_lut_entries = token_admin_registry_account
            .account_resolution
            .use_lookup_table_additional_entries();
        {
            // validate pool addresses
            let mut expected_keys: Vec<Pubkey> = self.required_entries().map(|x| x.key()).collect();
            let mut remaining_keys: Vec<Pubkey> =
                remaining_accounts.iter().map(|x| x.key()).collect();
            expected_keys.append(&mut remaining_keys);

            match (is_resolution_enabled, use_additional_lut_entries) {
                (true, true) => {
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
                }
                (true, false) => {
                    // With resolution but ignoring the additional entries of the LUT, we only validate that the
                    // required entries are present in the LUT, and we trust the pool to validate the rest of the accounts.
                    // This is useful by pools that may integrate with different protocols depending on the version (e.g. CCTP)
                    let n = TOKEN_POOL_LUT_REQUIRED_ENTRIES;
                    // The LUT must at least hold the required entries to compare against.
                    require!(
                        lookup_table_account.addresses.len() >= n,
                        CommonCcipError::InvalidInputsLookupTableAccounts
                    );
                    require!(
                        lookup_table_account.addresses[..n].as_ref() == expected_keys[..n].to_vec(),
                        CommonCcipError::InvalidInputsLookupTableAccounts
                    );
                }
                (false, _) => {
                    // When the pool does not support auto-derivation, the lookup table must contain all the expected accounts,
                    // and CCIP is expected to validate them.
                    require!(
                        lookup_table_account.addresses.as_ref() == expected_keys,
                        CommonCcipError::InvalidInputsLookupTableAccounts
                    );
                }
            }
        }
        {
            // validate pool address writable
            // token admin registry contains an array (binary) of indexes that are writable
            // check that the writability of the passed accounts match the writable configuration (using indexes)
            let mut expected_is_writable: Vec<bool> =
                self.required_entries().map(|x| x.is_writable).collect();

            let mut remaining_is_writable: Vec<bool> =
                match (is_resolution_enabled, use_additional_lut_entries) {
                    (true, true) => {
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
                    }
                    (true, false) => {
                        // when auto-derivation is supported but we don't use additional LUT entries, we only validate the required entries
                        Vec::new() // no remaining accounts to validate
                    }
                    (false, _) => {
                        // when auto-derivation is not supported, we validate all remaining accounts
                        // as they are all expected to be in the LUT
                        remaining_accounts.iter().map(|x| x.is_writable).collect()
                    }
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
    // Checked during fee distribution
    pub vault_ata: UncheckedAccount<'info>,
    #[account(
        // We don't have access to the sender account here, so we can't use the `associated_token::*` constraints.
        // So, we manually check the ATA address and, when requested, that it's initialized (via the ownership check).
        address = get_associated_token_address_with_program_id(
            &token_sender,
            &mint.key(),
            &token_program.key()
        ),
        constraint = !validate_sender_ata || *user_token_account.owner == token_program.key() @ CommonCcipError::InvalidInputsTokenAccounts,
    )]
    pub user_token_account: UncheckedAccount<'info>, // using Unchecked to not deserialize it (save resources)

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

    #[account(
        seeds = [seed::POOL_CHAINCONFIG_SEED_2_0, chain_selector.to_le_bytes().as_ref(), mint.key().as_ref()],
        seeds::program = pool_program.key(),
        // Could be uninitialized, so we don't check for ownership (seeds and seeds::program is sufficient)
        bump,
    )]
    pub pool_chain_config_v2: UncheckedAccount<'info>,

    #[account(
        seeds = [seed::POOL_OVERRIDE_CHAINCONFIG_SEED, chain_selector.to_le_bytes().as_ref(), mint.key().as_ref(), seed::POOL_CONFIG_OVERRIDE_TYPE_FTF],
        seeds::program = pool_program.key(),
        // Could be uninitialized, so we don't check for ownership (seeds and seeds::program is sufficient)
        bump,
    )]
    pub pool_chain_config_override: UncheckedAccount<'info>,

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
        associated_token::mint = mint,
        associated_token::authority = pool_signer,
        associated_token::token_program = token_program
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
    token_receiver_usage: SVMTokenReceiverUsage,
)]
pub struct OfframpTokenAccountsValidationContext<'info> {
    #[account(
        seeds = [
            seed::EXTERNAL_TOKEN_POOLS_SIGNER,
            pool_program.key().as_ref(),
        ],
        bump,
    )]
    pub ccip_offramp_pool_signer: UncheckedAccount<'info>,

    #[account(
        address = if token_receiver_usage.should_derive_ata() {
            get_associated_token_address_with_program_id(
                &token_receiver,
                &mint.key(),
                &token_program.key()
            )
        } else {
            token_receiver
        },

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

    #[account(
        seeds = [seed::POOL_CHAINCONFIG_SEED_2_0, chain_selector.to_le_bytes().as_ref(), mint.key().as_ref()],
        seeds::program = pool_program.key(),
        // Could be uninitialized, so we don't check for ownership (seeds and seeds::program is sufficient)
        bump,
    )]
    pub pool_chain_config_v2: UncheckedAccount<'info>,

    #[account(
        seeds = [seed::POOL_OVERRIDE_CHAINCONFIG_SEED, chain_selector.to_le_bytes().as_ref(), mint.key().as_ref(), seed::POOL_CONFIG_OVERRIDE_TYPE_FTF],
        seeds::program = pool_program.key(),
        // Could be uninitialized, so we don't check for ownership (seeds and seeds::program is sufficient)
        bump,
    )]
    pub pool_chain_config_override: UncheckedAccount<'info>,

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
        associated_token::mint = mint,
        associated_token::authority = pool_signer,
        associated_token::token_program = token_program
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

#[derive(AnchorSerialize, AnchorDeserialize, Clone, Debug)]
pub struct GetVerifierFeeParams {
    pub dest_chain_selector: u64,
    pub message: SVM2AnyMessage,
    pub extra_args: Vec<u8>,
    pub finality: FinalityConfig,
}
