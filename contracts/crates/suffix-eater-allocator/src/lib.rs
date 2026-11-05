//! A suffix-reclaiming allocator for Solana program heaps.
//!
//! Allocation grows upward from Solana's fixed heap start. Every allocation
//! carries a one-word footer, allowing deallocation to mark blocks as free and
//! immediately reclaim every freed block from the back
//!
//! Growing upward lets the allocator use a transaction's requested heap frame
//! without needing a runtime API that reports its effective size. The footer
//! write at the end of every allocation acts as the bounds check: if a block
//! exceeds the runtime-mapped heap, Solana aborts with a heap access violation
//! before the allocator can return an invalid pointer.
//!
//! The allocator API is compiled only for Solana, this crate's unit tests, API
//! documentation, and the `host-testing` feature used by the fuzz target. An
//! ordinary host build intentionally produces an empty crate because the
//! non-atomic allocator is unsupported in that environment.

#![cfg_attr(target_os = "solana", no_std)]

#[cfg(any(target_os = "solana", test, doc, feature = "host-testing"))]
mod allocator;
#[cfg(any(target_os = "solana", test, doc, feature = "host-testing"))]
mod types;

#[cfg(any(target_os = "solana", test, doc, feature = "host-testing"))]
pub use allocator::{Allocator, SOLANA_MAX_HEAP_LENGTH};

/// Op-sequence driver shared by the `cargo-fuzz` target and the Miri corpus replay test.
///
/// Not part of the on-chain build: it exercises [`Allocator`] as an ordinary object over
/// a host arena rather than as a `#[global_allocator]`.
#[cfg(any(test, feature = "host-testing"))]
pub mod harness;

/// Installs [`Allocator`] as the program's `#[global_allocator]`, spanning from `$heap_start`
/// to Solana's largest valid heap frame.
///
/// Takes the heap start address as an argument so the constant stays sourced from the Solana SDK
/// (`solana_program_entrypoint::HEAP_START_ADDRESS`, re-exported by Anchor as
/// `anchor_lang::solana_program::entrypoint::HEAP_START_ADDRESS`) rather than being duplicated here.
///
/// The expansion is gated on three things, all evaluated in the *calling* crate:
///
/// - `feature = "custom-heap"` — a magic feature name. `entrypoint!` omits its own fixed-32KB bump
///   allocator only when the program crate declares and enables a feature with exactly this name,
///   so the caller must declare `custom-heap = []` and put it in `default`. Without that, both this
///   allocator and Anchor's would be declared and the build would fail on two
///   `#[global_allocator]`s.
/// - `not(feature = "no-entrypoint")` — keeps the allocator out of builds where this program is
///   pulled in as a CPI dependency of another program, which installs its own.
/// - `target_os = "solana"` — the allocator is unsupported on the host, where `cargo test` builds
///   the program as an ordinary library.
///
/// The effective heap length is chosen by the *transaction*, not the program: Solana maps only the
/// requested prefix of the region, and each program invocation in a CPI stack gets its own heap of
/// that size. Callers needing more than the 32KB default must ask for it with
/// `ComputeBudgetInstruction::request_heap_frame`.
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
