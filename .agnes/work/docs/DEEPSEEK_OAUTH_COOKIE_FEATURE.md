# DeepSeek OAuth Cookie 自动捕获与 TLS 伪装实现说明

## 已完成的修改

### 1. tokenExtractionConfig.ts
添加了 DeepSeek 的 Cookie 提取配置：
```typescript
deepseek: {
  loginUrl: 'https://chat.deepseek.com',
  tokenSources: [
    { type: 'localStorage', key: 'userToken' },
    { type: 'cookie', key: 'cookies' },  // 新增：收集所有 Cookie
  ],
  targetDomains: ['.deepseek.com', 'deepseek.com'],
  successUrlPatterns: [/chat\.deepseek\.com/i],
  windowTitle: 'DeepSeek Login',
},
```

### 2. inAppLogin.ts  
添加了 Cookie 合并逻辑，在登录成功后将所有 deepseek.com 的 Cookie 组合成字符串：
```javascript
if (source.key === 'cookies' && allCookies.length > 0) {
  const cookieString = allCookies
    .filter(c => c.domain?.includes('deepseek.com'))
    .map(c => `${c.name}=${c.value}`)
    .join('; ')
  this.emit('tokenFound', { key: 'cookies', value: cookieString })
}
```

### 3. deepseek.ts adapter
添加了 cookie 支持：
- 从 `account.credentials.cookies` 读取保存的 Cookie
- 在请求头中自动添加 `Cookie` 字段（当有 Cookie 时）

### 4. AddAccountDialog.tsx
- 添加了 `cookies` 字段的映射处理
- 编辑账户时会显示 Cookie 输入框（textarea 类型）

### 5. zh-CN.json
添加了中文翻译：
- `deepseek.cookies`: "Cookie 字符串"
- `deepseek.cookiesPlaceholder`: "从浏览器 DevTools 复制完整 Cookie 字符串"
- `deepseek.cookiesHelp`: "包含 cf_clearance、session 等 WAF 相关 Cookie"

---

## TLS 伪装方案

当前实现使用 **Cookie 复用** 作为基础反检测手段。如需完整的 TLS 指纹伪装，建议安装以下 npm 包之一：

### 方案 A: curl-cffi-node (推荐)
```bash
npm install curl-cffi-node
```
```typescript
import { Session } from 'curl-cffi-node'

const session = new Session({ impersonate: 'chrome131' })
const response = await session.post(url, {
  headers: { ...FAKE_HEADERS, Cookie: this.cookies },
  data: requestBody,
})
```

### 方案 B: wreq-js
```bash
npm install wreq-js
```
```typescript
import { wreq } from 'wreq-js'

const result = await wreq.post(url, {
  impersonate: 'chrome131',
  headers: { ...FAKE_HEADERS, Cookie: this.cookies },
  data: requestBody,
})
```

### 方案 C: tls-impersonate
```bash
npm install @httptoolkit/tls-impersonate
```

---

## 工作流程

1. 用户点击"OAuth 登录"标签
2. 点击"打开 OAuth 登录"按钮
3. 弹出浏览器窗口，用户完成登录
4. 系统自动提取：
   - `userToken` (从 localStorage)
   - `cookies` (从浏览器 Cookie 存储)
5. OAuth 成功后，凭据填入表单
6. 用户点击"添加账号"保存
7. 后续 API 请求自动使用保存的 Cookie

---

## 注意事项

1. **Cookie 有效期**: DeepSeek 的 Cookie 可能有时效性，过期后需要重新登录
2. **多账号支持**: 每个账号独立保存 Cookie，round-robin 轮询使用
3. **安全存储**: Cookie 与 Token 一样加密存储在 accounts.json 中
