import { Command } from 'commander'
import { PublicKey } from '@solana/web3.js'

import setup from '../../config/index.ts'
import { extractProgramId, readCsvWithHeader, writeCsv } from '../../utils/csv.ts'
import { fetchMetadataMany } from '../../utils/metaplex.ts'
import { KNOWN_POOL_PROGRAMS } from '../../utils/strings.ts'

export default new Command()
  .description('List custom (non-burnmint / non-lockrelease) token pools from a token-pools CSV')
  .requiredOption('--csv <path>', 'token-pools.csv (token_mint, token_pool_program, token_pool_pda)')
  .option('--all', 'include known burnmint/lockrelease pools', false)
  .option('-o, --output <path>', 'write rows to this CSV instead of stdout')
  .action(async function (this: Command) {
    const ctx = await setup(this)
    const opts = this.opts<{ csv: string; all: boolean; output?: string }>()
    const connection = ctx.provider.connection

    const rows = readCsvWithHeader(opts.csv)
    const byProgram = new Map<string, string[]>() // programId -> [mints]
    for (const row of rows) {
      const programId = extractProgramId(row.token_pool_program)
      const mint = row.token_mint
      if (!programId || !mint) continue
      if (!opts.all && KNOWN_POOL_PROGRAMS[programId]) continue
      if (!byProgram.has(programId)) byProgram.set(programId, [])
      byProgram.get(programId)!.push(mint)
    }
    if (byProgram.size === 0) {
      console.info('No custom token pool programs found.')
      return
    }

    const allMints = Array.from(new Set(Array.from(byProgram.values()).flat()))
    console.error(`Programs: ${byProgram.size}`)
    console.error(`Mints:    ${allMints.length} unique`)
    console.error('Fetching token metadata...')
    const meta = await fetchMetadataMany(
      connection,
      allMints.map((m) => new PublicKey(m))
    )

    const outRows: { token_pool_program_id: string; token_identifier: string; token_mint: string }[] = []
    const sortedPids = Array.from(byProgram.keys()).sort(
      (a, b) => byProgram.get(b)!.length - byProgram.get(a)!.length || a.localeCompare(b)
    )
    for (const pid of sortedPids) {
      const mints = byProgram.get(pid)!.sort()
      for (const mint of mints) {
        outRows.push({
          token_pool_program_id: pid,
          token_identifier: meta.get(mint)?.symbol || 'N/A',
          token_mint: mint,
        })
      }
    }

    if (opts.output) {
      writeCsv(opts.output, outRows, {
        columns: ['token_pool_program_id', 'token_identifier', 'token_mint'],
      })
      console.error(`Wrote ${outRows.length} row(s) to ${opts.output}`)
    } else {
      // stdout: emit CSV directly so it can be piped
      const out = ['token_pool_program_id,token_identifier,token_mint']
      for (const r of outRows) out.push(`${r.token_pool_program_id},${r.token_identifier},${r.token_mint}`)
      process.stdout.write(out.join('\n') + '\n')
      console.error(`\nTotal: ${outRows.length} row(s)`)
    }
  })
