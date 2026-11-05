//! A probing, non-reclaiming bump allocator for Solana program heaps.
//!
//! Allocation grows upward from Solana's fixed heap start, and never comes back down:
//! `dealloc` is a no-op and `realloc` is a fresh allocation plus a copy. The only state is
//! one word at the base of the heap holding the cursor offset.
//!
//! Because a program cannot ask the runtime how large its heap frame actually is, the
//! allocator is configured with the largest frame Solana can grant and finds the real end
//! by touching it: every allocation performs a `write_volatile` at the last word of its
//! reservation before the frontier moves and before the pointer is returned. The stored
//! value is irrelevant — what matters is that the store is emitted, so that a reservation
//! past the mapped heap aborts the instruction with a heap access violation instead of
//! becoming a live pointer into unmapped memory.
//!
//! Compared to [`suffix-eater-allocator`](../../suffix-eater-allocator), this allocator
//! trades memory for simplicity: there is no per-allocation footer, no reclamation, and no
//! metadata a mismatched `dealloc` could corrupt, but peak usage is the sum of everything
//! an instruction ever allocates rather than its high-water mark of live bytes.
//!
//! The allocator API is compiled only for Solana, this crate's unit tests, API
//! documentation, and the `host-testing` feature used by the fuzz target. An ordinary host
//! build intentionally produces an empty crate because the non-atomic allocator is
//! unsupported in that environment.

#![cfg_attr(target_os = "solana", no_std)]

#[cfg(any(target_os = "solana", test, doc, feature = "host-testing"))]
mod allocator;
#[cfg(any(target_os = "solana", test, doc, feature = "host-testing"))]
mod types;

#[cfg(any(target_os = "solana", test, doc, feature = "host-testing"))]
pub use allocator::{Allocator, PROBE_PATTERN, SOLANA_MAX_HEAP_LENGTH};

#[cfg(any(test, feature = "host-testing"))]
pub use allocator::probe_word_of;

/// Op-sequence driver shared by the `cargo-fuzz` target and the Miri corpus replay test.
///
/// Not part of the on-chain build: it exercises [`Allocator`] as an ordinary object over a
/// host arena rather than as a `#[global_allocator]`.
#[cfg(any(test, feature = "host-testing"))]
pub mod harness;

/// Installs [`Allocator`] as the program's `#[global_allocator]`, spanning from
/// `$heap_start` to Solana's largest valid heap frame.
///
/// Takes the heap start address as an argument so the constant stays sourced from the
/// Solana SDK (`solana_program_entrypoint::HEAP_START_ADDRESS`, re-exported by Anchor as
/// `anchor_lang::solana_program::entrypoint::HEAP_START_ADDRESS`) rather than being
/// duplicated here.
///
/// The expansion is gated on three things, all evaluated in the *calling* crate:
///
/// - `feature = "custom-heap"` — a magic feature name. `entrypoint!` omits its own
///   fixed-32KB bump allocator only when the program crate declares and enables a feature
///   with exactly this name, so the caller must declare `custom-heap = []` and put it in
///   `default`. Without that, both this allocator and Anchor's would be declared and the
///   build would fail on two `#[global_allocator]`s.
/// - `not(feature = "no-entrypoint")` — keeps the allocator out of builds where this
///   program is pulled in as a CPI dependency of another program, which installs its own.
/// - `target_os = "solana"` — the allocator is unsupported on the host, where `cargo test`
///   builds the program as an ordinary library.
///
/// The effective heap length is chosen by the *transaction*, not the program: Solana maps
/// only the requested prefix of the region, and each program invocation in a CPI stack gets
/// its own heap of that size. Callers needing more than the 32KB default must ask for it
/// with `ComputeBudgetInstruction::request_heap_frame`. Since this allocator never
/// reclaims, a program installing it should size that request against the *total* it
/// allocates, not its live set.
#[macro_export]
macro_rules! install_global_allocator {
    ($heap_start:expr) => {
        #[cfg(all(
            feature = "custom-heap",
            not(feature = "no-entrypoint"),
            target_os = "solana"
        ))]
        #[global_allocator]
        static GLOBAL_ALLOCATOR: $crate::Allocator = unsafe {
            $crate::Allocator::with_fixed_address_range($heap_start, $crate::SOLANA_MAX_HEAP_LENGTH)
        };
    };
}
