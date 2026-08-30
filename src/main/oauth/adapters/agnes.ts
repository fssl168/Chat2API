/**
 * Agnes Auth Adapter
 * Authentication: Login to https://app.agnes-ai.com/login (AgnesCode app)
 * Cookie-based: extracts JWT from browser cookies after login, then validates via API
 */

import { shell } from 'electron'
import axios from 'axios'
import { BaseOAuthAdapter } from './base'
import {
  OAuthResult,
  OAuthOptions,
  TokenValidationResult,
  CredentialInfo,
  AdapterConfig,
} from '../types'

const AGNES_LOGIN_URL = 'https://app.agnes-ai.com/login'
const AGNES_API_HOST = 'api-agnes-code.agnes-ai.com'

export class AgnesOAuthAdapter extends BaseOAuthAdapter {
  constructor(config: AdapterConfig) {
    super({
      ...config,
      providerType: 'agnes',
      authMethods: ['manual', 'cookie'],
      loginUrl: AGNES_LOGIN_URL,
      apiUrl: `https://${AGNES_API_HOST}`,
    })
  }

  /**
   * Complete authentication with manually entered token
   */
  async loginWithToken(providerId: string, token: string): Promise<OAuthResult> {
    this.emitProgress('pending', 'Validating token...')

    try {
      const validation = await this.validateToken({ token })

      if (!validation.valid) {
        return {
          success: false,
          providerId,
          providerType: 'agnes',
          error: validation.error || 'Token validation failed',
        }
      }

      this.emitProgress('success', 'Token validation successful')

      return {
        success: true,
        providerId,
        providerType: 'agnes',
        credentials: { token },
        accountInfo: validation.accountInfo,
      }
    } catch (error) {
      const errorMessage = error instanceof Error ? error.message : 'Validation request failed'
      return {
        success: false,
        providerId,
        providerType: 'agnes',
        error: errorMessage,
      }
    }
  }

  /**
   * Start login flow - Open Agnes login page in browser
   */
  async startLogin(options: OAuthOptions): Promise<OAuthResult> {
    this.emitProgress('pending', 'Opening Agnes login page...')
    try {
      await shell.openExternal(AGNES_LOGIN_URL)
      this.emitProgress('pending', 'Please log in at app.agnes-ai.com, then enter your token manually')
      return {
        success: false,
        providerId: options.providerId,
        providerType: 'agnes',
        error: 'Please log in at https://app.agnes-ai.com, then click OK to enter token manually',
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

  async validateToken(credentials: Record<string, string>): Promise<TokenValidationResult> {
    try {
      const token = credentials?.token || ''
      if (!token.startsWith('eyJ')) {
        return { valid: false, error: 'Invalid JWT format: token must start with eyJ' }
      }

      // 实际调用 Agnes API 验证 Token 是否有效
      const response = await axios.get(`https://${AGNES_API_HOST}/v1/models`, {
        headers: { Authorization: `Bearer ${token}` },
        timeout: 15000,
        validateStatus: () => true,
      })

      if (response.status === 401) {
        return {
          valid: false,
          error: 'Token expired or invalid. Please re-login at app.agnes-ai.com and get a new token.',
        }
      }

      if (response.status !== 200) {
        return {
          valid: false,
          error: `Token validation failed with status ${response.status}. Please re-login.`,
        }
      }

      // Token 有效
      return { valid: true }
    } catch (e) {
      const errMsg = e instanceof Error ? e.message : String(e)
      return {
        valid: false,
        error: `Validation request failed: ${errMsg}. Check your network and try again.`,
      }
    }
  }
}
