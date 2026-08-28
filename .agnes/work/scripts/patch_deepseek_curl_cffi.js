#!/usr/bin/env node
const fs = require('fs');

const filePath = 'D:/projects/Chat2API/src/main/proxy/adapters/deepseek.ts';
let content = fs.readFileSync(filePath, 'utf-8');

// Add import for curl-cffi-node at the top
if (!content.includes('curl-cffi-node')) {
  const oldImport = "import axios, { AxiosResponse } from 'axios'";
  const newImport = `import axios, { AxiosResponse } from 'axios'
import { Session } from 'curl-cffi-node'`;
  content = content.replace(oldImport, newImport);
}

// Add cookies field and update constructor
const oldConstructor = `export class DeepSeekAdapter {
  private provider: Provider
  private account: Account
  private token: string

  constructor(provider: Provider, account: Account) {
    this.provider = provider
    this.account = account
    console.log('[DeepSeek] Account credentials:', JSON.stringify(account.credentials, null, 2))
    this.token = account.credentials.token || account.credentials.apiKey || account.credentials.refreshToken || ''
    console.log('[DeepSeek] Using token:', this.token.substring(0, 20) + '...')
  }`;

const newConstructor = `export class DeepSeekAdapter {
  private provider: Provider
  private account: Account
  private token: string
  private cookies: string = ''
  private session: Session | null = null

  constructor(provider: Provider, account: Account) {
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
   * Get a curl_cffi session for requests requiring TLS fingerprinting
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
   * Build headers with optional cookie injection
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

// Update createSession to use curl_cffi
const oldCreateSession = `  async createSession(): Promise<string> {
    const headers = this.buildHeaders({
      'Authorization': \`Bearer \${this.token}\`,
    });

    const response = await axios.post(\`\${DEEPSEEK_API_BASE}/v0/chat_session/create\`, {
      target_path: '/api/v0/chat/completion',
    }, {
      headers,
      timeout: 30000,
    });

    if (response.data.code !== 0) {
      throw new Error(\`Failed to create session: \${response.data.msg}\`)
    }

    return response.data.data.biz_data.id;
  }`;

const newCreateSession = `  async createSession(): Promise<string> {
    const session = this.getSession()
    const headers = this.buildHeaders({
      'Authorization': \`Bearer \${this.token}\`,
      'Content-Type': 'application/json',
    });

    const response = await session.post(\`\${DEEPSEEK_API_BASE}/v0/chat_session/create\`, {
      headers,
      data: { target_path: '/api/v0/chat/completion' },
      timeout: 30000,
    });

    const data = typeof response.data === 'string' ? JSON.parse(response.data) : response.data;
    if (data.code !== 0) {
      throw new Error(\`Failed to create session: \${data.msg}\`)
    }

    return data.data.biz_data.id;
  }`;

// Check if old pattern exists, otherwise skip replacement
if (content.includes('async createSession(): Promise<string>')) {
  // Try to find and replace
  const createSessionMatch = content.match(/async createSession\(\): Promise<string> \{[\s\S]*?return response\.data\.data\.biz_data\.id;[\s\S]*?\}/);
  if (createSessionMatch) {
    content = content.replace(createSessionMatch[0], newCreateSession);
  }
}

fs.writeFileSync(filePath, content, 'utf-8');
console.log('Updated deepseek.ts with curl_cffi support');
