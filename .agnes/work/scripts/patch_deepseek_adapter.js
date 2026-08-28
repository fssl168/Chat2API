#!/usr/bin/env node
const fs = require('fs');

const filePath = 'D:/projects/Chat2API/src/main/proxy/adapters/deepseek.ts';
let content = fs.readFileSync(filePath, 'utf-8');

// Check if curl_cffi is already imported
if (!content.includes('curl_cffi')) {
  // Add import after axios import
  const oldImport = "import axios, { AxiosResponse } from 'axios'";
  const newImport = `import axios, { AxiosResponse } from 'axios'
import { Session } from 'curl_cffi'`;
  content = content.replace(oldImport, newImport);
}

// Update FAKE_HEADERS to be more realistic
const oldHeaders = `const FAKE_HEADERS = {
  Accept: '*/*',
  'Accept-Encoding': 'gzip, deflate, br, zstd',
  'Accept-Language': 'zh-CN,zh;q=0.9,en;q=0.8,en-GB;q=0.7,en-US;q=0.6',
  Origin: 'https://chat.deepseek.com',
  Referer: 'https://chat.deepseek.com/',
  'Sec-Ch-Ua': '"Not/A)Brand";v="99", "Chromium";v="148"',
  'Sec-Ch-Ua-Mobile': '?0',
  'Sec-Ch-Ua-Platform': '"macOS"',
  'Sec-Fetch-Dest': 'empty',
  'Sec-Fetch-Mode': 'cors',
  'Sec-Fetch-Site': 'same-origin',
  'User-Agent': 'Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/148.0.0.0 Safari/537.36',
  'X-App-Version': '2.0.0',
  'X-Client-Locale': 'zh_CN',
  'X-Client-Platform': 'web',
  'x-Client-Timezone-Offset': '28800',
  'X-Client-Version': '2.0.0',
}`;

const newHeaders = `// TLS fingerprint to impersonate (chrome131 profile from curl_cffi)
const IM impersonate = 'chrome131' as const;

const FAKE_HEADERS = {
  Accept: '*/*',
  'Accept-Encoding': 'gzip, deflate, br, zstd',
  'Accept-Language': 'zh-CN,zh;q=0.9,en-US;q=0.8,en;q=0.7',
  Origin: 'https://chat.deepseek.com',
  Referer: 'https://chat.deepseek.com/',
  'Sec-Ch-Ua': '"Google Chrome";v="131", "Chromium";v="131", "Not_A Brand";v="24"',
  'Sec-Ch-Ua-Mobile': '?0',
  'Sec-Ch-Ua-Platform': '"Windows"',
  'Sec-Fetch-Dest': 'empty',
  'Sec-Fetch-Mode': 'cors',
  'Sec-Fetch-Site': 'same-origin',
  // Downgraded UA to match Chrome 131 TLS fingerprint
  'User-Agent': 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36',
  'X-Client-Version': '2.3.0',
  'X-Client-Platform': 'web',
  'X-Client-Locale': 'zh_CN',
  'X-Client-Timezone-Offset': '28800',
  'x-client-bundle-id': 'com.deepseek.chat',
}`;

content = content.replace(oldHeaders, newHeaders);

// Update the createSession and chatCompletion methods to use curl_cffi
// Find the class definition and add session creation with curl_cffi
const classMatch = content.match(/class DeepSeekAdapter \{[\s\S]*?constructor/);
if (classMatch) {
  // Add curl_cffi session initialization in constructor
  const oldConstructor = `constructor(private provider: Provider, private account: Account) {
    this.baseUrl = provider.apiEndpoint!;
    this.token = account.credentials.token || '';
  }`;
  
  const newConstructor = `private curlSession: Session | null = null;

  constructor(private provider: Provider, private account: Account) {
    this.baseUrl = provider.apiEndpoint!;
    this.token = account.credentials.token || '';
    // Initialize curl_cffi session with TLS fingerprinting
    this.curlSession = new Session({
      impersonate: IM impersonate,
    });
  }`;
  
  content = content.replace(oldConstructor, newConstructor);
}

fs.writeFileSync(filePath, content, 'utf-8');
console.log('Updated deepseek.ts adapter');
