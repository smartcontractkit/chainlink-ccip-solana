/**
 * Known CCIP destination chain selectors for mainnet. The on-chain ChainConfig
 * account does NOT store the selector — it lives only in the PDA seed — so
 * commands that show "which chain is this row for" must reference a table.
 *
 * Ported from scripts/debug/1_6/tokens/check_token_pool_rate_limits.py (Jun 2026).
 * Source of truth: chain-selectors.yml in smartcontractkit/chain-selectors.
 *
 * Add new selectors here as CCIP onboards new chains.
 */
export const CCIP_CHAIN_SELECTORS_MAINNET: Record<string, string> = {
  '5009297550715157269': 'ethereum-mainnet',
  '4949039107694359620': 'arbitrum-mainnet',
  '3734403246176062136': 'optimism-mainnet',
  '15971525489660198786': 'base-mainnet',
  '4051577828743386545': 'polygon-mainnet',
  '6433500567565415381': 'avalanche-mainnet',
  '11344663589394136015': 'bsc-mainnet',
  '124615329519749607': 'solana-mainnet',
  '5142893604156789321': 'wemix-mainnet',
  '1346049177634351622': 'celo-mainnet',
  '3719320017875267166': 'kroma-mainnet',
  '465200170687744372': 'gnosis-mainnet',
  '4411394078118774322': 'blast-mainnet',
  '1562403441176082196': 'zksync-mainnet',
  '4627098889531055414': 'linea-mainnet',
  '13204309965629103672': 'scroll-mainnet',
  '1556008542357238666': 'mantle-mainnet',
  '8805746078405598895': 'metis-mainnet',
  '7264351850409363825': 'mode-mainnet',
  '1673871237479749969': 'sonic-mainnet',
  '12505351618335765396': 'soneium-mainnet',
  '4348158687435793198': 'polygon-zkevm-mainnet',
  '465944652040885897': 'opbnb-mainnet',
  '16468599424800719238': 'taiko-mainnet',
  '1923510103922296319': 'unichain-mainnet',
  '2049429975587534727': 'worldchain-mainnet',
  '9027416829622342829': 'sei-mainnet',
  '1294465214383781161': 'berachain-mainnet',
  '17912061998839310979': 'plume-mainnet',
  '3461204551265785888': 'ink-mainnet',
}

/** Lookup helper — returns a `"<selector> (<name>)"` label or just the selector. */
export function labelChainSelector(selector: bigint | number | string): string {
  const k = String(selector)
  const name = CCIP_CHAIN_SELECTORS_MAINNET[k]
  return name ? `${k} (${name})` : k
}
