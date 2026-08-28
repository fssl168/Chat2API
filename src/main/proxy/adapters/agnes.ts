/**
 * Agnes Adapter
 * Implements Agnes AI API protocol via local gateway (BFF mode)
 * JWT is read from ~/AppData/Roaming/Agnes Gateway/jwt.txt or CredMan on Windows
 */

import axios, { AxiosResponse } from 'axios'
import * as fs from 'fs'
import * as path from 'path'
import * as os from 'os'
import { spawnSync } from 'child_process'
import { Account, Provider } from '../../store/types'
import type { ToolCallingPlan } from '../toolCalling/types'

const AGNES_GATEWAY_URL = process.env.AGNES_GATEWAY_URL || 'http://127.0.0.1:8787'
const AGNES_CONFIG_DIRS = [
  path.join(os.homedir(), 'AppData', 'Roaming', 'Agnes Gateway'),
  path.join(os.homedir(), '.agnes-gateway'),
  path.join(process.cwd(), 'data'),
]

// JWT cache
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

  constructor(provider: Provider, account: Account) {
    this.provider = provider
    this.account = account
  }

  /**
   * Find JWT config directory
   */
  private findConfigDir(): string | null {
    for (const dir of AGNES_CONFIG_DIRS) {
      if (fs.existsSync(dir)) {
        return dir
      }
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
          .map(s => s.trim())
          .find(s => s.startsWith('eyJ'))
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
      return jwtCache
    }

    // Check account credentials first
    const token = this.account.credentials.token || ''
    if (token.startsWith('eyJ')) {
      jwtCache = token
      jwtFetchedAt = now
      console.log('[Agnes] Using JWT from account credentials')
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
      return jwt
    }

    // Fallback: try environment variable
    const envJwt = process.env.AGNES_DESKTOP_JWT
    if (envJwt?.startsWith('eyJ')) {
      jwtCache = envJwt
      jwtFetchedAt = now
      console.log('[Agnes] Using JWT from environment')
      return envJwt
    }

    throw new Error('JWT not found. Please ensure Agnes gateway is running and logged in.')
  }

  /**
   * Send chat completion request to Agnes gateway
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

    console.log('[Agnes] Sending request to:', AGNES_GATEWAY_URL)

    const response = await axios.post(
      `${AGNES_GATEWAY_URL}/v1/chat/completions`,
      payload,
      {
        headers: {
          'Authorization': `Bearer ${token}`,
          'Content-Type': 'application/json',
          'Accept': 'application/json',
          'X-App-Id': '1',
          'X-Platform': '1',
        },
        timeout: 120000,
        validateStatus: () => true,
        responseType: request.stream ? 'stream' : 'json',
      }
    )

    console.log('[Agnes] Response status:', response.status)

    if (response.status === 401) {
      jwtCache = null // Clear cache on auth error
      throw new Error('JWT invalid or expired. Please re-login to Agnes gateway.')
    }

    return { response }
  }

  static isAgnesProvider(provider: Provider): boolean {
    return provider.id === 'agnes' || provider.apiEndpoint.includes('127.0.0.1:8787')
  }
}

export const agnesAdapter = {
  AgnesAdapter,
}
