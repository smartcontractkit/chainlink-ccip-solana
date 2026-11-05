// Test fixture for anchor-lint.py — INV-LINT-ACCTINIT and INV-LINT-ATAINIT.
// Not valid Rust; only needs to be parseable by the regex-based linter.
//
// Expected findings from this file:
//   INV-LINT-ACCTINIT:   3  (2 AccountNotInitialized narrow-match fns + 1 `.ok()`-swallow fn)
//   INV-LINT-ATAINIT:    2  (the one `init` ATA struct + the one non-idempotent create CPI;
//                            the `init_if_needed` struct and the `_idempotent` CPI are clean)
//   INV-LINT-REMACCT:    1  (the remaining_accounts try_from without a key check)
//   INV-LINT-CREATEACCT: 1  (the create_account CPI without allocate/assign fallback)

use anchor_lang::prelude::*;

// ── POSITIVE #1 (flagged) ────────────────────────────────────────────────────
// Absence decided from AccountNotInitialized alone; no owner check, no handling
// of AccountOwnedByWrongProgram → exploitable by pre-funding the PDA.
fn optional_config_bad<'info>(account: &'info AccountInfo<'info>) -> Result<Option<Config>> {
    let cfg = match Account::<Config>::try_from(account) {
        Ok(a) => Some(a.into_inner()),
        Err(e) if e == ErrorCode::AccountNotInitialized.into() => None,
        Err(e) => return Err(e),
    };
    Ok(cfg)
}

// ── POSITIVE #2 (flagged) ────────────────────────────────────────────────────
// Same anti-pattern in a second function — proves per-function scoping.
fn optional_other_bad<'info>(account: &'info AccountInfo<'info>) -> Result<Option<Other>> {
    match Account::<Other>::try_from(account) {
        Ok(a) => Ok(Some(a.into_inner())),
        Err(e) if e == ErrorCode::AccountNotInitialized.into() => Ok(None),
        Err(e) => Err(e),
    }
}

// ── NEGATIVE #1 (NOT flagged) ────────────────────────────────────────────────
// Lamport-agnostic: absence is decided from ownership before try_from.
fn optional_config_owner_guard<'info>(
    account: &'info AccountInfo<'info>,
) -> Result<Option<Config>> {
    if account.owner == &system_program::ID {
        return Ok(None);
    }
    let cfg = Account::<Config>::try_from(account)?;
    Ok(Some(cfg.into_inner()))
}

// ── NEGATIVE #2 (NOT flagged) ────────────────────────────────────────────────
// Handles both "uninitialized" error variants, so pre-funding cannot skip the
// absent branch.
fn optional_config_both_errors<'info>(
    account: &'info AccountInfo<'info>,
) -> Result<Option<Config>> {
    let cfg = match Account::<Config>::try_from(account) {
        Ok(a) => Some(a.into_inner()),
        Err(e)
            if e == ErrorCode::AccountNotInitialized.into()
                || e == ErrorCode::AccountOwnedByWrongProgram.into() =>
        {
            None
        }
        Err(e) => return Err(e),
    };
    Ok(cfg)
}

// ── NEGATIVE #3 (NOT flagged) ────────────────────────────────────────────────
// Mandatory account — no absence tolerated, so there is no absence decision to
// get wrong. Propagates every error and never references AccountNotInitialized.
fn mandatory_config<'info>(account: &'info AccountInfo<'info>) -> Result<Config> {
    let cfg = Account::<Config>::try_from(account)?;
    Ok(cfg.into_inner())
}

// ── NEGATIVE #4 (NOT flagged) ────────────────────────────────────────────────
// Explicit suppression for a context where pre-funding is provably impossible.
fn optional_config_suppressed<'info>(
    account: &'info AccountInfo<'info>,
) -> Result<Option<Config>> {
    let cfg = match Account::<Config>::try_from(account) {
        Ok(a) => Some(a.into_inner()),
        Err(e) if e == ErrorCode::AccountNotInitialized.into() => None,
        Err(e) => return Err(e),
    };
    Ok(cfg)
}

