#!/usr/bin/env node
const fs = require('fs');

const filePath = 'D:/projects/Chat2API/src/renderer/src/i18n/locales/zh-CN.json';
let content = fs.readFileSync(filePath, 'utf-8');

// Parse JSON and add translations
const translations = JSON.parse(content);

// Add DeepSeek translations
translations.deepseek = translations.deepseek || {};
translations.deepseek.cookies = 'Cookie 字符串';
translations.deepseek.cookiesPlaceholder = '从浏览器 DevTools 复制完整 Cookie 字符串';
translations.deepseek.cookiesHelp = '包含 cf_clearance、session 等 WAF 相关 Cookie';

fs.writeFileSync(filePath, JSON.stringify(translations, null, 2), 'utf-8');
console.log('Updated zh-CN.json translations');
