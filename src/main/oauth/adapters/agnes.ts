/**
 * Agnes Auth Adapter
 * Authentication: Login to https://app.agnes-ai.com/login (AgnesCode app)
 * Use BFF mode: local gateway handles code exchange and JWT persistence
 */

import { shell } from 'electron'
import axios from 'axios'
import { BaseOAuthAdapter } from './base'
import {
  OAuthResult,
  OAuthOptions,
  TokenValidationResult,
  AdapterConfig,
} from '../types'

const AGNES_LOGIN_URL = 'https://app.agnes-ai.com/login'
const AGNES_API_HOST = 'api-agnes-code.agnes-ai.com'
const GATEWAY_BASE = 'http://127.0.0.1:8787'

export class AgnesOAuthAdapter extends BaseOAuthAdapter {
  constructor(config: AdapterConfig) {
    super({
      ...config,
      providerType: 'agnes',
      authMethods: ['manual', 'oauth'],
      loginUrl: AGNES_LOGIN_URL,
      apiUrl: `https://${AGNES_API_HOST}`,
    })
  }

  /**
   * Start login flow - Open Agnes login page in browser
   */
  async startLogin(options: OAuthOptions): Promise<OAuthResult> {
    this.emitProgress('pending', 'Opening Agnes login page...')
    try {
      await shell.openExternal(AGNES_LOGIN_URL)
      this.emitProgress('pending', 'Please complete login at app.agnes-ai.com, then click OK when done')
      return {
        success: false,
        providerId: options.providerId,
        providerType: 'agnes',
        error: 'Please log in at https://app.agnes-ai.com/login, then click OK',
      }
    } catch (error) {
      return {
        success: false,
        providerId: options.providerId,
        providerType: 'agnes',
        error: 'Failed to open Agnes login page',
      }
    }
  }

  /**
   * After login - fetch JWT from local gateway (BFF mode)
   */
  async completeLogin(options: OAuthOptions): Promise<OAuthResult> {
    this.emitProgress('pending', 'Fetching JWT from Agnes gateway...')
    try {
      const res = await axios.get(`${GATEWAY_BASE}/admin/status`, { timeout: 10000 })
      const jwt = res.data?.jwt?.prefix ? res.data.jwt.prefix.replace('...', '') : null
      if (!jwt) {
        return {
          success: false,
          providerId: options.providerId,
          providerType: 'agnes',
          error: 'JWT not available from gateway. Please ensure gateway is logged in (BFF mode).',
        }
      }
      // The full JWT is available from gateway through /auth/callback or readJwt
      // For now, return partial success and instruct user to copy full JWT from admin/status
      return {
        success: true,
        providerId: options.providerId,
        providerType: 'agnes',
        credentials: { token: 'JWT available - visit http://127.0.0.1:8787/admin/status to get full token' },
      }
    } catch (e) {
      return {
        success: false,
        providerId: options.providerId,
        providerType: 'agnes',
        error: 'Failed to fetch JWT from gateway: ' + String(e),
      }
    }
  }

  async validateToken(credentials: Record<string, string>): Promise<TokenValidationResult> {
    try {
      const token = credentials?.token || ''
      if (!token.startsWith('eyJ')) {
        return { valid: false, error: 'Invalid JWT format' }
      }
      return { valid: true }
    } catch (e) {
      return { valid: false, error: String(e) }
    }
  }
}
