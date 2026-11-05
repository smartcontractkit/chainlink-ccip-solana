import { Command } from 'commander'
import chalk from 'chalk'
import Table from 'cli-table3'

import { Connection, PublicKey } from '@solana/web3.js'

import setup, { type SetupResult } from '../../config/index.ts'
import {
  decodeAccount,
  decodeAccountFromData,
  derivePda,
  routerSeeds,
  tokenPoolSeeds,
} from '../decode-pda.ts'
import { readPoolPdaCsv, readPoolsByMintCsv } from '../../utils/csv.ts'
import { fetchMetadata } from '../../utils/metaplex.ts'
import { fetchMintInfo } from '../../utils/mint.ts'
import { getMultipleAccountsBatched } from '../../utils/batched-rpc.ts'
import { displayPubkey, labelProgram } from '../../utils/strings.ts'
import { CCIP_CHAIN_SELECTORS_MAINNET } from '../../utils/chain-selectors.ts'
import {
  fetchChainConfigs,
  fmtTimestamp,
  isWritableEntry,
  pickPoolProgramKey,
  type PoolProgramKey,
  type PoolState,
  type TokenAdminRegistry,
} from './_shared.ts'

/**
 * Fixed semantic roles for the first 10 entries of every CCIP token-pool ALT.
 * Source: chainlink-ccip/chains/solana/contracts/programs/ccip-common/src/v1.rs L174-184.
 * Entries past index 9 are static extras (CCTP, etc.) or auto-derived per-message accounts.
 */
const LOOKUP_TABLE_LABELS: readonly string[] = [
  'lookup_table (self)',
  'token_admin_registry',
  'pool_program',
  'pool_config (state PDA)',
  'pool_token_account',
  'pool_signer',
  'token_program',
  'mint',
  'fee_token_config',
  'ccip_router_pool_signer',
]

/**
 * Fetch the TokenAdminRegistry PDA for a mint, or return null when the PDA
 * doesn't exist on-chain. Derived from `(token_admin_registry, mint)` against
 * the router program — same seed across CCIP 1.6 and 2.0.
 */
async function fetchTarForMint(
  ctx: SetupResult,
  mint: PublicKey
): Promise<{ publicKey: PublicKey; account: TokenAdminRegistry } | null> {
  const pda = derivePda(ctx, 'router', routerSeeds.tokenAdminRegistry(mint))
  try {
    const account = await decodeAccount<TokenAdminRegistry>(ctx, 'router', 'tokenAdminRegistry', pda)
    return { publicKey: pda, account }
  } catch {
    return null
  }
}

export default new Command()
  .description(
    'Inspect token pool(s). Use --state for a single PDA, --mint + --csv to look up pool(s) for a mint, ' +
      'or --csv alone for batch inspection of every pool in the CSV.'
  )
  .option('--state <pda>', 'single State PDA to inspect in detail')
  .option('--mint <addr>', 'mint address (requires --csv); inspects every pool listed for that mint')
  .option('--csv <path>', 'token-pools.csv — required for --mint mode, also drives batch mode when alone')
  .action(async function (this: Command) {
    const ctx = await setup(this)
    const { state, mint, csv } = this.opts<{ state?: string; mint?: string; csv?: string }>()

    if (!state && !mint && !csv) {
      throw new Error('Provide one of: --state <pda>, --mint <addr> --csv <path>, or --csv <path>')
    }
    if (state && mint) {
      throw new Error('--state and --mint are mutually exclusive')
    }
    if (mint && !csv) {
      throw new Error('--mint requires --csv (to look up the pool program(s))')
    }

    if (state) {
      await inspectSingle(ctx, new PublicKey(state))
    } else if (mint && csv) {
      const target = new PublicKey(mint)
      const matches = readPoolsByMintCsv(csv).filter((r) => r.mint.equals(target))
      if (matches.length === 0) {
        console.error(`Mint ${target.toBase58()} not found in ${csv}`)
        process.exit(1)
      }
      // Derive State PDA for each (program, mint) pair from the CSV — canonical
      // token-pool seeds, shared across 1.6 and 2.0 contracts.
      const seeds = tokenPoolSeeds.state(target)
      for (const m of matches) {
        const pda = PublicKey.findProgramAddressSync(seeds, m.programId)[0]
        await inspectSingle(ctx, pda)
      }
    } else if (csv) {
      await inspectBatch(ctx, readPoolPdaCsv(csv))
    }
  })

