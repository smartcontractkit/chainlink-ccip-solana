import { PublicKey } from '@solana/web3.js'
import { utils } from '@anchor-lang/core'
import { inflateSync } from 'zlib'
import { sha256 } from '@noble/hashes/sha2'

// Legacy Anchor IDL account: PDA = createWithSeed(base, "anchor:idl", programId),
// where base is the program-derived address with empty seeds.
export function legacyIdlAddress(programId: PublicKey): PublicKey {
  const [base] = PublicKey.findProgramAddressSync([], programId)
  return utils.publicKey.createWithSeedSync(base, 'anchor:idl', programId)
}

// Layout after the 8-byte Anchor discriminator:
//   authority (32) + data_len (u32 LE) + zlib(JSON)
export const LEGACY_IDL_HEADER_SIZE = 8 + 32 + 4

export function decodeLegacyIdlAccount(data: Buffer): any {
  const len = data.readUInt32LE(8 + 32)
  const compressed = data.subarray(LEGACY_IDL_HEADER_SIZE, LEGACY_IDL_HEADER_SIZE + len)
  return JSON.parse(inflateSync(compressed).toString('utf-8'))
}

export function readLegacyIdlAuthority(data: Buffer): PublicKey {
  return new PublicKey(data.subarray(8, 8 + 32))
}

// Anchor dispatches IDL self-management ixs by an 8-byte tag = sha256("anchor:idl")[..8],
// followed by the borsh-serialized IdlInstruction enum.
export const IDL_IX_TAG = Buffer.from(sha256('anchor:idl')).subarray(0, 8).toReversed() // 0x0a69e9a778bcf440 but little-endian (Anchor 0.29)

// Enum order in anchor-lang/src/idl.rs (anchor-lang 0.31+ / 0.32 / 1.x):
//   0 Create, 1 CreateBuffer, 2 Write, 3 SetBuffer, 4 SetAuthority, 5 Close, 6 Resize
export const RESIZE_VARIANT_INDEX = 6
