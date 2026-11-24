---
name: changesets-guidelines
description: 'Rules for writing and reviewing CLDF changesets and sequences for Solana CCIP in this repo (deployment/v2_0_0). Use when: writing a new changeset or sequence, reviewing a changeset PR, migrating token pools or lanes from 1.6 to 2.0, resolving address refs (pool programs, pool instances, tokens), choosing between direct send and MCMS, checking idempotency, or debugging ref resolution ("expected to find exactly 1 ref ... found N").'
---

# Changeset guidelines

A standard for every changeset, taken from what went wrong in the 2.0 rollout, followed by the
Solana-specific rules that make it concrete in this repo.

## Programming rules

1. **Idempotent.** A second run with the same input changes nothing and sends no transaction.
2. **Authority from the environment.** Read the owner on chain (config owner or upgrade authority),
   send directly when that is the deployer, and build an MCMS batch when it is not.
   `solcommon.ConfirmOrBatch` already does this for most operations; the MCMS input only carries
   proposal parameters. A missing timelock ref is an error where MCMS is required.
3. **Simple inputs, validated first.** Check refs, chain overrides, address widths and required
   fields before sending anything, name the missing item in the error, and default optional fields,
   so an invalid input never costs a run.
4. **Narrow and resumable.** One purpose per changeset and one chain or token per input. Each step
   checks on-chain state, so a retry after a partial failure continues without repeating or breaking
   anything. Delete-and-recreate steps are atomic or resumable.
5. **Declared prerequisites and outputs.** State what must exist before the changeset runs and which
   refs it writes, so the runbook order can be checked.
6. **Compatible with the previous release.** A change to a shared account or format is checked
   against the previous version's reader, and both address widths are written.
7. **Verifiable.** Success is confirmed at finalized commitment, the report lists what changed, and
   a proposal contains only the expected operations.
8. **Pinned releases only.** A changeset on a real network takes a pinned, CI-published release and
   refuses a local build. The artifact hash is checked against the release provenance before
   anything is sent.
9. **Ships with tests and a rollback.** A run-twice test, a proposal-mode test, and a rollback or
   recovery path (a changeset or a documented input).

## Shape of a changeset in this repo

- `verify` runs every precondition, including on-chain reads, so a bad input fails before a
  proposal is built. `apply` calls one sequence and re-checks inside it (state can change between
  verify and apply).
- Put pure validation (input shape, datastore resolution) in exported functions that unit tests
  call directly; keep RPC reads in a separate check function.
- Each step reads its target account and skips when it is already in the requested state. An
  operation that has nothing to do returns no batch op; with no batch ops `OutputBuilder.Build`
  emits no proposal, which is what makes run 2 a no-op.
- A direct send that happens even when the rest is a proposal (permissionless account creation,
  such as `EnsurePoolTokenAccounts`) runs as a CLDF operation whose output lists what it created,
  so the report shows it.
- An authority error names the fix: when the owner is another timelock, name its MCMS qualifier;
  when no timelock is selected, say that `mcms.qualifier` may be missing. A named MCMS qualifier
  without a timelock ref is an error.
- Datastore output is additive: `ds.Addresses().Add` only for refs that are not there yet.
  Replacing a ref is an add plus `RemoteDelete` of the old key (see `UpgradeProgramChangeset`).
- Document the rollback in the changeset comment (an existing changeset and its input, or a
  `RemoteDelete` for refs).
- Reference implementations: `sequences/migrate_token_pool.go`,
  `sequences/migrate_cctp_token_pool.go`, `changesets/migrate_*_changeset.go`.

## Solana model to get right

**A token pool is (pool program, mint), not a program.** One pool program serves many mints. The
per-mint state is the pool config PDA `[ccip_tokenpool_config, mint]`, the per-lane
`ChainConfig` `[ccip_tokenpool_chainconfig, selector, mint]`, the optional `ChainConfigV2`
`[ccip_tokenpool_chainconfig_2_0, selector, mint]`, and the Token Admin Registry entry.

- Upgrading the program binary does not migrate any token: the 2.0 binary keeps the v1
  instructions. A token moves to 2.0 when **its own** TAR entry gets `interface_version = 2`.
  Migrations are per mint; never require upgrading or touching sibling tokens.
- The base `ChainConfig` is a required account of both `burn/mint` and `burn_v2/mint_v2`.
  `ChainConfigV2` only adds CCVs, allowed finality and fee. An absent `ChainConfigV2` behaves as
  WAIT_FOR_FINALITY, no pool CCVs, fee disabled; `init_chain_config_v2` is payer = pool owner
  (the timelock signer must hold SOL) and creates the own-mint fee vault with `init_if_needed`.
