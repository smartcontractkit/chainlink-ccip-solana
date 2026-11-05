import { Command } from 'commander'
import { Connection, PublicKey } from '@solana/web3.js'
import type { ConfirmedSignatureInfo } from '@solana/web3.js'

import setup from '../../config/index.ts'
import { readCsvWithHeader, writeCsv } from '../../utils/csv.ts'
import { withRetry } from '../../utils/batched-rpc.ts'

/**
 * Find the earliest transaction signature for `mint` by paging backwards through
 * getSignaturesForAddress until the next page is empty.
 */
async function earliestSignatureTime(connection: Connection, mint: PublicKey): Promise<number | null> {
  let before: string | undefined = undefined
  let earliest: ConfirmedSignatureInfo | null = null
  // eslint-disable-next-line no-constant-condition
  while (true) {
    const page: ConfirmedSignatureInfo[] = await withRetry(() =>
      connection.getSignaturesForAddress(mint, { limit: 1000, before }, 'confirmed')
    )
    if (page.length === 0) break
    earliest = page[page.length - 1]
    if (page.length < 1000) break
    before = earliest!.signature
  }
  return earliest?.blockTime ?? null
}

function toIsoUtc(ts: number | null): string {
  if (!ts) return 'N/A'
  return new Date(ts * 1000).toISOString().replace('T', ' ').replace('.000Z', ' UTC')
}

export default new Command()
  .description('Enrich a CSV with each token mint’s creation date (in place).')
  .requiredOption('--csv <path>', 'CSV with a `token_mint` column (read + written back in place)')
  .action(async function (this: Command) {
    const ctx = await setup(this)
    const { csv } = this.opts<{ csv: string }>()
    const connection = ctx.provider.connection

    const rows = readCsvWithHeader(csv)
    if (rows.length === 0) {
      console.error('No rows found in CSV')
      process.exit(1)
    }
    if (!('token_mint' in rows[0])) {
      console.error('CSV is missing required column: token_mint')
      process.exit(1)
    }

    const columns = Object.keys(rows[0]).includes('mint_created_at')
      ? Object.keys(rows[0])
      : [...Object.keys(rows[0]), 'mint_created_at']

    for (const row of rows) {
      const mint = (row.token_mint ?? '').trim()
      if (!mint) continue
      console.info(`Looking up earliest tx for ${row.token_identifier ?? ''} ${mint}...`)
      try {
        const ts = await earliestSignatureTime(connection, new PublicKey(mint))
        row.mint_created_at = toIsoUtc(ts)
      } catch (e: any) {
        console.warn(`  failed: ${e.message ?? e}`)
        row.mint_created_at = 'ERROR'
      }
    }

    writeCsv(csv, rows, { columns })
    console.info(`Wrote ${rows.length} row(s) to ${csv}`)
  })
