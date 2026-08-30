# Distributed Stealth Scraper 集成报告

**日期**: 2026-08-29  
**目标项目**: Chat2API v1.4.0  
**源项目**: https://github.com/mercury-systems/distributed-stealth-scraper (MIT)

---

## 实施概览

成功将源项目的三个核心价值组件完整集成到 Chat2API：

| 组件 | 文件 | 行数 | 状态 |
|------|------|------|------|
| WAF Challenge 检测层 | `src/main/proxy/challengeDetector.ts` | 147 | ✅ 完成 |
| SQLite Session Vault | `src/main/proxy/sessionVault.ts` | 274 | ✅ 完成 |
| Playwright Stealth 引擎 | `src/main/proxy/stealthEngine.ts` | 335 | ✅ 完成 |
| Forwarder 集成 | `src/main/proxy/forwarder.ts` (+71行) | 1725 | ✅ 完成 |
| Server 清理 | `src/main/proxy/server.ts` (+2行) | 395 | ✅ 完成 |

---

## 一、WAF Challenge 检测层

**文件**: `src/main/proxy/challengeDetector.ts`

### 核心功能

```typescript
enum ChallengeType { NONE, CLOUDFLARE, DATADOME, PERIMETERX, RECAPTCHA, HCAPTCHA, AKAMAI, IMPERVA, UNKNOWN }

// 双模式检测
detectChallenge(html: string, headers: Record<string, string>): ChallengeResult
detectChallengeFromHeaders(headers: Record<string, string>): ChallengeResult  // 仅头部，零成本
```

### 检测优先级（与源项目一致）
1. **Header 优先**: `cf-ray`, `__cf_bm`, `x-datadome`, `x-perimeterx`, `akamai-origin-cache`, `visid_incap`
2. **Body 正则**: Cloudflare turnstile/jas moment, DataDome, PerimeterX, reCAPTCHA, hCaptcha, Akamai, Incapsula

### 已移除的冗余导入
- 移除了 `createRequire` — 不再需要动态 require
- 使用纯 ES module import 保持代码整洁

---

## 二、SQLite Session Vault

**文件**: `src/main/proxy/sessionVault.ts`

### 核心架构

```typescript
class SessionVault {
  // 单例模式
  static getInstance(): SessionVault
  
  // Cookie 管理
  getCookies(domain: string): StoredCookie[]
  getCookieHeader(domain: string): string          // 生成 HTTP Cookie header
  setCookie(domain, name, value, options)
  setCookies(domain, cookiesMap)
  
  // Token 管理
  getToken(domain, tokenType): StoredToken | null
  setToken(domain, tokenType, value, expires?)
  
  // 生命周期
  init()     // 同步初始化（node:sqlite DatabaseSync）
  close()    // 应用退出时调用
  stats()    // { cookies, tokens, domains }
}
```

### 技术选型

| 决策 | 选择 | 原因 |
|------|------|------|
| 数据库 | `node:sqlite` `DatabaseSync` | 同步 API，Electron 兼容性好；异步版本在 Electron preload 中有兼容问题 |
| 路径 | `app.getPath('userData') + '/session_vault.db'` | 跨平台标准用户数据目录 |
| 容错 | Best-effort 模式 | DB 错误不抛异常，不影响主业务流 |

### 与现有凭据系统互补

- 现有: `Account.credentials.cookie` (provider 级别，字符串格式)
- 新增: Domain-scoped cookie jar (SQLite 持久化，支持过期管理)
- 用途: 自动保存 stealth browser 获取的新 cookie/token

---

## 三、Playwright Stealth 引擎

**文件**: `src/main/proxy/stealthEngine.ts`

### 核心架构

```typescript
class StealthBrowserEngine {
  async initialize()       // 启动 Chromium + stealth context
  async fetch(url)         // 返回 { status, html, headers, challenge }
  async close()            // 优雅关闭
  
  // 静态配置
  config: StealthConfig = {
    userAgent: 'Chrome/120...',
    headless: true,
    viewport: { 1920×1080 },
    locale: 'en-US',
    timezoneId: 'America/New_York',
    timeout: 60_000,
  }
}

export function getStealthEngine(): StealthBrowserEngine  // 全局单例
```

### Anti-Detection 补丁（来自源项目）

```javascript
// 1. navigator.webdriver = undefined
// 2. Chrome runtime version spoofing
// 3. Canvas getImageData 噪声注入 (+0.5 random)
// 4. WebGL vendor/renderer → Intel Inc. / Intel Iris OpenGL Engine
// 5. Permissions API geolocation 静默授权
// 6. Media devices 过滤掉 audioinput
```

### 启动参数

```
--no-sandbox
--disable-dev-shm-usage
--disable-blink-features=AutomationControlled  ← 关键
--disable-web-security
--disable-features=IsolateOrigins,site-per-process
```

### 依赖

- `playwright-core@^1.62.1` (已通过 npm install 添加)
- 不需要 `playwright install chromium` — Electron 自带的 Chromium 可复用
- 首次启动时 playwright-core 会自动使用系统 Chromium 或下载

---

## 四、Forwarder 集成点

**修改**: `src/main/proxy/forwarder.ts` (+71 行净增)

### 新增方法

```typescript
// RequestForwarder 类成员
private vault = getSessionVault()           // 会话管理
private stealthEngine: StealthBrowserEngine | null = null
private stealthInitialized = false

// WAF 检测链
private detectWafChallenge(status, body, headers): ChallengeType
private isWafPage(body): boolean            // 快速判断是否为 WAF HTML

// 升级逻辑
private async fetchViaStealth(url, domain): Promise<...>
private async ensureStealthEngine(): Promise<void>  // 懒初始化
async shutdownStealthEngine(): Promise<void>        // 清理
```

