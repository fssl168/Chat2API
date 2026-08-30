/**
 * User-Agent Rotation Pool
 * Provides randomized browser profiles for HTTP requests, reducing fingerprint detectability.
 * Each profile is a self-consistent set of headers (UA + Sec-Ch-Ua + platform)
 * matching a real browser installation.
 */

export interface UaProfile {
  /** Platform label for logging/debugging */
  platform: 'windows' | 'macos' | 'linux'
  userAgent: string
  secChUa: string
  secChUaMobile: string
  secChUaPlatform: string
}

const PROFILES: UaProfile[] = [
  // Chrome / Windows
  {
    platform: 'windows',
    userAgent: 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36',
    secChUa: '"Google Chrome";v="131", "Chromium";v="131", "Not_A Brand";v="24"',
    secChUaMobile: '?0',
    secChUaPlatform: '"Windows"',
  },
  // Chrome / Windows (newer)
  {
    platform: 'windows',
    userAgent: 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/143.0.0.0 Safari/537.36 Edg/143.0.0.0',
    secChUa: '"Microsoft Edge";v="143", "Chromium";v="143", "Not A(Brand";v="24"',
    secChUaMobile: '?0',
    secChUaPlatform: '"Windows"',
  },
  // Chrome / macOS
  {
    platform: 'macos',
    userAgent: 'Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/148.0.0.0 Safari/537.36',
    secChUa: '"Not/A)Brand";v="99", "Chromium";v="148"',
    secChUaMobile: '?0',
    secChUaPlatform: '"macOS"',
  },
  // Chrome / macOS (older)
  {
    platform: 'macos',
    userAgent: 'Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/134.0.0.0 Safari/537.36',
    secChUa: '"Chromium";v="134", "Not:A-Brand";v="24", "Google Chrome";v="134"',
    secChUaMobile: '?0',
    secChUaPlatform: '"macOS"',
  },
  // Chrome / Linux
  {
    platform: 'linux',
    userAgent: 'Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36',
    secChUa: '"Chromium";v="131", "Not:A-Brand";v="99", "Google Chrome";v="131"',
    secChUaMobile: '?0',
    secChUaPlatform: '"Linux"',
  },
  // Firefox / Windows
  {
    platform: 'windows',
    userAgent: 'Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:135.0) Gecko/20100101 Firefox/135.0',
    secChUa: '"Firefox";v="135", "Chromium";v="135"',
    secChUaMobile: '?0',
    secChUaPlatform: '"Windows"',
  },
  // Safari / macOS
  {
    platform: 'macos',
    userAgent: 'Mozilla/5.0 (Macintosh; Intel Mac OS X 14_5) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.5 Safari/605.1.15',
    secChUa: '"Safari";v="17", "WebKit";v="17"',
    secChUaMobile: '?0',
    secChUaPlatform: '"macOS"',
  },
]

let _index = Math.floor(Math.random() * PROFILES.length)

/**
 * Return the next UaProfile using round-robin with random seed.
 * Call this once per adapter instance or per request batch.
 */
export function nextUaProfile(): UaProfile {
  const profile = PROFILES[_index]
  _index = (_index + 1) % PROFILES.length
  return profile
}

/**
 * Return a random UaProfile (pure randomness, no rotation).
 * Better for per-request randomness across parallel requests.
 */
export function randomUaProfile(): UaProfile {
  const i = Math.floor(Math.random() * PROFILES.length)
  return PROFILES[i]
}
