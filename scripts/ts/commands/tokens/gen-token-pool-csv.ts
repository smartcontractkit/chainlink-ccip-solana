import { Command } from 'commander'

import setup from '../../config/index.ts'
import { decodeAccountFromData } from '../decode-pda.ts'
import { readStatePdasCsv, writeCsv } from '../../utils/csv.ts'
import { getMultipleAccountsBatched } from '../../utils/batched-rpc.ts'
import { labelProgram } from '../../utils/strings.ts'
import { pickPoolProgramKey, type PoolState } from './_shared.ts'

export default new Command()
  .description('Generate token-pools.csv from a state-pdas.csv (one State PDA per line)')
  .requiredOption('--input <path>', 'state-pdas.csv — single column with header `token_pool`')
  .requiredOption('--output <path>', 'output path for the generated token-pools.csv')
  .action(async function (this: Command) {
    const ctx = await setup(this)
    const { input, output } = this.opts<{ input: string; output: string }>()
    const connection = ctx.provider.connection

    const pdas = readStatePdasCsv(input)
    if (pdas.length === 0) {
      console.error(`No addresses found in ${input}`)
      process.exit(1)
    }

    console.info(`Fetching ${pdas.length} State account(s)...`)
    const infos = await getMultipleAccountsBatched(connection, pdas)

    const rows: { token_mint: string; token_pool_program: string; token_pool_pda: string }[] = []
    for (let i = 0; i < pdas.length; i++) {
      const info = infos[i]
      if (!info) {
        console.warn(`  ${pdas[i].toBase58()}: not found on-chain`)
        continue
      }
      const programId = info.owner
      const programKey = pickPoolProgramKey(ctx, programId)
      try {
        // Decode from the buffer we already fetched — no per-row RPC.
        const state = decodeAccountFromData<PoolState>(ctx, programKey, 'state', info.data)
        rows.push({
          token_mint: state.config.mint.toBase58(),
          token_pool_program: labelProgram(programId),
          token_pool_pda: pdas[i].toBase58(),
        })
      } catch (e: any) {
        console.warn(`  ${pdas[i].toBase58()}: decode failed (${e.message ?? e})`)
      }
    }

    rows.sort((a, b) =>
      a.token_pool_program === b.token_pool_program
        ? a.token_mint.localeCompare(b.token_mint)
        : a.token_pool_program.localeCompare(b.token_pool_program)
    )

    writeCsv(output, rows, { columns: ['token_mint', 'token_pool_program', 'token_pool_pda'] })
    console.info(`Wrote ${rows.length} row(s) to ${output}`)
  })
