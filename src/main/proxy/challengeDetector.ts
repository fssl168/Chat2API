/**
 * WAF Challenge Detection Module
 * Detects various web application firewall challenges in HTTP responses.
 * Adapted from distributed-stealth-scraper (MIT licensed).
 */

import { createRequire } from 'module'
const require = createRequire(import.meta.url)

// Regex patterns compiled once for performance
const PATTERNS = {
  cloudflare: [
    /just a moment/i,
    /checking your browser/i,
    /cf-ray/i,
    /__cf_bm/i,
    /cf-challenge/i,
    /turnstile/i,
    /jschl-answer/i,
    /cftoken/i,
  ],
  datadome: [
    /datadome/i,
    /dd-captcha/i,
    /captcha-delivery/i,
  ],
  perimeterx: [
    /perimeterx/i,
    /px-captcha/i,
    /pxblocking/i,
  ],
  recaptcha: [
    /g-recaptcha/i,
    /recaptcha/i,
  ],
  hcaptcha: [
    /h-captcha/i,
    /hcaptcha/i,
  ],
  akamai: [
    /akamai/i,
    /ak_bmsc/i,
  ],
  imperva: [
    /imperva/i,
    /incapsula/i,
    /visid_incap/i,
  ],
} as const

export enum ChallengeType {
  NONE = 0,
  CLOUDFLARE = 1,
  DATADOME = 2,
  PERIMETERX = 3,
  RECAPTCHA = 4,
  HCAPTCHA = 5,
  AKAMAI = 6,
  IMPERVA = 7,
  UNKNOWN = 99,
}

export interface ChallengeResult {
  type: ChallengeType
  details?: string
}

/**
 * Detect WAF challenges from response body and headers.
 * Header checks take priority over body checks to reduce false positives.
 */
export function detectChallenge(
  html: string,
  headers: Record<string, string>
): ChallengeResult {
  const result: ChallengeResult = { type: ChallengeType.NONE }
  const htmlLower = html.toLowerCase()
  const headersLower: Record<string, string> = {}
  for (const [k, v] of Object.entries(headers)) {
    headersLower[k.toLowerCase()] = v.toLowerCase()
  }

  // Priority 1: Header-based detection (most reliable)
  if ('cf-ray' in headersLower || '__cf_bm' in headersLower) {
    return { type: ChallengeType.CLOUDFLARE, details: 'Detected via CF headers' }
  }
  if ('x-datadome' in headersLower) {
    return { type: ChallengeType.DATADOME, details: 'Detected via DataDome header' }
  }
  if ('x-perimeterx' in headersLower) {
    return { type: ChallengeType.PERIMETERX, details: 'Detected via PerimeterX header' }
  }
  if ('akamai-origin-cache' in headersLower || 'ak-bmsc' in headersLower) {
    return { type: ChallengeType.AKAMAI, details: 'Detected via Akamai header' }
  }
  if ('visid_incap' in headersLower || 'incapsula' in headersLower) {
    return { type: ChallengeType.IMPERVA, details: 'Detected via Incapsula header' }
  }

  // Priority 2: Body-based detection
  for (const [typeStr, regexes] of Object.entries(PATTERNS)) {
    for (const re of regexes) {
      if (re.test(htmlLower)) {
        const type = mapPatternName(typeStr)
        if (type !== ChallengeType.NONE) {
          return { type, details: `Matched pattern: ${re.source}` }
        }
      }
    }
  }

  // Check status codes that commonly indicate challenges
  // (passed separately — callers should pass status if available)
  return result
}

/**
 * Quick check: just headers (no body needed).
 * Useful for fast pre-flight detection before downloading full response.
 */
export function detectChallengeFromHeaders(headers: Record<string, string>): ChallengeResult {
  const hl: Record<string, string> = {}
  for (const [k, v] of Object.entries(headers)) {
    hl[k.toLowerCase()] = v.toLowerCase()
  }

  if ('cf-ray' in hl || '__cf_bm' in hl) return { type: ChallengeType.CLOUDFLARE, details: 'CF header' }
  if ('x-datadome' in hl) return { type: ChallengeType.DATADOME, details: 'DataDome header' }
  if ('x-perimeterx' in hl) return { type: ChallengeType.PERIMETERX, details: 'PerimeterX header' }
  if ('akamai-origin-cache' in hl || 'ak-bmsc' in hl) return { type: ChallengeType.AKAMAI, details: 'Akamai header' }
  if ('visid_incap' in hl) return { type: ChallengeType.IMPERVA, details: 'Incapsula header' }

  return { type: ChallengeType.NONE }
}

function mapPatternName(name: string): ChallengeType {
  switch (name) {
    case 'cloudflare': return ChallengeType.CLOUDFLARE
    case 'datadome': return ChallengeType.DATADOME
    case 'perimeterx': return ChallengeType.PERIMETERX
    case 'recaptcha': return ChallengeType.RECAPTCHA
    case 'hcaptcha': return ChallengeType.HCAPTCHA
    case 'akamai': return ChallengeType.AKAMAI
    case 'imperva': return ChallengeType.IMPERVA
    default: return ChallengeType.UNKNOWN
  }
}
