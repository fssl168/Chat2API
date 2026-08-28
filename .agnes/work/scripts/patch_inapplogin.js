#!/usr/bin/env node
const fs = require('fs');

const filePath = 'D:/projects/Chat2API/src/main/oauth/inAppLogin.ts';
let content = fs.readFileSync(filePath, 'utf-8');

// Find the section where cookies are collected and add logic to combine them for DeepSeek
const oldCookieCheck = `      for (const source of cookieSources) {
        if (!this.loginSession) continue

        const allCookies = await this.loginSession.cookies.get({})
        console.log('[InAppLogin] All cookies count:', allCookies.length)
        console.log('[InAppLogin] All cookies:', allCookies.map(c => `${c.name}=${c.value?.substring(0, 20)}...`))`;

const newCookieCheck = `      for (const source of cookieSources) {
        if (!this.loginSession) continue

        const allCookies = await this.loginSession.cookies.get({})
        console.log('[InAppLogin] All cookies count:', allCookies.length)
        console.log('[InAppLogin] All cookies:', allCookies.map(c => `${c.name}=${c.value?.substring(0, 20)}...`))
        
        // For DeepSeek, collect ALL cookies as a combined string
        if (source.key === 'cookies' && allCookies.length > 0) {
          const cookieString = allCookies
            .filter(c => c.domain?.includes('deepseek.com'))
            .map(c => `${c.name}=${c.value}`)
            .join('; ')
          console.log('[InAppLogin] DeepSeek cookies combined:', cookieString.substring(0, 100) + '...')
          if (cookieString.length > 0) {
            this.emit('tokenFound', { key: 'cookies', value: cookieString })
          }
        }`;

content = content.replace(oldCookieCheck, newCookieCheck);

fs.writeFileSync(filePath, content, 'utf-8');
console.log('Updated inAppLogin.ts');
