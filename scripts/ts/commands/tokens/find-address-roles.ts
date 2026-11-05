import { Command } from 'commander'
import { PublicKey } from '@solana/web3.js'

import setup from '../../config/index.ts'
import { decodeAccountFromData, derivePda, routerSeeds } from '../decode-pda.ts'
import { readPoolPdaCsv } from '../../utils/csv.ts'
import { getMultipleAccountsBatched } from '../../utils/batched-rpc.ts'
import { displayPubkey, labelProgram } from '../../utils/strings.ts'
import { pickPoolProgramKey, type PoolState, type TokenAdminRegistry } from './_shared.ts'

export default new Command()
  .description('Find all token pools where an address holds an admin/owner role')
  .requiredOption('--csv <path>', 'token-pools.csv (uses the token_pool_pda column)')
  .requiredOption('--address <addr>', 'address to search for across owner/admin fields')
  .action(async function (this: Command) {
    const ctx = await setup(this)
    const { csv, address } = this.opts<{ csv: string; address: string }>()
    const target = new PublicKey(address)
    const connection = ctx.provider.connection

    const pdas = readPoolPdaCsv(csv)
    if (pdas.length === 0) {
      console.info('No State PDA addresses found in CSV file.')
      return
    }
    console.info(`Searching for: ${target.toBase58()}`)
    console.info(`CSV:           ${csv} (${pdas.length} State PDA(s))`)
    console.info()
    console.info('Fetching State accounts...')
    const accountInfos = await getMultipleAccountsBatched(connection, pdas)

    type Row = { pda: PublicKey; programId: PublicKey; state: PoolState }
    const rows: Row[] = []
    // Decode the State buffers we already fetched — no per-row RPC.
    for (let i = 0; i < pdas.length; i++) {
      const info = accountInfos[i]
      if (!info) continue
      const programId = info.owner
      const programKey = pickPoolProgramKey(ctx, programId)
      try {
        const state = decodeAccountFromData<PoolState>(ctx, programKey, 'state', info.data)
        rows.push({ pda: pdas[i], programId, state })
      } catch {
        /* unexpected layout — skip silently like the Python script does */
      }
    }

    // Derive one TAR PDA per mint and batch-fetch — no program-wide scan.
    console.info('Fetching TokenAdminRegistry accounts...')
    const tarPdas = rows.map((r) => derivePda(ctx, 'router', routerSeeds.tokenAdminRegistry(r.state.config.mint)))
    const tarInfos = await getMultipleAccountsBatched(connection, tarPdas)
    const tarByMint = new Map<string, { publicKey: PublicKey; account: TokenAdminRegistry }>()
    for (let i = 0; i < rows.length; i++) {
      const info = tarInfos[i]
      if (!info) continue
      try {
        const account = decodeAccountFromData<TokenAdminRegistry>(
          ctx,
          'router',
          'tokenAdminRegistry',
          info.data
        )
        tarByMint.set(rows[i].state.config.mint.toBase58(), { publicKey: tarPdas[i], account })
      } catch {
        /* older layout — leave absent */
      }
    }

    let matched = 0
    for (const row of rows) {
      const tar = tarByMint.get(row.state.config.mint.toBase58())
      const roles: string[] = []
      if (row.state.config.owner.equals(target)) roles.push('State PDA owner')
      if (row.state.config.proposedOwner.equals(target)) roles.push('State PDA proposed owner')
      if (row.state.config.rateLimitAdmin.equals(target)) roles.push('State PDA rate limit admin')
      if (tar?.account.administrator.equals(target)) roles.push('TAR administrator')
      if (tar?.account.pendingAdministrator.equals(target)) roles.push('TAR pending admin')
      if (roles.length === 0) continue

      matched++
      console.info('='.repeat(80))
      console.info(`Token Pool Program ID:                ${labelProgram(row.programId)}`)
      console.info(`Token Mint:                           ${row.state.config.mint.toBase58()}`)
      console.info(`Token Pool State PDA:                 ${row.pda.toBase58()}`)
      console.info(`Matching roles:                       ${roles.join(', ')}`)
      console.info(`  State PDA owner:                    ${displayPubkey(row.state.config.owner)}`)
      console.info(`  State PDA proposed owner:           ${displayPubkey(row.state.config.proposedOwner)}`)
      console.info(`  State PDA rate limit admin:         ${displayPubkey(row.state.config.rateLimitAdmin)}`)
      if (tar) {
        console.info(`  TAR administrator:                  ${displayPubkey(tar.account.administrator)}`)
        console.info(`  TAR pending admin:                  ${displayPubkey(tar.account.pendingAdministrator)}`)
      } else {
        console.info(`  TAR:                                NOT FOUND`)
      }
    }
    console.info('='.repeat(80))
    console.info(`\nTotal: ${matched} match(es) out of ${pdas.length} State PDA(s)`)
  })
