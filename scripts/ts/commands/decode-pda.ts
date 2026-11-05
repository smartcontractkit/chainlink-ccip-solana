import { Command } from 'commander'
import chalk from 'chalk'
import bs58 from 'bs58'

import { PublicKey } from '@solana/web3.js'
import type { GetProgramAccountsFilter } from '@solana/web3.js'
import type { Program } from '@anchor-lang/core'
import {
  BorshAccountsCoder as LegacyBorshAccountsCoder,
  type Idl as LegacyIdl,
  type Program as LegacyProgram,
} from '@coral-xyz/anchor'

import setup, { type SetupResult } from '../config/index.ts'
import { capitalize } from '../utils/strings.ts'
import { parseSeed } from '../utils/seeds.ts'

// ---------------------------------------------------------------------------
// PDA seed constants
//
// Anchor 0.29 (CCIP 1.6) and Anchor 1.0 (CCIP 2.0) IDLs both omit `pda.seeds`
// for CCIP accounts — seeds live only in the Rust source. We mirror them here
// once so every command derives PDAs the same way for both versions.

const u64le = (n: bigint | number): Buffer => {
  const b = Buffer.alloc(8)
  b.writeBigUInt64LE(BigInt(n), 0)
  return b
}

/** Seeds for the base token-pool layout (burnmint, lockrelease, cctp, custom). */
export const tokenPoolSeeds = {
  state: (mint: PublicKey) => [Buffer.from('ccip_tokenpool_config'), mint.toBuffer()],
  signer: (mint: PublicKey) => [Buffer.from('ccip_tokenpool_signer'), mint.toBuffer()],
  chainConfig: (chainSelector: bigint | number, mint: PublicKey) => [
    Buffer.from('ccip_tokenpool_chainconfig'),
    u64le(chainSelector),
    mint.toBuffer(),
  ],
} as const

/** Seeds for the FeeQuoter program. */
export const feeQuoterSeeds = {
  config: () => [Buffer.from('config')],
  billingTokenConfig: (mint: PublicKey) => [Buffer.from('fee_billing_token_config'), mint.toBuffer()],
} as const

/** Seeds for the CCIP Router program. */
export const routerSeeds = {
  tokenAdminRegistry: (mint: PublicKey) => [Buffer.from('token_admin_registry'), mint.toBuffer()],
} as const

// ---------------------------------------------------------------------------
// Version-specific primitives
//
// 2.0 uses `@anchor-lang/core` (Anchor 1.0); 1.6 uses `@coral-xyz/anchor` 0.29.
// Both expose the same `.account[name].fetch()` / `.account[name].all(filters)`
// surface, but the underlying decoders differ — keep the two paths explicit
// so future divergences (e.g. discriminator format) stay isolated.

export function derivePda2_0(programId: PublicKey, seeds: (Buffer | Uint8Array)[]): PublicKey {
  return PublicKey.findProgramAddressSync(seeds, programId)[0]
}

export async function decodeAccount2_0<T = any>(
  program: Program<any>,
  accountName: string,
  address: PublicKey
): Promise<T> {
  return (await (program.account as any)[accountName].fetch(address)) as T
}

export async function findAllAccounts2_0<T = any>(
  program: Program<any>,
  accountName: string,
  filters?: GetProgramAccountsFilter[]
): Promise<Array<{ publicKey: PublicKey; account: T }>> {
  return scanAndDecode<T>(program as any, accountName, filters)
}

export function derivePda1_6(programId: PublicKey, seeds: (Buffer | Uint8Array)[]): PublicKey {
  return PublicKey.findProgramAddressSync(seeds, programId)[0]
}

export async function decodeAccount1_6<T = any>(
  program: LegacyProgram<LegacyIdl>,
  accountName: string,
  address: PublicKey
): Promise<T> {
  return (await (program.account as any)[accountName].fetch(address)) as T
}

export async function findAllAccounts1_6<T = any>(
  program: LegacyProgram<LegacyIdl>,
  accountName: string,
  filters?: GetProgramAccountsFilter[]
): Promise<Array<{ publicKey: PublicKey; account: T }>> {
  return scanAndDecode<T>(program as any, accountName, filters)
}

/**
 * Lenient `getProgramAccounts` + per-account Borsh decode. On-chain accounts can
 * predate IDL field additions (e.g. `TokenAdminRegistry.supportsAutoDerivation`
 * added later), which makes the trailing field overflow when decoding old data.
 * Anchor's `.account.<name>.all()` aborts the whole call when any one row fails;
 * we skip the row, log a debug line, and keep going — matching the Python
 * scripts' behaviour.
 *
 * Works for both `@anchor-lang/core` (1.0) and `@coral-xyz/anchor` (0.29) since
 * both expose the same `coder.accounts.{accountDiscriminator,decode}` API.
 */
