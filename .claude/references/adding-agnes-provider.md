# Chat2API Provider Operations - Reference Guide

## Adding a New Provider (Step-by-Step)

### 1. Builtin Config
Create `src/main/providers/builtin/<provider>.ts` with:
```typescript
import type { BuiltinProviderConfig } from '../../store/types'

export const <ProviderName>Config: BuiltinProviderConfig = {
  id: '<id>',
  name: '<Name>',
  type: 'builtin',
  authType: 'userToken',  // or 'oauth'
  apiEndpoint: '<endpoint>',
  supportedModels: ['model1', 'model2'],
  modelMappings: {},
}
```

### 2. Register in builtin/index.ts
```typescript
import <provider>Config from './<provider>.ts'
// Add to builtinProviderMap
```

### 3. Create Proxy Adapter
`src/main/proxy/adapters/<provider>.ts`:
- Read token from account credentials OR local file/source
- Forward request to provider API
- Return `{ response }` for streaming

### 4. Create Stream Handler (if needed)
`src/main/proxy/adapters/<provider>-stream.ts`
- Handle SSE stream conversion
- Export `handleStream()` and `handleNonStream()` methods

### 5. Register in adapters/index.ts
```typescript
export { <Provider>Adapter, <provider>Adapter } from './<provider>'
export { <Provider>StreamHandler } from './<provider>-stream'
```

### 6. Add to forwarder.ts
```typescript
import { <Provider>Adapter } from './adapters/<provider>'
// Add to providerForwarders array
{
  name: '<provider>',
  matches: <Provider>Adapter.is<Provider>Provider,
  forward: (request, account, provider, actualModel, startTime) =>
    this.forward<Provider>(request, account, provider, actualModel, startTime),
},
// Add method implementation at end of class
```

### 7. Add OAuth Token Guide (optional)
In `src/main/oauth/guides.ts` and `src/main/oauth/types.ts`:
```typescript
<provider>: {
  loginUrl: '...',
  steps: ['Step 1...', 'Step 2...'],
  tokenKey: 'token',
  tokenLabel: 'JWT Token',
  storageType: 'other' | 'localStorage' | 'cookie',
  placeholder: 'Paste the JWT...',
}
```

### 8. Build + Deploy
```bash
npm run build
npx asar extract "C:/Users/nhlogo/AppData/Local/Programs/Chat2API/resources/app.asar" "C:/tmp/deploy-check"
cp out/main/index.js "C:/tmp/deploy-check/out/main/index.js"
cp -r out/renderer/* "C:/tmp/deploy-check/out/renderer/"
npx asar pack "C:/tmp/deploy-check" "C:/Users/nhlogo/AppData/Local/Programs/Chat2API/resources/app.asar"
```

### 9. Test
```bash
node --test tests/providers/provider-flow.test.ts
# Expected: 25 tests, 20 pass (DeepSeek/GLM mapping failures are pre-existing)
```

---

## Agnes Provider Specifics (BFF Mode)

### JWT Auto-Fetch Order
1. Account credentials (`account.credentials.token`)
2. File: `~/AppData/Roaming/Agnes Gateway/jwt.txt` (Windows)
3. Windows CredMan via Python ctypes
4. Environment variable: `AGNES_DESKTOP_JWT`

### Gateway Endpoints
- Status: `GET http://127.0.0.1:8787/admin/status`
- API: `POST http://127.0.0.1:8787/v1/chat/completions`
- Models API: `GET http://127.0.0.1:8787/v1/models`

### Available Models
| Model ID | Description |
|----------|-------------|
| agnes-2.5-flash | 速度与能力更均衡 |
| agnes-2.5-pro | 旗舰推理模型 |
| agnes-2.0-flash | 快速稳定 |
| glm-5.2 | 擅长中文理解 |
| deepseek-v4-pro | 擅长编程/数学 |
| gemini-3.5-flash | 多模态/长内容 |
| gpt-5.5 | 综合能力强 |
| claude-opus-4-8 | 长文本/专业写作 |

### Auth Requirements
- Header: `Authorization: Bearer <jwt>`
- Optional: `X-App-Id: 1`, `X-Platform: 1`

---

## Testing Checklist

- [ ] `npm run build` succeeds
- [ ] `node --test tests/providers/provider-flow.test.ts` passes (expected: 20/25 pass)
- [ ] Asar deployment successful
- [ ] Provider visible in UI
- [ ] JWT auto-fetch works (check logs for `[Provider] Using cached JWT` or similar)
- [ ] Test request to `/v1/chat/completions` returns valid response

---

## Common Pitfalls

### TypeScript Import Errors
- Use `import { spawnSync } from 'child_process'` NOT `import * as { spawnSync }`
- Extension imports (`.ts`) work in esbuild but may fail in strict TS mode
- Use `allowImportingTsExtensions` if enabled in tsconfig

### Windows Path Handling
- Use `os.homedir()` instead of hardcoded `~` or `$HOME`
- Windows Credential Manager requires Python + ctypes
- LF will be replaced by CRLF on Git touch (warning only)

### JWT TTL
- Agnes JWT expires in ~10 minutes
- Cache for 9 minutes before refetch
- Clear cache on 401 response
