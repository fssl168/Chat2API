/**
 * Proxy-aware request utilities
 * Wraps axios/curl-cffi requests with ProxyHealthPool integration.
 * Extracts proxy URL from account credentials and auto-tracks health.
 */

import { getProxyHealthPool } from '../proxyPool'
import type { Account } from '../../store/types'

const PROXY_CREDENTIAL_KEYS = ['proxy', 'proxyUrl', 'proxy_url', 'http_proxy', 'https_proxy'] as const

/**
 * Extract proxy URL string from account credentials.
 * Returns undefined if no proxy configured.
 */
export function extractProxyUrl(account: Account): string | undefined {
  if (!account?.credentials) return undefined
  for (const key of PROXY_CREDENTIAL_KEYS) {
    const val = account.credentials[key]
    if (val && typeof val === 'string' && val.startsWith('http')) {
      return val.trim()
    }
  }
  return undefined
}

/**
 * Wrap an async request with proxy selection + health tracking.
 * - If account has proxy config: selects from pool, injects into options
 * - On success: marks proxy healthy with latency
 * - On failure: marks proxy unhealthy
 * - Returns undefined result on exhaustion (no healthy proxies left)
 */
export async function withProxy<T>(
  account: Account,
  fn: (proxyUrl?: string) => Promise<T>,
  label: string = 'request'
): Promise<T | undefined> {
  const proxyUrl = extractProxyUrl(account)
  if (!proxyUrl) {
    // No proxy configured — run directly
    try {
      return await fn(undefined)
    } catch (e) {
      throw e
    }
  }

  const pool = getProxyHealthPool()

  // Ensure pool knows about this proxy
  pool.addProxy(proxyUrl)

  const selected = pool.next()
  if (!selected) {
    console.warn(`[${label}] All proxies exhausted for ${account.id}`)
    return undefined
  }

  const start = Date.now()
  try {
    const result = await fn(selected)
    const latency = Date.now() - start
    pool.markSuccess(selected, latency)
    return result
  } catch (e) {
    const latency = Date.now() - start
    const msg = e instanceof Error ? e.message : String(e)
    pool.markFailed(selected, msg)
    throw e
  }
}
