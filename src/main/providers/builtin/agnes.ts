import type { BuiltinProviderConfig } from '../../store/types'

export const agnesConfig: BuiltinProviderConfig = {
  id: 'agnes',
  name: 'Agnes',
  type: 'builtin',
  authType: 'cookie',
  apiEndpoint: 'https://api-agnes-code.agnes-ai.com',
  chatPath: '/v1/chat/completions',
  headers: {
    'Content-Type': 'application/json',
    'Accept': 'application/json',
    'X-App-Id': '1',
    'X-Platform': '1',
    'X-User-Language': 'zh-CN',
  },
  enabled: true,
  description: 'Agnes AI - Cookie-based authentication via app.agnes-ai.com',
  supportedModels: [
    'agnes-2.0-flash',
    'agnes-2.5-flash',
    'agnes-2.5-pro',
  ],
  modelMappings: {
    'agnes-2.0-flash': 'agnes-2.0-flash',
    'agnes-2.5-flash': 'agnes-2.5-flash',
    'agnes-2.5-pro': 'agnes-2.5-pro',
  },
  credentialFields: [
    {
      name: 'token',
      label: 'JWT Token',
      type: 'password',
      required: true,
      placeholder: 'Enter Agnes JWT token (eyJ...)',
      helpText: 'Log in at app.agnes-ai.com and copy the token cookie value',
    },
  ],
  tokenCheckEndpoint: '/v1/models',
  tokenCheckMethod: 'GET',
}

export default agnesConfig
