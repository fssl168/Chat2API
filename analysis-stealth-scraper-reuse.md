# Distributed Stealth Scraper 项目分析与复用方案

## 一、源项目概览

**仓库**: https://github.com/mercury-systems/distributed-stealth-scraper
**许可证**: MIT
**语言**: Python 3.10+
**核心架构**: 双引擎 + 自动升级 + 代理池 + 会话管理

### 技术栈
| 组件 | 技术 | 作用 |
|------|------|------|
| Light Engine | `curl_cffi` (JA3 模拟 chrome120) | 轻量级 HTTP 请求，TLS 指纹欺骗 |
| Heavy Engine | `Playwright` + stealth 补丁 | 完整浏览器自动化，通过 JS 指纹检测 |
| 会话管理 | SQLite (`cookies` + `tokens` 表) | Cookie/Token 持久化与过期管理 |
| 代理池 | 环形轮换 + 健康追踪 + 自动恢复 | 多代理轮询、失败退避、延迟排序 |
| 挑战检测 | 正则匹配 HTML/Headers | Cloudflare/DataDome/PerimeterX/reCAPTCHA/hCaptcha/Akamai/Imperva |
| 批量抓取 | asyncio.Semaphore 并发控制 | 支持 N URL 并发爬取 |
| 自动升级 | ChallengeType != NONE → 切换 Heavy | 轻引擎遇阻时自动降级到重引擎 |

---

## 二、核心机制详解

### 2.1 双引擎架构
```
┌─────────────────────────────────────────────┐
│            StealthScraper (Orchestrator)     │
│                                             │
│  请求进入 → Light Engine                    │
│       ↓ 失败或检测到 Challenge              │
│  → 自动升级到 Heavy Engine                  │
│       ↓ force_heavy / 异常                  │
│  → Heavy Engine 兜底                       │
└─────────────────────────────────────────────┘
```

**Light Engine 关键特征**:
- `curl_cffi.AsyncSession(impersonate="chrome120")` — 精确模仿 Chrome 120 的 TLS 握手
- 随机 User-Agent 池（5 个不同 OS/浏览器组合）
- 完整的 Sec-Fetch-* 系列头
- 自动携带和存储 domain-scoped cookies

**Heavy Engine 关键特征**:
- `--no-sandbox --disable-blink-features=AutomationControlled` 启动参数
- `navigator.webdriver = undefined` 覆盖
- Canvas getImageData 噪声注入（每像素 +1 扰动）
- WebGL 渲染器伪装为 Intel Iris（防止 GPU 指纹暴露）
- 等待 2-5 秒让 JS 挑战完成，再提取 content

### 2.2 自动升级策略
```python
# engine.py 核心逻辑
status, html, headers = await self._light.fetch(url)
challenge = detect_challenge(html, headers)

if challenge != ChallengeType.NONE and self._heavy_available:
    # 自动升级到重型引擎
    status, html, headers = await self._heavy.fetch(url)
```

检测优先级：Header 优先于 Body，避免误判。

### 2.3 代理健康追踪
- `max_failures=3` 标记代理不健康
- `recovery_interval=300s` 后自动尝试恢复
- 指数退避重试：`sleep(2^attempt + random)`
- 成功率加权：新延迟按 α=0.3 平滑更新平均延迟

---

## 三、Chat2API 现状分析

### 3.1 已有防检测能力

Chat2API 各 Provider Adapter **已经部分实现了类似策略**：

| Provider | 当前方案 | 已实现的技术 |
|----------|---------|------------|
| DeepSeek | curl_cffi + WASM PoW | JA3 模拟(chrome148)、Cookie 注入、Challenge 求解 |
| Perplexity | Electron net API | 绕过 Cloudflare TLS 检测、Cookie 注入 |
| Agnes | curl_cffi + axios fallback | JA3 模拟、双引擎回退 |
| Z.ai | axios + captcha_verify_param | Header 伪造、验证码参数注入 |
| Kimi/GLM/Mimo/MiniMax | axios/curl_cffi | 基础 UA + Header 伪造 |

### 3.2 现有 gaps

