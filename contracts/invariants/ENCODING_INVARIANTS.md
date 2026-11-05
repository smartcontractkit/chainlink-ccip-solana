# Encoding Invariants — Solana

> **Base invariants**: [ENCODING_INVARIANTS.md](https://github.com/smartcontractkit/chainlink-ccip/blob/main/chains/evm/contracts/invariants/ENCODING_INVARIANTS.md)
> All base invariants (INV-MSG-1 through INV-MSG-11, INV-ENC-1 through INV-ENC-9, INV-ENC-10 through INV-ENC-14, INV-POOL-ENC-1 through INV-POOL-ENC-4) apply to Solana unless noted below.
> This file documents deviations and how they are implemented.

---

## Deviations from Base

### INV-MSG-7 — Address Byte Length

On Solana (SVM), native addresses are 32 bytes (Pubkey). Source-side SVM addresses are 32 bytes. The base says "Source-side addresses must be padded for legacy reasons (e.g. 20-byte EVM addresses encoded as 32 bytes)" — on SVM, addresses are natively 32 bytes so no padding is needed.

### INV-ENC-14 — Byte-String Validation

The base requires "even length, valid hex characters, and consistent case normalization." On Solana, addresses are raw 32-byte Pubkeys — hex encoding and case normalization are not applicable. The invariant applies as: all byte-string inputs at system boundaries must be validated for well-formedness and malformed byte strings must be rejected.

---

## Implementation Notes

### Section 2.2 — SVM Executor Args

**INV-ENC-7**: The SVM executor args encode the additional accounts and their writability for the destination-side execution. `useATA` indicates whether the token receiver is an Associated Token Account. `accountIsWritableBitmap` is a 64-bit bitmap indicating which accounts are writable. Each account is a 32-byte Solana Pubkey.

### Section 3 — Numeric Range Safety

**INV-ENC-10**: SPL tokens use `u64` amounts (max ~18.4 × 10¹⁸), not `u256`. The implementation must validate that the decoded uint256 value fits in `u64` before processing. Values exceeding `u64::MAX` must be rejected. `messageNumber` (u64), `executionGasLimit` and `ccipReceiveGasLimit` (u32) are natively supported.

**INV-ENC-12**: The `ccip-common` crate uses `U256` (from a custom or imported type) for cross-chain amount handling and validates the downcast to `u64` before any SPL token operations.

### Section 5 — Token Pool Remote Configuration

**INV-POOL-ENC-1**: For Solana-originated messages, remote pool and token addresses are 32-byte Pubkeys.

**INV-POOL-ENC-4**: Remote chain configuration is stored in `RemoteConfig` PDAs per pool, containing `token_address` (a `RemoteAddress` of variable length) and `pool_addresses` (a `Vec` of allowed remote pool addresses). Source pool validation during `releaseOrMint` checks the incoming `sourcePoolAddress` against this configured set.
