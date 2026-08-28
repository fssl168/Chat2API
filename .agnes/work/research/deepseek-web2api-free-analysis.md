# DeepSeek Web2API-Free 项目分析报告

**目标仓库**: https://github.com/snake-aabb-wtf/deepseek-web2api-free  
**分析日期**: 2026-08-28  
**分析者**: AgnesCode AI Agent

---

## 1. 项目概览

| 属性 | 值 |
|------|-----|
| 名称 | deepseek-web2api-free |
| 语言 | Python |
| 许可证 | The Unlicense (Public Domain) |
| Stars | ~99 (快速增长中) |
| Forks | 21 |
| 创建时间 | 2026-05-07 |
| 最后更新 | 2026-08-11 |
| 作者 | snake-aabb-wtf |

### 核心定位
基于 FastAPI 的 **DeepSeek Chat 反向代理服务器**，将 DeepSeek 网页版 (chat.deepseek.com) 的私有 API 转换为 **OpenAI 兼容格式** + **Anthropic Claude 兼容格式**。

> ⚠️ 关键声明: "本项目没有一行人工手写代码。所有的 API 端点设计、协议逆向、PoW 求解、SSE 解析、格式映射、文档编写等等全部由 DeepSeek v4 Flash 模型 + Claude Code 协作完成。"

---

## 2. 技术架构

### 2.1 整体分层

```
┌─────────────────────────────────────────────────────┐
│                 Client Layer                         │
│   OpenAI SDK / Anthropic SDK / curl / CLI tools     │
└─────────────────────────────────────────────────────┘
                          │
                          ▼
┌─────────────────────────────────────────────────────┐
│                FastAPI Server (server.py)            │
│  - /v1/chat/completions  (OpenAI format)           │
│  - /v1/messages          (Anthropic format)        │
│  - /v1/models            (model list)              │
│  - /health, /admin/*     (management)              │
│  - /webui/               (React SPA)               │
└─────────────────────────────────────────────────────┘
                          │
                          ▼
┌─────────────────────────────────────────────────────┐
│              Account Pool (account_pool.py)          │
│  - Multi-account management                        │
│  - Round-robin selection                           │
│  - State tracking: idle/busy/error                 │
│  - Auto-recovery with background health checks     │
│  - Credential encryption (Fernet/AES)              │
└─────────────────────────────────────────────────────┘
                          │
                          ▼
┌─────────────────────────────────────────────────────┐
│            DeepSeek Adapter (adapter.py)             │
│  - TLS fingerprint impersonation (curl_cffi)       │
│  - PoW challenge solving (WASM-based)              │
│  - Session management                              │
│  - Stream parsing (SSE → OpenAI format)            │
│  - DSML tool call injection                        │
│  - Expert mode (thinking + search)                 │
└─────────────────────────────────────────────────────┘
                          │
                          ▼
┌─────────────────────────────────────────────────────┐
│         DeepSeek Chat Backend (chat.deepseek.com)    │
└─────────────────────────────────────────────────────┘
```

### 2.2 核心模块

| 文件 | 职责 | 关键技术 |
|------|------|----------|
| `server.py` | FastAPI 入口，路由注册，CORS，鉴权中间件 | FastAPI, Uvicorn |
| `adapter.py` | 与 DeepSeek Chat 后端通信的核心适配器 | curl_cffi, WASM, SSE |
| `account_pool.py` | 多账号池管理，状态追踪，自动恢复 | threading, JSON, Fernet |
| `model_router.py` | 请求级 model→mode 路由决策 | 配置驱动 |
| `tool_dsml.py` | DSML (DeepSeek Markup Language) 工具调用注入 | Prompt Injection |
| `tool_sieve.py` | StreamSieve，实时过滤 DSML 标签 | Generator Pattern |
| `anthropic_format.py` | Anthropic Messages API 格式转换 | Response Format |
| `crypto.py` | 凭证加密存储 (Fernet + AES) | cryptography |
| `token_counter.py` | Token 计数估算 (tiktoken + fallback) | tiktoken |
| `rate_limiter.py` | IP/API Key 限速器 | sliding window |
| `session_cache.py` | 会话缓存管理 | TTL Cache |

### 2.3 依赖栈

