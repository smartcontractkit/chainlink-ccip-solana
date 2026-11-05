import { Command } from 'commander'
import setup from '../../config/index.ts'

export default new Command()
  .description('Initialize the test_ccip_receiver program')
  .action(async function (this: Command) {
    const {
      programs: { receiver, router },
      txLogger,
    } = await setup(this)

    if (!receiver || !router) {
      console.error('Missing required program(s). Please check your configuration.')
      process.exit(1)
    }

    const routerAddress = router.programId

    console.debug(`\nInitializing test_ccip_receiver with router: ${routerAddress.toBase58()}...`)

    const txSig = await receiver.methods.initialize(routerAddress).rpc()

    txLogger(txSig)
  })
