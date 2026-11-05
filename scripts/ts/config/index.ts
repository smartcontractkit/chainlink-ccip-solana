import path from 'path'
import fs from 'fs'

import chalk from 'chalk'
import Table from 'cli-table3'

import { PublicKey, Connection, Keypair } from '@solana/web3.js'
import { AnchorProvider, Program, Wallet } from '@anchor-lang/core'
import {
  AnchorProvider as LegacyAnchorProvider,
  Program as LegacyProgram,
  Wallet as LegacyWallet,
} from '@coral-xyz/anchor'
import type { Idl as LegacyIdl } from '@coral-xyz/anchor'

import type { TestCcipReceiver } from '../../../contracts/target/types/test_ccip_receiver.ts'
import testCcipReceiverIdl from '../../../contracts/target/idl/test_ccip_receiver.json' with { type: 'json' }
import type { CcipRouter } from '../../../contracts/target/types/ccip_router.ts'
import routerIdl from '../../../contracts/target/idl/ccip_router.json' with { type: 'json' }
import type { BurnmintTokenPool } from '../../../contracts/target/types/burnmint_token_pool.ts'
import burnmintIdl from '../../../contracts/target/idl/burnmint_token_pool.json' with { type: 'json' }
import type { FeeQuoter } from '../../../contracts/target/types/fee_quoter.ts'
import feeQuoterIdl from '../../../contracts/target/idl/fee_quoter.json' with { type: 'json' }
import type { LockreleaseTokenPool } from '../../../contracts/target/types/lockrelease_token_pool.ts'
import lockreleaseIdl from '../../../contracts/target/idl/lockrelease_token_pool.json' with { type: 'json' }
import type { CctpTokenPool } from '../../../contracts/target/types/cctp_token_pool.ts'
import cctpTokenPoolIdl from '../../../contracts/target/idl/cctp_token_pool.json' with { type: 'json' }
import type { CcipOfframp } from '../../../contracts/target/types/ccip_offramp.ts'
import offrampIdl from '../../../contracts/target/idl/ccip_offramp.json' with { type: 'json' }
import type { RmnRemote } from '../../../contracts/target/types/rmn_remote.ts'
import rmnIdl from '../../../contracts/target/idl/rmn_remote.json' with { type: 'json' }

// 1.6 IDLs — Anchor-generated typed bindings vendored under idls/1_6/.
// Each upstream module exports both `type X` (compile-time) and `const IDL`
// (runtime value used by `new Program(IDL, ...)`). We import the value form
// here; type extraction (via `IdlAccounts<>`/`IdlTypes<>`) is done from the
// 2.0 bindings in commands/tokens/_shared.ts since those have cross-IDL
// `defined:` references already inlined by Anchor 1.0's idl-build.
// Refresh with `npm run sync-idls`.
import { IDL as router16Idl } from '../idls/1_6/ccip_router.ts'
import { IDL as feeQuoter16Idl } from '../idls/1_6/fee_quoter.ts'
import { IDL as burnmint16Idl } from '../idls/1_6/burnmint_token_pool.ts'
import { IDL as lockrelease16Idl } from '../idls/1_6/lockrelease_token_pool.ts'
import { IDL as cctp16Idl } from '../idls/1_6/cctp_token_pool.ts'
import { IDL as offramp16Idl } from '../idls/1_6/ccip_offramp.ts'
import { IDL as common16Idl } from '../idls/1_6/ccip_common.ts'
import { IDL as baseTokenPool16Idl } from '../idls/1_6/base_token_pool.ts'
import { IDL as rmn16Idl } from '../idls/1_6/rmn_remote.ts'

// CCIP 1.6 splits types across IDLs (e.g. `BaseConfig` lives in base_token_pool,
// `TokenAdminRegistry` lives in ccip_common). Anchor 0.29's BorshAccountsCoder
// only resolves `defined:` references within a single IDL, so we merge all
// known types into every consumer at load time, and graft ccip_common's
// accounts onto the router IDL (the router program physically owns them).

const ALL_1_6_TYPE_SOURCES: any[] = [
  baseTokenPool16Idl,
  common16Idl,
  router16Idl,
  feeQuoter16Idl,
  burnmint16Idl,
  lockrelease16Idl,
  cctp16Idl,
  offramp16Idl,
  rmn16Idl,
]

const ALL_1_6_TYPES = (() => {
  const seen = new Map<string, any>()
  for (const idl of ALL_1_6_TYPE_SOURCES) {
    for (const t of (idl as any).types ?? []) {
      if (!seen.has(t.name)) seen.set(t.name, t)
    }
  }
  return Array.from(seen.values())
})()

function withMergedTypes(idl: any, extraAccounts: any[] = []): any {
  const existingTypeNames = new Set(((idl.types as any[]) ?? []).map((t) => t.name))
  const existingAccountNames = new Set(((idl.accounts as any[]) ?? []).map((a) => a.name))
  return {
    ...idl,
    types: [
      ...((idl.types as any[]) ?? []),
      ...ALL_1_6_TYPES.filter((t) => !existingTypeNames.has(t.name)),
    ],
    accounts: [
      ...((idl.accounts as any[]) ?? []),
      ...extraAccounts.filter((a) => !existingAccountNames.has(a.name)),
    ],
  }
}

