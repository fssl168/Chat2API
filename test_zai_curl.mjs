/**
 * Z.ai ESA WAF bypass test: axios vs curl-cffi
 * Tests whether TLS impersonation (chrome148) gets past the WAF
 * where plain axios is blocked.
 */
import { createRequire } from 'module'
const require = createRequire(import.meta.url)
const { Session } = require('curl-cffi-node')
const axios = require('axios')

const FAKE_TOKEN = 'eyJhbGciOiJIUzI1NiJ9.eyJpZCI6InRlc3RfdXNlciIsImVtYWlsIjoidGVzdEBleGFtcGxlLmNvbSJ9.fake_signature'
const ZAI_API_BASE = 'https://chat.z.ai'

const BROWSER_HEADERS = {
  'Accept': '*/*',
  'Accept-Encoding': 'gzip, deflate, br, zstd',
  'Accept-Language': 'zh-CN',
  'Cache-Control': 'no-cache',
  'Origin': ZAI_API_BASE,
  'Pragma': 'no-cache',
  'Sec-Fetch-Dest': 'empty',
  'Sec-Fetch-Mode': 'cors',
  'Sec-Fetch-Site': 'same-origin',
  'X-Region': 'domestic',
  'Sec-Ch-Ua': '"Google Chrome";v="148", "Chromium";v="148", "Not_A Brand";v="24"',
  'Sec-Ch-Ua-Mobile': '?0',
  'Sec-Ch-Ua-Platform': '"macOS"',
  'User-Agent': 'Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/148.0.0.0 Safari/537.36',
}

async function testChatCreate(client, label) {
  const body = {
    chat: { id: '', title: 'test', models: ['glm-5'], params: {}, history: { messages: {}, currentId: '' }, tags: [], features: [], mcp_servers: [], enable_thinking: false, timestamp: Date.now(), type: 'default' },
  }
  const headers = {
    ...BROWSER_HEADERS,
    Authorization: `Bearer ${FAKE_TOKEN}`,
    'Content-Type': 'application/json',
    'Cookie': `token=${FAKE_TOKEN}`,
    Referer: `${ZAI_API_BASE}/`,
  }
  const url = `${ZAI_API_BASE}/api/v1/chats/new`

  if (client === 'axios') {
    const res = await axios.post(url, body, { headers, timeout: 15000, validateStatus: () => true })
    return { status: res.status, body: typeof res.data === 'string' ? res.data.substring(0, 300) : JSON.stringify(res.data).substring(0, 300), contentType: res.headers?.['content-type']?.substring(0, 40) }
  }

  // curl-cffi
  const res = await client.post(url, { data: body, headers, timeout: 15 })
  const contentType = res.headers.get?.('content-type')?.substring(0, 40) ?? ''
  const text = res.text()
  return { status: res.status, body: text.substring(0, 300), contentType }
}

async function main() {
  console.log('=== Z.ai ESA WAF Test ===\n')

  // 1. Base endpoint (no auth needed)
  for (const [label, client] of [['axios', axios], ['curl-cffi (chrome148)', null]]) {
    if (client) {
      try {
        const res = await axios.get(ZAI_API_BASE, { timeout: 10000, validateStatus: () => true })
        console.log(`[${label}] GET / -> status=${res.status} body=${typeof res.data === 'string' ? res.data.substring(0, 100) : 'object'}`)
      } catch (e) {
        console.log(`[${label}] GET / -> ERROR: ${e.message}`)
      }
    }
  }

  // 2. Init curl-cffi Session
  let session
  try {
    session = new Session({ impersonate: 'chrome148', headers: BROWSER_HEADERS, timeout: 15, followRedirects: true })
    console.log('\n[curl-cffi] Session initialized ✓')
  } catch (e) {
    console.log(`\n[curl-cffi] Session init FAILED: ${e.message}`)
    return
  }

  // 3. Test chat creation endpoint
  console.log('\n--- POST /api/v1/chats/new ---')
  try {
    const result = await testChatCreate('axios', 'axios')
    console.log(`[axios]         status=${result.status} | type=${result.contentType}`)
    console.log(`         body=${result.body}`)
  } catch (e) {
    console.log(`[axios]         ERROR: ${e.message}`)
  }

  try {
    const result = await testChatCreate(session, 'curl-cffi')
    console.log(`[curl-cffi (chrome148)] status=${result.status} | type=${result.contentType}`)
    console.log(`         body=${result.body}`)
  } catch (e) {
    console.log(`[curl-cffi (chrome148)] ERROR: ${e.message}`)
  }

  // 4. Test chat completion endpoint
  console.log('\n--- POST /api/v2/chat/completions ---')
  const chatBody = {
    model: 'glm-5',
    messages: [{ role: 'user', content: 'hi' }],
    stream: false,
    features: { image_generation: false, web_search: false, auto_web_search: false, preview_mode: true, flags: [], vlm_tools_enable: false, vlm_web_search_enable: false, vlm_website_mode: false, enable_thinking: false },
    variables: { '{{USER_NAME}}': 'User' },
    chat_id: 'test',
    id: 'test-123',
    current_user_message_id: 'msg-1',
    current_user_message_parent_id: null,
    background_tasks: { title_generation: false, tags_generation: false },
  }
  const chatHeaders = {
    ...BROWSER_HEADERS,
    Authorization: `Bearer ${FAKE_TOKEN}`,
    'Content-Type': 'application/json',
    'Cookie': `token=${FAKE_TOKEN}`,
    Referer: `${ZAI_API_BASE}/c/test`,
    Priority: 'u=1, i',
    'X-FE-Version': 'prod-fe-1.1.92',
    'X-Signature': 'test-signature',
  }
  const qs = new URLSearchParams({ timestamp: String(Date.now()), requestId: 'test-123', user_id: 'test', version: '2.1.0', platform: 'web', token: FAKE_TOKEN, language: 'zh-CN', timezone: 'Asia/Shanghai', browser_name: 'Chrome', os_name: 'Mac OS' })
  const chatUrl = `${ZAI_API_BASE}/api/v2/chat/completions?${qs.toString()}`

  try {
    const res = await axios.post(chatUrl, chatBody, { headers: chatHeaders, timeout: 15000, validateStatus: () => true })
    const ct = res.headers?.['content-type']?.substring(0, 40) ?? 'N/A'
    const body = typeof res.data === 'string' ? res.data.substring(0, 300) : JSON.stringify(res.data).substring(0, 300)
    console.log(`[axios]         status=${res.status} | type=${ct}`)
    console.log(`         body=${body}`)
  } catch (e) {
    console.log(`[axios]         ERROR: ${e.message}`)
  }

  try {
    const res = await session.post(chatUrl, { data: chatBody, headers: chatHeaders, timeout: 15 })
    const ct = res.headers.get?.('content-type')?.substring(0, 40) ?? ''
    const text = res.text().substring(0, 300)
    console.log(`[curl-cffi (chrome148)] status=${res.status} | type=${ct}`)
    console.log(`         body=${text}`)
  } catch (e) {
    console.log(`[curl-cffi (chrome148)] ERROR: ${e.message}`)
  }

  console.log('\n=== Done ===')
}

main().catch(console.error)