/**
 * Agnes Adapter
 * Direct connection to official Agnes BFF: https://api-agnes-code.agnes-ai.com/v1
 * JWT is read from ~/AppData/Roaming/Agnes Gateway/jwt.txt or CredMan on Windows
 * Uses curl-cffi with TLS impersonation (JA3/JA4), axios fallback
 */

import axios from 'axios'
import * as fs from 'fs'
import * as path from 'path'
import * as os from 'os'
import { spawnSync } from 'child_process'
import { Session } from 'curl-cffi-node'
import { Account, Provider } from '../../store/types'
import type { AxiosResponse } from 'axios'
import { logManager } from '../../logger/manager'
import { randomUaProfile } from '../utils/uaPool'

const AGNES_BFF_URL = 'https://api-agnes-code.agnes-ai.com'
const AGNES_CONFIG_DIRS = [
  path.join(os.homedir(), 'AppData', 'Roaming', 'Agnes Gateway'),
  path.join(os.homedir(), '.agnes-gateway'),
  path.join(process.cwd(), 'data'),
]

// JWT cache (module-level, shared across adapter instances)
let jwtCache: string | null = null
let jwtFetchedAt = 0
const JWT_TTL_MS = 9 * 60 * 1000 // 9 minutes

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

    // curl-cffi: secondary fallback only.
    // Do NOT set verify:false here — curl-impersonate bundles its own CA store;
    // verify:false only disables chain validation but NOT hostname matching,
    // so Agnes's cert still fails with error 60. Use axios as the reliable path.
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
   * Find JWT config directory
   */
  private findConfigDir(): string | null {
    for (const dir of AGNES_CONFIG_DIRS) {
      if (fs.existsSync(dir)) return dir
    }
    return null
  }

  /**
   * Read JWT from file
   */
  private readJwtFromFile(): string | null {
    const configDir = this.findConfigDir()
    if (!configDir) return null
    const jwtFile = path.join(configDir, 'jwt.txt')
    try {
      const jwt = fs.readFileSync(jwtFile, 'utf8').trim()
      if (jwt && jwt.startsWith('eyJ')) {
        console.log('[Agnes] JWT loaded from:', jwtFile)
        return jwt
      }
    } catch (e) {
      console.error('[Agnes] Failed to read jwt.txt:', e)
    }
    return null
  }

  /**
   * Read JWT from Windows Credential Manager
   */
  private async readJwtFromCredMan(): Promise<string | null> {
    if (process.platform !== 'win32') return null
    const script = `
import ctypes, json
from ctypes import wintypes, POINTER, Structure, byref
class CREDENTIAL(Structure):
    _fields_=[("Flags",wintypes.DWORD),("Type",wintypes.DWORD),("TargetName",wintypes.LPWSTR),
              ("Comment",wintypes.LPWSTR),("LastWritten",wintypes.FILETIME),
              ("CredentialBlobSize",wintypes.DWORD),("CredentialBlob",ctypes.c_void_p),
              ("Persist",wintypes.DWORD),("AttributeCount",wintypes.DWORD),
              ("Attributes",ctypes.c_void_p),("TargetAlias",wintypes.LPWSTR),("UserName",wintypes.LPWSTR)]
p = ctypes.c_void_p()
if ctypes.windll.advapi32.CredReadW("secrets.agnes", 1, 0, byref(p)):
    c = ctypes.cast(p, POINTER(CREDENTIAL)).contents
    blob = ctypes.string_at(c.CredentialBlob, c.CredentialBlobSize).decode("utf-16-le", "replace")
    ctypes.windll.advapi32.CredFree(p)
    try:
        for v in json.loads(blob).values():
            if isinstance(v, str) and v.startswith("eyJ"):
                print(v, flush=True); break
    except Exception:
        pass
`
    try {
      const result = spawnSync('python', ['-c', script], {
        encoding: 'utf8',
        timeout: 15000,
      })
      if (result.status === 0) {
        const line = (result.stdout || '').split('\n')
          .map((s: string) => s.trim())
          .find((s: string) => s.startsWith('eyJ'))
        if (line) return line
      }
    } catch (e) {
      console.error('[Agnes] Failed to read CredMan:', e)
    }
    return null
  }

  /**
   * Get JWT token - tries multiple sources
   */
  private async acquireToken(): Promise<string> {
    const now = Date.now()

    // Return cached JWT if still valid
    if (jwtCache && now - jwtFetchedAt < JWT_TTL_MS) {
      console.log('[Agnes] Using cached JWT')
      logManager.log('info', '[Agnes] Using cached JWT'')
      return jwtCache
    }

    // Check account credentials first
    const token = this.account.credentials.token || ''
    if (token.startsWith('eyJ')) {
      jwtCache = token
      jwtFetchedAt = now
      console.log('[Agnes] Using JWT from account credentials')
      logManager.log('info', '[Agnes] Using JWT from account credentials'')
      return token
    }

    // Try reading from jwt.txt file
    let jwt = this.readJwtFromFile()
    if (jwt) {
      jwtCache = jwt
      jwtFetchedAt = now
      return jwt
    }

    // Try reading from Windows Credential Manager
    jwt = await this.readJwtFromCredMan()
    if (jwt) {
      jwtCache = jwt
      jwtFetchedAt = now
      console.log('[Agnes] Using JWT from CredMan')
      logManager.log('info', '[Agnes] Using JWT from CredMan'')
      return jwt
    }

    // Fallback: environment variable
    const envJwt = process.env.AGNES_DESKTOP_JWT
    if (envJwt?.startsWith('eyJ')) {
      jwtCache = envJwt
      jwtFetchedAt = now
      console.log('[Agnes] Using JWT from environment')
      logManager.log('info', '[Agnes] Using JWT from environment'')
      return envJwt
    }

    throw new Error('JWT not found. Ensure Agnes gateway has been logged in (JWT saved to jwt.txt or CredMan).')
  }

  /**
   * Send chat completion request directly to Agnes BFF
   */
  async chatCompletion(request: ChatCompletionRequest): Promise<{ response: AxiosResponse }> {
    const token = await this.acquireToken()

    const messages = request.messages.map(msg => ({
      role: msg.role,
      content: typeof msg.content === 'string' ? msg.content : '',
    }))

    const payload = {
      model: request.model,
      messages,
      stream: request.stream ?? false,
      ...(request.temperature !== undefined && { temperature: request.temperature }),
    }

    console.log('[Agnes] Sending request to:', AGNES_BFF_URL)
    logManager.log('info', '[Agnes] Sending request to:', AGNES_BFF_URL')

    let response: any
    let lastError: Error | null = null

    // Primary: axios (reliable TLS verification with system CA store)
    console.log('[Agnes] Using axios (primary)')
    logManager.log('info', '[Agnes] Using axios (primary)'')
    try {
      response = await axios.post(
        `${AGNES_BFF_URL}/v1/chat/completions`,
        payload,
        {
          headers: {
            'Authorization': `Bearer ${token}`,
            'Content-Type': 'application/json',
            'Accept': request.stream ? 'text/event-stream' : 'application/json',
          },
          timeout: 120000,
          validateStatus: () => true,
          responseType: request.stream ? 'stream' : 'json',
        }
      )
      console.log('[Agnes] axios succeeded')
      logManager.log('info', '[Agnes] axios succeeded'')
    } catch (e) {
      lastError = e as Error
      console.warn('[Agnes] axios failed:', lastError.message)
    }

    // Fallback/enhancement: curl-cffi (TLS impersonation / JA3 fingerprint)
    if (!response && this.session) {
      console.log('[Agnes] Falling back to curl-cffi (TLS impersonation)...')
      logManager.log('info', '[Agnes] Falling back to curl-cffi (TLS impersonation)...'')
      try {
        const res = await this.session.post(`${AGNES_BFF_URL}/v1/chat/completions`, {
          data: payload,
          headers: {
            'Authorization': `Bearer ${token}`,
            'Content-Type': 'application/json',
            'Accept': request.stream ? 'text/event-stream' : 'application/json',
          },
        })
        response = {
          status: (res as any).status || 200,
          statusText: '',
          headers: (res as any).headers || {},
          config: {},
          request: {},
          data: request.stream ? (res as any).body : (res as any).body,
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
    logManager.log('info', '[Agnes] Response status:', response.status')

    if (response.status === 401) {
      jwtCache = null
      throw new Error('JWT invalid or expired. Please re-login to Agnes gateway.')
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
