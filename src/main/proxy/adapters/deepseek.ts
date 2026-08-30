/**
 * DeepSeek Adapter
 * Implements DeepSeek web API protocol
 * 
 * NOTE: Tool prompt injection is handled by Forwarder.transformRequestForPromptToolUse()
 * This adapter only handles message format conversion and API communication
 */

import axios, { AxiosResponse } from 'axios'
import { Session } from 'curl-cffi-node'
import { getDeepSeekHash } from '../../lib/challenge'
import type { Account, Provider } from '../../store/types'
import { resolveDeepSeekChatOptions } from './providerModelOptions'
import { getProviderToolProfile } from '../toolCalling/providerProfiles'
import { randomUaProfile } from '../utils/uaPool'

const DEEPSEEK_API_BASE = 'https://chat.deepseek.com/api'

interface TokenInfo {
  accessToken: string
  refreshToken: string
  expiresAt: number
}

interface ChallengeResponse {
  algorithm: string
  challenge: string
  salt: string
  difficulty: number
  expire_at: number
  signature: string
}

interface DeepSeekMessage {
  role: 'user' | 'assistant' | 'system' | 'tool'
  content: string | null
  tool_call_id?: string
  tool_calls?: any[]
}

interface ChatCompletionRequest {
  model: string
  messages: DeepSeekMessage[]
  stream?: boolean
  temperature?: number
  web_search?: boolean
  reasoning_effort?: 'low' | 'medium' | 'high'
  tools?: any[]
  tool_choice?: any
}

const tokenCache = new Map<string, TokenInfo>()
const sessionCache = new Map<string, { sessionId: string; createdAt: number }>()

function generateRandomString(length: number, charset: string = 'alphanumeric'): string {
  const sets = {
    numeric: '0123456789',
    alphabetic: 'ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz',
    alphanumeric: '0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz',
    hex: '0123456789abcdef',
  }
  const chars = sets[charset as keyof typeof sets] || sets.alphanumeric
  let result = ''
  for (let i = 0; i < length; i++) {
    result += chars.charAt(Math.floor(Math.random() * chars.length))
  }
  return result
}

function uuid(): string {
  return 'xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx'.replace(/[xy]/g, (c) => {
    const r = (Math.random() * 16) | 0
    const v = c === 'x' ? r : (r & 0x3) | 0x8
    return v.toString(16)
  })
}

function generateCookie(): string {
  const timestamp = Date.now()
  return `intercom-HWWAFSESTIME=${timestamp}; HWWAFSESID=${generateRandomString(18, 'hex')}; Hm_lvt_${uuid()}=${Math.floor(timestamp / 1000)},${Math.floor(timestamp / 1000)},${Math.floor(timestamp / 1000)}; Hm_lpvt_${uuid()}=${Math.floor(timestamp / 1000)}; _frid=${uuid()}; _fr_ssid=${uuid()}; _fr_pvid=${uuid()}`
}

function unixTimestamp(): number {
  return Math.floor(Date.now() / 1000)
}

export class DeepSeekAdapter {
  private provider: Provider
  private account: Account
  private token: string
  private cookies: string = ''
  private session: Session | null = null
  /** Per-instance randomized browser headers (UA + Sec-Ch-Ua) */
  private readonly headers: Record<string, string>

  /**
   * Convert browser cookie format (name=value; name2=value2) to curl-cffi Netscape format
   */
  private browserCookiesToNetscape(cookieDict: Record<string, string>, domain: string): string[] {
    const lines: string[] = ['# Netscape HTTP Cookie File']
    for (const [name, value] of Object.entries(cookieDict)) {
      lines.push(`#HttpOnly_${domain}\tTRUE\t/\tFALSE\t0\t${name}\t${value}`)
    }
    return lines
  }

