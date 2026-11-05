# Solana Lint Invariants

Static analysis checks enforced by `scripts/anchor-lint/src/anchor-lint.py`. Run with `make anchor-lint` (report only) or `make anchor-lint-strict` (CI, exit 1 on any finding).

```
python3 scripts/anchor-lint/src/anchor-lint.py --only INV-LINT-SIGNER INV-LINT-PDA   # run specific checks
python3 scripts/anchor-lint/src/anchor-lint.py --list-checks                         # print check table
```

Findings are grouped by file and printed as `file:line  [severity]  INV-LINT-XXX  message`.

Exit code is **1** when any `error`-level finding is present (or on any finding with `--strict`).

---

## Suppressing Individual Findings

For **INV-LINT-SIGNER** and **INV-LINT-PDA**, add a comment in the surrounding code:

```rust
// PERMISSIONLESS: this instruction is intentionally open to any caller
pub authority: Signer<'info>,

// anchor-lint: suppress INV-LINT-PDA
#[account]
pub struct MyLegacyAccount { ... }
```

No other inline suppression mechanism exists — fix the code or add the appropriate comment.

---

## Checks

### INV-LINT-SERIAL — `.try_to_vec()` usage `[error]`

`try_to_vec()` is deprecated; it serialises into a newly allocated `Vec` with fallible
borsh encoding, hiding the allocation and masking the error path.

**What to use instead:**
- `borsh::to_vec(&value)?` — explicit, same semantics
- Write directly into a pre-allocated buffer with `borsh::BorshSerialize::serialize`

**Example (bad):**
```rust
let bytes = my_struct.try_to_vec()?;
```
**Example (good):**
```rust
let bytes = borsh::to_vec(&my_struct)?;
```

---

### INV-LINT-ASSERT — `require!()` — consider a typed variant `[info]`

Every `require!()` call is flagged as a reminder that Anchor provides narrower assertion
macros that produce better error messages.

| Typed macro | Condition | Trait requirements |
|---|---|---|
| `require_eq!(a, b, err)` | `a == b` | `PartialEq + Display` |
| `require_neq!(a, b, err)` | `a != b` | `PartialEq + Display` |
| `require_gt!(a, b, err)` | `a > b` | `PartialOrd + Display` |
| `require_gte!(a, b, err)` | `a >= b` | `PartialOrd + Display` |
| `require_keys_eq!(a, b, err)` | `a == b` (Pubkeys) | — |
| `require_keys_neq!(a, b, err)` | `a != b` (Pubkeys) | — |

If the type does not implement `Display`, fall back to plain `require!()` — that is fine.
This check is `info` only and does not block CI.

For an arbitrary boolean predicate that has no typed equivalent (a compound `&&`/`||`, or a
`bool`-returning method such as `x.is_valid()` or `!set.contains(y)`), silence the reminder with a
suppression comment on the `require!(` line or within the two lines above it:

```rust
// anchor-lint: suppress INV-LINT-ASSERT (boolean predicate, no typed variant applies)
require!(config.is_valid(), MyError::InvalidConfig);
```

---

### INV-LINT-EVENT — `emit!` / `emit_cpi!` inconsistency with `#[event_cpi]` contexts `[warning]`

Three complementary rules:

**Rule B — `emit_cpi!` in a program with no `#[event_cpi]` contexts:**
If a program has never defined a `#[event_cpi]` context, the event-authority accounts are
never added to any instruction. Calling `emit_cpi!()` will reference accounts that are not
present and cause a runtime failure.

```rust
// bad — program has no #[event_cpi] anywhere
emit_cpi!(MyEvent { ... });

// good
emit!(MyEvent { ... });
```

**Rule C — `emit!` in a program (or its dependencies) that has `#[event_cpi]` contexts:**
When a program uses `#[event_cpi]` in any instruction context, new event emissions should
use `emit_cpi!()` for consistency.  Legacy (v1.6) methods that predate `#[event_cpi]`
adoption may retain `emit!()` with a suppression comment.