| Gap | 现状 | 影响 |
|-----|------|------|
| **无 Playwright 兜底** | 所有 adapter 均为 HTTP-level 请求，无浏览器自动化路径 | 遇到强 WAF（如 Cloudflare turnstile）时无降级方案 |
| **无通用 Challenge 检测** | 仅 DeepSeek 有 PoW challenge，无 WAF 类型识别 | 无法统一判断何时需要升级策略 |
| **Cookie 粒度粗糙** | 存为 provider 级别 credentials，无 domain-scoped cookie jar | 多域共享同一套 cookie，无法精准管理 |
| **无代理池管理** | 纯出站代理转发，无代理健康检测 | 代理故障时无法自动切换 |
| **无 JS 指纹规避** | 没有 Canvas/WebGL 噪声注入等反检测技巧 | 仅 HTTP 层伪装，JS 层仍可被识别 |
| **无 batch 模式** | 单次请求处理，无并发抓取能力 | 无法批量刷新 token 或维持活跃度 |

---

## 四、复用方案

### 4.1 方案 A：增量引入通用 WAF 检测层

在 `src/main/proxy/challengeDetector.ts` 新增通用 WAF 挑战检测器：

```typescript
// src/main/proxy/challengeDetector.ts
export enum ChallengeType {
  NONE, CLOUDFLARE, DATADOME, PERIMETERX, RECAPTCHA, HCAPTCHA, AKAMAI, IMPERVA
}

export function detectChallenge(html: string, headers: Record<string, string>): ChallengeType {
  const hl = html.toLowerCase()
  const hlk = Object.fromEntries(Object.entries(headers).map(([k,v]) => [k.toLowerCase(), v.toLowerCase()]))
  
  if (hlk['cf-ray'] || hlk['__cf_bm']) return ChallengeType.CLOUDFLARE
  if (hlk['x-datadome']) return ChallengeType.DATADOME
  // ... 更多规则
  return ChallengeType.NONE
}
```

**集成点**: 在 `forwarder.ts` 的所有 provider forward 方法中，解析响应 HTML 时调用 `detectChallenge()`，若返回非 NONE 则记录日志触发告警。

**改动范围**: 新增 1 个文件 (~80 行)，修改 `forwarder.ts` 约 5-10 行 × provider 数量。

---

### 4.2 方案 B：引入 Playwright 重引擎作为全局兜底

在 `src/main/proxy/stealthEngine.ts` 封装一个轻量的 Playwright 引擎，供所有 provider 按需使用：

```typescript
// src/main/proxy/stealthEngine.ts
import { chromium } from 'playwright'

export class StealthBrowserEngine {
  private browser: ReturnType<typeof chromium> | null = null
  private context: any = null

  async initialize() {
    this.browser = await chromium.launch({
      headless: true,
      args: [
        '--no-sandbox',
        '--disable-blink-features=AutomationControlled',
      ]
    })
    this.context = await this.browser.newContext({
      userAgent: 'Mozilla/5.0 ... Chrome/120.0.0.0',
      viewport: { width: 1920, height: 1080 },
    })
    await this.context.addInitScript(`
      Object.defineProperty(navigator, 'webdriver', {get: () => undefined});
    `)
  }

  async fetch(url: string): Promise<{ status: number; html: string }> {
    const page = await this.context.newPage()
    const response = await page.goto(url, { waitUntil: 'networkidle', timeout: 60000 })
    const html = await page.content()
    await page.close()
    return { status: response.status() ?? 200, html }
  }
}
```

**集成方式**: 在每个 adapter 的请求失败后，若配置了 `stealthEnabled`，则自动切换到此引擎。

**依赖变更**: 需添加 `playwright` 到 `dependencies`（约 300MB 下载）。

---

### 4.3 方案 C：增强 curl_cffi 的多 impersonate 策略

参考源项目的 User-Agent 池 + impersonate 轮换模式，在现有 adapter 基础上增加**动态 impersonate 选择**：

```typescript
// 在当前 adapter 中，根据目标站点动态选择 impersonate 版本
const IMPERSONATE_MAP: Record<string, string> = {
  'chat.deepseek.com': 'chrome148',
  'perplexity.ai': 'chrome120',    // 换用更老的版本绕过特定检测
  'chat.z.ai': 'chrome131',
  // ...
}

async function createSession(baseUrl: string): Promise<AsyncSession> {
  const impersonate = IMPERSONATE_MAP[new URL(baseUrl).hostname] || 'chrome120'
  return new AsyncSession(impersonate=impersonate, timeout=30)
}
```

**改动范围**: 仅在 adapters 中添加映射表，不动核心逻辑。符合"最小改动"原则。

---

### 4.4 方案 D：统一 Cookie 会话管理

