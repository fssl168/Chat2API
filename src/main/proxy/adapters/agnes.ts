/**
 * Agnes Adapter
 * Direct connection to official Agnes BFF: https://api-agnes-code.agnes-ai.com/v1
 * JWT is read from account credentials (extracted via in-app browser cookie login)
 * Uses curl-cffi with TLS impersonation (JA3/JA4), axios fallback
 */

import axios from 'axios'
import * as os from 'os'
import { spawnSync } from 'child_process'
import { Session } from 'curl-cffi-node'
import { Account, Provider } from '../../store/types'
import type { AxiosResponse } from 'axios'
import { logManager } from '../../logger/manager'
import { randomUaProfile } from '../utils/uaPool'

const AGNES_BFF_URL = 'https://api-agnes-code.agnes-ai.com'

interface AgnesMessage {
  role: 'user' | 'assistant' | 'system'
  content: string
  tool_call_id?: string
  tool_calls?: any[]
}

interface ChatCompletionRequest {
  model: string
  messages: AgnesMessage[]
  stream?: boolean
  temperature?: number
  max_tokens?: number
  top_p?: number
  tools?: any[]
  tool_choice?: any
}

export class AgnesAdapter {
  private provider: Provider
  private account: Account
  private session: InstanceType<typeof Session> | null = null
  /** Per-instance randomized browser headers (UA + Sec-Ch-Ua) */
  private readonly headers: Record<string, string>

  constructor(provider: Provider, account: Account) {
    this.provider = provider
    this.account = account
    // Randomize UA per adapter instance to reduce fingerprint consistency
    const uaProfile = randomUaProfile()
    this.headers = {
      'Accept': 'application/json, text/event-stream',
      'Accept-Language': 'zh-CN,zh;q=0.9,en;q=0.8',
      'Origin': 'https://app.agnes-ai.com',
      'Referer': 'https://app.agnes-ai.com/',
      'Sec-Fetch-Dest': 'empty',
      'Sec-Fetch-Mode': 'cors',
      'Sec-Fetch-Site': 'same-site',
      'User-Agent': uaProfile.userAgent,
    }

    // curl-cffi: secondary fallback for TLS impersonation / JA3 fingerprint
    try {
      this.session = new Session({
        impersonate: 'chrome131',
        headers: this.headers,
        timeout: 120,
        followRedirects: true,
      })
      console.log('[Agnes] curl-cffi Session initialized (fallback only)')
      logManager.log('info', '[Agnes] curl-cffi Session initialized (fallback only)')
    } catch (e) {
      console.warn('[Agnes] curl-cffi Session failed, will use axios only:', e)
      this.session = null
    }
  }

  /**
   * Get JWT token from account credentials
   */
  private async acquireToken(): Promise<string> {
    const token = this.account.credentials.token || ''
    if (!token.startsWith('eyJ')) {
      throw new Error('JWT not found. Please re-login at app.agnes-ai.com to obtain a valid token.')
    }
    return token
  }

  /**
   * Send chat completion request directly to Agnes BFF
   *
   * Mirrors the verified forwarding mechanism from the reference gateway
   * (D:\leanpython\agnes server.js / agnes_bff_gateway.py):
   *   - BFF requires X-App-Id: 1, X-Platform: 1, X-User-Language headers
   *   - plain TLS (axios) works fine — TLS impersonation is NOT required
   *   - forward the request body mostly as-is (model, messages, stream, ...)
   */
  async chatCompletion(request: ChatCompletionRequest): Promise<{ response: AxiosResponse }> {
    const token = await this.acquireToken()

    const messages = request.messages.map(msg => ({
      role: msg.role,
      content: typeof msg.content === 'string' ? msg.content : '',
    }))

    const payload: Record<string, unknown> = {
      model: request.model,
      messages,
      stream: request.stream ?? false,
    }
    if (request.temperature !== undefined) payload['temperature'] = request.temperature
    if (request.max_tokens !== undefined) payload['max_tokens'] = request.max_tokens
    if (request.top_p !== undefined) payload['top_p'] = request.top_p

    console.log('[Agnes] Sending request to:', AGNES_BFF_URL)
    logManager.log('info', '[Agnes] Sending request to: ' + AGNES_BFF_URL)

    let response: any
    let lastError: Error | null = null

    // BFF-required headers (verified in reference gateway)
    const bffHeaders: Record<string, string> = {
      ...this.headers,
      'Authorization': `Bearer ${token}`,
      'Content-Type': 'application/json',
      'Accept': request.stream ? 'text/event-stream' : 'application/json',
      'X-App-Id': '1',
      'X-Platform': '1',
      'X-User-Language': 'zh-CN',
    }

    // Primary: axios (plain TLS, matches reference gateway)
    console.log('[Agnes] Using axios (primary)')
    logManager.log('info', '[Agnes] Using axios (primary)')
    try {
      response = await axios.post(
        `${AGNES_BFF_URL}/v1/chat/completions`,
        payload,
        {
          headers: bffHeaders,
          timeout: 120000,
          validateStatus: () => true,
          responseType: request.stream ? 'stream' : 'json',
        }
      )
      console.log('[Agnes] axios succeeded')
      logManager.log('info', '[Agnes] axios succeeded')
    } catch (e) {
      lastError = e as Error
      console.warn('[Agnes] axios failed:', lastError.message)
    }

    // Fallback/enhancement: curl-cffi (TLS impersonation / JA3 fingerprint)
    if (!response && this.session) {
      console.log('[Agnes] Falling back to curl-cffi (TLS impersonation)...')
      logManager.log('info', '[Agnes] Falling back to curl-cffi (TLS impersonation)...')
      try {
        const res = await this.session.post(`${AGNES_BFF_URL}/v1/chat/completions`, {
          data: payload,
          headers: bffHeaders,
          timeout: 120,
        })
        // Wrap curl-cffi Response into AxiosResponse-like shape for stream handler
        // Response API: content (Buffer), text(), json(), stream() — no `body` property
        const headerObj: Record<string, string> = {}
        res.headers.forEach((value, key) => {
          headerObj[key] = value
        })
        let nonStreamData: unknown
        if (request.stream) {
          nonStreamData = res.stream()
        } else {
          try {
            nonStreamData = res.json()
          } catch {
            nonStreamData = res.text()
          }
        }
        response = {
          status: res.status,
          statusText: '',
          headers: headerObj,
          config: {},
          request: {},
          data: nonStreamData,
        }
        console.log('[Agnes] curl-cffi fallback succeeded')
      } catch (e) {
        console.warn('[Agnes] curl-cffi fallback also failed:', (e as Error).message)
      }
    }

    if (!response) {
      throw new Error(`All requests failed. Last error: ${lastError?.message || 'Unknown error'}`)
    }

    console.log('[Agnes] Response status:', response.status)
    logManager.log('info', '[Agnes] Response status: ' + response.status)

    if (response.status === 401) {
      const err = new Error('JWT invalid or expired. Please re-login at app.agnes-ai.com.') as Error & { status?: number }
      err.status = 401
      throw err
    }

    return { response }
  }

  static isAgnesProvider(provider: Provider): boolean {
    return provider.id === 'agnes' || provider.apiEndpoint.includes('agnes-ai.com')
  }
}

export const agnesAdapter = {
  AgnesAdapter,
}
