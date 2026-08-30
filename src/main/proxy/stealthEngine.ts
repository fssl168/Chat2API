/**
 * Playwright Stealth Browser Engine
 * Heavy-tier browser automation with anti-detection patches.
 * Adapted from distributed-stealth-scraper (MIT licensed).
 *
 * This engine uses playwright-core (no full Playwright install required)
 * to launch a headless Chromium instance with stealth modifications:
 *   - navigator.webdriver = undefined
 *   - Canvas getImageData noise injection
 *   - WebGL renderer spoofing
 *   - AutomationControlled Blink feature disabled
 *   - Persistent session vault cookie sync
 */

import { createRequire } from 'module'
const require = createRequire(import.meta.url)
const playwrightCore = require('playwright-core') as typeof import('playwright-core')
const { chromium } = playwrightCore

import type { Browser, BrowserContext, Page } from 'playwright-core'
import { getSessionVault } from './sessionVault'
import { detectChallenge, ChallengeType } from './challengeDetector'

const STEALTH_SCRIPT = `
(() => {
  // 1. Hide webdriver flag
  Object.defineProperty(navigator, 'webdriver', {
    get: () => undefined,
    configurable: true,
  });

  // 2. Patch Chrome runtime version to avoid detection
  if (navigator.userAgentData) {
    Object.defineProperty(navigator, 'userAgentData', {
      get() {
        return {
          ...this,
          brands: [
            { brand: 'Chromium', version: '120.0.0.0' },
            { brand: 'Not_A Brand', version: '24.0.0.0' },
          ],
          platforms: ['Windows'],
          getHighEntropyValues() {
            return Promise.resolve({
              ...this.getHighEntropyValues(),
              platformVersion: '10.0.0',
              architecture: 'x86',
            });
          },
        };
      },
    });
  }

  // 3. Canvas fingerprint noise
  const origGetImageData = CanvasRenderingContext2D.prototype.getImageData;
  CanvasRenderingContext2D.prototype.getImageData = function(...args) {
    const imageData = origGetImageData.apply(this, args);
    const data = imageData.data;
    for (let i = 0; i < data.length; i += 4) {
      data[i]     = (data[i]     + Math.random() * 0.5) % 256;
      data[i + 1] = (data[i + 1] + Math.random() * 0.5) % 256;
      data[i + 2] = (data[i + 2] + Math.random() * 0.5) % 256;
    }
    return imageData;
  };

  // 4. WebGL vendor/renderer spoof
  const origGetParameter = WebGLRenderingContext.prototype.getParameter;
  WebGLRenderingContext.prototype.getParameter = function(parameter) {
    if (parameter === 37445) return 'Intel Inc.';
    if (parameter === 37446) return 'Intel Iris OpenGL Engine';
    return origGetParameter.apply(this, [parameter]);
  };
  const origGetParameterWebGL2 = WebGL2RenderingContext.prototype.getParameter;
  if (origGetParameterWebGL2) {
    WebGL2RenderingContext.prototype.getParameter = function(parameter) {
      if (parameter === 37445) return 'Intel Inc.';
      if (parameter === 37446) return 'Intel Iris OpenGL Engine';
      return origGetParameterWebGL2.apply(this, [parameter]);
    };
  }

  // 5. Permissions API - override to grant geolocation silently
  const origRequestPermission = Permissions.request.bind(Permissions);
  Permissions.request = async function(descriptor) {
    if (descriptor.name === 'geolocation') return 'granted' as PermissionState;
    return origRequestPermission(descriptor);
  };

  // 6. Media devices - hide extra hardware
  const origEnumerateDevices = navigator.mediaDevices.enumerateDevices.bind(navigator.mediaDevices);
  navigator.mediaDevices.enumerateDevices = async function() {
    const devices = await origEnumerateDevices();
    return devices.filter((d) => d.kind !== 'audioinput');
  };
})();
`

const DEFAULT_UA =
  'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36'

export interface StealthConfig {
  userAgent?: string
  headless?: boolean
  viewportWidth?: number
  viewportHeight?: number
  locale?: string
  timezoneId?: string
  timeout?: number  // ms, default 60_000
}

export interface StealthResult {
  status: number
  html: string
  headers: Record<string, string>
  challenge: ChallengeType
}

/**
 * StealthBrowserEngine manages a persistent Chromium context for
 * resolving WAF challenges that curl_cffi cannot bypass.
 */
export class StealthBrowserEngine {
  private browser: Browser | null = null
  private context: BrowserContext | null = null
  private config: Required<StealthConfig>
  private readonly vault

  constructor(config: StealthConfig = {}) {
    this.config = {
      userAgent: config.userAgent ?? DEFAULT_UA,
      headless: config.headless ?? true,
      viewportWidth: config.viewportWidth ?? 1920,
      viewportHeight: config.viewportHeight ?? 1080,
      locale: config.locale ?? 'en-US',
      timezoneId: config.timezoneId ?? 'America/New_York',
      timeout: config.timeout ?? 60_000,
    }
    this.vault = getSessionVault()
  }