This rule also checks **workspace dependency crates** (detected via `path = "../..."`
dependencies in `Cargo.toml`).  For example, if `burnmint-token-pool` has `#[event_cpi]`
and depends on `base-token-pool`, then `emit!()` calls in `base-token-pool` are also
flagged to enforce consistency across the crate graph.

```rust
// bad — program has #[event_cpi] in another file
emit!(MyEvent { ... });

// good
emit_cpi!(MyEvent { ... });
```

To suppress for intentional `emit!()` usage (e.g. a legacy v1.6 method that predates
`#[event_cpi]` adoption):

```rust
// anchor-lint: suppress INV-LINT-EVENT
emit!(MyEvent { ... });
```

---

### INV-LINT-SIGNER — `Signer<'info>` without an authorization constraint `[error]`

**Related invariants:** INV-LCFG-7, INV-LCFG-10, INV-RTR-5, INV-PCFG-3 (owner-only operations must enforce access control).

A `Signer` account with no constraint means **any wallet can sign the transaction**.
Unless the instruction is truly permissionless, every `Signer` field must have one of:

| Constraint | Meaning |
|---|---|
| `#[account(address = config.owner @ Err::Unauthorized)]` | Exact key check |
| `#[account(owner = some_program.key() @ Err::Unauthorized)]` | Owner-program check |
| `#[account(seeds = [...], bump)]` | PDA — only the owning program can sign |

For genuinely open instructions, add a comment instead of a constraint:

```rust
// PERMISSIONLESS: anyone may submit a price update; access is gated by the
// signed data, not by the caller's identity.
pub caller: Signer<'info>,
```

Indirect authorization (e.g. `constraint = program_data.upgrade_authority_address == Some(authority.key())`)
is also recognised and will not trigger this check.

---

### INV-LINT-INIT — `init_if_needed` without an authorized `Signer` `[warning]`

**Related invariants:** INV-LINT-SIGNER (access control on signers).

`init_if_needed` creates an account on the first call and re-uses it on subsequent calls.
Without an address-constrained `Signer` in the same context, any caller can front-run the
initialization and pre-populate the account with attacker-controlled data before the
legitimate owner calls the instruction.

Every context that uses `init_if_needed` must contain a `Signer` with an `address =` or
`seeds =` constraint (see INV-LINT-SIGNER), or an indirect key check that references the signer.
If the instruction is genuinely permissionless, add a `// PERMISSIONLESS: <reason>` comment
near the `Signer` field to suppress this warning (same convention as INV-LINT-SIGNER).

```rust
// bad — anyone can be the authority
#[account(init_if_needed, payer = authority, ...)]
pub my_account: Account<'info, MyState>,
pub authority: Signer<'info>,

// good — constrained signer
#[account(address = config.owner @ Error::Unauthorized)]
pub authority: Signer<'info>,

// good — permissionless with justification
#[account(init_if_needed, payer = caller, seeds = [&msg_id], bump, ...)]
pub my_account: Account<'info, MyState>,
// PERMISSIONLESS: account is a PDA seeded by the message ID; content is validated
// by constraints, so front-running cannot inject attacker-controlled data.
pub caller: Signer<'info>,
```

---

### INV-LINT-INSTR — `#[instruction]` param order mismatch `[error]`

Anchor decodes instruction arguments from the transaction data **by position**, not by name.
The parameters listed in `#[instruction(...)]` must appear in the **same order** as the
instruction function's parameters (after `ctx`).

If the same parameter names appear in both but in a different order → **error** (clear swap).
If names differ between `#[instruction]` and the function signature → **error** (could be
a silent positional mismatch disguised as a cosmetic rename).

```rust
// bad — b is listed first but is the second fn argument
#[instruction(b: Pubkey, a: u64)]
pub struct MyContext<'info> {
    #[account(seeds = [b.as_ref()], ...)]
    pub my_account: Account<'info, MyState>,
}

pub fn my_ix(ctx: Context<MyContext>, a: u64, b: Pubkey) -> Result<()> { ... }
```