// ── INV-LINT-ATAINIT POSITIVE (flagged) ──────────────────────────────────────
// A non-idempotent `init` on an associated token account — anyone can pre-create
// the ATA and permanently block this instruction.
#[derive(Accounts)]
pub struct InitAtaBad<'info> {
    #[account(
        init,
        payer = authority,
        associated_token::mint = mint,
        associated_token::authority = owner,
    )]
    pub receiver: InterfaceAccount<'info, TokenAccount>,
    #[account(mut, address = config.owner)]
    pub authority: Signer<'info>,
}

// ── INV-LINT-ATAINIT NEGATIVE (NOT flagged) ──────────────────────────────────
// Idempotent creation adopts a pre-created ATA instead of failing.
#[derive(Accounts)]
pub struct InitAtaGood<'info> {
    #[account(
        init_if_needed,
        payer = authority,
        associated_token::mint = mint,
        associated_token::authority = owner,
    )]
    pub receiver: InterfaceAccount<'info, TokenAccount>,
    #[account(mut, address = config.owner)]
    pub authority: Signer<'info>,
}

// ── INV-LINT-ATAINIT NEGATIVE (NOT flagged) ──────────────────────────────────
// `init` on a program-owned PDA (no associated_token:: constraint) — only the
// program can allocate it, so it is not pre-creatable by a third party.
#[derive(Accounts)]
pub struct InitPdaOk<'info> {
    #[account(
        init,
        payer = authority,
        seeds = [b"state", mint.key().as_ref()],
        bump,
        space = 8 + 64,
    )]
    pub state: Account<'info, Config>,
    #[account(mut, address = config.owner)]
    pub authority: Signer<'info>,
}

// ── INV-LINT-ATAINIT POSITIVE — CPI form (flagged) ───────────────────────────
// A direct CPI to the non-idempotent ATA-creation instruction.
fn create_receiver_ata_bad() -> Instruction {
    spl_associated_token_account::instruction::create_associated_token_account(
        &payer, &owner, &mint, &token_program,
    )
}

// ── INV-LINT-ATAINIT NEGATIVE — CPI form (NOT flagged) ───────────────────────
// The idempotent variant adopts a pre-created ATA instead of failing.
fn create_receiver_ata_good() -> Instruction {
    spl_associated_token_account::instruction::create_associated_token_account_idempotent(
        &payer, &owner, &mint, &token_program,
    )
}

// ── INV-LINT-ACCTINIT POSITIVE — swallow form (flagged) ──────────────────────
// The deserialization error is discarded with `.ok()`, so a wrong-owner account is
// silently treated as absent.
fn optional_config_swallow_bad<'info>(
    account: &'info AccountInfo<'info>,
) -> Option<Config> {
    Account::<Config>::try_from(account).ok().map(|a| a.into_inner())
}

// ── INV-LINT-REMACCT POSITIVE (flagged) ──────────────────────────────────────
// try_from on a remaining_accounts entry with no require_keys_eq!/find_program_address.
fn read_remaining_bad<'info>(ctx: Context<'info, Empty>) -> Result<u8> {
    let cfg = Account::<'info, Config>::try_from(&ctx.remaining_accounts[0])?;
    Ok(cfg.version)
}

// ── INV-LINT-REMACCT NEGATIVE (NOT flagged) ──────────────────────────────────
// The account's PDA identity is pinned before deserialization.
fn read_remaining_good<'info>(ctx: Context<'info, Empty>) -> Result<u8> {
    let (expected, _) = Pubkey::find_program_address(&[b"cfg"], &crate::ID);
    require_keys_eq!(ctx.remaining_accounts[0].key(), expected);
    let cfg = Account::<'info, Config>::try_from(&ctx.remaining_accounts[0])?;
    Ok(cfg.version)
}

// ── INV-LINT-CREATEACCT POSITIVE (flagged) ───────────────────────────────────
// Manual create_account with no allocate/assign fallback for the pre-funded path.
fn create_pda_bad() -> Result<()> {
    system_program::create_account(cpi_ctx(), lamports, space, &crate::ID)?;
    Ok(())
}

// ── INV-LINT-CREATEACCT NEGATIVE (NOT flagged) ───────────────────────────────
// Branches on the current balance and uses allocate + assign when pre-funded.
fn create_pda_good() -> Result<()> {
    if info.lamports() == 0 {
        system_program::create_account(cpi_ctx(), lamports, space, &crate::ID)?;
    } else {
        system_program::allocate(alloc_ctx(), space)?;
        system_program::assign(assign_ctx(), &crate::ID)?;
    }
    Ok(())
}
