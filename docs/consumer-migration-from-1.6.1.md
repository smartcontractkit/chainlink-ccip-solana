# Will your integration break? CCIP Solana 1.6.1 → current

- **Baseline:** release tag [`solana-contracts-anchor032-1.6.1`](https://github.com/smartcontractkit/chainlink-ccip-solana/releases/tag/solana-contracts-anchor032-1.6.1) (2026-03-06)
- **Target:** current `main` + in-flight branch work
- **Method:** mechanical diff of the committed Anchor IDLs at both points — instruction account
  lists and order, args, PDA field layouts, event sets and payloads, and error codes.

**`ccip-offramp` is out of scope, and none of its changes are breaking.** It ships as a
completely new contract at a new address rather than an in-place upgrade, so there is no existing
deployment for a consumer to be broken against. Its instructions, accounts, PDAs, events and error
codes all differ substantially from 1.6.1, but that is a new integration to build against, not a
migration. The same applies to the other new 2.0 programs (`committee-verifier`, `executor`,
`example-ccip-receiver-v2`). Test-only programs are excluded as well.

## Short answer

**Almost nothing breaks.** Sending and receiving CCIP messages is unchanged, and every 1.6
instruction keeps its exact 1.6 account list and arguments.

| Integration point | Status |
|---|---|
| `ccip_router::ccip_send` and all 26 surviving router instructions | **unchanged** |
| `ccip_receive` receiver callback (accounts) | **unchanged** |
| `Any2SVMMessage` / `SVMTokenAmount` payloads | **unchanged** |
| Every 1.6 admin/ops instruction in the token pools and `rmn-remote` | **unchanged** |
| Events emitted by 1.6 instructions (still in program logs) | **unchanged** |
| Error codes on the pools, `rmn-remote`, `fee-quoter`, `ccip-common` | **unchanged** |

Four things still require action, in descending order of likely impact:

1. **Reading `TokenAdminRegistry`** — layout changed.
2. **`rmn-remote::Config`** — layout changed.
3. **6 removed instructions.**
4. **`ccip-router::PoolEdited`** event no longer exists.

`cctp-token-pool` is fully identical to 1.6.1: instructions, accounts, PDAs, events and errors.

---

## 1. Reading `TokenAdminRegistry` — the main one

`ccip-router`'s `TokenAdminRegistry` PDA changed shape (the v1 → v3 migration):

| Added | Removed |
|---|---|
| `implements_account_resolution`, `bump`, `interface_version` | `supports_auto_derivation` |

Anyone deserializing this account — to discover a token's pool, its lookup table, or its
writable-index bitmap — must regenerate types. An old decoder returns garbage or fails outright.

> `bump` is populated only by the migration and registration paths. **Do not** assume a non-zero
> `bump` on a registry that predates the migration.

## 2. `rmn-remote::Config`

Gained `curser` and `bump`. Same class as above, but far fewer integrations read it.

## 3. Removed instructions

| Contract | Message | Note |
|---|---|---|
| `ccip-router` | `upgrade_token_admin_registry_from_v1` | migration helper; migration considered complete |
| `ccip-router` | `set_pool_supports_auto_derivation` | superseded by `interface_version` on the registry |
| `rmn-remote` | `migrate_config_v1_to_v2` | migration helper |
| `burnmint-token-pool` | `initialize_state_version` | migration helper |
| `lockrelease-token-pool` | `initialize_state_version` | migration helper |
| `lockrelease-token-pool` | `set_can_accept_liquidity` | no replacement |

All but the last are one-shot migration helpers. `set_can_accept_liquidity` is the only removal a
customer could plausibly still be calling.

## 4. `ccip-router::PoolEdited` no longer exists

Replaced by `PoolInterfaceImplementationUpdated`. Anything watching `PoolEdited` stops seeing it —
the event name and its discriminator both changed.

Also in `ccip-router`: error **7006** was renamed `ReachedMaxSequenceNumber` →
`ReachedMaxMessageNumber`. The code and its meaning are unchanged, so numeric matching keeps
working; only name-based references need regenerating.

---

## For observability: new 2.0 events use CPI delivery

Not a break — 1.6 events are untouched and still in the logs — but to see 2.0 activity you need a
second read path. These are emitted with `emit_cpi!`, which writes into a **self-CPI inner
instruction** rather than a `Program data:` log line:

| Contract | 2.0-only events, CPI-delivered |
|---|---|
| `burnmint-token-pool` | `LockedOrBurned`, `ReleasedOrMinted`, `ConfigV2Updated`, `OverrideChainConfigConfigured`, `OverrideChainConfigRemoved` |
| `lockrelease-token-pool` | same five |

`cctp-token-pool` and `rmn-remote` emit everything to logs, exactly as in 1.6.1.

To read a CPI event: take the transaction's inner instructions, keep those whose program ID is the
emitting program, strip the 8-byte Anchor CPI-event header, then Borsh-decode. In this repo:

```go
// program logs (all 1.6 events)
evt, err := common.ParseEventFromLogs(result.Meta.LogMessages, pkg.ParseEvent_X)

// CPI events (2.0-only events above)
tx, err := result.Transaction.GetTransaction()
evt, err := common.ExtractCPIEvent(tx, result.Meta, programID, pkg.ParseEvent_X)
```

`ExtractCPIEvent` needs transaction meta **with inner instructions**, so the RPC call must request
them.

## Note for anyone who read an earlier revision of this document

An earlier version listed 21 instructions gaining two accounts (`event_authority`, `program`) and
21 events leaving the program logs. **Those changes were rolled back** specifically to avoid
breaking existing integrations. The 1.6 instructions keep their original account lists and their
events keep going to the logs. Two related items were also restored:

- `base_token_pool` error codes 6018–6021, which had shifted by one when a new variant was
  inserted mid-enum. All 22 of the 1.6.1 codes now match exactly.
- `type_version` in `burnmint-token-pool` / `lockrelease-token-pool` no longer declares the
  `clock` account. Anchor ignores extra accounts, so a 1.6 client passing it still succeeds; this
  needs no client change.

## Checklist

1. **Update `TokenAdminRegistry` decoders** (§1) — the highest-impact item.
2. **Update `rmn-remote::Config` decoders** (§2).
3. **Replace the 6 removed instructions** (§3); decide what to do about
   `set_can_accept_liquidity`.
4. **Re-point anything watching `PoolEdited`** at `PoolInterfaceImplementationUpdated` (§4).
5. **Add CPI-event reading** if you need visibility into 2.0 pool activity.