async function inspectSingle(ctx: SetupResult, pda: PublicKey) {
  const connection = ctx.provider.connection

  console.info(chalk.gray(`Fetching State account ${pda.toBase58()}...`))
  // The State PDA's on-chain owner tells us which token-pool program decodes this account.
  const acct = await connection.getAccountInfo(pda)
  if (!acct) throw new Error(`State account ${pda.toBase58()} not found on-chain`)
  const programId = acct.owner
  const programKey = pickPoolProgramKey(ctx, programId)
  const stateAccount = await decodeAccount<PoolState>(ctx, programKey, 'state', pda)

  const { mint } = stateAccount.config
  const [metadata, mintInfo, tar] = await Promise.all([
    fetchMetadata(connection, mint),
    fetchMintInfo(connection, mint),
    fetchTarForMint(ctx, mint),
  ])

  const sym = metadata?.symbol ?? 'unknown'
  const name = metadata?.name ?? 'unnamed token'
  printHeader(`Token Pool — ${sym}  (${name})`)

  printSection('Token')
  printKV('Mint', mint.toBase58())
  printKV('Token Program', mintInfo.programLabel)
  printKV('Name', metadata?.name ?? '—')
  printKV('Symbol', metadata?.symbol ?? '—')
  printKV('Metadata URI', metadata?.uri ?? '—')

  printSection('Mint Authority')
  if (!mintInfo.mint.mintAuthority) {
    printKV('Authority', chalk.gray('— (fixed supply, no mint authority)'))
  } else if (mintInfo.multisig) {
    const { m, n, signers } = mintInfo.multisig
    printKV('Authority', `${mintInfo.mint.mintAuthority.toBase58()}  ${chalk.cyan(`(multisig ${m}/${n})`)}`)
    const poolSigner = stateAccount.config.poolSigner.toBase58()
    const present = new Set(signers.map((s) => s.toBase58()))
    signers.forEach((s, i) => {
      const tag =
        s.toBase58() === poolSigner ? chalk.green('  ← pool signer PDA ✓') : ''
      printKV(`  Signer ${i + 1}`, `${s.toBase58()}${tag}`)
    })
    if (!present.has(poolSigner)) {
      console.info(
        chalk.yellow(
          `  ⚠ WARNING: pool signer PDA ${poolSigner} is NOT a signer of this multisig!`
        )
      )
    }
  } else {
    printKV('Authority', `${mintInfo.mint.mintAuthority.toBase58()}  ${chalk.gray('(single signer)')}`)
  }

  printSection('Pool')
  printKV('Program', labelProgram(programId))
  const versionStatus =
    stateAccount.version === 0 ? chalk.red('NOT INITIALIZED') : chalk.green('initialized')
  printKV('State PDA', `${pda.toBase58()}  ${chalk.gray(`(v${stateAccount.version}, ${versionStatus})`)}`)
  printKV('Signer PDA', stateAccount.config.poolSigner.toBase58())
  printKV('Owner', formatPubkey(stateAccount.config.owner))
  printKV('Proposed Owner', formatPubkey(stateAccount.config.proposedOwner))
  printKV('Rate Limit Admin', formatPubkey(stateAccount.config.rateLimitAdmin))

  printSection('Token Admin Registry')
  if (!tar) {
    console.info(`  ${chalk.yellow('— TAR PDA not found on-chain for this mint')}`)
  } else {
    printKV('PDA', tar.publicKey.toBase58())
    printKV('Administrator', formatPubkey(tar.account.administrator))
    printKV('Pending Admin', formatPubkey(tar.account.pendingAdministrator))
    await printLookupTable(connection, tar.account)
  }

  await printRemoteChains(ctx, programKey, mint)

  printFooter()
}

// ---------------------------------------------------------------------------
// Rendering helpers
// ---------------------------------------------------------------------------

const WIDTH = 100

