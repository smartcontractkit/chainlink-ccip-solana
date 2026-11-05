import { Command } from 'commander'
import setup from '../../config/index.ts'

const MODES = ['accept-all', 'reject-all']

export default new Command()
  .description('Toggle configuration to reject-all/accept-all')
  .argument('mode', 'accept-all or reject-all')
  .action(async function (this: Command) {
    const {
      programs: { receiver },
      txLogger,
    } = await setup(this)

    const mode = this.args[0]
    if (!MODES.includes(mode)) {
      console.error(`Invalid mode: ${mode}. Must be one of: ${MODES.join(', ')}`)
      process.exit(1)
    }

    if (!receiver) {
      console.error('Missing required program(s). Please check your configuration.')
      process.exit(1)
    }

    const rejectAll = mode === 'reject-all'

    console.debug(`\nSetting test_ccip_receiver to ${mode}...`)

    const txSig = await receiver.methods.setRejectAll(rejectAll).rpc()

    txLogger(txSig)
  })