async function scanAndDecode<T = any>(
  program: any,
  accountName: string,
  extraFilters?: GetProgramAccountsFilter[]
): Promise<Array<{ publicKey: PublicKey; account: T }>> {
  const coder = program.coder.accounts
  // BorshAccountsCoder keys layouts by the exact `idl.accounts[].name` string,
  // so callers must pass it in the IDL's casing (camelCase in our vendored
  // bindings). accountDiscriminator() handles the camel→pascal conversion
  // internally so the SHA256 always matches the on-chain bytes.
  const disc: Buffer = LegacyBorshAccountsCoder.accountDiscriminator(accountName)
  const filters: GetProgramAccountsFilter[] = [
    { memcmp: { offset: 0, bytes: bs58.encode(disc) } },
    ...(extraFilters ?? []),
  ]
  const raws = await program.provider.connection.getProgramAccounts(program.programId, { filters })
  const out: Array<{ publicKey: PublicKey; account: T }> = []
  let skipped = 0
  for (const r of raws) {
    try {
      const decoded = coder.decode(accountName, r.account.data) as T
      out.push({ publicKey: r.pubkey, account: decoded })
    } catch (err) {
      skipped++
      console.debug(`  skipped ${r.pubkey.toBase58()}: ${(err as any)?.message ?? err}`)
    }
  }
  if (skipped > 0) {
    console.debug(`  ${skipped} ${accountName} account(s) failed to decode (older layout)`)
  }
  return out
}

// ---------------------------------------------------------------------------
// Version-dispatching facades — what command code should call.

export type ProgramKey = keyof NonNullable<SetupResult['programs']> | keyof NonNullable<SetupResult['legacyPrograms']>

/** Internal: pick the right Anchor `Program` instance for the active version. */
export function getProgram(ctx: SetupResult, key: ProgramKey): Program<any> | LegacyProgram<LegacyIdl> {
  const bag: Record<string, any> = (ctx.ccipVersion === '2.0' ? ctx.programs : ctx.legacyPrograms) as any
  const p = bag[key as string]
  if (!p) {
    const configured = Object.keys(bag).join(', ') || '(none)'
    throw new Error(
      `Program "${String(key)}" is not configured for ccipVersion=${ctx.ccipVersion}. ` +
        `Configured: ${configured}. Check your env config's "programs" map.`
    )
  }
  return p
}

/** Resolved on-chain address of a program in the active env+version. */
export function programId(ctx: SetupResult, key: ProgramKey): PublicKey {
  return (getProgram(ctx, key) as { programId: PublicKey }).programId
}

/** Derive a PDA owned by the named program. */
export function derivePda(
  ctx: SetupResult,
  key: ProgramKey,
  seeds: (Buffer | Uint8Array)[]
): PublicKey {
  return PublicKey.findProgramAddressSync(seeds, programId(ctx, key))[0]
}

/** Fetch + decode a single account using the program's IDL. */
export async function decodeAccount<T = any>(
  ctx: SetupResult,
  key: ProgramKey,
  accountName: string,
  address: PublicKey
): Promise<T> {
  const p = getProgram(ctx, key)
  return (await (p.account as any)[accountName].fetch(address)) as T
}

/**
 * Decode an account from a raw Buffer using the program's coder — no RPC.
 * Use this when you've already fetched the account data (e.g. via a batched
 * `getMultipleAccountsInfo`); avoids the extra per-row `getAccountInfo` that
 * `decodeAccount` would issue. `accountName` is the camelCase form used by
 * the IDL bindings.
 */
export function decodeAccountFromData<T = any>(
  ctx: SetupResult,
  key: ProgramKey,
  accountName: string,
  data: Buffer
): T {
  const p = getProgram(ctx, key)
  return (p as any).coder.accounts.decode(accountName, data) as T
}

/** Fetch + decode all accounts of the given type for the program. */
export async function findAllAccounts<T = any>(
  ctx: SetupResult,
  key: ProgramKey,
  accountName: string,
  filters?: GetProgramAccountsFilter[]
): Promise<Array<{ publicKey: PublicKey; account: T }>> {
  return scanAndDecode<T>(getProgram(ctx, key), accountName, filters)
}

// ---------------------------------------------------------------------------
// CLI command — `decode-pda <program> [type] [seeds...]`
//
// Uses the facades above, so it works for both 1.6 and 2.0. Same UX as before.

export default new Command()
  .description('Decode any PDA present in the supported IDLs')
  .argument('program', 'name for the program (e.g. router, offramp, ...)')
  .argument(
    '[type]',
    'name of the PDA type to decode (e.g. tokenAdminRegistry). Use camelCase, as defined in the IDL.'
  )
  .argument(
    '[seeds...]',
    'PDA seeds in <type>:<value> format as used in the solana CLI (e.g. string:ccip, pubkey:<base58>, u64le:123)'
  )
  .option('-l, --list', 'list all available PDA types for the specified program')
  .action(async function (this: Command) {
    const ctx = await setup(this)
    const { list } = this.opts()
    const [programName, type, ...seeds] = this.args

    const program = getProgram(ctx, programName as ProgramKey)

    if (list) {
      const accountNamespaces = Object.keys(program.account)
      console.info(`Available PDA types for program ${programName}:`, accountNamespaces)
      return
    }
    if (!type) {
      throw new Error('PDA type is required unless --list option is used')
    }

    const parsedSeeds = seeds.map((s) => parseSeed(s))
    const address = derivePda(ctx, programName as ProgramKey, parsedSeeds)
    const account = await decodeAccount(ctx, programName as ProgramKey, type, address)

    console.info(
      `${chalk.bold(capitalize(programName))} ${chalk.yellowBright(type)} (${chalk.cyan(address.toBase58())}):`,
      account
    )
  })