function printHeader(title: string) {
  console.info()
  console.info(chalk.bold.cyan('┌' + '─'.repeat(WIDTH - 2) + '┐'))
  console.info(
    chalk.bold.cyan('│') +
      ' ' +
      chalk.bold.white(title.padEnd(WIDTH - 4)) +
      ' ' +
      chalk.bold.cyan('│')
  )
  console.info(chalk.bold.cyan('└' + '─'.repeat(WIDTH - 2) + '┘'))
}

function printFooter() {
  console.info(chalk.gray('─'.repeat(WIDTH)))
  console.info()
}

function printSection(label: string) {
  console.info()
  console.info(
    chalk.bold.magenta('▎ ') +
      chalk.bold(label) +
      ' ' +
      chalk.gray('─'.repeat(Math.max(0, WIDTH - 3 - label.length)))
  )
}

/** Two-column key/value with consistent left padding. */
function printKV(key: string, value: string) {
  console.info(`  ${chalk.cyan(key.padEnd(22))}  ${value}`)
}

/** Render a pubkey, replacing the default SystemProgram sentinel with `—`. */
function formatPubkey(pk: PublicKey | string | null | undefined): string {
  const s = displayPubkey(pk)
  return s === 'Nil' ? chalk.gray('—') : s
}

/** Render the Token Admin Registry's ALT contents with per-entry semantic labels. */
async function printLookupTable(connection: Connection, tar: TokenAdminRegistry) {
  const lutAddr = tar.lookupTable
  if (lutAddr.equals(PublicKey.default)) {
    printKV('Lookup Table', chalk.gray('— (unset)'))
    return
  }
  const res = await connection.getAddressLookupTable(lutAddr)
  const alt = res.value
  if (!alt) {
    printKV('Lookup Table', `${lutAddr.toBase58()}  ${chalk.yellow('(account not found on-chain)')}`)
    return
  }
  const addresses = alt.state.addresses
  printKV('Lookup Table', `${lutAddr.toBase58()}  ${chalk.gray(`(${addresses.length} entries)`)}`)

  const t = new Table({
    head: [
      chalk.bold('#'),
      chalk.bold('Address'),
      chalk.bold('Access'),
      chalk.bold('Role'),
    ],
    style: { 'padding-left': 1, 'padding-right': 1, head: [], border: ['gray'] },
    chars: {
      top: '─', 'top-mid': '┬', 'top-left': '┌', 'top-right': '┐',
      bottom: '─', 'bottom-mid': '┴', 'bottom-left': '└', 'bottom-right': '┘',
      left: '│', 'left-mid': '├', mid: '─', 'mid-mid': '┼',
      right: '│', 'right-mid': '┤', middle: '│',
    },
  })
  for (let i = 0; i < addresses.length; i++) {
    const writable = isWritableEntry(tar.writableIndexes, i)
    const access = writable ? chalk.yellow('writable') : chalk.gray('readonly')
    const role =
      i < LOOKUP_TABLE_LABELS.length ? LOOKUP_TABLE_LABELS[i] : chalk.gray('extra')
    t.push([String(i), addresses[i].toBase58(), access, role])
  }
  // indent the table to match the KV column
  for (const line of t.toString().split('\n')) console.info(`  ${line}`)
}

/**
 * Render every remote chain configured for this pool: chain selector + name,
 * the remote-side token address / decimals / pool addresses, and the inbound/
 * outbound rate-limit buckets. Derives ChainConfig PDAs from
 * (ccip_tokenpool_chainconfig, selector_u64le, mint) — no program-wide scan.
 */
