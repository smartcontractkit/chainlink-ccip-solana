# CCV (Cross-Chain Verifier) Invariants — Solana

> **Base invariants**: [CCV_INVARIANTS.md](https://github.com/smartcontractkit/chainlink-ccip/blob/main/chains/evm/contracts/invariants/CCV_INVARIANTS.md)
> All base invariants (INV-CFG-1 through INV-CFG-9, INV-SRC-1 through INV-SRC-22, INV-DST-1 through INV-DST-22, INV-ADDR-1 through INV-ADDR-3, INV-CC-1 through INV-CC-4) apply to Solana unless noted below.
> Base INV-RES-1 through INV-RES-4 (EVM resolver pattern) are not applicable — see deviations.
> This file documents deviations and how they are implemented.

---

## Deviations from Base

### Section 4 — CCV Address Stability: No Resolver Pattern

The EVM resolver pattern (base INV-RES-1 through INV-RES-4) does **not** apply to Solana. Solana programs are upgradeable by the upgrade authority while retaining the same program ID. This natively satisfies INV-ADDR-1 through INV-ADDR-3 without a resolver/proxy pattern. The program ID is the stable reference used by all parties.

---

## Implementation Notes

### Section 1 — Configuration

**INV-CFG-2**: `Pubkey::default()` (all-zero 32 bytes) is the zero-value sentinel and must be rejected in CCV configuration sets.

### Section 2.6 — CCV Fees

**INV-SRC-18 / INV-SRC-19**: CCV fee computation is performed via CPI to each CCV program's `get_fee` instruction. The committee-verifier stores per-remote-chain fee config in `RemoteChainAdminConfig` (containing `fee`, `gas_for_verification`, `payload_size_bytes`).

### Section 2.7 — CCV+Executor Hash

**INV-SRC-20 / INV-SRC-21**: The router computes this hash using 32-byte Pubkeys for CCV program IDs and the executor address. The CCV signer PDA is derived as `[CCV_SIGNER, ccv_pubkey]` in the router program.

### Section 3.8 — Verification

**INV-DST-21 / INV-DST-22**: CCV verification is performed via CPI to each CCV program. The offramp passes `ccv_and_executor_hash` and verifier results. Each CCV program (e.g., `committee-verifier`) validates the signatures and returns success or reverts the CPI call.

---

## Solana CPI Safety Invariants (Solana-specific)

These have no EVM analog — they address Solana's `get_return_data()` semantics and the OffRamp's
dynamic remaining-account layouts used to drive CCV/finality CPIs.

### INV-SOL-CPI-RETURN — CPI return data must be bound to the invoked program

Solana's `get_return_data()` returns the data set by whichever program **last** called
`set_return_data()` anywhere in the current CPI call tree — not necessarily the program we directly
invoked. A program that internally CPIs another program which sets return data would otherwise let
the caller silently read the wrong output (e.g. attacker-chosen required-CCV list ⇒
under-verification). Therefore, **every consumer of CPI return data must verify the returning
program equals the program it invoked** before deserializing.

For the receiver and token-pool **CCV** return paths this is centralized in
`ccip_common::ccvs::parse_verified_cpi_return`, which reads the return data, verifies the
returning program matches the expected one, and deserializes (`CommonCcipError::InvalidCpiReturnProgram`
on any verification failure). `get_pool_required_ccvs_cpi` and `get_receiver_ccvs_cpi` both go
through it. All other CPI-return consumers (send / release-or-mint / fee quoting /
account-derivation) go through the same shared verification via the `ccip_common::cpi` helpers,
which take caller-supplied error values so each program keeps its own error codes. Any new
CPI-return path MUST use one of these helpers rather than re-implementing the check inline.

### INV-SOL-CPI-ACCOUNTS — CPI account slices: validate the target, don't re-validate Anchor-checked accounts

CPI helpers receive accounts in one of two ways, with different validation obligations:

- **Anchor-validated context** (e.g. token-pool accounts derived from
  `OfframpTokenAccountsValidationContext` after `try_accounts`): the accounts are already
  PDA/owner/mint-bound by Anchor. Do **not** re-check those invariants manually — assume Anchor
  validated them. `get_pool_required_ccvs_cpi` relies on this: its `pool_program` is the canonical,
  mint-bound pool, so no additional target check is needed.
- **Dynamic remaining-account slice** (e.g. the OffRamp's `receiver_ccv` segment, sliced from
  `remaining_accounts` by `AccountListLengths`): Anchor cannot validate an executor-supplied slice.
  The helper MUST (a) bounds-check the slice length before indexing, and (b) bind the CPI **target
  program** to its authoritative source. `get_receiver_ccvs_cpi` takes `expected_receiver` (the
  message's receiver) and `require_keys_eq!`s `accounts[0]` against it
  (`CommonCcipError::InvalidCpiTargetProgram`); otherwise a malicious executor could point the CCV
  query at a program that under-reports the receiver's required CCVs. The config PDA is left to the
  receiver's own account context to validate (a wrong PDA fails inside the CPI), so it is not
  re-derived by the caller.