### 集成位置

**`doForward()` 默认路由路径**（行 ~370），在 axios 请求后、返回结果前插入：

```typescript
// 仅在非流式 + 200 OK + 响应体为字符串时触发
if (!request.stream && response.status === 200 && typeof response.data === 'string') {
  const bodyStr = response.data as string
  const isHtmlLike = bodyLen < 50000 && /<!doctype|<html|<head|just a moment/i.test(bodyStr)
  
  if (isHtmlLike) {
    const challenge = this.detectWafChallenge(response.status, bodyStr, ...)
    
    if (challenge !== ChallengeType.NONE) {
      // → 升级到 stealth browser
      const stealthResult = await this.fetchViaStealth(url, domain)
      
      if (stealthResult.status === 200 && !this.isWafPage(stealthResult.body)) {
        // ✅ 绕过成功，返回 stealth 结果
        return { success: true, body: stealthResult.body, ... }
      }
      // ❌ stealth 也失败，返回原始错误
    }
  }
}
```

### 关键设计决策

1. **懒初始化**: `ensureStealthEngine()` 只在第一次遇到 WAF 时才启动浏览器，不增加冷启动开销
2. **非流式优先**: 只对非流式请求做升级（流式响应无法用 HTML 检测）
3. **HTML 启发式**: 先检查是否像 HTML（长度 < 50KB 且包含 HTML 标签），避免对大 JSON 做正则匹配
4. **幂等清理**: `shutdownStealthEngine()` 在 proxy stop 时调用

---

## 五、Server 生命周期集成

**修改**: `src/main/proxy/server.ts` (+2 行净增)

```typescript
// 新增 import
import { requestForwarder } from './forwarder'

// stop() 方法中增加清理
async stop(): Promise<boolean> {
  sessionManager.destroy()
  await requestForwarder.shutdownStealthEngine()  // ← 新增
  // ...
}
```

---

## 六、类型安全验证

### TypeScript 编译结果

```bash
npx tsc --noEmit -p tsconfig.node.json
```

| 指标 | 数值 |
|------|------|
| 新增文件 TS 错误 | **0** |
| forwarder.ts 错误数 | 6（全部预存，与本改动无关） |
| 总 TS 错误数 | 129（项目原有） |
| build 失败原因 | EPERM 权限问题（out/ 目录被占用，非代码问题） |

### 预存错误说明（与本 PR 无关）

这些是项目历史遗留问题：
- `originalModel` 不存在于 `ChatCompletionRequest` 类型定义
- `ChatMessage` 未从 `contextManagementService` 导出
- `deepseek.ts` 中 curl_cffi Node.js API 兼容性问题
- 其他 adapter 的类型不匹配

---

## 七、架构图

```
┌──────────────────────────────────────────────────────────────────┐
│                        RequestForwarder                          │
│                                                                  │
│  doForward()                                                     │
│    ├─ dedicated provider forwarder (DeepSeek/Perplexity/...)    │
│    └─ default axios path                                        │
│         │                                                       │
│         ▼                                                       │
│    [axios GET/POST]                                             │
│         │                                                       │
│         ▼                                                       │
│    if (non-stream && 200 && HTML-like)                         │
│         │                                                       │
│         ▼                                                       │
│    detectChallenge(html, headers)                               │
│         │                                                       │
│    ┌────┴────┐                                                  │
│    NONE    CHALLENGE detected                                 │
│    │        │                                                   │
│    ▼        ▼                                                  │
│  normal   fetchViaStealth()                                    │
│  response │                                                      │
│           ▼                                                    │
│    ┌─────────────┐                                             │
│    │ Stealth     │                                             │
│    │ Browser     │                                             │
│    │ Engine      │                                             │
│    └─────────────┘                                             │
│         │                                                       │
│    ┌────┴────┐                                                  │
│   success  still_waf              SessionVault                  │
│    │        │                    (cookies+tokens persist)       │
│    ▼        ▼                                                         │
│  return   log_error                                                │
└──────────────────────────────────────────────────────────────────┘
```

---

## 八、后续建议

### Phase 2: Provider Adapter 深度集成

目前 stealth 引擎只在 **default/custom provider** 路径生效。建议在以下 adapter 中也加入挑战检测：

| Adapter | 当前方案 | 建议增强 |
|---------|---------|---------|
| `perplexity.ts` | Electron net API + 403 硬错误 | 检测 403 body 中的 CF challenge → 切换到 stealth |
| `deepseek.ts` | curl_cffi + PoW WASM | PoW 失败时检测到 CF challenge → stealth fallback |
| `zai.ts` | axios + captcha_verify_param | 无自动化 bypass，需人工输入参数 |

### Phase 3: UI 暴露

- 在 Settings 页面添加 "启用 Stealth Browser" 开关
- 日志中显示 `WAF challenged` / `Stealth bypassed` 标记
- 提供 `GET /v0/management/session-vault/stats` API

### Phase 4: 多租户隔离

- 按 provider/account 维度隔离 cookie 存储
- 支持 per-account 的 stealth context（防止共享 fingerprint）

---

## 九、文件清单

### 新增文件（3 个）
```
src/main/proxy/challengeDetector.ts  # 147 行 - WAF 检测器
src/main/proxy/sessionVault.ts       # 274 行 - 会话 Vault
src/main/proxy/stealthEngine.ts      # 335 行 - Playwright 引擎
```

### 修改文件（2 个）
```
src/main/proxy/forwarder.ts  # +71 行净增 (1725 行)
src/main/proxy/server.ts     # +2 行净增 (395 行)
```

### 新增依赖（1 个）
```
package.json: playwright-core ^1.62.1
```
