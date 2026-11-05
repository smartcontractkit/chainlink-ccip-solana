// Test fixture for anchor-lint.py
// This file is intentionally full of lint issues to validate the linter.
// It is NOT valid Rust — it only needs to be parseable by the regex-based linter.
//
// Expected findings:
//   INV-LINT-SERIAL:  1
//   INV-LINT-ASSERT:  1
//   INV-LINT-EVENT:   2  (1 from Rule A in this file, 1 from Rule B in event_b.rs)
//   INV-LINT-SIGNER:  2  (UnsafeSigner + UnsafeInit payer)
//   INV-LINT-INIT:    1
//   INV-LINT-PDA:     1
//   INV-LINT-CLONE:   1  (2 sites on same var → 1 finding)
//   INV-LINT-ARITH:   1
//   INV-LINT-HEAP:    1
//   INV-LINT-CAST:    1
//   INV-LINT-UNWRAP:  1
//   INV-LINT-REALLOC: 1
//   INV-LINT-PARAMS:  1

use anchor_lang::prelude::*;

// ── INV-LINT-PARAMS (1 finding) ─────────────────────────────────────────────
#[program]
pub mod test_lint_program {
    use super::*;

    pub fn bad_params_ix(ctx: Context<MyEventCtx>, a: u64, b: Pubkey) -> Result<()> {
        let _ = (ctx, a, b);
        Ok(())
    }

    pub fn good_params_ix(ctx: Context<MyEventCtx>, params: GoodParams) -> Result<()> {
        let _ = (ctx, params.a, params.b);
        Ok(())
    }
}

pub struct GoodParams {
    pub a: u64,
    pub b: Pubkey,
}

// ── INV-LINT-SERIAL (1 finding) ──────────────────────────────────────────────
fn serialize_bad(data: &MyStruct) -> Vec<u8> {
    data.try_to_vec().unwrap()
}

// ── INV-LINT-ASSERT (1 finding) ──────────────────────────────────────────────
fn check_something(a: u64, b: u64) {
    require!(a == b, MyError::Mismatch);
}

// ── INV-LINT-EVENT Rule A (1 finding) ────────────────────────────────────────
// This file has an #[event_cpi] context, so plain emit! should be flagged.
#[derive(Accounts)]
#[event_cpi]
pub struct MyEventCtx<'info> {
    pub authority: Signer<'info>,
}

fn emit_wrong() {
    emit!(MyEvent { value: 42 });
}

// ── INV-LINT-SIGNER (1 finding) ─────────────────────────────────────────────
// Signer with no address/owner/seeds constraint and no PERMISSIONLESS comment.
#[derive(Accounts)]
pub struct UnsafeSigner<'info> {
    #[account(mut)]
    pub payer: Signer<'info>,
}

// ── INV-LINT-INIT (1 finding) ────────────────────────────────────────────────
// init_if_needed without an authorized signer in the context.
#[derive(Accounts)]
pub struct UnsafeInit<'info> {
    #[account(init_if_needed, payer = payer, space = 8 + 100)]
    pub my_account: Account<'info, MyData>,
    #[account(mut)]
    pub payer: Signer<'info>,
    pub system_program: Program<'info, System>,
}

// init_if_needed with PERMISSIONLESS — should NOT produce a finding.
#[derive(Accounts)]
pub struct PermissionlessInit<'info> {
    #[account(init_if_needed, payer = caller, seeds = [&msg_id], bump, space = 8 + 100)]
    pub my_account: Account<'info, MyData>,
    #[account(mut)]
    // PERMISSIONLESS: account is a PDA seeded by message ID; content validated by constraints.
    pub caller: Signer<'info>,
    pub system_program: Program<'info, System>,
}

// ── INV-LINT-PDA (1 finding) ─────────────────────────────────────────────────
// PDA struct missing version and bump fields.
#[account]
pub struct MissingFields {
    pub data: u64,
    pub owner: Pubkey,
}

// ── Hot-path checks ──────────────────────────────────────────────────────────
// The file path must contain one of: ccip_send, get_fee, execute
// We rely on the test runner passing --root so these are in a "programs/" tree.
// However, the hot-path checks look at the file *path*, not the content.
// We'll put hot-path examples in a separate file under a matching directory.
// ── INV-LINT-CAST (1 finding) ─────────────────────────────────────────────────
// Narrowing cast without SAFETY justification.
fn narrow_cast(val: u64) -> u8 {
    val as u8
}

// This one is suppressed and should NOT produce a finding.
fn safe_cast(val: u64) -> u8 {
    (val & 0xFF) as u8 // SAFETY: masked to 8 bits
}

// Widening cast to u64 — should NOT produce a finding.
fn widening_cast(val: u32) -> u64 {
    val as u64
}

// SAFETY comment on the *following* line — should NOT produce a finding.
fn safe_cast_following(items: &[u8]) -> u32 {
    items.len() as u32
    // SAFETY: Solana tx size ≤ 1232, fits in u32
}

// ── INV-LINT-UNWRAP (1 finding) ───────────────────────────────────────────────
fn unsafe_conversion(data: &[u8]) -> u64 {
    u64::from_be_bytes(data[0..8].try_into().unwrap())
}

// ── INV-LINT-REALLOC (1 finding) ──────────────────────────────────────────────
// realloc without zero = true
#[derive(Accounts)]
pub struct GrowAccount<'info> {
    #[account(mut, realloc = 256, realloc::payer = payer)]
    pub my_data: Account<'info, MyData>,
    #[account(mut, address = config.owner)]
    pub payer: Signer<'info>,
    pub system_program: Program<'info, System>,
}

// This one has realloc::zero = true and should NOT produce a finding.
#[derive(Accounts)]
pub struct GrowAccountSafe<'info> {
    #[account(mut, realloc = 256, realloc::payer = payer, realloc::zero = true)]
    pub my_data: Account<'info, MyData>,
    #[account(mut, address = config.owner)]
    pub payer: Signer<'info>,
    pub system_program: Program<'info, System>,
}
// ── INV-LINT-SET-INNER (1 finding) ────────────────────────────────────────────
// 4 consecutive field assignments to the same account base -> suggest set_inner.
fn populate_full(ctx: Context<SomeCtx>) -> Result<()> {
    ctx.accounts.thing.version = 1;
    ctx.accounts.thing.bump = 2;
    ctx.accounts.thing.owner = key;
    ctx.accounts.thing.router = other;
    Ok(())
}

// Two-field partial update: below the >=3 threshold, should NOT produce a finding.
fn partial_update(ctx: Context<SomeCtx>) -> Result<()> {
    ctx.accounts.thing.version = 1;
    ctx.accounts.thing.bump = 2;
    Ok(())
}

// Suppressed full population: should NOT produce a finding.
fn populate_suppressed(ctx: Context<SomeCtx>) -> Result<()> {
    ctx.accounts.thing.version = 1;
    ctx.accounts.thing.bump = 2;
    ctx.accounts.thing.owner = key;
    Ok(())
}
