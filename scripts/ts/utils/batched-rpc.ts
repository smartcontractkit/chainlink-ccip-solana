import { Connection, PublicKey } from '@solana/web3.js'
import type { AccountInfo } from '@solana/web3.js'

/**
 * `getMultipleAccountsInfo` is capped at 100 keys per call by the JSON-RPC
 * limit; this helper chunks the input and stitches results back in order.
 */
export async function getMultipleAccountsBatched(
  connection: Connection,
  addresses: PublicKey[],
  options: { batchSize?: number } = {}
): Promise<(AccountInfo<Buffer> | null)[]> {
  const batchSize = options.batchSize ?? 100
  const out: (AccountInfo<Buffer> | null)[] = []
  for (let i = 0; i < addresses.length; i += batchSize) {
    const chunk = addresses.slice(i, i + batchSize)
    const result = await withRetry(() => connection.getMultipleAccountsInfo(chunk))
    out.push(...result)
  }
  return out
}

/**
 * Exponential backoff for transient RPC failures (rate limits, network blips).
 * Retries on HTTP 429, JSON-RPC -32005 (rate limit), and matching error messages.
 */
export async function withRetry<T>(
  fn: () => Promise<T>,
  options: { maxAttempts?: number; baseDelayMs?: number } = {}
): Promise<T> {
  const maxAttempts = options.maxAttempts ?? 5
  const base = options.baseDelayMs ?? 250
  let attempt = 0
  while (true) {
    try {
      return await fn()
    } catch (err: any) {
      attempt++
      if (attempt >= maxAttempts || !isRetryable(err)) throw err
      const delay = base * 2 ** (attempt - 1)
      await sleep(delay)
    }
  }
}

function isRetryable(err: any): boolean {
  const msg = String(err?.message ?? err)
  if (/429/.test(msg)) return true
  if (/rate limit/i.test(msg)) return true
  if (/-32005/.test(msg)) return true
  if (/timeout|ETIMEDOUT|ECONNRESET|EAI_AGAIN/i.test(msg)) return true
  return false
}

function sleep(ms: number): Promise<void> {
  return new Promise((r) => setTimeout(r, ms))
}