借鉴源项目的 `SessionVault` SQLite 模式，为 Chat2API 增加**跨 provider 的细粒度 Cookie 管理**：

```typescript
// src/main/proxy/sessionVault.ts
import sqlite3 from 'sqlite3'
import { promisify } from 'util'

export class SessionVault {
  private db: sqlite3.Database

  async setCookie(domain: string, name: string, value: string, expires?: number) {
    // INSERT OR REPLACE INTO cookies (domain, name, value, expires, ...)
  }

  async getCookies(domain: string): Promise<Record<string, string>> {
    // SELECT WHERE domain=? AND (expires > NOW OR expires IS NULL)
  }

  async getValidToken(domain: string, type: string): Promise<string | null> {
    // SELECT token_value FROM tokens WHERE ... AND (expires > NOW OR expires IS NULL)
  }
}
```

**价值**: 
- 支持 Perplexity 等站点的 cookie 自动续期（目前依赖人工导入）
- 支持主动刷新 token 前保持会话活跃

---

### 4.5 方案 E：代理健康检测模块

若未来 Chat2API 支持用户配置出站代理，可引入 `ProxyPool` 模式：

```typescript
// src/main/proxy/proxyPool.ts
interface ProxyStatus {
  url: string
  healthy: boolean
  failCount: number
  avgLatencyMs: number
}

export class ProxyPool {
  private proxies: Map<string, ProxyStatus> = new Map()
  private index = 0

  async next(): Promise<string | null> {
    const available = [...this.proxies.values()]
      .filter(s => s.healthy || Date.now() - s.lastUsed > 300_000)
    if (!available.length) throw new Error('All proxies exhausted')
    const proxy = available[this.index++ % available.length]
    return proxy.url
  }

  markSuccess(proxy: string, latency: number) { /* 平滑更新延迟 */ }
  markFailed(proxy: string, error: string) { /* 累加失败计数 */ }
}
```

---

## 五、推荐实施路径

### Phase 1: 低风险高收益（立即实施）
1. **通用 Challenge 检测器**（方案 A）— 新增 `challengeDetector.ts`，集成到 forwarder，用于日志和统计
2. **动态 Impersonate 映射**（方案 C）— 为每个 adapter 优化 `impersonate` 版本匹配目标站点

### Phase 2: 中等投入
3. **Playwright 重引擎**（方案 B）— 作为全局兜底，仅在 Light Engine 遇到 `ChallengeType.CLOUDFLARE` 时使用
4. **Session Vault**（方案 D）— 支持跨请求的 fine-grained cookie 管理

### Phase 3: 可选扩展
5. **Proxy Pool**（方案 E）— 需要配合用户代理配置功能
6. **Batch Refresh** — 定期批量刷新所有 provider 的 token/cookie，防止过期

---

## 六、与项目现有原则的对齐

| CLAUDE.md 原则 | 本方案如何遵循 |
|---------------|--------------|
| Fail-Closed | Challenge 检测失败时返回错误而非降级，保持安全 |
| 契约单源 | `ChallengeType` 枚举和 `StealthConfig` 接口只在 `types.ts` 定义一次 |
| 最小改动 | Phase 1 仅新增 1 个文件 + forwarder 少量包装；不改现有 adapter 业务逻辑 |
| 逐步验证 | 每个 phase 单独可验证：先验证检测准确率，再验证 Playwright 兼容性 |

---

## 七、预期收益

| 指标 | 现状 | 预期改进 |
|------|------|---------|
| Cloudflare 绕过成功率 | ~70%（依赖 cookie 质量） | ~95%（Light→Heavy 自动升级） |
| Token 过期导致失败 | 频繁（需手动刷新） | 减少 80%（session vault 自动续期） |
| WAF 类型可观测性 | 无（仅日志） | 实时检测 + 告警分类 |
| 多代理容错 | 无 | 自动降级 + 健康轮询 |

---

## 八、风险与注意事项

1. **Playwright 体积**: 增加约 300MB 下载量，可通过可选安装（peer dependency）缓解
2. **Electron 兼容**: Playwright 在 Electron 中需要 `playwright-core` 而非完整版
3. **DeepSeek WASM**: 现有 `challenge.ts` 的 PoW 求解器独立工作，不受影响
4. **Z.ai captcha**: 验证码参数仍需用户手动输入，playwright 方案无法自动解决人机验证
5. **法律合规**: 所有防检测技术仅用于合法的 API 代理服务场景