  constructor(provider: Provider, account: Account) {
    this.provider = provider
    this.account = account
    console.log('[DeepSeek] Account credentials:', JSON.stringify(account.credentials, null, 2))
    this.token = account.credentials.token || account.credentials.apiKey || account.credentials.refreshToken || ''
    console.log('[DeepSeek] Using token:', this.token.substring(0, 20) + '...')

    // Randomize UA per adapter instance to reduce fingerprint consistency
    const uaProfile = randomUaProfile()
    this.headers = {
      Accept: '*/*',
      'Accept-Encoding': 'gzip, deflate, br, zstd',
      'Accept-Language': 'zh-CN,zh;q=0.9,en;q=0.8,en-GB;q=0.7,en-US;q=0.6',
      Origin: 'https://chat.deepseek.com',
      Referer: 'https://chat.deepseek.com/',
      'Sec-Ch-Ua': uaProfile.secChUa,
      'Sec-Ch-Ua-Mobile': '?0',
      'Sec-Ch-Ua-Platform': uaProfile.secChUaPlatform,
      'Sec-Fetch-Dest': 'empty',
      'Sec-Fetch-Mode': 'cors',
      'Sec-Fetch-Site': 'same-origin',
      'User-Agent': uaProfile.userAgent,
      'X-App-Version': '2.0.0',
      'X-Client-Locale': 'zh_CN',
      'X-Client-Platform': 'web',
      'x-Client-Timezone-Offset': '28800',
      'X-Client-Version': '2.0.0',
    }

    // Load saved cookies from credentials into curl_cffi Session for TLS impersonation
    const storedCookies = account.credentials.cookies as unknown as Record<string, string> | undefined
    if (storedCookies && Object.keys(storedCookies).length > 0) {
      const netscapeFormat = this.browserCookiesToNetscape(storedCookies, '.deepseek.com')
      console.log('[DeepSeek] Loading', Object.keys(storedCookies).length, 'cookies into curl_cffi Session')
      try {
        this.session = new Session({
          impersonate: 'chrome148',
          headers: this.headers,
          timeout: 30,
          followRedirects: true,
          cookies: netscapeFormat,
        })
        console.log('[DeepSeek] curl-cffi Session initialized with cookies (TLS impersonation active)')
      } catch (e) {
        console.warn('[DeepSeek] curl-cffi Session with cookies failed, retrying without:', e)
        try {
          this.session = new Session({
            impersonate: 'chrome148',
            headers: this.headers,
            timeout: 30,
            followRedirects: true,
          })
          this.session.importCookies(netscapeFormat)
          console.log('[DeepSeek] curl-cffi Session initialized (cookies imported after creation)')
        } catch (e2) {
          console.warn('[DeepSeek] curl-cffi Session failed entirely, falling back to axios:', e2)
          this.session = null
        }
      }
    } else {
      // curl-cffi with TLS impersonation (JA3/JA4 fingerprint maintained), axios fallback
      try {
        this.session = new Session({
          impersonate: 'chrome148',
          headers: this.headers,
          timeout: 30,
          followRedirects: true,
        })
        console.log('[DeepSeek] curl-cffi Session initialized (TLS impersonation active)')
      } catch (e) {
        console.warn('[DeepSeek] curl-cffi Session failed, falling back to axios:', e)
        this.session = null
      }
    }
  }

  private async acquireToken(): Promise<string> {
    if (!this.token) {
      throw new Error('DeepSeek Token not configured, please add Token in account settings')
    }

    const cached = tokenCache.get(this.token)
    if (cached && cached.expiresAt > unixTimestamp()) {
      return cached.accessToken
    }

    console.log('[DeepSeek] Acquiring token...')
    
    let result: any
    if (this.session) {
      const res = await (this.session as any).get(`${DEEPSEEK_API_BASE}/v0/users/current`, {
        headers: { Authorization: `Bearer ${this.token}` },
      })
      result = { status: res.status, data: res.json() }
    } else {
      result = await axios.get(`${DEEPSEEK_API_BASE}/v0/users/current`, {
        headers: { Authorization: `Bearer ${this.token}`, ...this.headers },
        timeout: 15000,
        validateStatus: () => true,
      })
    }

    console.log('[DeepSeek] Token response status:', result.status)
    
    if (result.status === 401 || result.status === 403) {
      throw new Error(`Token invalid or expired, please get a new Token`)
    }

    if (result.status !== 200) {
      throw new Error(`Failed to acquire token: HTTP ${result.status}`)
    }

    // Response structure: { code: 0, data: { biz_code: 0, biz_data: { token: "..." } } }
    const bizData = result.data?.data?.biz_data || result.data?.biz_data
    if (!bizData?.token) {
      const errorMsg = result.data?.msg || result.data?.data?.biz_msg || 'Unknown error'
      console.log('[DeepSeek] Token response data:', JSON.stringify(result.data, null, 2))
      throw new Error(`Failed to acquire token: ${errorMsg}`)
    }

    const accessToken = bizData.token
    tokenCache.set(this.token, {
      accessToken,
      refreshToken: this.token,
      expiresAt: unixTimestamp() + 3600,
    })

    console.log('[DeepSeek] Token acquired successfully')
    return accessToken
  }

