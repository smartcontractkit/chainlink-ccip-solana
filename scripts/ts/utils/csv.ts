import fs from 'node:fs'
import { parse } from 'csv-parse/sync'
import { stringify } from 'csv-stringify/sync'
import { PublicKey } from '@solana/web3.js'

// All readers in this module assume `token-pools.csv` — the one canonical input
// the CLI accepts. Header (required): token_mint,token_pool_program,token_pool_pda.
// `token_pool_program` may be either a bare base58 program ID or `<id> (label)`.

const B58 = /^[1-9A-HJ-NP-Za-km-z]{32,44}$/

function isBase58Address(s: string): boolean {
  return B58.test(s)
}

/** Strip a trailing ` (label)` suffix from a `token_pool_program` cell. */
export function extractProgramId(raw: string): string {
  return (raw ?? '').split(' ')[0].trim()
}

/**
 * Read a `state-pdas.csv` file. Header (required): `token_pool` — single column
 * of State PDA base58 addresses. This is the bootstrap input to
 * `gen-token-pool-csv`, which enriches it into `token-pools.csv`.
 */
export function readStatePdasCsv(path: string): PublicKey[] {
  const raw = fs.readFileSync(path, 'utf8')
  const firstLine = raw.split(/\r?\n/, 1)[0].trim()
  if (firstLine !== 'token_pool') {
    throw new Error(`${path} is not a state-pdas.csv: expected header line "token_pool", got "${firstLine}"`)
  }
  const rows = parse(raw, { columns: true, skip_empty_lines: true, trim: true }) as Record<string, string>[]
  return rows
    .map((r) => (r.token_pool || '').trim())
    .filter(isBase58Address)
    .map((s) => new PublicKey(s))
}

function readTokenPoolsCsv(path: string): Record<string, string>[] {
  const raw = fs.readFileSync(path, 'utf8')
  const firstLine = raw.split(/\r?\n/, 1)[0]
  if (
    !firstLine.includes('token_mint') ||
    !firstLine.includes('token_pool_program') ||
    !firstLine.includes('token_pool_pda')
  ) {
    throw new Error(
      `${path} is not a token-pools.csv: expected header to include ` +
        `token_mint, token_pool_program, token_pool_pda`
    )
  }
  return parse(raw, { columns: true, skip_empty_lines: true, trim: true }) as Record<string, string>[]
}

/** State PDAs from the `token_pool_pda` column. */
export function readPoolPdaCsv(path: string): PublicKey[] {
  return readTokenPoolsCsv(path)
    .map((r) => (r.token_pool_pda || '').trim())
    .filter(isBase58Address)
    .map((s) => new PublicKey(s))
}

/** Unique pool-program IDs from the `token_pool_program` column. */
export function readProgramIdCsv(path: string): PublicKey[] {
  const seen = new Set<string>()
  const out: PublicKey[] = []
  for (const r of readTokenPoolsCsv(path)) {
    const pid = extractProgramId(r.token_pool_program || '')
    if (!isBase58Address(pid) || seen.has(pid)) continue
    seen.add(pid)
    out.push(new PublicKey(pid))
  }
  return out
}

/** Mint → pool-program mappings from token-pools.csv (one entry per CSV row). */
export type PoolByMintRow = {
  programId: PublicKey
  /** token-pools.csv does not carry a human identifier; always `"N/A"`. */
  identifier: string
  mint: PublicKey
}

export function readPoolsByMintCsv(path: string): PoolByMintRow[] {
  const out: PoolByMintRow[] = []
  for (const r of readTokenPoolsCsv(path)) {
    const mintStr = (r.token_mint || '').trim()
    const progStr = extractProgramId(r.token_pool_program || '')
    if (!isBase58Address(mintStr) || !isBase58Address(progStr)) continue
    out.push({
      programId: new PublicKey(progStr),
      identifier: 'N/A',
      mint: new PublicKey(mintStr),
    })
  }
  return out
}

/**
 * Read a CSV with a header row, returning rows as objects keyed by header name.
 * Used by `get-mint-creation-dates` which enriches an existing CSV.
 */
export function readCsvWithHeader(path: string): Record<string, string>[] {
  const raw = fs.readFileSync(path, 'utf8')
  return parse(raw, { columns: true, skip_empty_lines: true, trim: true }) as Record<string, string>[]
}

export function writeCsv(
  path: string,
  rows: Record<string, string | number | undefined>[],
  options: { columns?: string[]; header?: boolean } = {}
): void {
  const out = stringify(rows, {
    header: options.header ?? true,
    columns: options.columns,
  })
  fs.writeFileSync(path, out)
}
