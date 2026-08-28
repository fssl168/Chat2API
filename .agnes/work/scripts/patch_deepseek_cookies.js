#!/usr/bin/env node
const fs = require('fs');

// Update the deepseek adapter to use cookie from account credentials
const filePath = 'D:/projects/Chat2API/src/main/proxy/adapters/deepseek.ts';
let content = fs.readFileSync(filePath, 'utf-8');

// First, check if there's a cookies field in Account type or credentials
const accountTypeCheck = fs.existsSync('D:/projects/Chat2API/src/main/store/types.ts')
  ? fs.readFileSync('D:/projects/Chat2API/src/main/store/types.ts', 'utf-8')
  : '';

// Check if cookies is already in the credentials handling
if (!content.includes("account.credentials.cookies")) {
  // Find where token is used and add cookies support
  const tokenUsage = "this.token";
  const cookieAddition = `// Use cookies from account if available (for anti-detection)
  private cookies: string = '';

  constructor(private provider: Provider, private account: Account) {
    this.baseUrl = provider.apiEndpoint!;
    this.token = account.credentials.token || '';
    // Extract cookies from credentials (set during OAuth)
    this.cookies = account.credentials.cookies || '';
  }`;
  
  // Replace constructor pattern
  const oldConstructor = `constructor(private provider: Provider, private account: Account) {
    this.baseUrl = provider.apiEndpoint!;
    this.token = account.credentials.token || '';
  }`;
  
  if (content.includes(oldConstructor)) {
    content = content.replace(oldConstructor, cookieAddition);
  }
}

// Update request headers to include cookie header when available
if (!content.includes('Cookie:') || !content.includes('this.cookies')) {
  // Find FAKE_HEADERS and add Cookie conditionally
  const headersUpdate = `  private buildHeaders(extraHeaders?: Record<string, string>): Record<string, string> {
    const headers = { ...FAKE_HEADERS, ...extraHeaders };
    // Add Cookie header if available (anti-detection)
    if (this.cookies) {
      headers['Cookie'] = this.cookies;
    }
    return headers;
  }

  private getAuthHeaders(): Record<string, string> {
    const headers: Record<string, string> = {
      ...FAKE_HEADERS,
      'Authorization': \`Bearer \${this.token}\`,
    };
    // Add Cookie header if available (anti-detection)
    if (this.cookies) {
      headers['Cookie'] = this.cookies;
    }
    return headers;
  }`;
  
  // Insert before the first method definition
  const insertPoint = content.indexOf('async createSession');
  if (insertPoint > 0) {
    content = content.slice(0, insertPoint) + headersUpdate + '\n\n' + content.slice(insertPoint);
  }
}

fs.writeFileSync(filePath, content, 'utf-8');
console.log('Updated deepseek.ts with cookie support');
