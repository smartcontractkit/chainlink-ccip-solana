import { Command } from 'commander'
import {
  ComputeBudgetProgram,
  PublicKey,
  SYSVAR_CLOCK_PUBKEY,
  TransactionMessage,
  VersionedTransaction,
  type SimulateTransactionConfig,
} from '@solana/web3.js'
import setup from '../config/index.ts'
import chalk from 'chalk'

export default new Command()
  .description('Query the type-version of a given program')
  .argument('address', 'address for the program')
  .action(async function (this: Command) {
    const { programs, provider } = await setup(this)

    const [address] = this.args

    const programId = new PublicKey(address)

    // Just use the router here to construct the generic ix, as the type-version instruction
    // is the same across all programs
    const ix = await programs.router?.methods
      .typeVersion()
      .accounts({
        clock: SYSVAR_CLOCK_PUBKEY,
      })
      .instruction()!

    ix.programId = programId // override the programId to the user-provided one

    const recentBlockhash = '11111111111111111111111111111112'
    const computeBudgetIx = ComputeBudgetProgram.setComputeUnitLimit({ units: 1_400_000 })

    const message = new TransactionMessage({
      payerKey: provider.wallet.publicKey,
      recentBlockhash,
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
      console.error(chalk.red('Failed to simulate transaction:'), {
        logs: result.value.logs,
        unitsConsumed: result.value.unitsConsumed,
        returnData: result.value.returnData,
        err: result.value.err,
      })
      return
    }

    const prefix = `Program return: ${address}`
    const b64 = result.value.logs?.find((log) => log.startsWith(prefix))?.replace(prefix, '')
    if (!b64) {
      console.error(chalk.red('Failed to find program return data in logs:'), {
        logs: result.value.logs,
        unitsConsumed: result.value.unitsConsumed,
        returnData: result.value.returnData,
      })
      return
    }
    const res = Buffer.from(b64, 'base64').toString('utf-8')

    console.info(`Program ${chalk.cyan(address)} is`, chalk.yellowBright(res))
  })
