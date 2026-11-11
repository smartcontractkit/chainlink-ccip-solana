# AGENTS.md

## Code Style
- No emojis in code or commit messages
- Make surgical changes — touch only what you must
- Avoid adding features "that might be useful later"

## Solana/Anchor Contracts

**Access Control:**
- Every `Signer` must have `address=`, `owner=`, or `seeds=` constraint (or `// PERMISSIONLESS:` comment)
- `init_if_needed` requires an authorized signer to prevent front-running

**PDA Structs:**
- All PDA account structs must have `version: u8` and `bump: u8` fields
- Suppress with `// anchor-lint: suppress INV-LINT-PDA` for legacy accounts

**Instruction Parameters:**
- `#[instruction]` params must match handler fn params in exact order
- Handlers with >1 param after `Context<T>` should use a params struct

**Events:**
- Use `emit_cpi!()` in programs with `#[event_cpi]`, use `emit!()` otherwise

**Serialization:**
- Use `borsh::to_vec(&val)?` instead of `.try_to_vec()`
- Use `.try_into().map_err(...)` instead of narrowing `as` casts
- Never use `.try_into().unwrap()` — propagate errors

**Hot Paths (ccip_send, get_fee, execute):**
- Avoid repeated `.clone()` on same variable
- Prefer stack arrays / `ArrayVec` over heap allocations
- Use `checked_*` arithmetic for financial calculations

## Testing
- Run `cargo build-sbf` in `contracts/` before pushing Rust changes
- Run `go test ./...` in `contracts/` before pushing Go binding changes
- Run anchor-lint: `make anchor-lint` (or `--strict` for CI)

## Generated Files
- Never manually edit `contracts/gobindings/v2/latest/` — regenerate with `make anchor-go-gen`
- Never manually edit `contracts/target/idl/*.json` — rebuild with `anchor build`
