# Message Lifecycle Invariants — Solana

> **Base invariants**: [MESSAGE_LIFECYCLE_INVARIANTS.md](https://github.com/smartcontractkit/chainlink-ccip/blob/main/chains/evm/contracts/invariants/MESSAGE_LIFECYCLE_INVARIANTS.md)
> All base invariants (INV-SEQ-1 through INV-SEQ-4, INV-ID-1/2, INV-SRC-1 through INV-SRC-6, INV-DST-1 through INV-DST-3, INV-EXEC-1 through INV-EXEC-9, INV-RECV-1/2, INV-TR-1 through INV-TR-4, INV-TO-1 through INV-TO-5, INV-NOEXEC-1/2) apply to Solana unless noted below.
> This file documents Solana-specific invariants, deviations, and how they are implemented.

---

## Solana-Specific Invariants

### INV-SOL-EXEC-1 — Buffered Execution

On Solana, large execution reports may be split into chunks across multiple transactions due to transaction size limits. The `ExecutionReportBuffer` tracks received chunks via a bitmap and chunk count. The execution report must not be processed until all chunks are received and validated.

The offramp uses `ExecutionReportBuffer` PDAs that accumulate chunks. A bitmap tracks which chunks have been received. Only when all expected chunks are present does the final execution proceed.

---

## Deviations from Base

### INV-TO-1 — The Source Classifies Token-Only Transfers More Conservatively Than the Destination

The OffRamp matches the base (`OffRamp._isTokenOnlyTransfer`): `ccip_common::messages::is_offramp_token_only_transfer` classifies a message as token-only when it carries no receiver, or when it has empty `data` and a zero `ccip_receive_gas_limit`. The receiver is neither consulted for CCVs nor CPI'd into in that case. `MessageV1::is_offramp_arbitrary_message()` is its negation and is the single gate used by `execute_v2`, by execute account resolution, and by the `get_ccvs_for_msg` view.

The router's source-side `is_pure_token_transfer` adds one condition the base does not have: on a lane with `token_receiver_allowed`, the `receiver` must also be all-zero. A send with a receiver, empty `data` and zero `gas_limit` therefore still carries the lane's default CCVs even though the destination would not require them. Over-provisioning the source list is safe — quorum only requires that the destination's required set be a subset of the supplied CCVs.

This is the condition that gates INV-TO-4 (default and receiver CCVs excluded for token-only transfers) in the router's `parse_extra_args_with_defaults`, which receives it as the `is_pure_token_transfer` argument.

### INV-NOEXEC-1 — No-Execution Address Width

The base uses `address(bytes20(0xeba517d2))` (20-byte EVM address). On Solana, the 32-byte representation of the `NO_EXECUTION_TAG` (`keccak256("NO_EXECUTION_TAG")[:4]`) is used.

---

## Implementation Notes

### Section 1 — Message Sequencing

> Naming: the counter these invariants call the "sequence number" is named `message_number` in code,
> following EVM 1.7 (`OnRamp.DestChainConfig.messageNumber`). The legacy 1.6 `RampMessageHeader` still
> exposes it as `sequence_number`, matching EVM 1.6. The `INV-SEQ-*` identifiers are shared with the
> cross-chain base spec and are unchanged.

**INV-SEQ-1**: The message number is stored in `DestChain.state.message_number`, a PDA seeded by the destination chain selector. Each destination lane has its own independent counter.

**INV-SEQ-2**: `message_number = message_number + 1` on every `ccip_send`. An overflow check (`ReachedMaxMessageNumber`) prevents wrapping past `u64::MAX`.

**INV-SEQ-4**: Lane reconfiguration via `UpdateDestChainSelectorConfig` modifies configuration fields but preserves the `DestChain.state` which contains the message number. A `restore_on_action` enum (None/Upgrade/Rollback) supports controlled restoration of that counter during version transitions.

### Section 2 — Message Identity

**INV-ID-2**: Message execution state is stored in a PDA seeded by `[MESSAGE_EXEC_STATE, message_id]`. Each unique message ID maps to exactly one execution state account.

### Section 3 — Source Side Flow

**INV-SRC-6**: The message number is incremented in the same transaction as the message emission. Since Solana transactions are atomic (all-or-nothing within a single transaction), the message number increment and event emission are committed together or not at all.

### Section 4 — Destination Side Flow

**INV-DST-3**: The offramp's execution flow enforces ordering through the instruction sequence. CCV programs are invoked via CPI for verification, then token pool CPI for release/mint, then receiver CPI for `ccipReceive`.

### Section 5 — Execution Semantics

**INV-EXEC-1**: The `execute` instruction on the offramp has no signer restrictions beyond the transaction fee payer. Any party can submit an execution transaction.

**INV-EXEC-4 through INV-EXEC-7**: The `MessageExecutionState` enum tracks four states: `Untouched` (0), `InProgress` (1), `Success` (2), and `Failure` (3). `InProgress` and `Failure` are not used in SVM but are defined for EVM compatibility. The execution state PDA (seeded by `[MESSAGE_EXEC_STATE, message_id]`) is created via `init_if_needed` on first execution. If the state is already `Success`, execution is skipped. `InProgress` is used for multi-transaction execution flows (see INV-SOL-EXEC-1).

### Section 6 — Receiver Callback

**INV-RECV-1**: The offramp invokes the receiver program via CPI. The receiver program validates the caller by checking the `allowed_offramp` PDA (seeded by `[ALLOWED_OFFRAMP, source_chain_selector, offramp_pubkey]`) which is maintained by the router.

**INV-SOL-RECV-2 — Receiver CPI target and accounts are bound to the signed message**: The program the offramp CPIs into (`ccip_receive` / `ccip_receive_v2`), and the receiver-specific accounts it forwards (identity, order, and per-account writability), MUST match the *signed* message. `message.receiver` and `message.dest_blob` are part of the `encoded_message` that the CCV quorum verifies, so they are authoritative; the account slice the offramp receives at execution time is executor-supplied (`remaining_accounts` sliced by the executor-supplied `AccountListLengths`) and cannot be trusted on its own.

**Relationship to CCIP 1.6 (deviation):** in 1.6 the offramp verified the merkle root *itself* and hashed the actual passed receiver + messaging account keys into the leaf (`recv_and_msg_account_keys` in `hash()`), so account identity and order were bound by the merkle proof, and writability came from the (also-hashed) `is_writable_bitmap` in `extra_args`. In v2 message verification moved into the committee-verifier CCV, which authenticates only `encoded_message` (and therefore `dest_blob`), **not** the live `remaining_accounts`. The identity/order binding that 1.6 got "for free" from its own hashing must therefore be re-established explicitly by the offramp at delivery time.

At delivery time the offramp enforces, in `execute::processor::call_receiver`:
- the CPI target program equals `message.receiver` (`InvalidReceiverAddress`);
- the number of receiver accounts equals `SVMDestBlob::from_bytes(message.dest_blob).accounts.len()` (`InvalidReceiverAccounts`);
- for each index `i`, the forwarded account's key equals `dest_blob.accounts[i]` (`InvalidReceiverAccounts`);
- **writability is taken from the authenticated `dest_blob.account_is_writable_bitmap`** when building each `AccountMeta`, so the receiver sees exactly the writability the sender signed. The per-account check only enforces *feasibility* — a bitmap-writable account must be passed writable at the tx level (`InvalidReceiverAccounts`) — mirroring 1.6's one-directional `InvalidWritabilityBitmap` guard (`!is_writable(bitmap, i) || acc.is_writable`). A bitmap-readonly account passed tx-writable is accepted and forced readonly for the receiver, since the same pubkey may need to be writable in another section of the same transaction.

The three fixed accounts the offramp itself prepends to the CPI — `external_execution_signer` (re-derived and checked separately, `InvalidExternalExecutionSigner`), `offramp`, and `allowed_offramp` — are not part of `dest_blob` and are excluded from this comparison. Account resolution (`resolve_accounts::second`) already derives the messaging segment from `dest_blob`, but resolution is an off-chain-driven helper; execution does not trust its output, so the binding is re-checked authoritatively at execution. Without this check a permissionless executor could redirect delivery to a different program (marking the message `Success` against a receiver the sender never chose) or hand the intended receiver substituted/reordered accounts or escalated writability. This is the delivery-CPI analog of INV-SOL-CPI-ACCOUNTS (which binds the CCV-query CPI target to `expected_receiver`).
