/**
 * SQLite-backed Session Vault
 * Manages domain-scoped cookies and auth tokens across requests.
 * Adapted from distributed-stealth-scraper (MIT licensed).
 *
 * Uses node:sqlite DatabaseSync (synchronous, Electron-compatible).
 */

import { app } from 'electron'
import { join } from 'path'
import { createRequire } from 'module'
const require = createRequire(import.meta.url)

let sqliteMod: typeof import('node:sqlite') | null = null

try {
  sqliteMod = require('node:sqlite') as typeof import('node:sqlite')
} catch {
  // node:sqlite may not be available in some Electron contexts
}

const DatabaseSync = sqliteMod?.DatabaseSync

export interface StoredCookie {
  name: string
  value: string
  path?: string
  expires?: number
  secure?: boolean
  httpOnly?: boolean
}

export interface StoredToken {
  tokenType: string
  value: string
  expires?: number
}

export class SessionVault {
  private db: any = null
  private dbPath: string
  private initialized = false

  constructor() {
    const dataDir = app.getPath('userData')
    this.dbPath = join(dataDir, 'session_vault.db')
  }

  /**
   * Initialize the database connection. Must be called before any operation.
   * Available only when node:sqlite is present (Electron >= 28 + Node 22+).
   */
  init(): void {
    if (this.initialized || !DatabaseSync) return

    try {
      this.db = new DatabaseSync(this.dbPath)
      this.db.exec(`
        CREATE TABLE IF NOT EXISTS cookies (
          domain TEXT NOT NULL,
          name TEXT NOT NULL,
          value TEXT NOT NULL,
          path TEXT DEFAULT '/',
          expires INTEGER,
          secure INTEGER DEFAULT 0,
          http_only INTEGER DEFAULT 0,
          created_at INTEGER DEFAULT (unixepoch()),
          PRIMARY KEY (domain, name)
        )
      `)
      this.db.exec(`
        CREATE TABLE IF NOT EXISTS tokens (
          domain TEXT NOT NULL,
          token_type TEXT NOT NULL,
          token_value TEXT NOT NULL,
          expires INTEGER,
          created_at INTEGER DEFAULT (unixepoch()),
          PRIMARY KEY (domain, token_type)
        )
      `)
      this.db.exec('CREATE INDEX IF NOT EXISTS idx_cookies_domain ON cookies(domain)')
      this.db.exec('CREATE INDEX IF NOT EXISTS idx_tokens_domain ON tokens(domain)')
      this.initialized = true
    } catch (err) {
      console.error('[SessionVault] Init failed:', err)
    }
  }

  /**
   * Get all non-expired cookies for a domain.
   */
  getCookies(domain: string): StoredCookie[] {
    this.init()
    if (!this.db) return []

    const now = Math.floor(Date.now() / 1000)
    try {
      const rows = this.db.prepare(
        `SELECT name, value, path, expires, secure, http_only FROM cookies WHERE domain = ? AND (expires IS NULL OR expires > ?)`
      ).all(domain, now) as Array<{
        name: string
        value: string
        path: string
        expires: number | null
        secure: number
        http_only: number
      }>
      return rows.map((row) => ({
        name: row.name,
        value: row.value,
        path: row.path ?? '/',
        expires: row.expires ?? undefined,
        secure: Boolean(row.secure),
        httpOnly: Boolean(row.http_only),
      }))
    } catch {
      return []
    }
  }

  /**
   * Build a semicolon-separated cookie header string for a domain.
   */
  getCookieHeader(domain: string): string {
    const cookies = this.getCookies(domain)
    if (cookies.length === 0) return ''
    return cookies.map((c) => `${c.name}=${c.value}`).join('; ')
  }

  /**
   * Set a single cookie for a domain.
   */
  setCookie(
    domain: string,
    name: string,
    value: string,
    options: {
      path?: string
      expires?: number
      secure?: boolean
      httpOnly?: boolean
    } = {}
  ): void {
    this.init()
    if (!this.db) return

    try {
      this.db.prepare(`
        INSERT OR REPLACE INTO cookies (domain, name, value, path, expires, secure, http_only)
        VALUES (?, ?, ?, ?, ?, ?, ?)
      `).run(
        domain,
        name,
        value,
        options.path ?? '/',
        options.expires ?? null,
        options.secure ? 1 : 0,
        options.httpOnly ? 1 : 0,
      )
    } catch {
      // Best-effort — don't crash on DB errors
    }
  }

  /**
   * Bulk-set cookies from a record.
   */
  setCookies(domain: string, cookies: Record<string, string>, expires?: number): void {
    for (const [name, value] of Object.entries(cookies)) {
      this.setCookie(domain, name, value, { expires })
    }
  }

  /**
   * Get an auth token for a domain/type combination.
   */
  getToken(domain: string, tokenType: string): StoredToken | null {
    this.init()
    if (!this.db) return null

    const now = Math.floor(Date.now() / 1000)
    try {
      const row = this.db.prepare(
        `SELECT token_value, expires FROM tokens WHERE domain = ? AND token_type = ? AND (expires IS NULL OR expires > ?)`
      ).get(domain, tokenType, now) as { token_value: string; expires: number | null } | undefined

      if (!row) return null
      return {
        tokenType,
        value: row.token_value,
        expires: row.expires ?? undefined,
      }
    } catch {
      return null
    }
  }

  /**
   * Store an auth token.
   */
  setToken(domain: string, tokenType: string, value: string, expires?: number): void {
    this.init()
    if (!this.db) return

    try {
      this.db.prepare(`
        INSERT OR REPLACE INTO tokens (domain, token_type, token_value, expires)
        VALUES (?, ?, ?, ?)
      `).run(domain, tokenType, value, expires ?? null)
    } catch {
      // Best-effort
    }
  }

  /**
   * Clear all stored data for a domain.
   */
  clearDomain(domain: string): void {
    this.init()
    if (!this.db) return

    try {
      this.db.prepare(`DELETE FROM cookies WHERE domain = ?`).run(domain)
      this.db.prepare(`DELETE FROM tokens WHERE domain = ?`).run(domain)
    } catch {
      // Best-effort
    }
  }

  /**
   * Get storage statistics.
   */
  stats(): { cookies: number; tokens: number; domains: string[] } {
    this.init()
    if (!this.db) return { cookies: 0, tokens: 0, domains: [] }

    try {
      const cookieCount = (this.db.prepare('SELECT COUNT(*) AS c FROM cookies').get() as { c: number }).c
      const tokenCount = (this.db.prepare('SELECT COUNT(*) AS c FROM tokens').get() as { c: number }).c
      const rows = this.db.prepare(
        `SELECT DISTINCT domain FROM cookies UNION SELECT DISTINCT domain FROM tokens`
      ).all() as Array<{ domain: string }>

      return {
        cookies: cookieCount,
        tokens: tokenCount,
        domains: rows.map((r) => r.domain),
      }
    } catch {
      return { cookies: 0, tokens: 0, domains: [] }
    }
  }

  /**
   * Close the database connection. Call on app shutdown.
   */
  close(): void {
    if (this.db) {
      try { this.db.close() } catch {}
      this.db = null
      this.initialized = false
    }
  }
}

// Singleton instance
let vaultInstance: SessionVault | null = null

export function getSessionVault(): SessionVault {
  if (!vaultInstance) {
    vaultInstance = new SessionVault()
  }
  return vaultInstance
}