// Router needs TokenAdminRegistry (owned by router program, but defined in ccip_common).
const router16Merged = withMergedTypes(router16Idl, (common16Idl as any).accounts ?? [])
const feeQuoter16Merged = withMergedTypes(feeQuoter16Idl)
const burnmint16Merged = withMergedTypes(burnmint16Idl)
const lockrelease16Merged = withMergedTypes(lockrelease16Idl)
const cctp16Merged = withMergedTypes(cctp16Idl)
const offramp16Merged = withMergedTypes(offramp16Idl)
const rmn16Merged = withMergedTypes(rmn16Idl)

import { capitalize } from '../utils/strings.ts'
import type { Command } from 'commander'
import { bs58 } from '@anchor-lang/core/dist/cjs/utils/bytes/index.js'

const __dirname = import.meta.dirname

// -- Schema -------------------------------------------------------------------

export type CcipVersion = '1.6' | '2.0'

type IdlTypeMap = {
  receiver: TestCcipReceiver
  router: CcipRouter
  testPool: BurnmintTokenPool
  feeQuoter: FeeQuoter
  burnmintTokenPool: BurnmintTokenPool
  lockReleaseTokenPool: LockreleaseTokenPool
  cctpTokenPool: CctpTokenPool
  offramp: CcipOfframp
  rmn: RmnRemote
}

const IDL_MAP_2_0: { [Property in keyof IdlTypeMap]: any } = {
  receiver: testCcipReceiverIdl,
  router: routerIdl,
  testPool: burnmintIdl,
  feeQuoter: feeQuoterIdl,
  burnmintTokenPool: burnmintIdl,
  lockReleaseTokenPool: lockreleaseIdl,
  cctpTokenPool: cctpTokenPoolIdl,
  offramp: offrampIdl,
  rmn: rmnIdl,
}

// 1.6 program keys are a subset (no test helpers; testPool is the same as burnmintTokenPool).
const IDL_MAP_1_6: Partial<Record<keyof IdlTypeMap, LegacyIdl>> = {
  router: router16Merged as unknown as LegacyIdl,
  feeQuoter: feeQuoter16Merged as unknown as LegacyIdl,
  burnmintTokenPool: burnmint16Merged as unknown as LegacyIdl,
  testPool: burnmint16Merged as unknown as LegacyIdl,
  lockReleaseTokenPool: lockrelease16Merged as unknown as LegacyIdl,
  cctpTokenPool: cctp16Merged as unknown as LegacyIdl,
  offramp: offramp16Merged as unknown as LegacyIdl,
  rmn: rmn16Merged as unknown as LegacyIdl,
}

// Type that TS config files must export as default. Loaded by setup() below.
export type Config = {
  rpcUrl: string
  explorerUrl: (s: string) => string
  /**
   * Optional. When omitted and no `-k` flag is passed, setup uses an ephemeral
   * keypair — suitable for read-only inspection commands. Any command that
   * sends a transaction must run with `-k` or a configured wallet.
   */
  walletPath?: string
  ccipVersion: CcipVersion
  programs: {
    [Property in keyof IdlTypeMap]?: PublicKey
  }
}

// Type of the resolved setup
export type SetupResult = {
  ccipVersion: CcipVersion
  provider: AnchorProvider | LegacyAnchorProvider
  // Populated when ccipVersion === '2.0'; empty object when '1.6'.
  programs: {
    [Property in keyof IdlTypeMap]?: Program<IdlTypeMap[Property]>
  }
  // Populated when ccipVersion === '1.6'; empty object when '2.0'.
  legacyPrograms: {
    [Property in keyof IdlTypeMap]?: LegacyProgram<LegacyIdl>
  }
  config: Config
  txLogger: (txSig: string) => void
}

// -- Loaders ------------------------------------------------------------------

export async function setupWithoutConfig(cmd: Command) {
  const { quiet } = cmd.optsWithGlobals()
  setupLogLevel(quiet)
}