  private async createSession(): Promise<string> {
    const cacheKey = this.account.id
    const cached = sessionCache.get(cacheKey)
    if (cached && Date.now() - cached.createdAt < 300000) {
      return cached.sessionId
    }

    const token = await this.acquireToken()
    let result: any
    if (this.session) {
      const res = await (this.session as any).post(`${DEEPSEEK_API_BASE}/v0/chat_session/create`, {
        data: {},
        headers: { Authorization: `Bearer ${token}` },
      })
      result = { status: res.status, data: res.json() }
    } else {
      result = await axios.post(
        `${DEEPSEEK_API_BASE}/v0/chat_session/create`,
        {},
        {
          headers: {
            Authorization: `Bearer ${token}`,
            ...this.headers,
            Cookie: generateCookie(),
          },
          timeout: 15000,
          validateStatus: () => true,
        }
      )
    }

    console.log('[DeepSeek] Create session response:', JSON.stringify(result.data, null, 2))

    // Response structure: { code: 0, data: { biz_code: 0, biz_data: { id: "..." } } }
    const bizData = result.data?.data?.biz_data || result.data?.biz_data
    if (result.status !== 200 || !bizData?.chat_session?.id) {
      throw new Error(`Failed to create session: ${result.data?.msg || result.data?.data?.biz_msg || result.status}`)
    }

    const sessionId = bizData?.chat_session?.id
    sessionCache.set(cacheKey, { sessionId, createdAt: Date.now() })

    return sessionId
  }

  async deleteSession(sessionId: string): Promise<boolean> {
    try {
      const token = await this.acquireToken()
      let result: any
    if (this.session) {
      const res = await (this.session as any).post(`${DEEPSEEK_API_BASE}/v0/chat_session/delete`, {
        data: { chat_session_id: sessionId },
        headers: { Authorization: `Bearer ${token}` },
      })
      result = { status: res.status, data: res.json() }
    } else {
      result = await axios.post(
        `${DEEPSEEK_API_BASE}/v0/chat_session/delete`,
        { chat_session_id: sessionId },
        {
          headers: {
            Authorization: `Bearer ${token}`,
            ...this.headers,
          },
          timeout: 15000,
          validateStatus: () => true,
        }
      )
    }

      console.log('[DeepSeek] Delete session response:', JSON.stringify(result.data, null, 2))

      const success = result.status === 200 && result.data?.code === 0
      if (success) {
        // Clear cache
        const cacheKey = this.account.id
        sessionCache.delete(cacheKey)
        console.log('[DeepSeek] Session deleted:', sessionId)
      }
      return success
    } catch (error) {
      console.error('[DeepSeek] Failed to delete session:', error)
      return false
    }
  }

  private async getChallenge(targetPath: string): Promise<ChallengeResponse> {
    const token = await this.acquireToken()
    let result: any
    if (this.session) {
      const res = await (this.session as any).post(`${DEEPSEEK_API_BASE}/v0/chat/create_pow_challenge`, {
        data: { target_path: targetPath },
        headers: { Authorization: `Bearer ${token}` },
      })
      result = { status: res.status, data: res.json() }
    } else {
      result = await axios.post(
        `${DEEPSEEK_API_BASE}/v0/chat/create_pow_challenge`,
        { target_path: targetPath },
        {
          headers: {
            Authorization: `Bearer ${token}`,
            ...this.headers,
          },
          timeout: 15000,
          validateStatus: () => true,
        }
      )
    }

    // Response structure: { code: 0, data: { biz_code: 0, biz_data: { challenge: {...} } } }
    const bizData = result.data?.data?.biz_data || result.data?.biz_data
    if (result.status !== 200 || !bizData?.challenge) {
      throw new Error(`Failed to get challenge: ${result.data?.msg || result.data?.data?.biz_msg || result.status}`)
    }

    return bizData.challenge
  }

  private async calculateChallengeAnswer(challenge: ChallengeResponse): Promise<string> {
    const { algorithm, challenge: challengeStr, salt, difficulty, expire_at, signature } = challenge
    
    if (algorithm !== 'DeepSeekHashV1') {
      throw new Error(`Unsupported algorithm: ${algorithm}`)
    }
    
    console.log('[DeepSeek] Challenge parameters:', { difficulty })
    
    const deepSeekHash = await getDeepSeekHash()
    const answer = deepSeekHash.calculateHash(algorithm, challengeStr, salt, difficulty, expire_at)
    
    if (answer === undefined) {
      throw new Error('Challenge calculation failed')
    }
    
    console.log('[DeepSeek] Challenge answer found:', answer)

    return Buffer.from(JSON.stringify({
      algorithm,
      challenge: challengeStr,
      salt,
      answer,
      signature,
      target_path: '/api/v0/chat/completion',
    })).toString('base64')
  }

