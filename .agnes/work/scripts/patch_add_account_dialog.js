#!/usr/bin/env node
const fs = require('fs');

const filePath = 'D:/projects/Chat2API/src/renderer/src/components/providers/AddAccountDialog.tsx';
let content = fs.readFileSync(filePath, 'utf-8');

// Update mapOAuthCredentials to handle DeepSeek cookies
const oldMapFn = `function mapOAuthCredentials(providerId: string | undefined, credentials: Record<string, string>): Record<string, string> {
  if (!providerId) return credentials

  const credentialKeyMap: Record<string, string> = {
    'glm': 'chatglm_refresh_token',
    'deepseek': 'userToken',
    'qwen': 'tongyi_sso_ticket',
    'qwen-ai': 'tongyi_sso_ticket',
    'zai': 'tongyi_sso_ticket',
    'perplexity': '__Secure-next-auth.session-token',
    'mimo': 'serviceToken',
  }

  const providerFieldNames: Record<string, string> = {
    'glm': 'refresh_token',
    'deepseek': 'token',
    'qwen': 'ticket',
    'qwen-ai': 'ticket',
    'zai': 'ticket',
    'perplexity': 'sessionToken',
    'mimo': 'service_token',
  }`;

const newMapFn = `function mapOAuthCredentials(providerId: string | undefined, credentials: Record<string, string>): Record<string, string> {
  if (!providerId) return credentials

  const credentialKeyMap: Record<string, string> = {
    'glm': 'chatglm_refresh_token',
    'deepseek': 'userToken',
    'deepseek-cookies': 'cookies',  // Special handling for DeepSeek cookies
    'qwen': 'tongyi_sso_ticket',
    'qwen-ai': 'tongyi_sso_ticket',
    'zai': 'tongyi_sso_ticket',
    'perplexity': '__Secure-next-auth.session-token',
    'mimo': 'serviceToken',
  }

  const providerFieldNames: Record<string, string> = {
    'glm': 'refresh_token',
    'deepseek': 'token',
    'deepseek-cookies': 'cookies',  // Map cookies key
    'qwen': 'ticket',
    'qwen-ai': 'ticket',
    'zai': 'ticket',
    'perplexity': 'sessionToken',
    'mimo': 'service_token',
  }`;

content = content.replace(oldMapFn, newMapFn);

// Add cookies handling after the DeepSeek token handling
const oldDeepSeekHandling = `      // Handle JSON-wrapped tokens (DeepSeek stores token as {"value":"..."})
      let tokenValue = credentials[oauthKey]
      if (providerId === 'deepseek' && tokenValue && tokenValue.startsWith('{') && tokenValue.endsWith('}')) {
        try {
          const parsed = JSON.parse(tokenValue)
          if (parsed.value) {
            tokenValue = parsed.value
          }
        } catch (e) {
          console.error('[AddAccountDialog] Error parsing JSON token:', e)
        }
      }
      return { [fieldName]: tokenValue }`;

const newDeepSeekHandling = `      // Handle JSON-wrapped tokens (DeepSeek stores token as {"value":"..."})
      let tokenValue = credentials[oauthKey]
      if (providerId === 'deepseek' && tokenValue && tokenValue.startsWith('{') && tokenValue.endsWith('}')) {
        try {
          const parsed = JSON.parse(tokenValue)
          if (parsed.value) {
            tokenValue = parsed.value
          }
        } catch (e) {
          console.error('[AddAccountDialog] Error parsing JSON token:', e)
        }
      }
      const result: Record<string, string> = { [fieldName]: tokenValue }
      
      // Add cookies if available (for anti-detection)
      if (providerId === 'deepseek' && credentials['cookies']) {
        result['cookies'] = credentials['cookies']
      }
      return result`;

content = content.replace(oldDeepSeekHandling, newDeepSeekHandling);

// Update the translations to include DeepSeek cookies field
const oldTranslations = `      deepseek: {
        token: {
          label: t('deepseek.userToken'),
          placeholder: t('deepseek.userTokenPlaceholder'),
          helpText: t('deepseek.userTokenHelp'),
        },
      },`;

const newTranslations = `      deepseek: {
        token: {
          label: t('deepseek.userToken'),
          placeholder: t('deepseek.userTokenPlaceholder'),
          helpText: t('deepseek.userTokenHelp'),
        },
        cookies: {
          label: t('deepseek.cookies'),
          placeholder: t('deepseek.cookiesPlaceholder'),
          helpText: t('deepseek.cookiesHelp'),
        },
      },`;

content = content.replace(oldTranslations, newTranslations);

fs.writeFileSync(filePath, content, 'utf-8');
console.log('Updated AddAccountDialog.tsx');