```rust
// good
#[instruction(a: u64, b: Pubkey)]
pub struct MyContext<'info> { ... }
```

Note: Rust's convention of prefixing unused parameters with `_` (e.g. `_a` in the handler
while `#[instruction]` uses `a`) is recognised and does not trigger this check.

---

### INV-LINT-PARAMS — Program handlers should use a single `*Params` arg after `Context` `[warning]`

For Anchor `#[program]` entrypoints, prefer a single instruction params struct after
`Context<...>` instead of multiple scalar arguments.

Why this pattern is preferred:
- Reduces risk of positional argument/IDL parsing mistakes.
- Scales better when account resolution and instruction payload evolve.
- Keeps instruction APIs consistent across programs.

```rust
// bad
pub fn init_chain_config_override(
    ctx: Context<InitializeChainConfigOverride>,
    remote_chain_selector: u64,
    mint: Pubkey,
    override_type: OverrideType,
    inbound: RateLimitConfig,
    outbound: RateLimitConfig,
) -> Result<()> { ... }

// good
pub fn init_chain_config_override(
    ctx: Context<InitializeChainConfigOverride>,
    params: InitChainConfigOverrideParams,
) -> Result<()> { ... }
```

To suppress intentional legacy signatures:

```rust
// anchor-lint: suppress INV-LINT-PARAMS
pub fn legacy_ix(ctx: Context<LegacyCtx>, a: u64, b: Pubkey) -> Result<()> { ... }
```

---

### INV-LINT-PDA — PDA `#[account]` struct missing `version` / `bump` fields `[error]`

**Related invariants:** INV-UPG-1 through INV-UPG-4 (upgradability), INV-REPLAY-1 (execution state persistence across upgrades).

Every persistent PDA account struct must declare both fields:

```rust
pub version: u8,  // guards against use with wrong program version
pub bump:    u8,  // stored so seeds can be recomputed without find_program_address
```

**Why `version`:**  Without a version field, an upgraded program cannot distinguish between
accounts written by an old version and accounts written by the new one. This makes safe
migrations impossible.

**Why `bump`:**  Storing the canonical bump avoids repeated calls to
`find_program_address` (which iterates through bumps 255 → 0) on every instruction. It
also makes the account self-describing.

**Persist both fields on initialization (not lint-enforced — reviewer must check).**
Declaring the fields is not enough: the instruction that creates the PDA (`init` /
`init_if_needed`) must **write** them in its handler body, otherwise they stay `0`. A stored
`bump` of `0` breaks every later `bump = account.bump` constraint (INV-LINT-BUMP): Anchor
derives the address with bump `0`, which does not match the account created with the canonical
bump, so the seeds check fails and the account can never be read again. A `version` of `0`
defeats the migration guard. The `init` constraint itself uses a **bare `bump`** (Anchor computes
the canonical bump); the handler then persists it from `ctx.bumps`:

```rust
// context: init uses bare `bump` (canonical bump is computed here)
#[account(init, seeds = [SEED], bump, payer = authority,
          space = ANCHOR_DISCRIMINATOR + MyPda::INIT_SPACE)]
pub my_pda: Account<'info, MyPda>,

// handler: persist version + bump so later `bump = my_pda.bump` reads succeed
pub fn initialize(ctx: Context<Initialize>) -> Result<()> {
    let bump = ctx.bumps.my_pda;
    let pda = &mut ctx.accounts.my_pda;
    pda.version = MY_PDA_V;   // schema version constant
    pda.bump = bump;
    Ok(())
}
```

This step is handler-body logic, so `anchor-lint` cannot detect its absence — verify it during
review whenever a PDA is created.

To suppress for an existing account that intentionally omits these fields:

```rust
// anchor-lint: suppress INV-LINT-PDA
#[account]
pub struct LegacyAccount {
    pub data: u64,
}
```

