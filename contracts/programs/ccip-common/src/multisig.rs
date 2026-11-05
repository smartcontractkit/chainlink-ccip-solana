use anchor_lang::prelude::*;
use anchor_lang::solana_program::program_pack::Pack;
use anchor_spl::token::spl_token::{self, instruction::MAX_SIGNERS};
use anchor_spl::token_2022::spl_token_2022;
use anchor_spl::token_interface::Mint;
use arrayvec::arrayvec_ro;

/// Unified multisig account representation for both SPL Token and Token-2022 programs.
#[allow(dead_code)]
pub struct MultisigAccount {
    /// Number of signers required (threshold)
    pub m: u8,
    /// Number of valid signers. Private: read it via [`MultisigAccount::len`] so it can't drift
    /// from the slice actually returned by [`MultisigAccount::signers`].
    n: u8,
    /// Is `true` if this structure has been initialized
    pub is_initialized: bool,
    /// Signer public keys. Private: only the first `n` entries are initialized, the rest are
    /// `Pubkey::default()` padding. Go through [`MultisigAccount::signers`] to read them.
    signers: [Pubkey; MAX_SIGNERS],
}

// Read-only: this type is only ever built from unpacked account data and then queried, so it
// exposes no mutation API at all.
arrayvec_ro!(MultisigAccount, Pubkey, MAX_SIGNERS, u8, signers, n);

impl MultisigAccount {
    /// The initialized signers only, i.e. the `..n` prefix of the backing array.
    pub fn signers(&self) -> &[Pubkey] {
        self.as_slice()
    }
}

impl From<spl_token_2022::state::Multisig> for MultisigAccount {
    fn from(multisig: spl_token_2022::state::Multisig) -> Self {
        Self {
            m: multisig.m,
            n: multisig.n,
            is_initialized: multisig.is_initialized,
            signers: multisig.signers,
        }
    }
}

impl From<spl_token::state::Multisig> for MultisigAccount {
    fn from(multisig: spl_token::state::Multisig) -> Self {
        Self {
            m: multisig.m,
            n: multisig.n,
            is_initialized: multisig.is_initialized,
            signers: multisig.signers,
        }
    }
}

/// Returns the mint authority pubkey if present, or `None`.
pub fn get_mint_authority(mint: &InterfaceAccount<Mint>) -> Option<Pubkey> {
    match mint.mint_authority {
        anchor_lang::solana_program::program_option::COption::Some(ma) => Some(ma),
        anchor_lang::solana_program::program_option::COption::None => None,
    }
}

/// Checks whether `authority` is the mint authority directly.
pub fn is_mint_authority(mint: &InterfaceAccount<Mint>, authority: &Pubkey) -> bool {
    get_mint_authority(mint) == Some(*authority)
}

/// Checks whether `authority` is a valid signer in the mint authority's 1-of-N multisig account.
///
/// Returns `true` if:
/// - The multisig account key matches `mint.mint_authority`
/// - The multisig account is owned by the same token program as the mint
/// - The multisig threshold is exactly 1 (only 1-of-N multisigs are supported)
/// - `authority` appears in the multisig's signers list
pub fn is_1_of_n_multisig_signer(
    mint: &InterfaceAccount<Mint>,
    authority: &Pubkey,
    mint_authority_multisig: &AccountInfo,
) -> bool {
    let mint_authority = match get_mint_authority(mint) {
        Some(ma) => ma,
        None => return false,
    };

    // The multisig account key must match the mint authority
    if mint_authority_multisig.key() != mint_authority {
        return false;
    }

    // The multisig account must be owned by the same token program as the mint
    let mint_owner = mint.to_account_info().owner;
    if mint_authority_multisig.owner != mint_owner {
        return false;
    }

    // Deserialize and check signers
    let multisig = match load_multisig(mint_authority_multisig) {
        Ok(Some(ms)) => ms,
        _ => return false,
    };

    if !multisig.is_initialized {
        return false;
    }

    // `Multisig::unpack_from_slice` reads `n` as a raw byte and never checks it against
    // MAX_SIGNERS, so a malformed account can claim more signers than the array can hold.
    if !multisig.is_len_valid() {
        return false;
    }

    // Only 1-of-N multisigs are supported for self-serve onboarding.
    // A threshold > 1 would require multiple signers to co-sign the transaction,
    // which is not supported in this flow.
    if multisig.m != 1 {
        return false;
    }

    // Check if authority is one of the multisig's valid signers
    multisig.signers().iter().any(|s| s == authority)
}

fn load_multisig(mint_authority_multisig: &AccountInfo<'_>) -> Result<Option<MultisigAccount>> {
    let data = mint_authority_multisig.data.borrow();

    match *mint_authority_multisig.owner {
        spl_token_2022::ID => Ok(Some(MultisigAccount::from(
            spl_token_2022::state::Multisig::unpack_from_slice(&data)?,
        ))),
        spl_token::ID => Ok(Some(MultisigAccount::from(
            spl_token::state::Multisig::unpack_from_slice(&data)?,
        ))),
        _ => Ok(None),
    }
}

pub fn is_multisig(mint_authority_multisig: &AccountInfo<'_>) -> bool {
    matches!(load_multisig(mint_authority_multisig), Ok(Some(_)))
}
