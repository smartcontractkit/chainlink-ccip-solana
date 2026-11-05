---
name: solana-ccip-invariants
description: 'Solana CCIP contract invariants for code review, auditing, and compliance checks. Use when: reviewing Solana CCIP contract code, checking if code violates invariants, writing new contract logic, auditing security properties, understanding authorization rules, encoding formats, fee structures, token pool constraints, message lifecycle guarantees, lane configuration rules, CCV quorum requirements, finality guarantees, rate limiting, or Solana-specific lint rules enforced by anchor-lint.'
---

# Solana CCIP Invariants

On-demand reference for the Solana CCIP contract invariant specifications. Use this skill when reviewing, auditing, or writing Solana CCIP smart contract code to ensure compliance with documented invariants.

## When to Use

- Reviewing PRs that touch `contracts/programs/` code
- Checking if new contract logic violates any invariant
- Auditing access control, encoding, fees, token pools, or message flow
- Writing new contract instructions or modifying existing ones
- Understanding Solana-specific deviations from the base EVM invariants
- Verifying anchor-lint compliance

## Procedure

### Step 1: Load the Invariants Overview

Read the invariants README for context on the Solana runtime model, Anchor framework, and how these invariants relate to the base EVM set:

```
Read file: contracts/invariants/README.md
```

### Step 2: Load Topic-Specific Invariant Files

Based on the user's question, read the relevant invariant files:

**Authorization / access control:**
- `contracts/invariants/AUTHORIZATION_INVARIANTS.md` — signer uniqueness, signature domain separation

**Cross-chain verifier (CCV):**
- `contracts/invariants/CCV_INVARIANTS.md` — configuration, quorum, fees, address stability

**Encoding / wire format:**
- `contracts/invariants/ENCODING_INVARIANTS.md` — MessageV1/TokenTransferV1, ExtraArgs, numeric safety

**Fees:**
- `contracts/invariants/FEE_INVARIANTS.md` — fee structure, CCV/pool/executor/network fees, LINK discount, price requirements

**Finality:**
- `contracts/invariants/FINALITY_INVARIANTS.md` — Faster-Than-Finality encoding, opt-in, pool/executor/CCV finality

**Lane configuration:**
- `contracts/invariants/LANE_INVARIANTS.md` — lane config, pause/disable, routing, RMN curse, upgradability, replay protection

**Message lifecycle:**
- `contracts/invariants/MESSAGE_LIFECYCLE_INVARIANTS.md` — sequencing, identity, send/execute flow, receiver callback, token-only transfers

**Token pools:**
- `contracts/invariants/TOKEN_POOL_INVARIANTS.md` — pool types, access control, V1/V2, decimals, rate limiting, allow-list

**Static analysis / linting:**
- `contracts/invariants/SOLANA_LINT_INVARIANTS.md` — checks enforced by `anchor-lint` (Solana-only)

### Step 3: Cross-Reference Base EVM Invariants

The Solana invariants document **deviations and additions** relative to the base EVM invariants maintained in `chainlink-ccip`. If a question involves cross-chain consistency or an invariant not covered in the Solana files, check the base set:

- Base invariants repo: `github.com/smartcontractkit/chainlink-ccip/chains/evm/contracts/invariants/`
- The Solana README explains which EVM invariants are satisfied by the Solana runtime (e.g., reentrancy guards are unnecessary due to Solana's runtime model)

### Step 4: Apply to Code Review

When reviewing contract code against invariants:

1. Identify which invariant categories apply (authorization, encoding, fees, etc.)
2. Load the relevant invariant files
3. Check each invariant ID (e.g., INV-AUTH-1, INV-FEE-3) against the code
4. Flag any violations as FAIL with the invariant ID and explanation
5. Note any missing features — per the README, missing validations are a FAIL

### Step 5: Run anchor-lint

Use the helper script to run static analysis:

```bash
# Check all programs
.agents/skills/solana-ccip-invariants/scripts/run-anchor-lint.sh

# CI mode (exit 1 on any finding)
.agents/skills/solana-ccip-invariants/scripts/run-anchor-lint.sh --strict

# Check specific rule(s)
.agents/skills/solana-ccip-invariants/scripts/run-anchor-lint.sh --only INV-LINT-PDA

# List all checks
.agents/skills/solana-ccip-invariants/scripts/run-anchor-lint.sh --list-checks
```

Or run the linter directly:
```bash
python scripts/anchor-lint/src/anchor-lint.py --root contracts/programs/
```