Structs whose names end in `Params`, `Event`, `Response`, `Output`, or `Input` are
excluded automatically (they are transient data types, not PDAs).

---

### INV-LINT-PDA-SEEDS — PDA variable seeds not present as account fields `[error]`

**Related invariants:** INV-LINT-PDA (PDA struct must store `version` and `bump`).

Every variable seed used to derive a PDA must be stored as a field (directly or via a
nested struct) in the account. This ensures the account is self-describing and can always
be identified from its own data.

**What counts as a variable seed:**
- Chain selectors (e.g. `chain_selector.to_le_bytes()`)
- Pubkeys (e.g. `mint.key().as_ref()`)
- Enum variants (e.g. `override_type.as_ref()`)

**What does NOT count (constant seeds):**
- ALL_CAPS constants (e.g. `POOL_STATE_SEED`, `CONFIG`, `REMOTE_CHAIN`)
- Literal byte strings (e.g. `b"config"`)

**How seeds are detected:**
The check looks for comments near the struct that document the PDA seeds, following these
patterns:
- `// seeds: [...]`
- `// PDA with seeds: [...]`
- `// Stored in ... PDA with seeds:`

```rust
// bad — seeds include chain_selector but it's not a field
// Stored in PDA with seeds: [REMOTE_CHAIN, chain_selector.to_le_bytes()]
#[account]
pub struct RemoteChainConfig {
    pub version: u8,
    pub bump: u8,
    pub config: SomeConfig,  // chain_selector is missing!
}

// good — all variable seeds are fields (chain_selector is in base)
// Stored in PDA with seeds:
//   [POOL_OVERRIDE_CHAINCONFIG_SEED, chain_selector, mint, override_type]
#[account]
pub struct ChainConfigOverride {
    pub bump: u8,
    pub version: u8,
    pub base: BaseOverrideConfig,  // contains: remote_chain_selector, mint, override_type
}

// good — all variable seeds are direct fields
// seeds: [DEST_CHAIN, chain_selector.to_le_bytes()]
#[account]
pub struct DestChain {
    pub version: u8,
    pub chain_selector: u64,  // ✓ matches seed
    pub state: DestChainState,
    pub config: DestChainConfig,
}
```

To suppress for an existing account (e.g. marker accounts or legacy accounts):

```rust
// anchor-lint: suppress INV-LINT-PDA-SEEDS
#[account]
pub struct AllowedOfframp {}  // marker account — existence is the only data
```

---

### INV-LINT-CLONE — Repeated `.clone()` on the same variable in a hot path `[warning]`

Applies to files whose path contains `ccip_send`, `get_fee`, or `execute`.

Cloning a value twice means allocating twice. In a hot path this matters. Either:
- Pass the value by reference instead of cloning, or
- Clone once into a local and reuse the local.

```rust
// bad — two heap allocations for the same data
let a = send_ctx(msg.data.clone());
let b = log_ctx(msg.data.clone());

// good
let data = msg.data.clone();
let a = send_ctx(&data);
let b = log_ctx(&data);
```

---

### INV-LINT-ARITH — Raw arithmetic operator in a hot path `[info]`

**Related invariants:** INV-ENC-10 (numeric range safety), INV-ENC-12 (no narrowing in intermediate conversions).

Applies to files whose path contains `ccip_send`, `get_fee`, or `execute`.

All build profiles already set `overflow-checks = true`, so integer overflow will panic at
runtime. This check is an `[info]`-level reminder to auditors that these lines exist and
to verify that overflow is structurally impossible (e.g. values are bounded by protocol
constants) or to use explicit safe-math for documentation purposes.

```rust
// acceptable if fee and max_fee are u64 and fee <= max_fee is enforced above
let remaining = max_fee - fee;

// preferred in financial calculations
let remaining = max_fee.checked_sub(fee).ok_or(Error::Underflow)?;
```

---

