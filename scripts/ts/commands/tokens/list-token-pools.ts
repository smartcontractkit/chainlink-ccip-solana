import { Command } from 'commander'
import { PublicKey } from '@solana/web3.js'

import setup from '../../config/index.ts'
import { Program } from '@anchor-lang/core'
import { Program as LegacyProgram, AnchorProvider as LegacyProvider } from '@coral-xyz/anchor'
import type { Idl as LegacyIdl } from '@coral-xyz/anchor'

import { readProgramIdCsv } from '../../utils/csv.ts'
import { fetchMetadataMany } from '../../utils/metaplex.ts'
import { labelProgram } from '../../utils/strings.ts'
import burnmintIdl2_0 from '../../../../contracts/target/idl/burnmint_token_pool.json' with { type: 'json' }
import { IDL as burnmint16IdlRaw } from '../../idls/1_6/burnmint_token_pool.ts'
import { IDL as baseTokenPool16Idl } from '../../idls/1_6/base_token_pool.ts'

import type { PoolState } from './_shared.ts'

/**
 * For custom (third-party) pool programs, we need an Anchor Program instance bound
 * to that program ID — but using the base_token_pool layout for the State account.
 * Build it on the fly using the burnmint IDL (which matches base_token_pool shape).
 */
function ephemeralPoolProgram(ctx: Awaited<ReturnType<typeof setup>>, programId: PublicKey) {
  if (ctx.ccipVersion === '2.0') {
    const idl = JSON.parse(JSON.stringify(burnmintIdl2_0))
    idl.address = programId.toBase58()
    return new Program(idl as any, ctx.provider as any) as Program<any>
  }
  // 1.6: merge base_token_pool types into burnmint legacy IDL
  const baseTypes = (baseTokenPool16Idl as any).types ?? []
  const existing = new Set(((burnmint16IdlRaw as any).types ?? []).map((t: any) => t.name))
  const merged = {
    ...(burnmint16IdlRaw as any),
    types: [
      ...((burnmint16IdlRaw as any).types ?? []),
      ...baseTypes.filter((t: any) => !existing.has(t.name)),
    ],
  }
  return new LegacyProgram(merged as unknown as LegacyIdl, programId, ctx.provider as LegacyProvider)
}

export default new Command()
  .description('List all token pool State PDAs (with mint metadata) for the given pool programs')
  .requiredOption('--csv <path>', 'token-pools.csv (unique pool-program IDs are extracted)')
  .action(async function (this: Command) {
    const ctx = await setup(this)
    const { csv } = this.opts<{ csv: string }>()
    const connection = ctx.provider.connection

    const programIds = readProgramIdCsv(csv)
    if (programIds.length === 0) {
      console.error(`No program IDs found in ${csv}`)
      process.exit(1)
    }

    type Row = { programId: PublicKey; pda: PublicKey; mint: PublicKey }
    const all: Row[] = []
    for (const programId of programIds) {
      console.info(`Fetching State accounts for ${programId.toBase58()}...`)
      const program = ephemeralPoolProgram(ctx, programId)
      try {
        const items = await (program.account as any).state.all()
        for (const it of items) {
          all.push({ programId, pda: it.publicKey, mint: (it.account as PoolState).config.mint })
        }
      } catch (e: any) {
        console.warn(`  decode failed: ${e.message ?? e}`)
      }
    }

    if (all.length === 0) {
      console.info('No pools found.')
      return
    }

    const uniqueMints = Array.from(new Set(all.map((r) => r.mint.toBase58()))).map((s) => new PublicKey(s))
    console.info(`Fetching metadata for ${uniqueMints.length} unique mint(s)...`)
    const meta = await fetchMetadataMany(connection, uniqueMints)

    let lastProgram = ''
    for (const row of all.sort(
      (a, b) =>
        a.programId.toBase58().localeCompare(b.programId.toBase58()) ||
        a.mint.toBase58().localeCompare(b.mint.toBase58())
    )) {
      if (row.programId.toBase58() !== lastProgram) {
        console.info('='.repeat(90))
        console.info(`Pool program: ${labelProgram(row.programId)}`)
        console.info('-'.repeat(90))
        lastProgram = row.programId.toBase58()
      }
      const m = meta.get(row.mint.toBase58())
      const name = m?.name ?? 'N/A'
      const symbol = m?.symbol ?? 'N/A'
      console.info(`  ${row.pda.toBase58()}  mint=${row.mint.toBase58()}  ${symbol} (${name})`)
    }
    console.info('='.repeat(90))
    console.info(`Total: ${all.length} pool(s)`)
  })
