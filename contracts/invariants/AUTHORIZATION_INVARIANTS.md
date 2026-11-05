# Authorization & Cryptographic Safety Invariants — Solana

> **Base invariants**: [AUTHORIZATION_INVARIANTS.md](https://github.com/smartcontractkit/chainlink-ccip/blob/main/chains/evm/contracts/invariants/AUTHORIZATION_INVARIANTS.md)
> All base invariants (INV-AUTH-1, INV-AUTH-2) apply to Solana without deviation. This file documents how they are implemented.

---

## Implementation Notes

### INV-AUTH-1 — Signer/Key Uniqueness

The `committee-verifier` program enforces this in two ways:
1. At **configuration time** (`set_signer_config`): The signer set is validated for duplicates and zero addresses before being stored.
2. At **verification time**: Recovered signer addresses from Secp256k1 signatures must be in **strictly ascending order**. This enforces uniqueness at the cryptographic level — the same key cannot produce two signatures that pass the ascending-order check. Binary search is used to locate each recovered address in the authorized signer set.

### INV-AUTH-2 — Signature Domain Separation

The `committee-verifier` computes the signed hash as `Keccak256(version_tag || ccv_address || message_id)`, where:
- `version_tag` is a 4-byte constant that provides protocol-level domain separation.
- `ccv_address` is the address of the verifier instance, binding the signature to a specific verifier and preventing cross-instance replay.
- `message_id` is the 32-byte Keccak256 hash of the entire encoded message, which includes source/destination chain selectors, the on-ramp address, and the message number — binding the signature to a specific message context.
- Secp256k1 signature recovery (with recovery ID always 0) produces an Ethereum-style address (last 20 bytes of the Keccak256 hash of the recovered public key), which is verified against the authorized signer set.
