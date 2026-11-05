import { Connection, PublicKey } from '@solana/web3.js'

export const METAPLEX_PROGRAM_ID = new PublicKey('metaqbxxUerdq28cj1RbAWkYQm3ybzjb6a8bt518x1s')

export type TokenMetadata = {
  name: string
  symbol: string
  uri: string
}

export function metadataPda(mint: PublicKey): PublicKey {
  return PublicKey.findProgramAddressSync(
    [Buffer.from('metadata'), METAPLEX_PROGRAM_ID.toBuffer(), mint.toBuffer()],
    METAPLEX_PROGRAM_ID
  )[0]
}

/** Read a length-prefixed Borsh string at `offset`. Returns the string and the next offset. */
function readBorshString(data: Buffer, offset: number): [string, number] {
  const length = data.readUInt32LE(offset)
  const start = offset + 4
  const end = start + length
  return [data.subarray(start, end).toString('utf8').replace(/\0/g, '').trim(), end]
}

/**
 * Fetch and decode the Metaplex Token Metadata account for a mint. Returns
 * `null` if the metadata PDA does not exist on-chain (common for raw SPL mints).
 *
 * Layout: key(1) | updateAuthority(32) | mint(32) | name | symbol | uri | ...
 */
export async function fetchMetadata(
  connection: Connection,
  mint: PublicKey
): Promise<TokenMetadata | null> {
  const acct = await connection.getAccountInfo(metadataPda(mint))
  if (!acct) return null
  const headerSize = 1 + 32 + 32 // key + updateAuthority + mint
  const data = acct.data.subarray(headerSize)
  try {
    const [name, off1] = readBorshString(data, 0)
    const [symbol, off2] = readBorshString(data, off1)
    const [uri] = readBorshString(data, off2)
    return { name, symbol, uri }
  } catch {
    return null
  }
}

/** Batched variant — derives PDAs for all mints, fetches in one RPC, returns map keyed by mint base58. */
export async function fetchMetadataMany(
  connection: Connection,
  mints: PublicKey[]
): Promise<Map<string, TokenMetadata | null>> {
  const pdas = mints.map(metadataPda)
  const accounts = await connection.getMultipleAccountsInfo(pdas)
  const out = new Map<string, TokenMetadata | null>()
  const headerSize = 1 + 32 + 32
  for (let i = 0; i < mints.length; i++) {
    const acct = accounts[i]
    if (!acct) {
      out.set(mints[i].toBase58(), null)
      continue
    }
    try {
      const data = acct.data.subarray(headerSize)
      const [name, off1] = readBorshString(data, 0)
      const [symbol, off2] = readBorshString(data, off1)
      const [uri] = readBorshString(data, off2)
      out.set(mints[i].toBase58(), { name, symbol, uri })
    } catch {
      out.set(mints[i].toBase58(), null)
    }
  }
  return out
}