```
fastapi>=0.115.0          # Web framework
uvicorn>=0.30.0           # ASGI server
curl_cffi>=0.7.1          # TLS fingerprint impersonation
wasmtime>=15.0.0          # WASM runtime for PoW
python-dotenv>=1.0.0      # Env config
httpx2>=2.0.0             # Test client transport
cryptography>=42.0.0      # Credential encryption
tiktoken>=0.7.0           # Token estimation
pytest>=8.0.0             # Testing
```

---

## 3. 核心功能分析

### 3.1 请求转换链路

1. **接收 OpenAI 格式请求** → `/v1/chat/completions`
2. **Model 路由决策** → `model_router.py` 判断 default/expert 模式
3. **Token 估算** → `token_counter.py` 计算 usage (若 upstream 不返回)
4. **账号获取** → `account_pool.acquire()` round-robin 分配
5. **会话管理** → `session_cache.py` 维护上下文
6. **适配器调用** → `adapter.chat_completion()` 
   - 构建请求头 (TLS fingerprint + Chrome headers)
   - 处理 WAF 挑战 (Cloudflare/AWS WAF)
   - 发送 DeepSeek Chat 私有 API 请求
7. **响应解析**
   - SSE 流式解析 → 转换为 OpenAI format
   - DSML 工具调用注入 → 实时过滤 → 输出标准 function_call
8. **统计记录** → `stats_history.py` 记录 latency, tokens, errors

### 3.2 关键技术实现

#### TLS 指纹伪装 (Anti-Detection)
```python
# adapter.py 核心升级
IMPERSONATE = "chrome131"  # TLS/JA3 impersonation via curl_cffi
# Header set captured from real Chrome 149 + chat.deepseek.com session
headers = {
    'User-Agent': 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) ...',
    'Accept': '*/*',
    'Accept-Language': 'zh-CN,zh;q=0.9,en;q=0.8',
    'Origin': 'https://chat.deepseek.com',
    'Referer': 'https://chat.deepseek.com/',
}
```

#### PoW 求解 (WASM-based)
```python
# 使用 wasmtime 加载 sha3_wasm_bg.wasm
# 自动求解 DeepSeekHashV1 难度挑战
_WASM_PATH = Path(__file__).resolve().parent / "sha3_wasm_bg.wasm"
with open(_WASM_PATH, "rb") as f:
    _WASM_BYTES = f.read()

instance = Instance(store, module, [pow_trapfunc])
# 自动寻找 nonce 满足哈希难度
```

#### DSML 工具调用注入
```python
# tool_dsml.py - 通过 prompt injection 实现 function calling
# 将 DSML 标签注入到用户 prompt 中
build_dsml_tool_prompt(tools, messages)

# tool_sieve.py - StreamSieve 实时过滤 DSML 标签
# 在 SSE 流中解析 <DSML:tool_call>...</DSML> 并转换为标准 format
```

#### 多账号池管理
```python
# account_pool.py - 线程安全账号池
class AccountPool:
    def acquire(self) -> Optional[Account]:
        # round-robin 选择 idle 账号
        # 自动标记 busy/error
        
    def mark_error(self, acct, error_msg):
        # 错误计数 + 后台健康检查恢复
        if error_count >= 3:
            threading.Thread(target=self._background_recover).start()
```

---

## 4. 安全风险分析

### 4.1 高优先级风险

| 风险 | 说明 | 影响 |
|------|------|------|
| **凭证明文存储** | `.env` 中的 token/cookies 可能明文存储 | 账户被盗用 |
| **无加密默认开启** | `DEEPSEEK_ENCRYPTION_KEY` 可选启用 | 数据泄露 |
| **WAF 绕过** | 项目明确用于绕过 DeepSeek 的反爬机制 | 法律风险 |
| **Cookie 复用** | 使用用户真实 session cookie | 账户封禁 |

### 4.2 中优先级风险

| 风险 | 说明 | 影响 |
|------|------|------|
| **无审计日志** | 账号操作缺乏完整审计 | 责任追溯困难 |
| **WebUI 密码弱** | 默认 admin 密码 | 管理面板被入侵 |
| **无 API 密钥轮换** | 长期有效密钥 | 密钥泄露风险累积 |

### 4.3 合规风险

> ⚠️ **严重声明**: 
> - 本项目属于 **非官方逆向工程**，违反 DeepSeek 服务条款
> - 使用此项目可能触发 Cloudflare/AWS WAF 封禁
> - 生产环境使用可能导致 **账户封禁**
> - 免责声明: "仅限学习研究使用，不保证稳定性"

---

## 5. 与 Chat2API 对比