  private messagesToPrompt(messages: DeepSeekMessage[], isMultiTurn: boolean = false): string {
    const toolProfile = getProviderToolProfile('deepseek')
    const processedMessages = messages.map(message => {
      let text: string

      // Handle tool calls in assistant message
      if (message.role === 'assistant' && message.tool_calls && message.tool_calls.length > 0) {
        text = toolProfile.formatAssistantToolCalls(message.tool_calls.map(tc => ({
          id: tc.id,
          name: tc.function.name,
          arguments: tc.function.arguments,
        })))
      }
      // Handle tool response message
      else if (message.role === 'tool' && message.tool_call_id) {
        text = toolProfile.formatToolResult({
          toolCallId: message.tool_call_id,
          content: String(message.content || ''),
        })
      }
      else if (Array.isArray(message.content)) {
        const texts = message.content
          .filter((item: any) => item.type === 'text')
          .map((item: any) => item.text)
        text = texts.join('\n')
      } else {
        text = String(message.content || '')
      }
      return { role: message.role, text }
    })

    if (processedMessages.length === 0) return ''

    // For multi-turn mode, only send the last user message
    if (isMultiTurn) {
      let lastUserIdx = -1
      for (let i = processedMessages.length - 1; i >= 0; i--) {
        if (processedMessages[i].role === 'user') {
          lastUserIdx = i
          break
        }
      }
      
      if (lastUserIdx !== -1) {
        const lastUserMsg = processedMessages[lastUserIdx]
        let text = lastUserMsg.text
        for (let i = lastUserIdx + 1; i < processedMessages.length; i++) {
          if (processedMessages[i].role === 'tool') {
            text += `\n\n${processedMessages[i].text}`
          }
        }
        return `<｜User｜>${text}`
      }
    }

    const mergedBlocks: { role: string; text: string }[] = []
    let currentBlock = { ...processedMessages[0] }

    for (let i = 1; i < processedMessages.length; i++) {
      const msg = processedMessages[i]
      if (msg.role === currentBlock.role) {
        currentBlock.text += `\n\n${msg.text}`
      } else {
        mergedBlocks.push(currentBlock)
        currentBlock = { ...msg }
      }
    }
    mergedBlocks.push(currentBlock)

    return mergedBlocks
      .map((block, index) => {
        if (block.role === 'assistant') {
          return `<｜Assistant｜>${block.text}<｜end of sentence｜>`
        }
        if (block.role === 'user' || block.role === 'system') {
          return index > 0 ? `<｜User｜>${block.text}` : block.text
        }
        if (block.role === 'tool') {
          return `<｜User｜>${block.text}`
        }
        return block.text
      })
      .join('')
      .replace(/!\[.+\]\(.+\)/g, '')
  }