  /**
   * Initialize the browser and context. Call once on app startup.
   */
  async initialize(): Promise<void> {
    if (this.browser) return

    const launchArgs = [
      '--no-sandbox',
      '--disable-dev-shm-usage',
      '--disable-blink-features=AutomationControlled',
      '--disable-web-security',
      '--disable-features=IsolateOrigins,site-per-process',
      '--lang=en-US',
    ]

    this.browser = await chromium.launch({
      headless: this.config.headless,
      args: launchArgs,
    })

    this.context = await this.browser.newContext({
      userAgent: this.config.userAgent,
      viewport: {
        width: this.config.viewportWidth,
        height: this.config.viewportHeight,
      },
      locale: this.config.locale,
      timezoneId: this.config.timezoneId,
      permissions: ['geolocation'],
    })

    await this.context.addInitScript(STEALTH_SCRIPT)
  }

  /**
   * Fetch a URL using the stealth browser.
   * Automatically syncs cookies to/from the SessionVault.
   */
  async fetch(url: string): Promise<StealthResult> {
    if (!this.context) throw new Error('StealthEngine not initialized — call initialize() first')

    const domain = new URL(url).hostname
    const page = await this.context.newPage()

    try {
      // Load stored cookies
      const storedCookies = this.vault.getCookies(domain)
      if (storedCookies.length > 0) {
        const pwCookies = storedCookies.map((c) => ({
          name: c.name,
          value: c.value,
          domain: this._normalizeDomain(domain),
          path: c.path ?? '/',
          expires: c.expires
            ? Math.floor(c.expires / 1000)
            : undefined,
          secure: Boolean(c.secure),
          httpOnly: Boolean(c.httpOnly),
        }))
        await this.context.addCookies(pwCookies)
      }

      // Navigate
      const response = await page
        .goto(url, { waitUntil: 'domcontentloaded', timeout: this.config.timeout })
        .catch(async () => {
          // Fallback: don't wait for full load
          return await page.goto(url, { timeout: this.config.timeout })
        })

      // Wait briefly for any JS-based challenges (Cloudflare turnstile etc.)
      await this._waitForChallenge(page)

      const html = await page.content()
      const headers: Record<string, string> = {}
      if (response) {
        for (const [k, v] of Object.entries(response.headers())) {
          headers[k] = v
        }
      }
      const status = response?.status() ?? 200

      // Sync new cookies back to vault
      await this._syncCookies(page, domain)

      // Extract tokens from localStorage
      await this._extractTokens(page, domain)

      const challenge = detectChallenge(html, headers).type
      return { status, html, headers, challenge }
    } finally {
      await page.close()
    }
  }

  /**
   * Destroy the browser instance.
   */
  async close(): Promise<void> {
    if (this.context) {
      await this.context.close().catch(() => {})
      this.context = null
    }
    if (this.browser) {
      await this.browser.close().catch(() => {})
      this.browser = null
    }
  }

  // ---- Private helpers ----

  private _normalizeDomain(domain: string): string {
    return domain.startsWith('.') ? domain : `.${domain}`
  }

  private async _waitForChallenge(page: Page): Promise<void> {
    const selectors = [
      '#cf-challenge-running',
      '.dd-captcha',
      '.px-captcha',
      '#rc-anchor-container',
      '.h-captcha',
      '[data-cf-modified]',
    ]
    const waitMs = Math.min(this.config.timeout / 3, 10_000)
    for (const sel of selectors) {
      try {
        await page.waitForSelector(sel, { timeout: waitMs }).catch(() => null)
        await new Promise((r) => setTimeout(r, 3000))
        break
      } catch {
        continue
      }
    }
    await new Promise((r) => setTimeout(r, 2000))
  }

  private async _syncCookies(page: Page, domain: string): Promise<void> {
    try {
      const pwCookies = await this.context!.cookies(page.url())
      for (const c of pwCookies) {
        this.vault.setCookie(
          c.domain.replace(/^\./, ''),
          c.name,
          c.value,
          {
            path: c.path ?? '/',
            expires: c.expires && c.expires > 0
              ? Math.floor(c.expires * 1000)
              : undefined,
            secure: c.secure,
            httpOnly: c.httpOnly,
          }
        )
      }
    } catch {
      // Non-fatal — cookies may not be accessible cross-domain
    }
  }

  private async _extractTokens(page: Page, domain: string): Promise<void> {
    try {
      const tokens = await page.evaluate(() => {
        const result: Record<string, string> = {}
        for (let i = 0; i < localStorage.length; i++) {
          const key = localStorage.key(i)
          if (key && /token|auth|session/i.test(key)) {
            const val = localStorage.getItem(key)
            if (val) result[key] = val
          }
        }
        return result
      })
      for (const [type, value] of Object.entries(tokens)) {
        if (value) {
          this.vault.setToken(domain, type, value)
        }
      }
    } catch {
      // Non-fatal
    }
  }
}

// Singleton
let engineInstance: StealthBrowserEngine | null = null

export function getStealthEngine(): StealthBrowserEngine {
  if (!engineInstance) {
    engineInstance = new StealthBrowserEngine()
  }
  return engineInstance
}