### INV-LINT-HEAP — Heap allocation in a hot path `[warning]`

Applies to files whose path contains `ccip_send`, `get_fee`, or `execute`.

`Box::new`, `Vec::new`, `vec![]`, `.to_vec()`, `String::new`, and `String::from` all
trigger a heap allocation. In a hot path, prefer:

- **Fixed-size arrays** on the stack when the size is statically known.
- **Pre-allocated buffers** passed in from the caller.
- **`arrayvec::ArrayVec`** (already a workspace dependency) for small variable-length
  sequences with a known upper bound.

```rust
// bad — allocates on every call
let mut buf = Vec::new();

// good — stack-allocated, no allocator involved
let mut buf = arrayvec::ArrayVec::<u8, 64>::new();
```

---

### INV-LINT-CAST — Narrowing `as` cast `[warning]`

**Related invariants:** INV-ENC-10 (numeric range safety), INV-ENC-12 (no narrowing in intermediate conversions).

A bare `as u8`, `as u16`, `as u32`, or `as u64` (and signed variants) silently truncates
values that exceed the target type's range. On Solana this produces incorrect data without
any runtime signal — unlike `try_into()` which returns an error.

This applies to **all** program files, not just hot paths. Encoding and decoding bugs are
critical regardless of frequency.

```rust
// bad — silently truncates if val > 255
let byte = val as u8;

// good — returns an error if val > 255
let byte: u8 = val.try_into().map_err(|_| error!(MyError::Overflow))?;
```

To suppress a false positive where the range is provably safe:

```rust
let byte = (val & 0xFF) as u8; // SAFETY: masked to 8 bits
```

---

### INV-LINT-UNWRAP — `try_into()` / `try_from().unwrap()` — fallible conversion `[warning]`

**Related invariants:** INV-ENC-10 (values must survive round-trips without panic).

`.try_into().unwrap()` and `try_from(...).unwrap()` will panic on conversion failure,
causing an opaque Solana transaction abort with no useful error message. In production
code, always propagate the error:

```rust
// bad — panics if data is not exactly 8 bytes
let arr: [u8; 8] = data[0..8].try_into().unwrap();

// good
let arr: [u8; 8] = data[0..8]
    .try_into()
    .map_err(|_| error!(MyError::InvalidSliceLength))?;
```

To suppress where the conversion is provably infallible:

```rust
let arr: [u8; 8] = data[0..8].try_into().unwrap(); // SAFETY: slice is exactly 8 bytes
```

---

### INV-LINT-REALLOC — `realloc` without `realloc::zero = true` `[info]`

**Related invariants:** INV-ATOMIC-2 (config updates must not corrupt state), INV-UPG-1–4 (stale bytes after realloc could be misinterpreted by upgraded programs).

When an account is grown via `realloc`, the newly allocated bytes are uninitialised by
default. If the program or a future upgrade reads those bytes as structured data, stale
memory contents could be misinterpreted.

Adding `realloc::zero = true` zeroes the extended region, preventing stale-data bugs at
the cost of a small compute overhead.

```rust
// flagged — extended bytes are uninitialised
#[account(mut, realloc = new_size, realloc::payer = payer)]
pub my_account: Account<'info, MyData>,

// clean
#[account(mut, realloc = new_size, realloc::payer = payer, realloc::zero = true)]
pub my_account: Account<'info, MyData>,
```

To suppress:

```rust
// anchor-lint: suppress INV-LINT-REALLOC
#[account(mut, realloc = new_size, realloc::payer = payer)]
pub my_account: Account<'info, MyData>,
```

---

### INV-LINT-BUMP — Bare `bump` in non-init PDA field constraint `[error]`

**Related invariants:** INV-LINT-PDA (PDA struct must store `bump: u8`).

