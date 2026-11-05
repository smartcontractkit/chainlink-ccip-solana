import { Command } from 'commander'
import chalk from 'chalk'
import { findCodec, detectCodec, codecs, codecNames } from './codecs.ts'
import { setupWithoutConfig } from '../../config/index.ts'
import { text } from 'node:stream/consumers'

export default new Command()
  .description('Encode/decode between hex, base58, intarrays, base64, and other formats')
  .argument('[input]', 'value to encode/decode (reads from stdin if omitted)')
  .option('-f, --from <format>', `source format (${codecNames}). Auto-detected if omitted`)
  .option('-t, --to <format>', `target format (${codecNames}). Shows all if omitted`)
  .option('-p, --pad <bytes>', 'left-pad with zeroes to reach this many bytes')
  .addHelpText(
    'after',
    `
Examples:
  encode 48656c6c6f
  encode 0x48656c6c6f --to base58
  encode --from base64 --to hex SGVsbG8=
  encode "[72,101,108,108,111]" --to utf8
  echo SGVsbG8= | encode --from base64
  encode 0x1234abcd --pad 32 --to base58   # pad 4 bytes to 32 bytes`
  )
  .action(async function (this: Command, inputArg: string | undefined) {
    setupWithoutConfig(this)

    let input = inputArg
    if (!input) {
      if (process.stdin.isTTY) {
        throw new Error('No input provided. Please provide a value as an argument or via stdin.')
      }
      input = (await text(process.stdin)).trim()
    }

    const opts = this.opts()
    const fromCodec = opts.from ? findCodec(opts.from) : detectCodec(input)
    let bytes = fromCodec.decode(input.trim())

    if (opts.pad) {
      const targetLen = parseInt(opts.pad, 10)
      if (isNaN(targetLen) || targetLen <= 0) {
        throw new Error('--pad must be a positive integer (number of bytes)')
      }
      if (bytes.length > targetLen) {
        throw new Error(`Input is ${bytes.length} bytes, cannot pad to ${targetLen}`)
      }
      const padded = Buffer.alloc(targetLen)
      bytes.copy(padded, targetLen - bytes.length)
      bytes = padded
    }

    if (opts.to) {
      console.log(findCodec(opts.to).encode(bytes))
    } else {
      const maxLen = Math.max(...codecs.map((c) => c.name.length))
      console.log()
      for (const codec of codecs) {
        if (codec.shouldDisplay && !codec.shouldDisplay(bytes)) continue

        const isInput = codec === fromCodec
        const encoded = codec.encode(bytes)
        const label = codec.name.padEnd(maxLen + 2)

        if (isInput) {
          console.log(`  ${chalk.green('▶')} ${chalk.bold.yellow(label)} ${chalk.dim(encoded)}`)
        } else {
          console.log(`    ${chalk.yellow(label)} ${encoded}`)
        }
      }
      console.log()
    }
  })