export default async function setup(cmd: Command, processEnv: boolean = true): Promise<SetupResult> {
  setupWithoutConfig(cmd)
  const { env, keypair: keypairOption, ccipVersion: ccipVersionFlag } = cmd.optsWithGlobals()

  const config = await loadEnvConfig(env)
  const ccipVersion: CcipVersion = resolveCcipVersion(ccipVersionFlag, config.ccipVersion)

  const keypairSource = keypairOption ?? config.walletPath
  const keypair = keypairSource ? loadKeypair(keypairSource) : ephemeralKeypair()
  const connection = new Connection(config.rpcUrl, 'confirmed')

  let provider: AnchorProvider | LegacyAnchorProvider
  const programs: SetupResult['programs'] = {}
  const legacyPrograms: SetupResult['legacyPrograms'] = {}
  const addressesForLogging: Array<[string, string]> = []

  if (ccipVersion === '2.0') {
    const wallet = new Wallet(keypair)
    provider = new AnchorProvider(connection, wallet, { commitment: 'confirmed' })

    // Mutate the IDLs to use the configured program addresses.
    for (const [name, idl] of Object.entries(IDL_MAP_2_0)) {
      idl.address = config.programs[name as keyof Config['programs']]?.toBase58() ?? idl.address
    }

    programs.receiver = new Program<TestCcipReceiver>(testCcipReceiverIdl as TestCcipReceiver, provider)
    programs.router = new Program<CcipRouter>(routerIdl as CcipRouter, provider)
    programs.testPool = new Program<BurnmintTokenPool>(burnmintIdl as BurnmintTokenPool, provider)
    programs.feeQuoter = new Program<FeeQuoter>(feeQuoterIdl as FeeQuoter, provider)
    programs.burnmintTokenPool = new Program<BurnmintTokenPool>(burnmintIdl as BurnmintTokenPool, provider)
    programs.lockReleaseTokenPool = new Program<LockreleaseTokenPool>(
      lockreleaseIdl as LockreleaseTokenPool,
      provider
    )
    programs.cctpTokenPool = new Program<CctpTokenPool>(cctpTokenPoolIdl as CctpTokenPool, provider)
    programs.offramp = new Program<CcipOfframp>(offrampIdl as CcipOfframp, provider)
    programs.rmn = new Program<RmnRemote>(rmnIdl as RmnRemote, provider)

    for (const [name, p] of Object.entries(programs)) {
      addressesForLogging.push([name, p!.programId.toBase58()])
    }
  } else {
    const wallet = new LegacyWallet(keypair)
    provider = new LegacyAnchorProvider(connection, wallet, { commitment: 'confirmed' })

    for (const [name, idl] of Object.entries(IDL_MAP_1_6) as Array<
      [keyof IdlTypeMap, LegacyIdl]
    >) {
      const address = config.programs[name]
      if (!address) continue // env config didn't supply this program — skip rather than crash
      legacyPrograms[name] = new LegacyProgram(idl, address, provider)
      addressesForLogging.push([name, address.toBase58()])
    }
  }

  const table = new Table({ style: { compact: true } })
  table.push(
    { Network: chalk.yellow(env) },
    { 'CCIP Version': chalk.magenta(ccipVersion) },
    { RPC: chalk.gray(config.rpcUrl) },
    { Wallet: chalk.blue(provider.wallet.publicKey.toBase58()) },
    { Programs: '-----------------' },
    ...addressesForLogging.map(([name, address]) => ({
      [`↪ ${capitalize(name)}`]: chalk.green(address),
    }))
  )
  console.debug(table.toString())

  const txLogger = (txSig: string) => {
    console.info(`✅ Transaction successful: ${chalk.green(txSig)}`)
    console.debug(chalk.gray(`🔗 Explorer URL: ${config.explorerUrl(txSig)}`))
  }

  return { ccipVersion, provider, programs, legacyPrograms, config, txLogger }
}

// -- Internal helpers --------------------------------------------------------------

function resolveCcipVersion(flag: string | undefined, fromConfig: CcipVersion | undefined): CcipVersion {
  const candidate = flag ?? fromConfig
  if (candidate !== '1.6' && candidate !== '2.0') {
    throw new Error(
      `Invalid CCIP version "${candidate}". Expected "1.6" or "2.0". ` +
        `Set --ccip-version on the CLI or ccipVersion in the env config.`
    )
  }
  return candidate
}

function setupLogLevel(quiet: boolean) {
  console.log = console.info

  if (!quiet) {
    return
  }
  // Levels: trace, debug, log/info, warn, error
  console.trace = () => {}
  console.debug = () => {}
}

async function loadEnvConfig(envName: string): Promise<Config> {
  try {
    const imported = await import(`./envs/${envName}.ts`)
    return imported.default
  } catch (err) {
    throw new Error(
      `Config for env "${envName}" not found. Available envs: ${listAvailableEnvs().join(', ')}`
    )
  }
}

function listAvailableEnvs(): string[] {
  const configDir = path.resolve(__dirname, 'envs')
  if (!fs.existsSync(configDir)) return []
  return fs
    .readdirSync(configDir)
    .filter((f) => f.endsWith('.ts'))
    .map((f) => f.replace('.ts', ''))
}

function ephemeralKeypair(): Keypair {
  console.debug('No wallet configured; using an ephemeral keypair (read-only commands only).')
  return Keypair.generate()
}

function loadKeypair(privatekeyOrPath: string): Keypair {
  try {
    const bytes = bs58.decode(privatekeyOrPath)
    if (bytes.length === 64) {
      console.debug("Loaded keypair from base58 private key provided via flags.")
      return Keypair.fromSecretKey(bytes)
    }
  } catch {
      console.debug("Using keypair from file path provided via flags.")
  }

  const resolved = privatekeyOrPath.startsWith('~')
    ? path.join(process.env.HOME!, privatekeyOrPath.slice(1))
    : path.resolve(__dirname, privatekeyOrPath)

  const raw = JSON.parse(fs.readFileSync(resolved, 'utf8'))
  return Keypair.fromSecretKey(Uint8Array.from(raw))
}