When accessing an existing PDA (not initialising it), the `#[account]` field constraint
should reference a **stored** bump (`bump = stored_bump`) instead of using a bare
`bump`. The bump can be stored on the PDA itself (for example `bump = account.bump`) or
in another validated account (for example a central config PDA). A bare `bump` forces
Anchor to call
`find_program_address` at runtime, which iterates bumps 255 → 0 until one works — wasting
compute units and ignoring the bump already stored in the account.

During `init` or `init_if_needed`, bare `bump` is correct because the account does not
exist yet and the bump must be computed.  Bare `bump` is also acceptable for
`UncheckedAccount` fields, which are used when the account state may or may not have been
migrated to a new version — the program must first read which version the PDA stores and
then parse it accordingly.  This requires manual validations (owner, discriminator, etc.)
that Anchor would normally handle, but `seeds` + `bump` still provides address validation.

```rust
// bad — recomputes bump at runtime
#[account(
    mut,
    seeds = [POOL_SEED, mint.key().as_ref()],
    bump,
)]
pub state: Account<'info, State>,

// good — uses stored bump
#[account(
    mut,
    seeds = [POOL_SEED, mint.key().as_ref()],
    bump = state.bump,
)]
pub state: Account<'info, State>,
```

During initialisation, bare `bump` is expected:

```rust
// good — bump is computed during init
#[account(
    init,
    seeds = [POOL_SEED, mint.key().as_ref()],
    bump,
    payer = authority,
    space = ANCHOR_DISCRIMINATOR + State::INIT_SPACE,
)]
pub state: Account<'info, State>,
```

To suppress for legacy accounts that do not store a bump:

```rust
// anchor-lint: suppress INV-LINT-BUMP
#[account(
    seeds = [LEGACY_SEED],
    bump,
)]
pub legacy: Account<'info, LegacyAccount>,
```

---

### INV-LINT-ACCTINIT — account-absence keyed on `AccountNotInitialized` `[error]`

**Related invariants:** [INV-ACCT-INIT-2, INV-ACCT-INIT-3](ACCOUNT_INIT_INVARIANTS.md) (absence detection must be lamport-agnostic).

When an instruction accepts an **optional** account and falls back to a default when the
account is absent, "absent" must be decided from the account's **owner**, not from a
single Anchor error variant.

Anchor's `Account::try_from` returns `ErrorCode::AccountNotInitialized` **only** when the
account is owned by the System Program **and** has zero lamports. A System-Program transfer
requires only the *sender* to sign — the recipient never signs — so **anyone can pre-fund
any address**, including an unallocated PDA whose seeds are public. A pre-funded PDA is
still System-owned but no longer zero-lamport, so `try_from` returns
`AccountOwnedByWrongProgram` instead. Code that maps only `AccountNotInitialized` to the
"absent" branch propagates the other error and reverts — a cheap, permissionless,
persistent denial of service on that instruction.

This check flags two forms whose enclosing function does **not** contain a lamport-agnostic
safeguard (an `owner`-vs-System-Program comparison, or explicit `AccountOwnedByWrongProgram`
handling):
1. a reference to `AccountNotInitialized` used to decide absence; and
2. a typed-account deserialization whose Result is swallowed with `.ok()` / `unwrap_or(..)`
   — which additionally treats a wrong-owner or malformed account as "absent".

**Example (bad):**
```rust
let cfg = match Account::<Config>::try_from(account_info) {
    Ok(a) => Some(a.into_inner()),
    Err(e) if e == ErrorCode::AccountNotInitialized.into() => None, // flagged
    Err(e) => return Err(e),
};
```

**Example (good) — decide absence from ownership (lamport-agnostic):**
```rust
if account_info.owner == &system_program::ID {
    return Ok(None);
}
let cfg = Account::<Config>::try_from(account_info)?; // enforces owner + discriminator
```

**Example (good) — handle both "uninitialized" error variants:**
```rust
Err(e) if e == ErrorCode::AccountNotInitialized.into()
       || e == ErrorCode::AccountOwnedByWrongProgram.into() => None,
```

