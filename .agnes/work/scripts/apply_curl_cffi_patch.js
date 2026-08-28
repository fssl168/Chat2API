#!/usr/bin/env node
/**
 * Patch deepseek.ts to use curl-cffi-node for TLS fingerprint impersonation
 */
const fs = require('fs');

const filePath = 'D:/projects/Chat2API/src/main/proxy/adapters/deepseek.ts';
let content = fs.readFileSync(filePath, 'utf-8');

// 1. Add curl-cffi-node import
if (!content.includes("import { Session } from 'curl-cffi-node'")) {
  content = content.replace(
    "import axios, { AxiosResponse } from 'axios'",
    `import axios, { AxiosResponse } from 'axios'
import { Session } from 'curl-cffi-node'`
  );
}

// 2. Update class definition to add cookies and session fields
const classDefPattern = /export class DeepSeekAdapter \{[\s\S]*?private token: string/;
const newClassDef = `export class DeepSeekAdapter {
  private provider: Provider
  private account: Account
  private token: string
  private cookies: string = ''
  private session: Session | null = null`;

content = content.replace(classDefPattern, newClassDef);

// 3. Update constructor
const oldConstructor = `constructor(provider: Provider, account: Account) {
    this.provider = provider
    this.account = account
    console.log('[DeepSeek] Account credentials:', JSON.stringify(account.credentials, null, 2))
    this.token = account.credentials.token || account.credentials.apiKey || account.credentials.refreshToken || ''
    console.log('[DeepSeek] Using token:', this.token.substring(0, 20) + '...')
  }`;

const newConstructor = `constructor(provider: Provider, account: Account) {
    this.provider = provider
    this.account = account
    console.log('[DeepSeek] Account credentials:', JSON.stringify(account.credentials, null, 2))
    this.token = account.credentials.token || account.credentials.apiKey || account.credentials.refreshToken || ''
    // Extract cookies from credentials (set during OAuth login)
    this.cookies = account.credentials.cookies || ''
    console.log('[DeepSeek] Using token:', this.token.substring(0, 20) + '...')
    console.log('[DeepSeek] Cookies available:', !!this.cookies)
    
    // Initialize curl_cffi session with TLS fingerprinting
    this.session = new Session({
      impersonate: 'chrome131',
      timeout: 120000,
    })
  }
  
  /**
   * Get curl_cffi session for requests requiring TLS fingerprinting
   */
  private getSession(): Session {
    if (!this.session) {
      this.session = new Session({
        impersonate: 'chrome131',
        timeout: 120000,
      })
    }
    return this.session
  }
  
  /**
   * Build headers with cookie injection for anti-detection
   */
  private buildHeaders(extra?: Record<string, string>): Record<string, string> {
    const headers: Record<string, string> = { ...FAKE_HEADERS, ...extra }
    // Use stored cookies if available, otherwise generate fake ones
    if (this.cookies) {
      headers['Cookie'] = this.cookies
    } else {
      headers['Cookie'] = generateCookie()
    }
    return headers
  }`;

content = content.replace(oldConstructor, newConstructor);

// 4. Update createSession to use curl_cffi session
const oldCreateSession = `async createSession(): Promise<string> {
    const cacheKey = this.account.id
    const cached = sessionCache.get(cacheKey)
    if (cached && Date.now() - cached.createdAt < 300000) {
      return cached.sessionId
    }

    const token = await this.acquireToken()
    const result = await axios.post(
      \`\${DEEPSEEK_API_BASE}/v0/chat_session/create\`,
      {},
      {
        headers: {
          Authorization: \`Bearer \${token}\`,
          ...FAKE_HEADERS,
          Cookie: generateCookie(),
        },
        timeout: 15000,
        validateStatus: () => true,
      }
    )

    console.log('[DeepSeek] Create session response:', JSON.stringify(result.data, null, 2))

    // Response structure: { code: 0, data: { biz_code: 0, biz_data: { id: "..." } } }
    const bizData = result.data?.data?.biz_data || result.data?.biz_data
    if (result.status !== 200 || !bizData?.chat_session?.id) {
      throw new Error(\`Failed to create session: \${result.data?.msg || result.data?.data?.biz_msg || result.status}\`)
    }

    const sessionId = bizData?.chat_session?.id
    sessionCache.set(cacheKey, { sessionId, createdAt: Date.now() })

    return sessionId
  }`;

const newCreateSession = `async createSession(): Promise<string> {
    const cacheKey = this.account.id
    const cached = sessionCache.get(cacheKey)
    if (cached && Date.now() - cached.createdAt < 300000) {
      return cached.sessionId
    }

    const token = await this.acquireToken()
    const session = this.getSession()
    
    const result = await session.post(
      \`\${DEEPSEEK_API_BASE}/v0/chat_session/create\`,
      {
        headers: this.buildHeaders({
          Authorization: \`Bearer \${token}\`,
          'Content-Type': 'application/json',
        }),
        data: {},
        timeout: 30000,
      }
    )

    console.log('[DeepSeek] Create session response:', JSON.stringify(result.data, null, 2))

    // Parse response (curl_cffi returns data as string or object)
    const responseData = typeof result.data === 'string' ? JSON.parse(result.data) : result.data
    const bizData = responseData?.data?.biz_data || responseData?.biz_data
    if (result.status !== 200 || !bizData?.chat_session?.id) {
      throw new Error(\`Failed to create session: \${responseData?.msg || responseData?.data?.biz_msg || result.status}\`)
    }

    const sessionId = bizData?.chat_session?.id
    sessionCache.set(cacheKey, { sessionId, createdAt: Date.now() })

    return sessionId
  }`;

content = content.replace(oldCreateSession, newCreateSession);

fs.writeFileSync(filePath, content, 'utf-8');
console.log('Successfully patched deepseek.ts with curl_cffi support');
