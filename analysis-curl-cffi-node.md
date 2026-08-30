# curl-cffi-node 库深度分析

## 概述

**curl-cffi-node**（npm 包名：`curl-cffi`）是一个基于 libcurl 的高性能 Node.js HTTP 客户端库，核心卖点是 **浏览器指纹模拟** 能力。它源自 Python 生态的知名项目 [curl_cffi](https://github.com/lexiforest/curl_cffi)，后者又基于 [curl-impersonate](https://github.com/lexiforest/curl-impersonate) 补丁。

项目地址：https://github.com/fssl168/curl-cffi-node（实际维护者为 `tocha688`）

---

## 一、架构设计

### 1.1 整体分层

```
┌─────────────────────────────────────────────┐
│  Public API                                  │
│  req / CurlRequest / CurlRequestMulti        │
│  CurlSession / CurlRequestSync               │
├─────────────────────────────────────────────┤
│  RequestClientBase (抽象基类)                │
│  - 拦截器(axios-like)                        │
│  - 参数合并、URL拼接                          │
│  - CORS预检、重试封装                         │
│  - CurlPool 连接复用                         │
├─────────────────────────────────────────────┤
│  Core: CurlPool                              │
│  - 连接池管理（maxSize / idleTTL）           │
│  - acquire / release / prune / close         │
├─────────────────────────────────────────────┤
│  Helper: setRequestOptions / parseResponse   │
│  - 将所有选项转换为 libcurl setOption 调用   │
│  - 解析响应头栈（支持重定向追踪）            │
├─────────────────────────────────────────────┤
│  Impl: request() / requestSync()             │
│  - 同步模式: curl.performSync()              │
│  - 异步模式: curl.perform()                  │
│  - Multi模式: CurlMultiImpl (event-driven)   │
├─────────────────────────────────────────────┤
│  Native: @tocha688/libcurl                   │
│  - libcurl 的 Node.js NAPI 绑定              │
│  - Curl / CurlMulti / CurlOpt 等底层 API     │
├─────────────────────────────────────────────┤
│  Native Binary: libcurl-impersonate          │
│  - 运行时自动下载（scripts/install.cjs）      │
│  - 按平台/架构分发包                          │
│  - CA 证书自动下载                            │
└─────────────────────────────────────────────┘
```

### 1.2 核心文件结构

| 文件 | 职责 |
|------|------|
| `src/index.ts` | 顶层导出入口 |
| `src/request/global.ts` | 全局初始化 + 全局请求实例单例 |
| `src/request/BaseClient.ts` | 提供 get/post/put/delete/head/options 便捷方法 |
| `src/request/RequestClientBase.ts` | 抽象基类，统一请求流程 |
| `src/request/CurlRequest.ts` | 单请求客户端（默认 CurlPool 模式） |
| `src/request/CurlRequestMulti.ts` | 批量客户端（CurlMultiImpl 模式） |
| `src/request/session.ts` | Session 客户端（自带 CookieJar） |
| `src/core/CurlPool.ts` | Curl 连接池实现 |
| `src/helper.ts` | setRequestOptions + parseResponse 核心逻辑 |
| `src/impl/request_sync.ts` | 同步执行层 |
| `src/impl/request_async.ts` | 异步执行层 |
| `src/impl/curl_multi_timer.ts` | CurlMulti 事件驱动封装 |
| `src/app.ts` | 平台检测 + libcurl 路径解析 |
| `src/utils.ts` | URL构建、cookie解析、HTTP版本转换 |
| `src/logger.ts` | 分级日志工具 |
| `src/type/*.ts` | TypeScript 类型定义 |
| `scripts/install.cjs` | 安装脚本：下载 native binary + CA 证书 |

---

## 二、核心特性详解

### 2.1 浏览器指纹模拟（核心卖点）

```typescript
const response = await req.get("https://example.com", {
  impersonate: "chrome136",  // 模拟 Chrome 136 的 TLS/JA3/HTTP2 指纹
});
```

工作原理：
- 底层使用 **libcurl-impersonate** 补丁编译的二进制，该补丁修改了 libcurl 的 TLS 握手行为
- 可以伪造 JA3 指纹、TLS 扩展顺序、ALPN 协议协商顺序等
- 支持的浏览器模式：`chrome131`, `chrome136`, `firefox135`, `safari18_0`, `edge21` 等
- `defaultHeaders: true` 时还会自动设置浏览器风格的 HTTP 请求头

### 2.2 三种请求模式

**① 单请求模式（CurlRequest）**
```typescript
const client = new CurlRequest();
const res = await client.get("https://api.example.com/data");
```
- 使用 CurlPool 管理 Curl handle
- 默认自动复用连接（keepAlive）
- 适合大部分场景

**② 多请求模式（CurlRequestMulti）**
```typescript
const client = new CurlRequestMulti({ impl: new CurlMultiImpl() });
const results = await client.batch([
  { url: "https://a.com" },
  { url: "https://b.com" },
]);
```
- 基于 libcurl multi interface 的事件循环
- 适合高并发批量请求
- 内部通过 `CurlMultiTimer` 管理 timer socket 回调

**③ Session 模式（CurlSession）**
```typescript
const session = new CurlSession();
// 自动携带 cookie jar，跨请求保持登录态
await session.get("https://example.com/login");
await session.get("https://example.com/profile");
```
- 继承自 CurlRequest
- 构造时自动注入 `CookieJar`（使用 tough-cookie）
- 适合需要状态保持的场景（爬虫、自动化）

### 2.3 请求拦截器系统

与 axios 类似的拦截器模式：

```typescript
client.interceptors.request.use(async (opts) => {
  // 修改请求前...
  return opts;
});

client.interceptors.response.use(
  (res) => res,                          // fulfilled
  (err, res) => ({ ...res, status: 200 }) // rejected handler
);
```

支持请求拦截和响应拦截两个方向，均支持 async 处理函数。

### 2.4 连接池（CurlPool）

```typescript
type CurlPoolOptions = {
  maxSize?: number;    // 最大连接数，默认无限制
  idleTTL?: number;    // 空闲连接存活时间(ms)，默认 60s
};
```

- `acquire()`: 查找空闲 handle → 池满则新建 → 超 max 则创建临时 handle
- `release()`: 标记为空闲，记录 lastUsed
- `prune()`: 定期清理超过 TTL 的空闲连接
- 临时 handle（超出 maxSize 时创建的）不在池中，使用后直接关闭

### 2.5 自动重试

```typescript
const res = await req.get(url, { retryCount: 3 });
```

在 `withRetry()` 封装中实现，失败后自动重试指定次数。

### 2.6 CORS 预检支持

```typescript
const res = await req.options(url, { cors: true });
```

当 `cors: true` 时，先发送 OPTIONS 预检请求，再执行实际请求。

---

## 三、安装与运行时机制

### 3.1 安装流程（scripts/install.cjs）

```bash
npm install curl-cffi
# 自动执行 postinstall 脚本
```

安装脚本做的事情：
1. 检测平台+架构（x86_64-win32 / aarch64-linux-gnu / arm64-macos 等）
2. 从 GitHub releases 查找匹配的 libcurl-impersonate 预编译包
3. 下载并解压到 `node_modules/curl-cffi/libs/` 目录
4. 同时下载 CA 证书到同目录
5. 更新 `libcurl.config.json` 记录实际版本

**设计亮点**：
- 使用独立 JSON 配置文件避免 ESM/CJS 模块加载差异
- GitHub API 查询失败时回退到配置版本直接构造 URL
- 已存在相同版本目录时跳过下载

### 3.2 原生绑定

依赖 `@tocha688/libcurl` — 这是作者维护的 libcurl Node.js NAPI 绑定包，提供了：
- `Curl` 类：单个 curl easy handle
- `CurlMulti` 类：multi interface
- `CurlOpt` 枚举：所有 curl 选项
- `CurlInfo` 枚举：所有 curl 信息获取
- `globalInit()` / `globalCleanup()`：libcurl 全局初始化和清理

### 3.3 libcurl-impersonate 版本

当前配置为 `v1.5.6`，来自 [lexiforest/curl-impersonate](https://github.com/lexiforest/curl-impersonate) 项目的 GitHub releases。

---

## 四、配置选项详解

### 4.1 RequestOptions 完整列表

```typescript
{
  method?: 'GET' | 'POST' | 'PUT' | 'DELETE' | 'PATCH' | 'HEAD' | 'OPTIONS';
  url?: string;
  params?: Record<string, any>;      // URL 查询参数
  data?: Record<string, any> | string | Buffer | null;  // 请求体
  jar?: CookieJar;                   // tough-cookie 的 CookieJar
  headers?: Record<string, string>;
  auth?: { username: string; password: string };
  timeout?: number;                  // 超时毫秒，默认 30000
  allowRedirects?: boolean;          // 跟随重定向，默认 true
  maxRedirects?: number;             // 最大重定向数，默认 30
  proxy?: string;                    // http://user:pass@host:port
  referer?: string;
  acceptEncoding?: string;           // 默认 'gzip, deflate, br, zstd'
  impersonate?: CURL_IMPERSONATE;    // 浏览器指纹模式
  ja3?: string;                      // 自定义 JA3 指纹
  akamai?: string;                   // Akamai HTTP/2 指纹
  defaultHeaders?: boolean;          // 是否应用浏览器默认头
  httpVersion?: "v1" | "v2" | "v3";  // HTTP 版本
  cert?: string | { cert: string; key: string };  // 客户端证书
  verify?: boolean;                  // SSL 验证开关
  maxRecvSpeed?: number;             // 最大接收速率 (bytes/s)
  curlOptions?: Record<CurlOpt, any>; // 直接传递原始 curl 选项
  ipType?: 'ipv4' | 'ipv6' | 'auto';
  impl?: CurlMultiImpl;              // 使用 multi 模式
  retryCount?: number;               // 重试次数
  keepAlive?: boolean;               // 连接复用
  sync?: boolean;                    // 强制同步模式
  dev?: boolean;                     // 开启 verbose 调试
  cors?: boolean;                    // CORS 预检
}
```

### 4.2 Response 结构

```typescript
{
  url: string;              // 最终 URL（含重定向后）
  status: number;           // HTTP 状态码
  dataRaw: Buffer;          // 原始响应体
  headers: HttpHeaders;     // 响应头对象
  request: CurlRequestInfo; // 发起的请求信息
  options: RequestOptions;  // 使用的配置
  stacks: Array<CurlRequestInfo>;  // 所有重定向步骤的请求记录
  index: number;            // 当前在 stacks 中的索引
  redirects: number;        // 重定向次数
  text: string;             // 文本格式响应
  data: any;                // 自动解析 JSON 或返回原始数据
  curl: Curl;               // 底层 Curl 对象引用
  jar: CookieJar;           // 关联的 CookieJar
}
```

---

## 五、与 Chat2API 项目的潜在结合点

基于对 curl-cffi-node 的理解，以下是它与 Chat2API 项目的可能结合方式：

### 5.1 Provider 认证阶段替代 fetch

Chat2API 的 OAuth 登录流程（`src/main/oauth/`）目前可能使用 Node.js 内置 `fetch` 或 `axios`。curl-cffi-node 可以提供：
- **更好的反检测能力**：在登录 DeepSeek/Kimi/Qwen 等平台时模拟真实浏览器指纹
- **原生 HTTP/2 支持**：比 Node.js 内置 fetch 更完善的 h2 实现
- **精确的 TLS 控制**：对于有严格指纹校验的上游平台尤为重要

### 5.2 自定义 Provider 的快速接入

新增 Provider 时，可以用 curl-cffi-node 替代手写的 curl 请求，因为：
- 内置 `impersonate` 参数一行搞定指纹
- 自动处理 cookies（CookieJar 集成）
- 支持同步/异步两种模式

### 5.3 注意事项

- **二进制体积**：每个平台都会下载 ~10MB 的 libcurl-impersonate 二进制，需要评估对用户安装包的影响
- **安装脚本依赖网络**：首次安装需要联网下载 native binary
- **Node.js 版本要求**：需要支持 NAPI 的较新版本

---

## 六、代码质量评价

### 优点
1. **分层清晰**：RequestClientBase → CurlRequest/CurlRequestMulti → Impl → Native，职责分离明确
2. **拦截器设计**：axios-style interceptor 增加了可扩展性
3. **连接池管理**：CurlPool 实现了完整的生命周期（acquire/release/prune/close）
4. **安装脚本健壮**：多重回退策略保证不同环境下都能正常工作
5. **类型安全**：全面的 TypeScript 类型定义

### 可改进之处
1. **错误处理**：部分 catch 块过于宽松（`catch { /* ignore */ }`），可能掩盖问题
2. **CurlPool 超配行为**：超出 maxSize 时创建临时 handle 但不追踪，可能泄露资源
3. **缺少单元测试覆盖说明**：GitHub 上测试文件较少可见
4. **内存泄漏风险**：CurlMultiImpl 的 `close()` 会 reject 所有 pending promise，但 `processData()` 中的异常处理不够完善

---

## 总结

curl-cffi-node 是一个设计良好的 libcurl wrapper，核心差异化价值在于**浏览器指纹模拟**和**高性能连接复用**。其架构借鉴了 axios 的拦截器模式和成熟的连接池管理，适合用于：

1. 需要绕过基础反爬检测的爬虫场景
2. 对 TLS 指纹敏感的 API 调用
3. 高并发的批量 HTTP 请求
4. 需要 session 保持的自动化测试

对于 Chat2API 项目，最值得关注的用途是在 **Provider OAuth 登录环节**使用其指纹模拟能力，提高登录成功率。

---

Sources:
- [curl-cffi-node README](https://github.com/fssl168/curl-cffi-node/blob/main/README.md)
- [curl-cffi-node Chinese README](https://github.com/fssl168/curl-cffi-node/blob/main/README.zh.md)
- [curl_cffi (Python original)](https://github.com/lexiforest/curl_cffi)
- [curl-impersonate (libcurl patch)](https://github.com/lexiforest/curl-impersonate)
