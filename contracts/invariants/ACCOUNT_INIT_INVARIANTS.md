# Account Initialization Invariants — Solana

> **Base invariants**: The chain-agnostic base set is maintained in
> [chainlink-ccip](https://github.com/smartcontractkit/chainlink-ccip/tree/main/chains/evm/contracts/invariants/).
> Account initialization is a Solana-specific concern: EVM has no account model, so
> these invariants have no direct base counterpart. They complement the base
> access-control and state-validation invariants.

---

## Purpose

Define when a program may safely treat a Solana account as an **initialized instance
of a known type**, and how to correctly recognise an **absent / uninitialized**
account. Getting this wrong leads to two symmetric failure modes:

1. **Trusting an account that was never initialized** — reading attacker-controlled or
   zeroed bytes as if they were validated program state (a soundness / privilege bug).
2. **Failing to recognise an account that is legitimately absent** — reverting on an
   input state that an unprivileged actor can create at will (a griefing / denial-of-service bug).

## Rationale

On Solana every account has three properties that matter for initialization:

| Property | Meaning |
|---|---|
| **owner** | The program that owns the account. A freshly allocated address is owned by the **System Program** (`system_program::ID`). A program-owned account is owned by that program's ID. |
| **lamports** | The account's SOL balance. Rent-exemption requires a minimum balance, but **lamports alone say nothing about initialization** — anyone can transfer lamports to any address. |
| **data / discriminator** | Anchor writes an 8-byte type discriminator at the start of every `#[account]`'s data. A correct discriminator is what distinguishes an initialized `T` from arbitrary bytes. |

An account is a valid, initialized instance of a program-owned type `T` **iff** it is
owned by the expected program **and** its discriminator matches `T`. Anchor's typed
account wrappers (`Account<'info, T>`, `InterfaceAccount<'info, T>`, `AccountLoader<'info, T>`)
verify **both** of these on successful deserialization. The untyped wrappers
(`UncheckedAccount`, `AccountInfo`) verify **neither**.

The critical subtlety is how "not initialized" is detected. Anchor's
`Account::try_from` returns `ErrorCode::AccountNotInitialized` **only** when the account
is owned by the System Program **and** has zero lamports. The moment the account holds
any lamports it falls through to the ownership check and returns
`AccountOwnedByWrongProgram` instead. Because a System-Program transfer requires only the
*sender* to sign — the recipient never signs — **anyone can pre-fund any address**,
including an as-yet-unallocated PDA whose seeds are public. This does not initialize the
account, but it flips which error a strict deserializer produces. Any code that keys its
"account is absent" decision on the specific `AccountNotInitialized` error variant is
therefore controllable by an unprivileged third party.

The lamport-agnostic, tamper-proof signal for "absent" is **ownership**: an account that
is still owned by the System Program has never been initialized by this program,
regardless of its lamport balance.

---

## Invariants

**INV-ACCT-INIT-1 — Initialization is proven by owner + discriminator.**
Code may treat an account as an initialized instance of a program-owned type `T` only
after both are established: (a) the account is owned by the expected program, and (b) its
type discriminator matches `T`. A successful deserialization through `Account<'info, T>`,
`InterfaceAccount<'info, T>`, or `AccountLoader<'info, T>` satisfies this. Data read from
an `UncheckedAccount`/`AccountInfo` does **not** satisfy this until the program performs
the equivalent checks itself.

**INV-ACCT-INIT-2 — Absence detection must be lamport-agnostic.**
When an instruction accepts an **optional** account and falls back to a default when it is
absent, "absent" must be determined from account **ownership** (owner is the System
Program), never from the lamport balance and never from a single deserialization-error
variant. An account that is System-owned is uninitialized irrespective of how many
lamports it holds.

**INV-ACCT-INIT-3 — `AccountNotInitialized` is not a sufficient absence signal.**
`ErrorCode::AccountNotInitialized` is returned by Anchor only for the
`System-owned AND zero-lamport` state. It must not be used as the *sole* discriminator for
"the optional account is absent", because a System-owned account with non-zero lamports —
a state any unprivileged actor can create — yields `AccountOwnedByWrongProgram` instead.
Absence handling must treat **every** System-owned account as absent (equivalently, handle
both error variants), so that the outcome cannot be changed by pre-funding.

**INV-ACCT-INIT-4 — Manual deserialization must validate owner, length, and identity.**
When a program reads an account passed through `remaining_accounts` or typed as
`UncheckedAccount`/`AccountInfo` and deserializes it manually (`try_from_slice`, zero-copy
`load`, byte slicing), it must validate: program ownership, the expected data length, and
— where the account's identity matters — the account key or PDA derivation, **before**
trusting the contents.

**INV-ACCT-INIT-5 — Initialization of permissionlessly-creatable accounts must be idempotent.**
When an instruction initializes an account whose address an unprivileged third party can
create — most notably an **Associated Token Account**, whose creation is permissionless for
any `(mint, authority)` pair — it must use an **idempotent** allocation (`init_if_needed`,
or an explicit "create-if-absent, otherwise adopt" flow) rather than a non-idempotent
`init`. A bare `init` performs a hard create that fails if the account already exists, so
anyone can pre-create the account and **permanently block** the instruction, including an
admin recovery path that depends on it. This does **not** apply to program-owned PDAs
created with `init`: only the program can allocate and assign them, so a third party cannot
pre-create them, and a mere lamport prefund does not block their `init` (see
INV-ACCT-INIT-3 for why prefunding still matters for *absence detection*).

**INV-ACCT-INIT-6 — Manual account creation must tolerate a pre-funded address.**
When a program creates an account by hand (a `system_program::create_account` CPI rather
than Anchor's `#[account(init)]`), it must handle the case where the target address already
holds lamports. The System Program's `create_account` **fails** if the destination is not a
zero-lamport System account, so — for a public-seed PDA, whose address anyone can pre-fund —
a bare `create_account` can be permanently griefed. The robust pattern branches on the
current balance and uses `allocate` + `assign` (transferring only the rent-exemption delta)
when the account is already funded. Anchor's `#[account(init)]` performs this internally;
hand-rolled creation must replicate it. This is the manual-creation analogue of
INV-ACCT-INIT-5.

---

## Unsafe Patterns

- Mapping only the `AccountNotInitialized` error to the "absent" branch and propagating
  every other error:
  ```rust
  match Account::<T>::try_from(account_info) {
      Ok(a)  => Some(a.into_inner()),
      Err(e) if e == ErrorCode::AccountNotInitialized.into() => None, // ⚠ narrow signal
      Err(e) => return Err(e),                                        // ⚠ reverts on prefund
  }
  ```
  An unprivileged pre-fund of the (public-seed) PDA turns the intended `None` path into a
  hard revert.

- Swallowing a typed-account deserialization error with `.ok()` / `unwrap_or(..)`:
  ```rust
  let cfg = Account::<T>::try_from(account_info).ok(); // ⚠ wrong-owner is silently "absent"
  ```
  This maps *every* failure — including `AccountOwnedByWrongProgram` for a substituted
  account — to `None`, hiding a misconfigured or attacker-supplied account.

- Reading fields out of an `UncheckedAccount`/`AccountInfo` (or `remaining_accounts`
  entry) without first checking `owner` and the discriminator/length, or (for a
  `remaining_accounts` entry) without pinning its key/PDA identity (INV-ACCT-INIT-4).

- Treating a non-zero lamport balance, or the mere existence of an address, as proof that
  an account is initialized.

- Creating an Associated Token Account with a non-idempotent `init` (INV-ACCT-INIT-5):
  ```rust
  #[account(init, payer = authority,
            associated_token::mint = mint, associated_token::authority = owner)]
  pub receiver: InterfaceAccount<'info, TokenAccount>, // ⚠ anyone can pre-create this ATA
  ```
  A third party pre-creates the ATA, and the `init` then fails on every call.

## Safe Patterns

- Decide absence from ownership, which cannot be perturbed by a lamport transfer:
  ```rust
  // The account is still System-owned ⇒ this program never initialized it ⇒ absent.
  if account_info.owner == &system_program::ID {
      return Ok(None);
  }
  let account = Account::<T>::try_from(account_info)?; // now owner+discriminator are enforced
  Ok(Some(account.into_inner()))
  ```

- Equivalently, accept both "uninitialized" error variants when relying on `try_from`:
  ```rust
  Err(e) if e == ErrorCode::AccountNotInitialized.into()
         || e == ErrorCode::AccountOwnedByWrongProgram.into() => None,
  ```
  (Ownership-based detection is preferred because it states the intent directly.)

- Always pair `remaining_accounts` / `UncheckedAccount` reads with explicit owner, length,
  and key/PDA validation before use.

- Initialize a permissionlessly-creatable account (e.g. an ATA) idempotently, so a
  pre-created account is adopted instead of causing a hard failure (INV-ACCT-INIT-5):
  ```rust
  #[account(init_if_needed, payer = authority,
            associated_token::mint = mint, associated_token::authority = owner)]
  pub receiver: InterfaceAccount<'info, TokenAccount>,
  ```
  (`init_if_needed` must still be paired with proper authorization on the instruction —
  see [INV-LINT-INIT](SOLANA_LINT_INVARIANTS.md#inv-lint-init).)

## Examples

**Optional per-token config, safe:**
```rust
pub fn optional_config<'info>(
    account: &'info AccountInfo<'info>,
    expected_key: Pubkey,
) -> Result<Option<Config>> {
    require_keys_eq!(account.key(), expected_key, MyError::InvalidConfigAccount);
    // Absent ⇔ System-owned, regardless of lamports (INV-ACCT-INIT-2/3).
    if account.owner == &system_program::ID {
        return Ok(None);
    }
    let cfg = Account::<Config>::try_from(account)?; // enforces owner + discriminator
    Ok(Some(cfg.into_inner()))
}
```

**Mandatory config (must exist), safe:**
```rust
// try_from propagates on any non-initialized state — correct, because absence is not tolerated here.
let cfg = Account::<Config>::try_from(account_info)?;
```

## Limitations

- The invariant governs *how* a program decides initialization/absence. Whether an
  optional account *should* exist for a given input is a protocol-design question outside
  its scope.
- Static analysis can flag the narrow-error-match anti-pattern (see
  [INV-LINT-ACCTINIT](SOLANA_LINT_INVARIANTS.md#inv-lint-acctinit)) and non-idempotent ATA
  initialization (see [INV-LINT-ATAINIT](SOLANA_LINT_INVARIANTS.md#inv-lint-atainit)), but
  it cannot in general prove that a manual deserialization path validates every required
  property; some cases require manual review.

## False-Positive Considerations

- A `try_from(...)?` that propagates all errors is **not** a violation of
  INV-ACCT-INIT-2/3: it does not tolerate absence, so there is no absence decision to get
  wrong. Only code that maps *some* deserialization outcome to a non-error "absent" result
  is in scope.
- Contexts where Anchor already guarantees the property — a typed `Account<'info, T>`
  field with `init`, `seeds`/`bump`, `owner =`, or `has_one` constraints — are not
  violations; the runtime enforces owner and discriminator before the handler runs.