| 特性 | deepseek-web2api-free | Chat2API (本项目) |
|------|------------------------|-------------------|
| 框架 | FastAPI (Python) | Electron + Koa (Node.js) |
| 部署 | 本地服务器 | 桌面应用 |
| 认证 | Cookie/Token 复用 | 官方 API Key |
| 多账号 | ✅ 支持池化管理 | ✅ 支持多 provider |
| OAuth | ❌ 不支持 | ✅ 支持各 provider OAuth |
| 跨平台 | Python (跨平台) | macOS/Windows/Linux |
| 反检测 | curl_cffi TLS 伪装 | 无 (官方 API) |
| PoW 求解 | ✅ WASM-based | N/A |
| 合规性 | ⚠️ 灰色地带 | ✅ 官方授权 |
| 安全性 | 🔴 凭证明文存储 | 🟢 加密存储 |

---

## 6. 项目健康度评估

### 6.1 活跃度
- **最近提交**: 2026-08-11 (约 2 周前)
- **持续维护**: ✅ 有更新
- **Issues**: 1 个 open issue
- **Stars 增长**: 38 → 99 (快速上升)

### 6.2 代码质量
- **测试覆盖**: ✅ 有 tests/ 目录
  - test_account_pool.py
  - test_adapter.py
  - test_dsml.py
  - test_rate_limiter.py
- **CI**: ✅ GitHub Actions (ci.yml)
- **类型注解**: 部分支持

### 6.3 文档完整性
- **README.md**: ✅ 详细中文文档
- **AGENTS.md**: ✅ 架构说明
- **docs/release-notes/**: ✅ v2.0.0 - v3.3.1 版本记录
- **API 文档**: ✅ 包含在 README 中

---

## 7. 关键代码片段解读

### 7.1 Adapter 核心逻辑 (adapter.py)

```python
class DeepSeekAdapter:
    def __init__(self, token, cookies, proxy=None):
        self.session = cffi_requests.Session(
            impersonate=IMPERSONATE,  # chrome131
            proxies={"http": proxy, "https": proxy} if proxy else None,
        )
        
    def create_session(self):
        """创建 DeepSeek Chat 会话"""
        resp = self.session.post(
            f"{BASE_URL}/api/chat-session/create",
            headers={
                "Authorization": f"Bearer {self.token}",
                "Cookie": self.cookies,
            },
        )
        return resp.json()
    
    def chat_completion(self, request, account):
        """发送请求并解析响应"""
        # 1. PoW 挑战求解
        # 2. 构建请求体
        # 3. 发送 SSE 流
        # 4. 解析并转换为 OpenAI format
```

### 7.2 Account Pool 核心逻辑 (account_pool.py)

```python
class AccountPool:
    def __init__(self):
        self._lock = threading.Lock()
        self._accounts: list[Account] = []
        self._next_idx = 0  # round-robin index
        
    def acquire(self) -> Optional[Account]:
        with self._lock:
            for _ in range(len(self._accounts)):
                idx = self._next_idx % len(self._accounts)
                self._next_idx += 1
                acct = self._accounts[idx]
                if acct.state == "idle":
                    acct.state = "busy"
                    return acct
            return None
    
    def release(self, acct: Account):
        with self._lock:
            if acct.state == "busy":
                acct.state = "idle"
```

---

## 8. 总结与建议

### 8.1 技术亮点
1. **WASM-based PoW 求解** - 自动化难度挑战，无需人工干预
2. **TLS 指纹伪装** - 使用 curl_cffi 模拟真实浏览器，降低被封风险
3. **多账号池 + 自动恢复** - 提高并发能力和容错性
4. **DSML Prompt Injection** - 巧妙实现 function calling 兼容

### 8.2 主要缺陷
1. **安全风险** - 凭证明文存储、无审计日志
2. **合规风险** - 违反服务条款，可能触发封禁
3. **单 Provider** - 仅支持 DeepSeek，无法扩展到其他 AI 服务
4. **无 OAuth** - 依赖手动提取 Cookie/Token

### 8.3 改进建议
1. **增加凭证加密** - 默认启用 Fernet 加密
2. **添加审计日志** - 记录所有账号操作
3. **扩展多 Provider** - 参考 Chat2API 架构
4. **集成 OAuth** - 支持自动登录流程

---

**分析完成**: 2026-08-28  
**数据来源**: GitHub API, README, 源码分析
