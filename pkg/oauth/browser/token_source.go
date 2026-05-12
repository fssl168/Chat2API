package browser

import "github.com/fssl168/chat2api-go/oauth/pkg/oauth"

// TokenSources configures where to extract tokens for each provider.
var TokenSources = map[oauth.ProviderType]oauth.TokenSource{
	oauth.ProviderDeepSeek: {
		ProviderType:    oauth.ProviderDeepSeek,
		LocalStorageKey: "userToken",
		ResultKey:       "token",
	},
	oauth.ProviderGLM: {
		ProviderType: oauth.ProviderGLM,
		CookieName:   "chatglm_refresh_token",
	},
	// Kimi: token comes from kimi-auth cookie (JWT format)
	oauth.ProviderKimi: {
		ProviderType: oauth.ProviderKimi,
		CookieName:   "kimi-auth",
		ResultKey:    "token",
	},
	// MiniMax: _token from localStorage + realUserID from user_detail_agent JSON
	oauth.ProviderMiniMax: {
		ProviderType:      oauth.ProviderMiniMax,
		LocalStorageKey:   "_token",
		ResultKey:         "token",
		ExtraLocalStorage: []string{"user_detail_agent"},
		JSONExtractField:  "realUserID",
		JSONExtractField2: "id",
	},
	oauth.ProviderQwen: {
		ProviderType: oauth.ProviderQwen,
		CookieName:   "tongyi_sso_ticket",
	},
	// Qwen AI: token from localStorage + cookie (dual source)
	oauth.ProviderQwenAI: {
		ProviderType:    oauth.ProviderQwenAI,
		LocalStorageKey: "token",
		CookieName:      "token",
	},
	// Z.ai: token from localStorage + cookie (dual source)
	oauth.ProviderZai: {
		ProviderType:    oauth.ProviderZai,
		LocalStorageKey: "token",
		CookieName:      "token",
	},
	// Perplexity: session token from cookie (with fallback to non-secure variant)
	oauth.ProviderPerplexity: {
		ProviderType: oauth.ProviderPerplexity,
		CookieName:   "__Secure-next-auth.session-token",
		ExtraCookies: []string{"next-auth.session-token"},
	},
	oauth.ProviderMimo: {
		ProviderType: oauth.ProviderMimo,
		CookieName:   "serviceToken",
		ExtraCookies: []string{"userId", "xiaomichatbot_ph"},
	},
}

// ProviderLoginURL returns the login URL for each provider.
var ProviderLoginURL = map[oauth.ProviderType]string{
	oauth.ProviderDeepSeek:   "https://chat.deepseek.com",
	oauth.ProviderGLM:        "https://chatglm.cn",
	oauth.ProviderKimi:       "https://www.kimi.com",
	oauth.ProviderMiniMax:    "https://agent.minimaxi.com",
	oauth.ProviderQwen:       "https://www.qianwen.com",
	oauth.ProviderQwenAI:     "https://chat.qwen.ai",
	oauth.ProviderZai:        "https://chat.z.ai",
	oauth.ProviderPerplexity: "https://www.perplexity.ai",
	oauth.ProviderMimo:       "https://aistudio.xiaomimimo.com",
}
