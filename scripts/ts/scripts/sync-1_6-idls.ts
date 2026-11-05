#!/usr/bin/env node
/**
 * Syncs vendored CCIP v1.6 IDLs from smartcontractkit/chainlink-ccip.
 *
 *   - Reads the currently pinned commit from `idls/1_6/.commit`.
 *   - Looks up the latest `solana-v1.6.*` tag on GitHub.
 *   - If unchanged: prints a message and exits 0.
 *   - If changed: downloads each tracked IDL from raw.githubusercontent.com at the new SHA,
 *     writes the updated `.commit`, and prints a summary.
 *
 * Run: `npm run sync-idls`
 */

import fs from 'node:fs'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

const __dirname = path.dirname(fileURLToPath(import.meta.url))
const IDL_DIR = path.resolve(__dirname, '../idls/1_6')
const COMMIT_FILE = path.join(IDL_DIR, '.commit')

const REPO = 'smartcontractkit/chainlink-ccip'
// We pull the Anchor-generated typed bindings (target/types/*.ts) rather than
// the raw IDL JSONs. Each typed module exports both `type X` (for IdlAccounts<>
// / IdlTypes<> at compile time) and `const IDL` (for `new Program(IDL, ...)`
// at runtime), so a single import covers both needs.
const REMOTE_TYPES_PATH = 'chains/solana/contracts/target/types'

// IDLs we vendor. If you add a new file under idls/1_6/, add its base name here.
const TRACKED = [
  'ccip_router',
  'fee_quoter',
  'burnmint_token_pool',
  'lockrelease_token_pool',
  'cctp_token_pool',
  'base_token_pool',
  'ccip_common',
  'ccip_offramp',
  'rmn_remote',
]

type GhTag = { name: string; commit: { sha: string } }

async function latestV1_6Tag(): Promise<{ tag: string; sha: string }> {
  // Page 1 is enough — tags returns most recent first by default isn't guaranteed,
  // so we filter by pattern and sort by semver ourselves.
  const res = await fetch(`https://api.github.com/repos/${REPO}/tags?per_page=100`, {
    headers: { 'User-Agent': 'chainlink-ccip-solana-sync', Accept: 'application/vnd.github+json' },
  })
  if (!res.ok) throw new Error(`GitHub tags fetch failed: ${res.status} ${res.statusText}`)
  const tags = (await res.json()) as GhTag[]
  const matches = tags
    .filter((t) => /^solana-v1\.6\.\d+$/.test(t.name))
    .sort((a, b) => {
      const av = parseInt(a.name.split('.').pop()!, 10)
      const bv = parseInt(b.name.split('.').pop()!, 10)
      return bv - av
    })
  if (matches.length === 0) throw new Error(`No solana-v1.6.* tag found in ${REPO}`)
  return { tag: matches[0].name, sha: matches[0].commit.sha }
}

async function downloadIdl(sha: string, name: string): Promise<void> {
  const url = `https://raw.githubusercontent.com/${REPO}/${sha}/${REMOTE_TYPES_PATH}/${name}.ts`
  const res = await fetch(url)
  if (!res.ok) throw new Error(`Download failed for ${name}: ${res.status} ${res.statusText}`)
  const body = await res.text()
  // Sanity-check the upstream binding still has the dual `type` + `const IDL` shape we depend on.
  if (!/^export type \w+ = \{/m.test(body) || !/^export const IDL: \w+ = \{/m.test(body)) {
    throw new Error(
      `${name}.ts does not look like an Anchor typed-IDL binding ` +
        `(missing "export type" or "export const IDL"); upstream layout may have changed`
    )
  }
  fs.writeFileSync(path.join(IDL_DIR, `${name}.ts`), body)
}

async function main() {
  const pinned = fs.existsSync(COMMIT_FILE) ? fs.readFileSync(COMMIT_FILE, 'utf8').trim() : ''
  const { tag, sha } = await latestV1_6Tag()

  if (pinned === sha) {
    console.log(`v1.6 IDLs already up-to-date (pinned to ${tag} @ ${sha.slice(0, 7)})`)
    return
  }

  console.log(`Updating v1.6 IDLs: ${pinned ? pinned.slice(0, 7) : '(none)'} -> ${tag} @ ${sha.slice(0, 7)}`)
  for (const name of TRACKED) {
    process.stdout.write(`  ${name}.ts ... `)
    await downloadIdl(sha, name)
    console.log('ok')
  }

  fs.writeFileSync(COMMIT_FILE, sha + '\n')
  console.log(`Wrote ${COMMIT_FILE} = ${sha}`)
}

main().catch((err) => {
  console.error('sync-1_6-idls failed:', err.message)
  process.exit(1)
})
