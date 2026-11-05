import { Command } from 'commander'
import chalk from 'chalk'
import Table from 'cli-table3'
import { PublicKey } from '@solana/web3.js'
import type { IdlAccounts } from '@anchor-lang/core'

import setup from '../../config/index.ts'
import { decodeAccount, derivePda, feeQuoterSeeds } from '../decode-pda.ts'
import type { FeeQuoter } from '../../../../contracts/target/types/fee_quoter.ts'

// Default billing tokens checked when --mints isn't passed: LINK SPL, deprecated Token-2022 LINK, WSOL.
const DEFAULT_MINTS = [
  'LinkhB3afbBKb2EQQu7s7umdZceV3wcvAUJhQAfQ23L', // LINK (SPL Token)
  'LNKVT1HtgkkddxRGoCTLi4yYzukNi3dN6kCkkurnYbz', // LINK (Token-2022, deprecated)
  'So11111111111111111111111111111111111111112', // WSOL
]

type FeeQuoterConfig = IdlAccounts<FeeQuoter>['config']
type BillingTokenConfigWrapper = IdlAccounts<FeeQuoter>['billingTokenConfigWrapper']

export default new Command()
  .description('Validate FeeQuoter config + billing-token configs for the given mints')
  .option('--mints <list>', `comma-separated mint addresses (default: ${DEFAULT_MINTS.join(',')})`)
  .action(async function (this: Command) {
    const ctx = await setup(this)
    const { mints } = this.opts<{ mints?: string }>()
    const mintList = (mints?.split(',').map((s) => s.trim()).filter(Boolean) ?? DEFAULT_MINTS).map(
      (s) => new PublicKey(s)
    )

    console.info('Fetching FeeQuoter config...')
    const cfgPda = derivePda(ctx, 'feeQuoter', feeQuoterSeeds.config())
    const cfg = await decodeAccount<FeeQuoterConfig>(ctx, 'feeQuoter', 'config', cfgPda)
    console.info(`Config PDA:           ${cfgPda.toBase58()}`)
    console.info(`Owner:                ${cfg.owner.toBase58()}`)
    console.info(`Proposed owner:       ${cfg.proposedOwner.toBase58()}`)
    console.info(`LINK token mint:      ${cfg.linkTokenMint.toBase58()}`)
    console.info(`LINK local decimals:  ${cfg.linkTokenLocalDecimals}`)
    console.info(`Max fee juels / msg:  ${cfg.maxFeeJuelsPerMsg.toString()}`)
    console.info()

    const t = new Table({ head: ['Mint', 'Config PDA', 'Enabled'] })
    const issues: string[] = []
    for (const mint of mintList) {
      const pda = derivePda(ctx, 'feeQuoter', feeQuoterSeeds.billingTokenConfig(mint))
      try {
        const wrapped = await decodeAccount<BillingTokenConfigWrapper>(
          ctx,
          'feeQuoter',
          'billingTokenConfigWrapper',
          pda
        )
        const enabled = wrapped.config.enabled
        t.push([mint.toBase58(), pda.toBase58(), enabled ? chalk.green('YES') : chalk.red('NO')])
        if (!enabled) issues.push(`Billing config for mint ${mint.toBase58()} is DISABLED`)
        if (!wrapped.config.mint.equals(mint)) {
          issues.push(
            `Billing config for ${mint.toBase58()} has mismatched inner mint ${wrapped.config.mint.toBase58()}`
          )
        }
      } catch (e: any) {
        t.push([mint.toBase58(), pda.toBase58(), chalk.red(`MISSING (${e.message ?? e})`)])
        issues.push(`Billing config not found for mint ${mint.toBase58()}`)
      }
    }
    console.info(t.toString())
    if (issues.length) {
      console.warn(chalk.yellow(`\nIssues:`))
      for (const i of issues) console.warn(chalk.yellow(`  - ${i}`))
    } else {
      console.info(chalk.green('\nAll billing tokens look OK.'))
    }
  })
