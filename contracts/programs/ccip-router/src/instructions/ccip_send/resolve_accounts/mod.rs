#![allow(clippy::doc_overindented_list_items, clippy::doc_lazy_continuation)]
//! Account resolution for `ccip_send_v2`.
//!
//! Resolution is a sequential chain of stages: each stage resolves part of the account list the
//! final `ccip_send_v2` instruction needs and points at the next stage via
//! `ResolveAccountsResponse.next_ix_discriminator`, threading state through serialized metadata.
//! Each stage lives in its own module below; [`common`] holds the helpers and metadata shared by
//! more than one stage.
//!
//! Stage flow
//! ─────────
//! Stages are marked with `★`. `↺` marks a stage that may repeat;
//! `⇒ name` is an edge that jumps ahead to the named stage.
//!
//! ★ start
//!   │
//!   │
//!   ▼
//! * fee_token_billing
//!   |
//!   |
//!   ├─ token transfer?  no  ⇒ executor
//!   │
//!   └─ token transfer?  yes
//!                        │
//!                        ▼
//!           ★ token_transfer_lut
//!               │
//!               ▼
//! ★ token_transfer_static ──↺ (repeats while the pool's lookup table does not fit one response)
//!   │
//!   ├─ v2 pool implements resolution?  yes
//!   │                                   │
//!   │                                   ▼
//!   │                            ★ nested_token ──↺ (repeating once per nested stage)
//!   │                                 │
//!   │                                 └─ done  ⇒ executor
//!   │
//!   └─ no  ⇒ executor
//!
//! ★ executor
//!   │
//!   ▼
//! ★ construct_final_ccv_list
//!   │
//!   ├─ no CCVs   ⇒ DONE
//!   └─ has CCVs
//!        │
//!        ▼
//! ★ ccv ──↺ (repeats per CCV in the list, and per nested stage a CCV asks for)
//!    │
//!    ▼
//! ★ DONE

pub(crate) mod common;

mod ccv;
mod construct_final_ccv_list;
mod executor;
mod fee_token_billing;
mod nested_token;
mod start;
mod token_transfer_lut;
mod token_transfer_static;

pub use ccv::*;
pub use construct_final_ccv_list::*;
pub use executor::*;
pub use fee_token_billing::*;
pub use nested_token::*;
pub use start::*;
pub use token_transfer_lut::*;
pub use token_transfer_static::*;