  async chatCompletion(request: ChatCompletionRequest): Promise<{ response: AxiosResponse; sessionId: string }> {
    // Normalize unknown DeepSeek model variants to supported model
    const normalizedModel = request.model
      .replace('deepseek-v4-flash-search', 'deepseek-chat')
      .replace('deepseek-v4-flash-search', 'deepseek-chat')
      .replace('deepseek-v4-flash', 'deepseek-chat')
      .replace('deepseek-v4-flash-think', 'deepseek-r1')
      .replace('deepseek-v4-think', 'deepseek-r1')
      .replace('deepseek-v4', 'deepseek-chat')
    if (normalizedModel !== request.model) {
      console.log('[DeepSeek] Normalized model:', request.model, '->', normalizedModel)
      request = { ...request, model: normalizedModel }
    }
    const token = await this.acquireToken()
    
    const sessionId = await this.createSession()
    console.log('[DeepSeek] Created new session:', sessionId)
    
    const challenge = await this.getChallenge('/api/v0/chat/completion')
    const challengeAnswer = await this.calculateChallengeAnswer(challenge)

    // Clone messages to avoid modifying original request
    // Note: Tool prompt injection is already handled by Forwarder.transformRequestForPromptToolUse()
    const messages = [...request.messages]

    let prompt = this.messagesToPrompt(messages, false)

    const { modelType, searchEnabled, thinkingEnabled } = resolveDeepSeekChatOptions(request, prompt)

    if (request.web_search || request.model.toLowerCase().includes('search')) {
      console.log('[DeepSeek] Web search enabled')
    }

    if (request.reasoning_effort || thinkingEnabled) {
      console.log('[DeepSeek] Reasoning mode enabled, effort:', request.reasoning_effort)
    }

    // Build Cookie header from stored cookies (if any) + generated WAF cookies
    const cookieParts = generateCookie().split('; ').filter(Boolean)
    const storedCookies = this.account.credentials.cookies as unknown as Record<string, string> | undefined
    if (storedCookies) {
      for (const [name, value] of Object.entries(storedCookies)) {
        cookieParts.unshift(`${name}=${value}`)
      }
    }
    const cookieHeader = cookieParts.join('; ')

    let response: AxiosResponse | null = null

    if (this.session) {
      // Use curl_cffi Session with TLS impersonation for all requests
      try {
        const res = await (this.session as any).post(
          `${DEEPSEEK_API_BASE}/v0/chat/completion`,
          {
            data: {
              chat_session_id: sessionId,
              parent_message_id: null,
              prompt,
              model_type: modelType,
              ref_file_ids: [],
              search_enabled: searchEnabled,
              thinking_enabled: thinkingEnabled,
              preempt: false,
            },
            headers: {
              Authorization: `Bearer ${token}`,
              Referer: `https://chat.deepseek.com/a/chat/s/${sessionId}`,
              'Cookie': cookieHeader,
              'X-Ds-Pow-Response': challengeAnswer,
            },
          }
        )
        // Wrap curl-cffi Response into AxiosResponse-like shape for stream handler
        // Response API: content (Buffer), text(), json(), stream() — no `body` property
        const headerObj: Record<string, string> = {}
        res.headers.forEach((value: string, key: string) => {
          headerObj[key] = value
        })
        response = {
          status: res.status,
          data: res.stream(),
          headers: headerObj,
        } as unknown as AxiosResponse
        console.log('[DeepSeek] Request completed via curl-cffi (TLS impersonation)')
      } catch (e) {
        console.warn('[DeepSeek] curl-cffi post failed, falling back to axios:', e)
        this.session = null
        // Fall through to axios below
      }
    }

    if (!response) {
      // Fallback to axios without TLS impersonation
      response = await axios.post(
        `${DEEPSEEK_API_BASE}/v0/chat/completion`,
        {
          chat_session_id: sessionId,
          parent_message_id: null,
          prompt,
          model_type: modelType,
          ref_file_ids: [],
          search_enabled: searchEnabled,
          thinking_enabled: thinkingEnabled,
          preempt: false,
        },
        {
          headers: {
            Authorization: `Bearer ${token}`,
            ...this.headers,
            Referer: `https://chat.deepseek.com/a/chat/s/${sessionId}`,
            Cookie: cookieHeader,
            'X-Ds-Pow-Response': challengeAnswer,
          },
          timeout: 120000,
          validateStatus: () => true,
          responseType: 'stream',
        }
      )
      console.log('[DeepSeek] Request completed via axios fallback')
    }

    if (!response) {
      throw new Error('Failed to complete DeepSeek request (both curl-cffi and axios failed)')
    }

    return { response, sessionId }
  }

  async deleteAllChats(): Promise<boolean> {
    try {
      const token = await this.acquireToken()
      const result = await axios.post(
        `${DEEPSEEK_API_BASE}/v0/chat_session/delete_all`,
        {},
        {
          headers: {
            Authorization: `Bearer ${token}`,
            ...this.headers,
          },
          timeout: 30000,
          validateStatus: () => true,
        }
      )

      console.log('[DeepSeek] Delete all chats response:', JSON.stringify(result.data, null, 2))

      const success = result.status === 200 && result.data?.code === 0
      if (success) {
        sessionCache.clear()
        console.log('[DeepSeek] All chats deleted')
      }
      return success
    } catch (error) {
      console.error('[DeepSeek] Failed to delete all chats:', error)
      return false
    }
  }

  static isDeepSeekProvider(provider: Provider): boolean {
    return provider.id === 'deepseek' || provider.apiEndpoint.includes('deepseek.com')
  }

  /**
   * Clear session cache for a specific account
   * This should be called when a session is deleted externally (e.g., from web)
   */
  static clearSessionCache(accountId: string): void {
    sessionCache.delete(accountId)
    console.log('[DeepSeek] Cleared session cache for account:', accountId)
  }
}

export const deepSeekAdapter = {
  DeepSeekAdapter,
}
