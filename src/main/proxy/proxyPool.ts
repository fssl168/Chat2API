/**
 * Proxy Health Pool
 * Tracks proxy health, latency, and auto-fails unhealthy proxies.
 * Adapted from distributed-stealth-scraper ProxyPool (MIT licensed).
 *
 * Each proxy starts healthy. After N consecutive failures it is marked
 * unhealthy and excluded from rotation. After recoveryInterval seconds
 * it is re-tested. Latency-weighted round-robin scheduling prefers fast proxies.
 */

export interface ProxyStatus {
  url: string
  healthy: boolean
  lastUsed: number
  failCount: number
  successCount: number
  avgLatencyMs: number
  lastError?: string
}

const MAX_FAILURES = 3
const RECOVERY_INTERVAL_MS = 5 * 60 * 1000 // 5 minutes
const ALPHA = 0.3 // exponential moving average smoothing factor

export class ProxyHealthPool {
  private readonly proxies = new Map<string, ProxyStatus>()
  private currentIndex = 0
  private readonly lock = Symbol('lock') // simple sequential access

  constructor(proxies?: string[]) {
    if (proxies) {
      for (const url of proxies) {
        this.proxies.set(url, { url, healthy: true, lastUsed: 0, failCount: 0, successCount: 0, avgLatencyMs: 0 })
      }
    }
  }

  get hasProxies(): boolean {
    return this.proxies.size > 0
  }

  get proxyCount(): number {
    return this.proxies.size
  }

  get healthyCount(): number {
    const now = Date.now()
    let count = 0
    for (const status of Array.from(this.proxies.values())) {
      if (status.healthy || now - status.lastUsed > RECOVERY_INTERVAL_MS) {
        count++
      }
    }
    return count
  }

  /**
   * Get the next available proxy URL. Returns undefined if no proxies configured.
   */
  next(): string | undefined {
    if (this.proxies.size === 0) return undefined

    const now = Date.now()
    const available: ProxyStatus[] = []
    for (const status of Array.from(this.proxies.values())) {
      if (status.healthy) {
        available.push(status)
      } else if (now - status.lastUsed > RECOVERY_INTERVAL_MS) {
        // Recovery trial: reset and include
        status.healthy = true
        status.failCount = 0
        available.push(status)
      }
    }

    if (available.length === 0) return undefined

    // Sort by avgLatencyMs ascending, then pick round-robin
    available.sort((a, b) => a.avgLatencyMs - b.avgLatencyMs)
    const chosen = available[this.currentIndex % available.length]
    this.currentIndex = (this.currentIndex + 1) % available.length
    chosen.lastUsed = now
    return chosen.url
  }

  markSuccess(proxyUrl: string, latencyMs: number): void {
    const status = this.proxies.get(proxyUrl)
    if (!status) return
    status.successCount++
    status.failCount = Math.max(0, status.failCount - 1)
    status.avgLatencyMs = ALPHA * latencyMs + (1 - ALPHA) * status.avgLatencyMs
    status.healthy = true
    status.lastError = undefined
  }

  markFailed(proxyUrl: string, error?: string): void {
    const status = this.proxies.get(proxyUrl)
    if (!status) return
    status.failCount++
    status.lastError = error
    status.lastUsed = Date.now()
    if (status.failCount >= MAX_FAILURES) {
      status.healthy = false
    }
  }

  addProxy(url: string): void {
    if (!this.proxies.has(url)) {
      this.proxies.set(url, { url, healthy: true, lastUsed: 0, failCount: 0, successCount: 0, avgLatencyMs: 0 })
    }
  }

  removeProxy(url: string): void {
    this.proxies.delete(url)
  }

  clear(): void {
    this.proxies.clear()
    this.currentIndex = 0
  }

  /**
   * Stats for monitoring/debugging
   */
  getStats(): Record<string, { healthy: boolean; successes: number; failures: number; avgLatencyMs: number; lastError?: string }> {
    const result: Record<string, any> = {}
    for (const [url, status] of Array.from(this.proxies.entries())) {
      result[url] = {
        healthy: status.healthy,
        successes: status.successCount,
        failures: status.failCount,
        avgLatencyMs: Math.round(status.avgLatencyMs),
        lastError: status.lastError,
      }
    }
    return result
  }
}

// Singleton instance — shared across all adapters
let poolInstance: ProxyHealthPool | null = null

export function getProxyHealthPool(): ProxyHealthPool {
  if (!poolInstance) {
    poolInstance = new ProxyHealthPool()
  }
  return poolInstance
}
