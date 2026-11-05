// Hot-path test fixture for anchor-lint.py
// This file is under a "ccip_send" directory so hot-path checks apply.
//
// Expected findings from this file:
//   INV-LINT-CLONE:  1  (2 sites for same var → 1 finding)
//   INV-LINT-ARITH:  1
//   INV-LINT-HEAP:   1

use anchor_lang::prelude::*;

// ── INV-LINT-CLONE (1 finding: 2 clone sites on same var) ───────────────────
fn hot_clone(config: &Config) {
    let a = config.owner.clone();
    let b = config.owner.clone();
    process(a, b);
}

// ── INV-LINT-ARITH (1 finding) ──────────────────────────────────────────────
fn hot_arith(fee: u64, discount: u64) -> u64 {
    fee + discount
}

// ── INV-LINT-HEAP (1 finding) ───────────────────────────────────────────────
fn hot_alloc(data: &[u8]) -> Vec<u8> {
    data.to_vec()
}

// ── Regression: non-allocating forms must NOT be flagged ────────────────────
// `Vec::new()`, `String::new()` and the empty `vec![]` literal defer allocation
// until first use, so none of these is a heap allocation.
fn hot_no_alloc() -> (Vec<u8>, Vec<u8>, String) {
    let a = Vec::new();
    let b = vec![];
    let c = String::new();
    (a, b, c)
}

// ── Regression: HEAP is suppressible like every other rule ──────────────────
fn hot_alloc_suppressed(data: &[u8]) -> Vec<u8> {
    data.to_vec()
}

// ── Regression: clones in mutually exclusive branches are not repeats ───────
// Only one of these two can ever run, so this is not a repeated clone.
fn hot_clone_branches(config: &Config, flag: bool) -> Pubkey {
    if flag {
        config.owner.clone()
    } else {
        config.owner.clone()
    }
}

// ── Regression: clones in separate functions are not repeats ────────────────
fn hot_clone_other_fn(config: &Config) -> Pubkey {
    config.owner.clone()
}

// ── Regression: widening casts are not narrowing ────────────────────────────
struct Widths {
    small: u16,
}

fn hot_widening(w: &Widths) -> u32 {
    w.small as u32
}

// ── Regression: a nearby SAFETY: comment suppresses INV-LINT-HEAP ───────────
// Comment-based suppression via `// anchor-lint:` is gone, but the idiomatic
// `SAFETY:` escape still applies to HEAP. Exercises the preceding-3-lines window.
fn hot_alloc_safety(data: &[u8]) -> Vec<u8> {
    // SAFETY: the caller needs an owned copy; the allocation is unavoidable here.
    data.to_vec()
}
