import { Connection, PublicKey } from '@solana/web3.js'
import {
  TOKEN_PROGRAM_ID,
  TOKEN_2022_PROGRAM_ID,
  MULTISIG_SIZE,
  unpackMint,
  unpackMultisig,
  type Mint,
  type Multisig,
} from '@solana/spl-token'

export type TokenProgramLabel = 'SPL Token' | 'Token-2022' | 'unknown'

export type MintInfo = {
  programLabel: TokenProgramLabel
  /** Owner of the mint account — either TOKEN_PROGRAM_ID or TOKEN_2022_PROGRAM_ID for valid mints. */
  programId: PublicKey
  mint: Mint
  /** Set when `mint.mintAuthority` itself is an initialized SPL multisig. */
  multisig?: {
    address: PublicKey
    m: number
    n: number
    signers: PublicKey[]
  }
}

function labelFor(programId: PublicKey): TokenProgramLabel {
  if (programId.equals(TOKEN_PROGRAM_ID)) return 'SPL Token'
  if (programId.equals(TOKEN_2022_PROGRAM_ID)) return 'Token-2022'
  return 'unknown'
}

/**
 * Fetch a mint account, auto-detect SPL Token vs Token-2022 from its owner,
 * and (if the mint authority is itself a multisig) also fetch and decode the
 * multisig signers.
 */
export async function fetchMintInfo(connection: Connection, mint: PublicKey): Promise<MintInfo> {
  const acct = await connection.getAccountInfo(mint)
  if (!acct) throw new Error(`Mint account ${mint.toBase58()} not found on-chain`)
  const programLabel = labelFor(acct.owner)
  if (programLabel === 'unknown') {
    throw new Error(
      `Account ${mint.toBase58()} is not owned by SPL Token or Token-2022 (owner=${acct.owner.toBase58()})`
    )
  }
  const mintAccount = unpackMint(mint, acct, acct.owner)

  const info: MintInfo = { programLabel, programId: acct.owner, mint: mintAccount }

  if (mintAccount.mintAuthority) {
    const multisig = await tryFetchMultisig(connection, mintAccount.mintAuthority)
    if (multisig) info.multisig = multisig
  }

  return info
}

async function tryFetchMultisig(
  connection: Connection,
  address: PublicKey
): Promise<MintInfo['multisig'] | undefined> {
  const acct = await connection.getAccountInfo(address)
  if (!acct) return undefined
  if (!acct.owner.equals(TOKEN_PROGRAM_ID) && !acct.owner.equals(TOKEN_2022_PROGRAM_ID)) return undefined
  if (acct.data.length !== MULTISIG_SIZE) return undefined
  let parsed: Multisig
  try {
    parsed = unpackMultisig(address, acct, acct.owner)
  } catch {
    return undefined
  }
  if (!parsed.isInitialized) return undefined
  const signerKeys: PublicKey[] = [
    parsed.signer1,
    parsed.signer2,
    parsed.signer3,
    parsed.signer4,
    parsed.signer5,
    parsed.signer6,
    parsed.signer7,
    parsed.signer8,
    parsed.signer9,
    parsed.signer10,
    parsed.signer11,
  ]
  return {
    address,
    m: parsed.m,
    n: parsed.n,
    signers: signerKeys.slice(0, parsed.n),
  }
}
