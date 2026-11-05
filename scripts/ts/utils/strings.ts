import { PublicKey } from '@solana/web3.js'

export function capitalize(s: string): string {
  return s.charAt(0).toUpperCase() + s.slice(1)
}

/** Render a pubkey as base58, or "Nil" if it's the SystemProgram default (used as a sentinel by CCIP). */
export function displayPubkey(pk: PublicKey | string | null | undefined): string {
  if (!pk) return 'Nil'
  const s = typeof pk === 'string' ? pk : pk.toBase58()
  return s === PublicKey.default.toBase58() ? 'Nil' : s
}

/** Known token-pool program IDs, displayed alongside the address as `<addr> (label)` when matched. */
export const KNOWN_POOL_PROGRAMS: Record<string, string> = {
  '41FGToCmdaWa1dgZLKFAjvmx6e6AjVTX7SVRibvsMGVB': 'burnmint-token-pool',
  '8eqh8wppT9c5rw4ERqNCffvU6cNFJWff9WmkcYtmGiqC': 'lockrelease-token-pool',
  CCiTPESGEevd7TBU8EGBKrcxuRq7jx3YtW6tPidnscaZ: 'cctp-token-pool',
}

export function labelProgram(programId: PublicKey | string): string {
  const s = typeof programId === 'string' ? programId : programId.toBase58()
  const label = KNOWN_POOL_PROGRAMS[s]
  return label ? `${s} (${label})` : s
}
