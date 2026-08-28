#!/usr/bin/env node
const fs = require('fs');
const path = require('path');

const filePath = 'D:/projects/Chat2API/src/main/oauth/tokenExtractionConfig.ts';
let content = fs.readFileSync(filePath, 'utf-8');

// Replace the deepseek config to add cookies source
const oldDeepseek = `  deepseek: {
    loginUrl: 'https://chat.deepseek.com',
    tokenSources: [
      {
        type: 'localStorage',
        key: 'userToken',
      },
    ],
    targetDomains: ['.deepseek.com', 'deepseek.com'],`;

const newDeepseek = `  deepseek: {
    loginUrl: 'https://chat.deepseek.com',
    tokenSources: [
      {
        type: 'localStorage',
        key: 'userToken',
      },
      // Collect all cookies for anti-detection (WAF bypass)
      {
        type: 'cookie',
        key: 'cookies',
      },
    ],
    targetDomains: ['.deepseek.com', 'deepseek.com'],`;

if (content.includes(oldDeepseek)) {
  content = content.replace(oldDeepseek, newDeepseek);
  fs.writeFileSync(filePath, content, 'utf-8');
  console.log('Updated tokenExtractionConfig.ts');
} else {
  console.log('Pattern not found');
}
