#!/usr/bin/env node
const fs = require('fs');

const filePath = 'D:/projects/Chat2API/src/main/proxy/adapters/deepseek.ts';
let content = fs.readFileSync(filePath, 'utf-8');

// Remove duplicate method definitions and fix the class
// First, find and remove the broken section with double private keywords
content = content.replace(/private   private buildHeaders[\s\S]*?return headers;\s*\}/g, '');
content = content.replace(/private getAuthHeaders\(\)[\s\S]*?return headers;\s*\}/g, '');

// Now properly add cookies support to the class
const classStart = 'export class DeepSeekAdapter {';
const classEnd = 'constructor(provider: Provider, account: Account) {';

if (content.includes(classStart) && content.includes(classEnd)) {
  // Add cookies field after token field
  const oldFields = `export class DeepSeekAdapter {
  private provider: Provider
  private account: Account
  private token: string`;
  
  const newFields = `export class DeepSeekAdapter {
  private provider: Provider
  private account: Account
  private token: string
  private cookies: string = ''
  private session: any = null`;
  
  content = content.replace(oldFields, newFields);
  
  // Update constructor to include cookies
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
   * Get a curl_cffi session for requests requiring TLS fingerprinting
   */
  private getSession(): any {
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
}

// Update createSession to use buildHeaders
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
    )`;

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
    )`;

content = content.replace(oldCreateSession, newCreateSession);

fs.writeFileSync(filePath, content, 'utf-8');
console.log('Fixed deepseek.ts with proper curl_cffi integration');
