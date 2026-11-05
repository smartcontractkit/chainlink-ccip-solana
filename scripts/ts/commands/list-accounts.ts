import { Command } from 'commander'
import chalk from 'chalk'

import setup from '../config/index.ts'
import { findAllAccounts, getProgram, type ProgramKey } from './decode-pda.ts'

export default new Command()
  .description(
    'List all on-chain accounts of a given type for a program, using getProgramAccounts filtered by discriminator'
  )
  .argument('program', 'program name (e.g. router, offramp, feeQuoter)')
  .argument(
    '[accountType]',
    'account type to search for, in camelCase as defined in the IDL (e.g. destChain, tokenAdminRegistry). ' +
      'Required unless --list is used.'
  )
  .option('-l, --list', 'list all account types defined in the IDL for this program')
  .option('-d, --decode', 'print the full decoded account data (JSON) alongside each address')
  .option('-o, --output <format>', 'output format for decoded account data (js or json)', 'js')
  .action(async function (this: Command) {
    const ctx = await setup(this)

    const { quiet, list, decode, output } = this.optsWithGlobals()
    if (output !== 'js' && output !== 'json') {
      throw new Error(`Invalid output format: ${output}. Must be one of: js, json`)
    }

    const [programName, accountType] = this.args

    const prog = getProgram(ctx, programName as ProgramKey)

    if (list) {
      const types = Object.keys(prog.account)
      console.info(`Account types for ${chalk.bold(programName)}:\n  ${types.join('\n  ')}`)
      return
    }

    if (!accountType) {
      throw new Error('accountType argument is required unless --list is used')
    }

    const results = await findAllAccounts(ctx, programName as ProgramKey, accountType)

    if (results.length === 0) {
      console.info(
        `No ${chalk.yellowBright(accountType)} accounts found for program ${chalk.bold(programName)}.`
      )
      return
    }

    if (!quiet) {
      console.info(
        `Found ${chalk.greenBright(results.length)} ${chalk.yellowBright(accountType)} account(s) for ${chalk.bold(programName)}:\n`
      )
    }

    if (decode) {
      if (output === 'json') {
        console.info(JSON.stringify(results, null, 4))
        return
      }

      for (const { publicKey, account } of results) {
        console.info(`${chalk.cyan(publicKey.toBase58())}:`, account)
      }
      return
    }


    const pubkeys = results.map((r) => r.publicKey.toBase58())
    if (output == 'json') {
      console.info(JSON.stringify(pubkeys, null, 4))
      return
    }

    for (const pubkey of pubkeys) {
      console.info(pubkey)
    }
  })
