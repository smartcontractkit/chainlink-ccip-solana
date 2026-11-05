import { Command } from 'commander'
import { AnchorProvider, Program, utils, type Idl } from '@anchor-lang/core'
import { PublicKey, type AccountInfo } from '@solana/web3.js'
import chalk from 'chalk'
import Table from 'cli-table3'
import setup from '../config/index.ts'
import { capitalize } from '../utils/strings.ts'
import { legacyIdlAddress, decodeLegacyIdlAccount, readLegacyIdlAuthority } from '../utils/legacy-idl.ts'

const BPF_LOADER_UPGRADEABLE_PROGRAM_ID = new PublicKey('BPFLoaderUpgradeab1e11111111111111111111111')

export default new Command()
  .description(
    'For every configured program, show the last upgrade timestamp and the version from the on-chain IDL metadata.'
  )
  .option('-w, --wide', 'Show additional columns')
  .action(async function (this: Command) {
    const { programs, provider } = await setup(this)

    const { wide } = this.opts()

    const table = new Table({
      head: [
        'Program',
        'Address',
        ...(wide ? ['ProgramData'] : []),
        'Last upgrade (UTC)',
        'Verified',
        'Metadata IDL version',
        'Legacy IDL version',
        ...(wide ? ['Legacy IDL authority'] : []),
      ],
      style: { head: ['bold'] },
    })

    const entries = Object.entries(programs).filter(([, p]) => p)

    const rows = await Promise.all(
      entries.map(async ([name, program]) => {
        const programId = program!.programId
        const [programDataAddress] = PublicKey.findProgramAddressSync(
          [programId.toBuffer()],
          BPF_LOADER_UPGRADEABLE_PROGRAM_ID
        )

        const legacyIdlAddr = legacyIdlAddress(programId)
        console.debug('Legacy IDL Address for', programId.toBase58(), '->', legacyIdlAddr.toBase58())

        const [programData, idl, legacyAccountInfo, verification] = await Promise.all([
          utils.registry.fetchData(provider.connection, programId).catch((e: Error) => e),
          Program.fetchIdl(programId, provider).catch((e: Error) => e),
          provider.connection.getAccountInfo(legacyIdlAddr).catch((e: Error) => e),
          verificationStatus(programId).catch((e: Error) => e),
        ])

        let timeCell: string = await timeInfo(programData, provider)
        let metadataVersionCell: string = metadataIdlInfo(idl)
        let { legacyVersionCell, legacyAuthorityCell } = legacyIdlInfo(legacyAccountInfo)
        let verifiedCell: string = verificationInfo(verification)

        return [
          chalk.bold(capitalize(name)),
          chalk.cyan(programId.toBase58()),
          ...(wide ? [chalk.cyan(programDataAddress.toBase58())] : []),
          timeCell,
          verifiedCell,
          metadataVersionCell,
          legacyVersionCell,
          ...(wide ? [legacyAuthorityCell] : []),
        ]
      })
    )

    for (const row of rows) table.push(row)
    console.info(table.toString())
  })

type VerificationStatus = { is_verified: boolean; repo_url?: string }

async function verificationStatus(programId: PublicKey): Promise<VerificationStatus> {
  const res = await fetch(`https://verify.osec.io/status/${programId.toBase58()}`)
  if (!res.ok) throw new Error(`osec ${res.status}`)
  return res.json() as Promise<VerificationStatus>
}

function verificationInfo(status: Error | VerificationStatus): string {
  if (status instanceof Error) return chalk.red('n/a')
  return status.is_verified ? chalk.green('✓ verified') : chalk.red('✗ not verified')
}

async function timeInfo(programData: utils.registry.ProgramData | Error, provider: AnchorProvider) {
  let timeCell: string
  if (programData instanceof Error) {
    // slotCell = chalk.red(programData.message)
    timeCell = chalk.red('n/a')
  } else {
    const slot = programData.slot.toNumber()
    // slotCell = slot.toString()
    const blockTime = await provider.connection.getBlockTime(slot).catch(() => null)
    timeCell = blockTime ? new Date(blockTime * 1000).toISOString() : chalk.gray('unknown')
  }
  return timeCell
}

function metadataIdlInfo(idl: Error | Idl | null) {
  let metadataVersionCell: string
  if (idl instanceof Error) {
    metadataVersionCell = chalk.red(idl.message)
  } else if (!idl) {
    metadataVersionCell = chalk.gray('no on-chain IDL')
  } else {
    metadataVersionCell = chalk.yellowBright((idl as any).metadata?.version ?? 'missing')
  }
  return metadataVersionCell
}

function legacyIdlInfo(legacyAccountInfo: Error | AccountInfo<Buffer<ArrayBufferLike>> | null): {
  legacyVersionCell: string
  legacyAuthorityCell: string
} {
  let legacyVersionCell: string
  let legacyAuthorityCell: string
  if (legacyAccountInfo instanceof Error) {
    legacyVersionCell = chalk.red(legacyAccountInfo.message)
    legacyAuthorityCell = chalk.red(legacyAccountInfo.message)
  } else if (!legacyAccountInfo) {
    legacyVersionCell = chalk.gray('no on-chain IDL')
    legacyAuthorityCell = chalk.gray('no on-chain IDL')
  } else {
    try {
      const legacyIdl = decodeLegacyIdlAccount(legacyAccountInfo.data)
      legacyVersionCell = chalk.yellowBright(legacyIdl.metadata?.version ?? 'missing')
      legacyAuthorityCell = chalk.cyan(readLegacyIdlAuthority(legacyAccountInfo.data).toBase58())
    } catch (e) {
      legacyVersionCell = chalk.red((e as Error).message)
      legacyAuthorityCell = chalk.red((e as Error).message)
    }
  }
  return { legacyVersionCell, legacyAuthorityCell }
}
