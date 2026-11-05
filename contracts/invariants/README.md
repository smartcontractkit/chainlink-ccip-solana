# Solana CCIP Invariants

> **Base invariants**: The chain-agnostic base set of invariants is maintained in
> [chainlink-ccip](https://github.com/smartcontractkit/chainlink-ccip/tree/main/chains/evm/contracts/invariants/).
> The files below contain only Solana-specific deviations, new invariants, and implementation notes.
> AI tools: fetch both the base files and these files for complete coverage.

## Invariant Files

| File | Description |
|------|-------------|
| [ACCOUNT_INIT_INVARIANTS.md](ACCOUNT_INIT_INVARIANTS.md) | When an account may be assumed initialized; lamport-agnostic absence detection |
| [AUTHORIZATION_INVARIANTS.md](AUTHORIZATION_INVARIANTS.md) | Signer uniqueness, signature domain separation |
| [CCV_INVARIANTS.md](CCV_INVARIANTS.md) | Cross-chain verifier configuration, quorum, fees, address stability |
| [ENCODING_INVARIANTS.md](ENCODING_INVARIANTS.md) | MessageV1/TokenTransferV1 wire format, ExtraArgs, numeric safety |
| [FEE_INVARIANTS.md](FEE_INVARIANTS.md) | Fee structure, CCV/pool/executor/network fees, LINK discount, price requirements |
| [FINALITY_INVARIANTS.md](FINALITY_INVARIANTS.md) | Faster-Than-Finality encoding, opt-in, pool/executor/CCV finality |
| [LANE_INVARIANTS.md](LANE_INVARIANTS.md) | Lane config, pause/disable, routing, RMN curse, upgradability, replay protection |
| [MESSAGE_LIFECYCLE_INVARIANTS.md](MESSAGE_LIFECYCLE_INVARIANTS.md) | Sequencing, identity, send/execute flow, receiver callback, token-only transfers |
| [TOKEN_POOL_INVARIANTS.md](TOKEN_POOL_INVARIANTS.md) | Pool types, access control, V1/V2, decimals, rate limiting, allow-list |
| [SOLANA_LINT_INVARIANTS.md](SOLANA_LINT_INVARIANTS.md) | Static analysis checks enforced by `anchor-lint` (Solana-only) |

## Context

## Solana (SVM) Chain Family

Solana is one of the five chain families that supported CCIP v1: EVM, Solana, Aptos, Sui, and TON. As a legacy v1 chain, Solana must support V1 pool interfaces alongside V2. When `IPoolV2` is mentioned, it means the CCIP v2 pool interface. V1 pool support must remain until all inflight v1 messages are fully drained.

## Zero-Address Sentinel

The zero address is used to signal "use the default CCV(s)." On Solana, `Pubkey::default()` (all zeros, 32 bytes) serves this role. Any logic that checks for the zero-value address must compare against `Pubkey::default()`.

## Solana Runtime Model

The following Solana-specific properties affect how invariants are implemented compared to EVM:

### No Reentrancy by Default

The Solana runtime prevents reentrancy: a program cannot be re-invoked while it is already on the call stack. This means explicit reentrancy guards (like EVM's `nonReentrant` modifier) are unnecessary. EVM invariants INV-REENT-1 and INV-REENT-2 are satisfied by the runtime itself.

### Signer-Based Access Control

Like EVM's `msg.sender` checks, Solana enforces access control by verifying that a transaction signer matches an expected address. Anchor's `#[account]` constraints automate this: `address =` checks the signer's public key against a known value, `owner =` verifies account ownership, and `seeds =` ensures a PDA was derived by the expected program. These constraints are validated at account deserialization time.

### Multi-Transaction Flows

Solana's compute unit limits and account size constraints mean some operations (account resolution, large execution reports) span multiple transactions. This makes atomicity invariants (INV-ATOMIC-1, INV-ATOMIC-2) critical: partially completed flows must be recoverable or cancellable.

### Account Model

All program state is stored in accounts (PDAs), not in contract storage slots. Configuration, execution state, and message numbers are stored in deterministic PDAs seeded by relevant identifiers (chain selectors, message IDs, etc.).

### Compute Units vs Gas

Solana uses compute units (CUs) rather than gas. Fee calculations reference destination-chain gas costs for cross-chain pricing, but local execution costs are bounded by CU limits.

### Token Amounts

SPL tokens use `u64` amounts (max ~18.4 × 10¹⁸), not `u256`. Cross-chain decimal conversion from larger-range source amounts requires overflow validation before downcasting to `u64`.

### Program Upgradability

Solana programs are upgradeable by the upgrade authority. Unlike EVM, there is no proxy/resolver pattern — the program address remains stable across upgrades. CCV addresses are program IDs that persist through upgrades, satisfying CCV address stability requirements natively.

## Anchor Framework

All Solana CCIP programs use the Anchor framework, which provides:
- Automatic account deserialization and constraint validation
- PDA derivation and verification via `seeds` and `bump` constraints
- CPI (Cross-Program Invocation) helpers for inter-program calls
- `init`, `init_if_needed`, and `realloc` for account lifecycle management

## Missing Features

Missing features should be flagged as FAIL unless context is provided on why they are not implemented. Partially implemented interfaces are a FAIL. A missing validation check is a FAIL. Further invariants relying on a missing feature are also a FAIL.