**Example (bad) — swallowed error:**
```rust
let cfg = Account::<Config>::try_from(account_info).ok(); // flagged
```

**Not flagged:** a plain `Account::<T>::try_from(info)?` that propagates every error — it
does not tolerate absence, so there is no absence decision to get wrong.

To suppress where the surrounding context provably makes pre-funding impossible (for
example, the account is required to be a signer, or is allocated earlier in the same
instruction):

```rust
// anchor-lint: suppress INV-LINT-ACCTINIT
Err(e) if e == ErrorCode::AccountNotInitialized.into() => None,
```

A `// SAFETY: <reason>` comment within the 4 lines above the match arm suppresses it too.

---

### INV-LINT-ATAINIT — non-idempotent `init` on an associated token account `[error]`

**Related invariants:** [INV-ACCT-INIT-5](ACCOUNT_INIT_INVARIANTS.md) (initialization of permissionlessly-creatable accounts must be idempotent).

Associated Token Account creation is **permissionless**: anyone can create the ATA for any
`(mint, authority)` pair with a standard instruction. A non-idempotent `init` performs a
hard create that fails if the account already exists, so a third party can pre-create the
ATA and **permanently block** the instruction — including an admin recovery path that
depends on it. `init_if_needed` adopts an existing ATA instead of failing.

This check flags two forms:
1. a field `#[account(...)]` constraint combining a bare `init` with an `associated_token::`
   constraint; and
2. a direct CPI to the non-idempotent create (`create_associated_token_account` /
   `associated_token::create`) instead of its `_idempotent` variant.

**Example (bad):**
```rust
#[account(
    init,                                       // flagged
    payer = authority,
    associated_token::mint = fee_token_mint,
    associated_token::authority = fee_billing_signer,
)]
pub fee_token_receiver: InterfaceAccount<'info, TokenAccount>,
```

**Example (good):**
```rust
#[account(
    init_if_needed,
    payer = authority,
    associated_token::mint = fee_token_mint,
    associated_token::authority = fee_billing_signer,
)]
pub fee_token_receiver: InterfaceAccount<'info, TokenAccount>,
```

**Example (bad) — CPI form:**
```rust
let ix = spl_associated_token_account::instruction::create_associated_token_account(/* … */); // flagged
```
**Example (good) — CPI form:**
```rust
let ix = spl_associated_token_account::instruction::create_associated_token_account_idempotent(/* … */);
```

**Not flagged:** program-owned PDAs created with `init` (only the program can allocate and
assign them, so a third party cannot pre-create them), ATA fields that merely reference an
existing account (no `init`), and the `_idempotent` CPI variants.

