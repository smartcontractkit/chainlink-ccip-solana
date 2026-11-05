# Lane Configuration & Lifecycle Invariants — Solana

> **Base invariants**: [LANE_INVARIANTS.md](https://github.com/smartcontractkit/chainlink-ccip/blob/main/chains/evm/contracts/invariants/LANE_INVARIANTS.md)
> All base invariants apply to Solana unless noted below.
> This file documents deviations and how they are implemented.

---

## Deviations from Base

### Section 5 — Router Configuration

On Solana, the router is integrated into the `ccip-router` program (it acts as the OnRamp). Base INV-RTR-3 ("Setting the OnRamp to the zero-value address for a destination chain disables sending") does not apply — lane disabling is handled via the fee-quoter's `DisableDestChainConfigV2` instruction instead (see INV-PAUSE-1 implementation note).

### Section 8 — Reentrancy Protection

Base INV-REENT-1 and INV-REENT-2 do **not** apply to Solana. The Solana runtime prevents reentrancy by design: a program cannot be re-invoked while it is already on the call stack. See [README.md](README.md) for details.

### Section 10 — Access Control Summary

Solana-specific access control table (replaces base):

| Operation | Required Authority |
|-----------|--------------------|
| OnRamp lane configuration | Contract owner |
| OffRamp source chain configuration | Contract owner |
| Router offramp registration | Contract owner |
| Token pool remote chain configuration | Pool owner |
| Rate limit configuration | Pool owner or rate limit admin |
| Message sending (`ccipSend`) | Permissionless (allow-list if enabled) |
| Message execution (`execute`) | Permissionless |
| Curse / uncurse | RMN Remote owner |
| Ownership transfer | Current owner (propose) + new owner (accept) |

---

## Implementation Notes

### Section 1 — Lane Configuration (Source Side)

**INV-LCFG-1**: Per-lane configuration is stored in `DestChain` PDAs, seeded by `[DEST_CHAIN_STATE, chain_selector]` (seed constant `b"dest_chain_state"`). Configuration includes allowed senders, code version overrides, and message-number state.

> **TODO**: This PDA structure is likely to change when disambiguating the 1.6 lane from the 2.0 parallel lane. The seed scheme above reflects the current implementation and is not yet finalized as an invariant.

**INV-LCFG-5**: The default executor is a `Pubkey` in the lane configuration. `Pubkey::default()` (all zeros) is invalid and must be rejected at configuration time.

**INV-LCFG-7**: The lane-configuration instructions require the signer to match `config.owner` — `AddChainSelector` and `UpdateDestChainSelectorConfig` on the 1.6 path, `AddDestChainV2` and `UpdateDestChainSelectorConfigV2` on the 2.0 path. The two-step ownership transfer pattern ensures ownership cannot be accidentally transferred.

### Section 2 — Lane Configuration (Destination Side)

**INV-LCFG-8**: Source chain configuration is stored in per-source-chain PDAs on the offramp. These include the enabled flag and allowed OnRamp addresses.

### Section 3 — Lane Pause and Disable

**INV-PAUSE-1**: The fee-quoter has a `DisableDestChainConfigV2` instruction that sets `is_enabled = false` on the destination chain configuration. When disabled, fee quoting reverts, which prevents sending.

**INV-PAUSE-3**: The offramp checks the source chain's `is_enabled` flag before executing. If `false`, execution reverts with `UnsupportedSourceChainSelector`.

### Section 5 — Router Configuration

**INV-RTR-2**: The router maintains `allowed_offramp` PDAs seeded by `[ALLOWED_OFFRAMP, source_chain_selector, offramp_pubkey]`. Multiple offramp PDAs can coexist for the same source chain.

**INV-RTR-4**: Token pools validate the offramp caller by checking the existence of the `allowed_offramp` PDA. If the PDA does not exist, the CPI call fails at account validation.

### Section 7 — Risk Management (Curse Mechanism)

**INV-RMN-1**: The `rmn-remote` program maintains a `Curses` PDA containing a `Vec<CurseSubject>`. Curse subjects are 16-byte identifiers — either global (`0x01000000000000000000000000000001`) or chain-specific (derived from the chain selector).

**INV-RMN-4**: Router and Offramp invoke `rmn_remote::cpi::verify_not_cursed(CurseSubject::from_chain_selector(selector))` via CPI during both lock/burn and release/mint operations. The RMN program checks both the chain-specific subject and the global curse subject.

### Section 9 — Upgradability

**INV-UPG-1**: Solana programs are upgradeable by the upgrade authority. The program ID remains the same across upgrades, so all references to CCV addresses remain valid. No resolver/proxy pattern is needed.

**INV-UPG-2**: `RemoteConfig.pool_addresses` is a `Vec` that supports multiple remote pool addresses. Pool addresses can be appended or overwritten with reallocation.

### Section 11 — Multi-Step Transaction Safety

Solana's send and execute flows may span multiple transactions due to compute unit limits and the account resolution process.

**INV-ATOMIC-1**: The account resolution flow (`resolve_accounts_start`) gathers required accounts across multiple transactions before the final send instruction. Token lock/burn happens atomically within the final send transaction, ensuring no tokens are locked without message finalization.

**INV-ATOMIC-2**: Execution report buffers track in-progress multi-chunk executions. If configuration changes between chunks, the execution can be retried from scratch with a new buffer.

### Section 12 — Replay Protection

**INV-REPLAY-1**: Execution state is stored in PDAs that are independent of the program binary. Program upgrades do not affect existing PDA contents, so replay protection survives upgrades automatically. Message execution state PDAs (seeded by `[MESSAGE_EXEC_STATE, message_id]`) persist across program upgrades. A message that was executed as `Success` before an upgrade cannot be re-executed after the upgrade.
