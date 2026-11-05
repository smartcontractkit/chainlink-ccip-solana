import fs from 'fs'
import path from 'path'
import { pathToFileURL } from 'url'
import { Command } from 'commander'

const program = new Command()
  .name('cli')
  .description('CCIP Solana scripts')
  .option('--env <name>', 'env config (file in ./config/envs, without the .ts extension)', 'local')
  .option('-k, --keypair <path|base58>', 'path to a Solana wallet keypair file, or the base58 privatekey directly (overrides config)')
  .option('-q, --quiet', 'suppress logs and only print the final output (if any)')
  .option('--ccip-version <version>', 'CCIP contract version to target: 1.6 or 2.0 (overrides config)')

/**
 * Discovers and loads commands from the `commands` directory, recursively.
 * Each command is named after its filename, except for `index.ts` files which are treated as namespaces and take the name of their parent directory.
 * For example:
 * - `commands/receiver/initialize.ts` becomes `receiver initialize`
 * - `commands/receiver/toggle-rejection.ts` becomes `receiver toggle-rejection`
 * - `commands/receiver/index.ts` becomes the `receiver` namespace command, which doesn't do anything itself but serves as a parent for the other two commands.
 * - `commands/encode/index.ts` becomes the `encode` namespace command, and so on.
 * This allows for organizing commands in a hierarchical structure using directories, while keeping the command definitions modular and easy to manage.
 * The commands are expected to be the default export of their respective files, and must be instances of `Command` from the `commander` library.
 */

const commandsDir = path.resolve(import.meta.dirname, 'commands')

const commandsMap: Record<string, Command> = {
  [commandsDir]: program,
}

// Sort so that each directory's `index.ts` (the namespace marker) is processed
// before any of its siblings — otherwise sibling commands look up an unregistered parent.
const files = (fs.readdirSync(commandsDir, { recursive: true }) as string[]).map(String).sort((a, b) => {
  const aIsIndex = a === 'index.ts' || a.endsWith('/index.ts')
  const bIsIndex = b === 'index.ts' || b.endsWith('/index.ts')
  if (aIsIndex !== bIsIndex) return aIsIndex ? -1 : 1
  return a.localeCompare(b)
})

for (const file of files) {
  const filename = file.toString()
  if (!filename.endsWith('.ts')) {
    continue
  }

  const filepath = path.join(commandsDir, filename)
  const href = pathToFileURL(filepath).href
  const mod = await import(href)
  if (!(mod.default instanceof Command)) {
    continue
  }

  // drop extension, and take `index.ts` to represent the folder
  const commandPath = filepath.replace(/((\/index)?\.ts)$/, '').split('/')

  // the filename identifies the command, except for `index.ts` files which are considered namespaces and
  // take the name of their parent directory
  const commandName = commandPath[commandPath.length - 1]

  const key = commandPath.join('/')
  const parentKey = commandPath.slice(0, commandPath.length - 1).join('/')

  // Register the command in the program, correctly nested, and in the cached commands map
  // for later reference by child commands
  const cmd = mod.default.name(commandName)
  commandsMap[key] = cmd
  commandsMap[parentKey].addCommand(cmd)
}

await program.parseAsync(process.argv)
