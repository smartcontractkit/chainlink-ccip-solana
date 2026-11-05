# CCIP Solana CLI (`scripts/ts`)

TypeScript CLI for inspecting and operating Solana CCIP contracts.
Built on `commander` + Anchor; commands target either CCIP **1.6** (the production version on mainnet) or **2.0** (everywhere else), selected per environment with a per-invocation override.

---

## Setup

```bash
cd scripts/ts
npm install
npm run sync-idls   # first-time only — downloads vendored CCIP 1.6 IDLs
npm run cli -- --help
```

**Requirements:** Node ≥ 24.14 (the CLI ships ESM + experimental TS stripping; no build step).

> The whole `idls/` directory is gitignored. `npm run sync-idls` downloads the
> latest `solana-v1.6.*` tag from `smartcontractkit/chainlink-ccip` and writes
> a local `idls/1_6/.commit` pin; re-running is a no-op until upstream changes.

### Environment configs

Each env lives in `config/envs/<name>.ts` and default-exports a `Config`:

```ts
// config/envs/mainnet.ts
import { PublicKey } from '@solana/web3.js'
import type { Config } from '../index.ts'

const config: Config = {
  rpcUrl: 'https://api.mainnet-beta.solana.com',
  explorerUrl: (sig) => `https://explorer.solana.com/tx/${sig}`,
  walletPath: '~/.config/solana/id.json',
  ccipVersion: '1.6',                       // mainnet default; override with --ccip-version
  programs: {
    router:               new PublicKey('Ccip842gzYHhvdDkSyi2YVCoAWPbYJoApMFzSxQroE9C'),
    feeQuoter:            new PublicKey('FeeQPGkKDeRV1MgoYfMH6L8o3KeuYjwUZrgn4LRKfjHi'),
    burnmintTokenPool:    new PublicKey('41FGToCmdaWa1dgZLKFAjvmx6e6AjVTX7SVRibvsMGVB'),
    lockReleaseTokenPool: new PublicKey('8eqh8wppT9c5rw4ERqNCffvU6cNFJWff9WmkcYtmGiqC'),
    cctpTokenPool:        new PublicKey('CCiTPESGEevd7TBU8EGBKrcxuRq7jx3YtW6tPidnscaZ'),
    offramp:              new PublicKey('off43sBHBsmWTU6VbfwKYCNwyJ4rSDhunYvz3QL7bF9'),
  },
}
export default config
```

Env files are gitignored (`config/envs/*` except `.gitkeep`) — they typically contain wallet paths and per-operator RPC endpoints.

**Convention:** `mainnet.ts` sets `ccipVersion: '1.6'`; every other env (local, devnet, staging) sets `'2.0'`.

### Wallet

Pass via `-k`:

- File path: `-k ~/.config/solana/id.json` (uses Anchor JSON keypair format)
- Base58: `-k <base58 secret key>` (auto-detected by length)

If `-k` is omitted, falls back to `config.walletPath`.

---

## Global flags

| Flag | Description | Default |
|---|---|---|
| `--env <name>` | Env config file name in `config/envs/` (without `.ts`) | `local` |
| `-k, --keypair <path \| base58>` | Solana keypair file or base58 secret key | from config |
| `-q, --quiet` | Suppress `debug`/`trace` logs (keep `info`/`warn`/`error`) | off |
| `--ccip-version <1.6 \| 2.0>` | Override `config.ccipVersion` for this invocation | from config |

---

## Commands

### General

#### `program-info [-w]`
Show last upgrade timestamp and on-chain IDL versions for every program in the active env.
- `-w, --wide` — extra columns (ProgramData address)

#### `type-version <address>`
Simulate the program's `type_version` instruction and print the returned version string.

#### `decode-pda <program> [type] [seeds...]`
Decode any PDA from any configured program's IDL.
- `program` — name as configured (`router`, `feeQuoter`, `burnmintTokenPool`, ...)
- `type` — PDA account type in camelCase (`tokenAdminRegistry`, `state`, ...)
- `seeds...` — `<type>:<value>` pairs (e.g. `string:ccip`, `pubkey:<base58>`, `u64le:42`)
- `-l, --list` — list available account types for the program instead of fetching

#### `encode [input] [-f from] [-t to] [-p pad]`
Convert between `hex`, `base58`, `intarray`, `base64`, `utf8`, `u16/u32/u64 le/be`. Reads from stdin when `input` is omitted; auto-detects `--from` and shows all targets when `--to` is omitted.

#### `receiver initialize`
Initialize the `test_ccip_receiver` program (binds it to the configured router).

#### `receiver toggle-rejection <mode>`
Set the test receiver to accept-all or reject-all (`mode = accept-all | reject-all`).

#### `resize-legacy-idl <program> [--bytes N | --from-idl path] [--dry-run]`
Resize a program's legacy Anchor IDL account (the on-chain `anchor:idl` PDA).

---

### Tokens (`cli tokens <subcommand>`)

All token tools target the active `--ccip-version`. They use the IDL bindings (no hand-rolled byte offsets) and the SPL Token / Metaplex helpers for mint and metadata inspection.

CSV inputs live under `scripts/ts/resources/` (gitignored). Drop your own copies there, or pass `--csv <absolute/path>`.

#### CSV formats

`token-pools.csv` is the canonical input for every command except `gen-token-pool-csv`, which bootstraps it from a `state-pdas.csv`.

| Format | Columns | Header | Used by | Notes |
|---|---|---|---|---|
| **`token-pools.csv`** | `token_mint,token_pool_program,token_pool_pda` | yes | `inspect`, `find-address-roles`, `inspect-custom-pools`, `list-token-pools`, `get-mint-creation-dates` | The canonical file. `token_pool_program` may be `<addr>` or `<addr> (label)`. |
| **`state-pdas.csv`** | `token_pool` (one State PDA per row) | yes | `gen-token-pool-csv` (input only) | Bootstrap input. Required header `token_pool`. |

#### `tokens inspect [--state <pda> | --mint <addr> --csv <path> | --csv <path>]`
Inspect token pool(s). Three input modes:
- `--state <pda>` — single State PDA, fully resolved on-chain (program from account owner, no CSV needed). Detailed view: mint, mint authority + multisig, Metaplex metadata, owner/proposed owner/rate-limit admin, TokenAdminRegistry.
- `--mint <addr> --csv <path>` — look up every pool listed for that mint in `token-pools.csv`, derive each State PDA from `(ccip_tokenpool_config, mint)` against the row's pool program, then run the detailed view for each match.
- `--csv <path>` alone — batch mode over every entry's `token_pool_pda`, compact one-block-per-pool output.

#### `tokens find-address-roles --csv <path> --address <addr>`
Search all token pools in `token-pools.csv` for `<addr>` appearing as owner, proposed owner, rate-limit admin, TokenAdminRegistry administrator, or TAR pending admin. Prints only matching entries.

#### `tokens inspect-custom-pools --csv <path> [--all] [-o <file>]`
List rows from `token-pools.csv` for *custom* (non-burnmint, non-lockrelease, non-cctp) pool programs, enriched with Metaplex token symbols.
- `--all` — include known programs too.
- `-o, --output <file>` — write CSV to file (default: stdout).

#### `tokens gen-token-pool-csv --input <path> --output <path>`
Bootstrap a `token-pools.csv` from a `state-pdas.csv` (single column, header `token_pool`). For each PDA, fetches the State account, reads the mint, looks up the owner program, and writes a sorted CSV with columns `token_mint, token_pool_program, token_pool_pda`. This is how you first produce the `token-pools.csv` that every other command consumes.

#### `tokens list-token-pools --csv <path>`
Call `getProgramAccounts` for each unique pool program found in `token-pools.csv`, decode every State PDA the program owns, and print `pool_pda  mint=...  SYM (NAME)` grouped by program.

#### `tokens get-mint-creation-dates --csv <path>`
Enrich a CSV (must include `token_mint`) in place with a `mint_created_at` column derived from the earliest `getSignaturesForAddress` for each mint.

#### `tokens check-fee-quoter-billing-tokens [--mints <csv>]`
Decode the FeeQuoter `Config` PDA and the `BillingTokenConfigWrapper` for each mint (defaults to LINK SPL + Token-2022 LINK + WSOL). Reports if any billing config is missing, disabled, or has a mismatched inner mint.

---

## IDLs

| Version | Source | Loaded from |
|---|---|---|
| 2.0 | Local Anchor build | `contracts/target/idl/*.json` + `contracts/target/types/*.ts` |
| 1.6 | Downloaded on demand | `scripts/ts/idls/1_6/*.json` (entire directory is gitignored) |

### Fetching / refreshing 1.6 IDLs

```bash
npm run sync-idls
```

Looks up the latest `solana-v1.6.*` tag from `smartcontractkit/chainlink-ccip`, downloads the matching IDL JSONs into `idls/1_6/`, and writes the resolved SHA to `idls/1_6/.commit`. Re-running is a no-op until upstream cuts a new tag. The pin file is local-only — fresh clones must run `sync-idls` once before the CLI can target 1.6.

The 1.6 IDLs are Anchor 0.29 format (no top-level `metadata`/`address`, accounts use `type` not `discriminator`). `config/index.ts` merges shared types from `base_token_pool` and `ccip_common` into each consumer at load time so Anchor 0.29's `BorshAccountsCoder` can resolve cross-IDL `defined:` references.

---

## Adding a new command

1. Create `commands/<name>.ts` (or `commands/<group>/<name>.ts` with a sibling `index.ts` for the namespace).
2. `export default new Command().description('...').action(async function (this) { ... })`.
3. In the action: `const ctx = await setup(this)` to get `{ ccipVersion, programs, legacyPrograms, provider, config, txLogger }`.
4. PDA derivation and account decoding: use the facades in `commands/decode-pda.ts` — `derivePda(ctx, programKey, seeds)`, `decodeAccount(ctx, programKey, accountName, address)`, `findAllAccounts(ctx, programKey, accountName, filters?)`. The facades dispatch between 1.6 (`@coral-xyz/anchor` 0.29) and 2.0 (`@anchor-lang/core` 1.0) automatically.
5. PDA seed constants: re-use the maps in `decode-pda.ts` (`tokenPoolSeeds`, `feeQuoterSeeds`, `routerSeeds`); add new ones there rather than scattering byte literals across commands.
6. Output: `console.info` for results, `chalk` for accents, `cli-table3` for tables. Never `console.log` (it's remapped to `info` and silenced by `--quiet`).
