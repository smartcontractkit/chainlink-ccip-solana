import fs from 'fs'
import { Command, Option } from 'commander'
import {
  ComputeBudgetProgram,
  SystemProgram,
  Transaction,
  TransactionInstruction,
  TransactionMessage,
  VersionedTransaction,
  type SimulateTransactionConfig,
} from '@solana/web3.js'
import anchor from '@anchor-lang/core'
const { BN } = anchor
import { sha256 } from '@noble/hashes/sha2'
import { deflateSync } from 'zlib'
import chalk from 'chalk'
import setup from '../config/index.ts'
import {
  IDL_IX_TAG,
  LEGACY_IDL_HEADER_SIZE,
  legacyIdlAddress,
  readLegacyIdlAuthority,
  RESIZE_VARIANT_INDEX,
} from '../utils/legacy-idl.ts'

function encodeResizeIxData(newDataLen: number | bigint): Buffer {
  const dataLen = new BN(newDataLen.toString())
  return Buffer.concat([
    IDL_IX_TAG, // on-chain program expects little-endian tag
    Buffer.from([RESIZE_VARIANT_INDEX]),
    dataLen.toArrayLike(Buffer, 'le', 8),
  ])
}

export default new Command()
  .description("Resize a program's legacy Anchor IDL account (the on-chain idl PDA).")
  .argument('program', 'name of a configured program (router, receiver, feeQuoter, ...)')
  .addOption(new Option('--bytes <n>', 'new total account size in bytes').argParser((v) => parseInt(v, 10)))
  .addOption(new Option('--from-idl <path>', 'compute the new size to fit this IDL JSON file'))
  .option('--dry-run', 'simulate the transaction instead of sending it')
  .action(async function (this: Command) {
    const { programs, provider, txLogger } = await setup(this)
    const { bytes, fromIdl, dryRun } = this.opts() as {
      bytes?: number
      fromIdl?: string
      dryRun?: boolean
    }

    const [programName] = this.args

    if (!Object.keys(programs).includes(programName)) {
      throw new Error(`Invalid program: ${programName}. Must be one of: ${Object.keys(programs).join(', ')}`)
    }
    const program = programs[programName as keyof typeof programs]
    if (!program) {
      throw new Error(`Program ${programName} is not configured. Please check your configuration.`)
    }

    if ((bytes == null) === (fromIdl == null)) {
      throw new Error('Provide exactly one of --bytes <n> or --from-idl <path>')
    }

    let newSize: number
    if (bytes != null) {
      if (!Number.isFinite(bytes) || bytes <= 0) {
        throw new Error(`--bytes must be a positive integer, got: ${bytes}`)
      }
      newSize = bytes
    } else {
      const raw = fs.readFileSync(fromIdl!, 'utf-8')
      // round-trip to strip whitespace before compressing, mirroring `anchor idl` behavior.
      const minified = JSON.stringify(JSON.parse(raw))
      const compressed = deflateSync(Buffer.from(minified, 'utf-8'))
      newSize = LEGACY_IDL_HEADER_SIZE + compressed.length
      console.debug(
        `Computed new size from IDL ${chalk.cyan(fromIdl)}: ${chalk.yellowBright(newSize)} bytes ` +
          `(${LEGACY_IDL_HEADER_SIZE} header + ${compressed.length} compressed payload)`
      )
    }

    const programId = program.programId
    const idlAddr = legacyIdlAddress(programId)

    const accountInfo = await provider.connection.getAccountInfo(idlAddr)
    if (!accountInfo) {
      throw new Error(
        `Legacy IDL account ${idlAddr.toBase58()} does not exist for ${programName} (${programId.toBase58()}). Initialize it first with \`anchor idl init\`.`
      )
    }
    if (!accountInfo.owner.equals(programId)) {
      throw new Error(
        `Account ${idlAddr.toBase58()} is owned by ${accountInfo.owner.toBase58()}, not by ${programId.toBase58()}.`
      )
    }

    const currentSize = accountInfo.data.length
    if (newSize < currentSize) {
      throw new Error(
        `Refusing to shrink: current size is ${currentSize}, requested ${newSize}. (Shrinking is not supported in v1.)`
      )
    }
    if (newSize === currentSize) {
      console.info(chalk.gray(`No-op: account is already ${currentSize} bytes.`))
      return
    }

    const onchainAuthority = readLegacyIdlAuthority(accountInfo.data)
    if (!onchainAuthority.equals(provider.wallet.publicKey)) {
      console.warn(
        chalk.yellow(
          `⚠️  IDL authority is ${onchainAuthority.toBase58()}, but the configured wallet is ${provider.wallet.publicKey.toBase58()}. The transaction will fail on-chain.`
        )
      )
    }

    const ix = new TransactionInstruction({
      programId,
      keys: [
        { pubkey: idlAddr, isSigner: false, isWritable: true },
        { pubkey: provider.wallet.publicKey, isSigner: true, isWritable: true },
        { pubkey: SystemProgram.programId, isSigner: false, isWritable: false },
      ],
      data: encodeResizeIxData(newSize),
    })

    console.debug('Resize instruction:', ix)

    console.info(`Resizing legacy IDL for ${chalk.bold(programName)} (${chalk.cyan(programId.toBase58())}):`)
    console.info(`  account:    ${chalk.cyan(idlAddr.toBase58())}`)
    console.info(`  size:       ${currentSize} → ${chalk.yellowBright(newSize)} bytes`)

    if (dryRun) {
      const computeBudgetIx = ComputeBudgetProgram.setComputeUnitLimit({ units: 1_400_000 })
      const message = new TransactionMessage({
        payerKey: provider.wallet.publicKey,
        recentBlockhash: '11111111111111111111111111111112',
        instructions: [computeBudgetIx, ix],
      })
      const tx = new VersionedTransaction(message.compileToV0Message())
      const config: SimulateTransactionConfig = {
        commitment: 'confirmed',
        replaceRecentBlockhash: true,
        sigVerify: false,
      }
      const result = await provider.connection.simulateTransaction(tx, config)
      if (result.value.err) {
        console.error(chalk.red('Dry-run failed:'), {
          err: result.value.err,
          logs: result.value.logs,
          unitsConsumed: result.value.unitsConsumed,
        })
        process.exitCode = 1
        return
      }
      console.info(chalk.green('Dry-run succeeded.'))
      console.debug({ logs: result.value.logs, unitsConsumed: result.value.unitsConsumed })
      return
    }

    const txSig = await provider.sendAndConfirm(new Transaction().add(ix))
    txLogger(txSig)
  })