async function printRemoteChains(ctx: SetupResult, programKey: PoolProgramKey, mint: PublicKey) {
  const selectors = Object.keys(CCIP_CHAIN_SELECTORS_MAINNET).map((s) => BigInt(s))
  const found = await fetchChainConfigs(ctx, programKey, mint, selectors)

  printSection(
    `Remote Chains  ${chalk.gray(`(${found.length}/${selectors.length} known mainnet selectors configured)`)}`
  )
  if (found.length === 0) {
    console.info(chalk.gray('  — no configured remote chains'))
    return
  }

  const hex = (b: { address: Buffer | number[] }) =>
    '0x' + Buffer.from(b.address as any).toString('hex')

  for (const { selector, config } of found) {
    const r = config.base.remote
    const ib = config.base.inboundRateLimit
    const ob = config.base.outboundRateLimit
    const name = CCIP_CHAIN_SELECTORS_MAINNET[selector.toString()] ?? 'unknown'
    console.info()
    console.info(`  ${chalk.bold.green('●')} ${chalk.bold(name)}  ${chalk.gray(`(selector ${selector})`)}`)
    printRemoteKV('Remote token', `${hex(r.tokenAddress)}  ${chalk.gray(`(decimals ${r.decimals})`)}`)
    if (r.poolAddresses.length === 0) {
      printRemoteKV('Remote pools', chalk.gray('— (none)'))
    } else {
      r.poolAddresses.forEach((p, i) => {
        printRemoteKV(i === 0 ? 'Remote pools' : '', hex(p))
      })
    }
    printRemoteKV('Inbound', fmtBucketLine(ib.cfg.enabled, ib.tokens, ib.cfg.capacity, ib.cfg.rate, ib.lastUpdated))
    printRemoteKV('Outbound', fmtBucketLine(ob.cfg.enabled, ob.tokens, ob.cfg.capacity, ob.cfg.rate, ob.lastUpdated))
  }
}

function printRemoteKV(key: string, value: string) {
  console.info(`      ${chalk.cyan(key.padEnd(14))}  ${value}`)
}

function fmtBucketLine(
  enabled: boolean,
  tokens: any,
  capacity: any,
  rate: any,
  lastUpdated: any
): string {
  const state = enabled ? chalk.green('enabled ') : chalk.gray('disabled')
  return `${state}  tokens=${tokens}  capacity=${capacity}  rate=${rate}/s  last_updated=${fmtTimestamp(lastUpdated)}`
}

async function inspectBatch(ctx: SetupResult, pdas: PublicKey[]) {
  if (pdas.length === 0) {
    console.info('No addresses found in CSV file.')
    return
  }
  const connection = ctx.provider.connection
  console.info(`Fetching ${pdas.length} State account(s)...`)
  const stateInfos = await getMultipleAccountsBatched(connection, pdas)

  type Row = { pda: PublicKey; programId: PublicKey; state: PoolState }
  const rows: Row[] = []
  for (let i = 0; i < pdas.length; i++) {
    const info = stateInfos[i]
    if (!info) {
      console.info(`State PDA ${pdas[i].toBase58()}: account not found on-chain`)
      continue
    }
    const programId = info.owner
    const programKey = pickPoolProgramKey(ctx, programId)
    try {
      // Decode from the buffer we already fetched — no per-row RPC.
      const state = decodeAccountFromData<PoolState>(ctx, programKey, 'state', info.data)
      rows.push({ pda: pdas[i], programId, state })
    } catch (e: any) {
      console.warn(`State PDA ${pdas[i].toBase58()}: decode failed (${e.message ?? e})`)
    }
  }

  // Derive one TAR PDA per mint and batch-fetch — avoids a program-wide scan.
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
      /* old layout — leave absent, printed as NOT FOUND */
    }
  }

  console.info('='.repeat(80))
  for (const row of rows) {
    const tar = tarByMint.get(row.state.config.mint.toBase58())
    console.info(`Token Pool Program ID:                ${labelProgram(row.programId)}`)
    console.info(`Token Mint:                           ${row.state.config.mint.toBase58()}`)
    console.info(`Token Pool State PDA:                 ${row.pda.toBase58()}`)
    console.info(`Token Pool State PDA owner:           ${displayPubkey(row.state.config.owner)}`)
    console.info(`Token Pool State PDA proposed owner:  ${displayPubkey(row.state.config.proposedOwner)}`)
    console.info(`Token Pool State PDA rate limit admin:${displayPubkey(row.state.config.rateLimitAdmin)}`)
    if (tar) {
      console.info(`Token Admin Registry PDA:             ${tar.publicKey.toBase58()}`)
      console.info(`Token Admin Registry administrator:   ${displayPubkey(tar.account.administrator)}`)
      console.info(`Token Admin Registry pending_admin:   ${displayPubkey(tar.account.pendingAdministrator)}`)
    } else {
      console.info(`Token Admin Registry PDA:             NOT FOUND`)
    }
    console.info('-'.repeat(80))
  }
  console.info('='.repeat(80))
  console.info(`\nTotal: ${rows.length} token pool(s)`)
}
