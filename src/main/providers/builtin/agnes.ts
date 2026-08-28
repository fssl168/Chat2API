import type { BuiltinProviderConfig } from '../../store/types'

export const agnesConfig: BuiltinProviderConfig = {
  id: 'agnes',
  name: 'Agnes',
  type: 'builtin',
  authType: 'userToken',
  apiEndpoint: 'http://127.0.0.1:8787',
  chatPath: '/v1/chat/completions',
  headers: {
    'Content-Type': 'application/json',
    'Accept': 'application/json',
  },
  enabled: true,
  description: 'Agnes AI - Local gateway with JWT authentication',
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
      helpText: 'Get JWT from Agnes gateway admin page (http://127.0.0.1:8787/admin) or jwt.txt file',
    },
  ],
  tokenCheckEndpoint: '/admin/status',
  tokenCheckMethod: 'GET',
}

export default agnesConfig