Note: switching to `init_if_needed` requires the account's instruction to enforce
authorization on its signer — see [INV-LINT-INIT](#inv-lint-init).

To suppress:

```rust
// anchor-lint: suppress INV-LINT-ATAINIT
#[account(init, payer = authority, associated_token::mint = m, associated_token::authority = a)]
pub receiver: InterfaceAccount<'info, TokenAccount>,
```

---

### INV-LINT-REMACCT — `remaining_accounts` manual deserialization without an identity check `[warning]`

**Related invariants:** [INV-ACCT-INIT-4](ACCOUNT_INIT_INVARIANTS.md) (manual deserialization must validate owner, length, and identity).

`Account::try_from` / `try_from_slice` on a `remaining_accounts` entry proves the account's
**owner + discriminator**, but not its **key/PDA identity**. Without an explicit
`require_keys_eq!` against the expected address, a caller can substitute a *different, valid*
account of the same type (e.g. the config for a different chain/token), which then drives
whatever the handler derives from it.

This check flags a `try_from`/`try_from_slice` on a `remaining_accounts[..]` entry whose
enclosing function contains no `require_keys_eq!` / `find_program_address` identity pin. It
is a `warning` because a function may legitimately not need to pin identity (approximated
without data flow).

**Example (bad):**
```rust
let cfg = Account::<ChainConfig>::try_from(&ctx.remaining_accounts[0])?; // flagged
```

**Example (good):**
```rust
let (expected, _) = Pubkey::find_program_address(&[SEED, sel.to_le_bytes().as_ref(), mint.as_ref()], &crate::ID);
require_keys_eq!(ctx.remaining_accounts[0].key(), expected, MyError::InvalidInputs);
let cfg = Account::<ChainConfig>::try_from(&ctx.remaining_accounts[0])?;
```

To suppress:
```rust
// anchor-lint: suppress INV-LINT-REMACCT
let cfg = Account::<ChainConfig>::try_from(&ctx.remaining_accounts[0])?;
```

---

### INV-LINT-CREATEACCT — manual `create_account` without pre-funded handling `[warning]`

**Related invariants:** [INV-ACCT-INIT-6](ACCOUNT_INIT_INVARIANTS.md) (manual account creation must tolerate a pre-funded address).

The System Program's `create_account` fails if the target address already holds lamports.
For a public-seed PDA — whose address anyone can pre-fund — a bare `create_account` can be
permanently griefed. The robust pattern branches on the current balance and uses
`allocate` + `assign` (transferring only the rent-exemption delta) when the account is
already funded. Anchor's `#[account(init)]` does this internally.

This check flags a `create_account(` CPI whose enclosing function contains no `allocate` /
`assign` fallback. It is a `warning` (the pre-funded handling is detected heuristically).

**Example (good):**
```rust
if account_info.lamports() == 0 {
    system_program::create_account(/* … */)?;
} else {
    // pre-funded: repurpose existing lamports
    system_program::allocate(/* … */)?;
    system_program::assign(/* … */)?;
}
```

To suppress:
```rust
// anchor-lint: suppress INV-LINT-CREATEACCT
system_program::create_account(/* … */)?;
```

---

### INV-LINT-SET-INNER — Field-by-field account population — prefer `set_inner` `[info]`

**Related invariants:** INV-LINT-PDA (PDA structs must store `version`/`bump`), INV-UPG-* (upgradability).

When fully populating an Anchor account (typically one just created by `init` / `init_if_needed`),
prefer [`Account::set_inner`] with a struct literal over assigning each field individually. A struct
literal must name **every** field, so when the account struct later grows a field, `set_inner`
becomes a **compile error** at every initializer — whereas field-by-field assignment silently leaves
the new field at its default, which is easy to miss in review and can be a security-relevant bug (an
unset `version`, `bump`, or a permission/limit field defaulting to zero).

The check flags a run of **≥3 consecutive field assignments to the same base** (`x.a = …; x.b = …;
x.c = …` or `ctx.accounts.x.a = …` …). The threshold targets full-struct population and ignores 1–2
field partial updates of an existing account (which are legitimate and where `set_inner` would wrongly
clobber the untouched fields).

```rust
// bad — a new field on SourceChainCcvConfig would be silently defaulted here
let cfg = &mut ctx.accounts.source_chain_ccv_config;
cfg.version = SOURCE_CHAIN_CCV_CONFIG_V;
cfg.bump = ctx.bumps.source_chain_ccv_config;
cfg.required_ccvs = params.required_ccvs.clone();
// ... (more fields)

// good — adding a field to the struct is a compile error until it's set here
ctx.accounts.source_chain_ccv_config.set_inner(SourceChainCcvConfig {
    version: SOURCE_CHAIN_CCV_CONFIG_V,
    bump: ctx.bumps.source_chain_ccv_config,
    required_ccvs: params.required_ccvs.clone(),
    // ... (compiler requires every field)
});
```

Suppress when field-by-field assignment is intentional — e.g. a partial update, or an initializer
that must run per-field logic (like calling a `BaseState::init` helper):

```rust
// anchor-lint: suppress INV-LINT-SET-INNER
config.version = CONFIG_V;
config.bump = ctx.bumps.config;
config.init(authority, router)?; // set_inner can't express the validation init() does
```
