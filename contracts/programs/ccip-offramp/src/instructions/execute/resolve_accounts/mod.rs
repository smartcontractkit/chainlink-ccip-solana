#![allow(clippy::doc_overindented_list_items, clippy::doc_lazy_continuation)]
//! Account resolution for `execute` (`ccip_execute`).
//!
//! Resolution is a sequential chain of stages: each stage resolves part of the account list the
//! final `execute` instruction needs and points at the next stage via
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
//!   ▼
//! ★ message stage ──↺ (repeats while the messaging accounts do not fit one response)
//!   │
//!   ├─ message receiver v1?  yes, no account   ⇒ DONE
//!   │
//!   ├─ message receiver v2?  yes, receiver v2 accounts  ⇒ DONE
//!   │
//!   ├─ token transfer?  no, no CCVs   ⇒ DONE
//!   │
//!   ├─ token transfer?  no, has CCVs  ⇒ ccv
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
//!   │                                 ├─ done, no CCVs   ⇒ DONE
//!   │                                 └─ done, has CCVs  ⇒ ccv
//!   │
//!   ├─ no, no CCVs   ⇒ DONE
//!   └─ no, has CCVs  ⇒ ccv
//!
//! ★ ccv ──↺ (repeats per CCV in the list, and per nested stage a CCV asks for)
//!    │
//!    ▼
//! ★ DONE
mod common;

mod ccv;
mod message;
mod nested_token;
mod start;
mod token_transfer_lut;
mod token_transfer_static;

pub use ccv::*;
pub use message::*;
pub use nested_token::*;
pub use start::*;
pub use token_transfer_lut::*;
pub use token_transfer_static::*;