- Fee vaults: `[fee_vault, mint]` under the pool program, one ATA per fee token (wSOL, LINK) plus
  the pool's own mint (BPS fee). `ccip_send` pays a v2 pool's receipt into them only when its fee is
  non-zero, so a missing vault breaks sends the day a fee is enabled (`MissingAtaForReceiptIssuer`).
  Every flow that leaves a pool on 2.0 or enables its fee calls `ensurePoolTokenAccounts`. ATA creation
  is permissionless and idempotent here, so the deployer sends it outside the proposal.
- The TAR of a v2 pool is always interface 2 + `WithoutExtendedLookupTable` (the only pair the
  Router operation accepts); flows normalize a legacy v2 + `Standard` registry to it.
- A 2.0 lane is a `DestChainCcipV2` account on the Router (`routerops.LaneExists`) and a source
  chain account on the **2.0 OffRamp**, which is a separate program from the 1.6 one. Router,
  FeeQuoter and RMNRemote are upgraded in place.
- The TAR lookup table holds the pool program at index 2 (`routerops.SetPool`); use it to check
  which program a mint actually routes to. 1.6 and 2.0 share the table layout.

## Address refs

- **Never resolve by address alone.** A shared pool program carries many refs (one per
  qualifier and version). Address-only lookups (`FindAndFormatRef` on an address) fail with
  `expected to find exactly 1 ref ... found N`. Resolve by type + qualifier (+ version).
- Pool **program** qualifiers (1.6 pool metadata): empty or `CLLCCIP` is the shared program; any
  other qualifier is a separate pool program registered under it. `TokenPoolProgram` is
  the 2.0 alias of the first upgraded legacy program, so do not use it to pick a specific program.
- Pool **instance** refs use the token's qualifier (`solcommon.PoolRefByQualifier`). The instance
  ref's version is what 2.0 tooling uses to choose the adapter, so a migrated instance gets a 2.x ref
  at the upgraded program version.
- Adapter selection follows the resolved ref's version (`GetTokenAdapter(family, version)`, same-major
  fallback). For a shared program, "highest version at the address" is wrong: decide per token from
  the instance ref or the TAR `interface_version`.
- Token refs exist as `SPLTokens` **and** `SPL2022Tokens`; look up both.
- Keep cross-major refs (1.6) while that token's 1.6 lanes are alive: the 1.6 adapter still resolves
  the pool by them. Delete them in a separate cleanup with `RemoteDelete` once those lanes are gone.
- `TokenRefResolver` registration is first-wins per family: when both the 1.6 (chainlink-ccip) and
  2.0 (this repo) adapters are linked, the 1.6 resolver handles address-only refs. Passing a pool
  config PDA goes through it.

## Authority checks

A changeset acts only on pools whose program upgrade authority, pool owner and TAR administrator
are the deployer or the timelock signer (`solcommon.FindTimelockSignerPDA` with the MCMS
qualifier). Otherwise fail in verify, naming the controlling key, instead of building a proposal
nobody can execute.

## Checking idempotency against a real environment

Before claiming a changeset is safe on an environment, run its real resolution and check code
read-only against that environment:

1. Load the environment's address refs into a memory datastore (use `Upsert`: the file can hold
   duplicate keys).
2. Call the exported resolution and check functions with an RPC-only `cldf_solana.Chain` (no
   deployer key), and read each account a step would write.
3. Write down, per step, the current state, what run 1 does (direct send, batch op or nothing) and
   what run 2 does after the proposal executes. Run 2 must produce no batch ops and no refs.

## Tests

- Unit: input validation, datastore resolution and authority decisions, without RPC.
- Run-twice: a devenv test that applies the changeset, checks the on-chain result, applies it again
  on the merged datastore, and asserts no proposal, no refs and an unchanged deployer balance; see
  `devenv/tests/smoke/migrate_token_pool_test.go`. A test that changes shared devenv state is opt-in
  and skips attached environments (`SOLANA_PRIVATE_KEY` set).
- Proposal mode needs a timelock-owned pool; the Solana devenv has no MCMS yet.

## Review checklist

- [ ] verify fails on every missing prerequisite, naming it, before any send
- [ ] refs resolved by type + qualifier, never by address alone; both token types handled
- [ ] signer read on chain; direct vs MCMS decided by `ConfirmOrBatch`
- [ ] each step skips when already applied; run-twice test exists
- [ ] proposal-mode test exists
- [ ] refs written are declared; cross-major refs kept; same-major superseded via `RemoteDelete`
- [ ] pools not controlled by the deployer or the timelock rejected in verify
- [ ] rollback or recovery path documented
