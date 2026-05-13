package oauth

import (
	"net/http"
	"time"
)

// ProviderType represents the type of AI provider.
type ProviderType string

const (
	ProviderDeepSeek   ProviderType = "deepseek"
	ProviderGLM        ProviderType = "glm"
	ProviderKimi       ProviderType = "kimi"
	ProviderMiniMax    ProviderType = "minimax"
	ProviderQwen       ProviderType = "qwen"
	ProviderQwenAI     ProviderType = "qwen-ai"
	ProviderZai        ProviderType = "zai"
	ProviderPerplexity ProviderType = "perplexity"
	ProviderMimo       ProviderType = "mimo"
)

// AuthMethod represents the authentication method supported by a provider.
type AuthMethod string

const (
	AuthMethodOAuth  AuthMethod = "oauth"
	AuthMethodToken  AuthMethod = "token"
	AuthMethodCookie AuthMethod = "cookie"
	AuthMethodManual AuthMethod = "manual"
)

// OAuthStatus represents the current status of an OAuth login flow.
type OAuthStatus string

const (
	OAuthStatusIdle      OAuthStatus = "idle"
	OAuthStatusPending   OAuthStatus = "pending"
	OAuthStatusSuccess   OAuthStatus = "success"
	OAuthStatusError     OAuthStatus = "error"
	OAuthStatusCancelled OAuthStatus = "cancelled"
)

// TokenType represents the type of authentication token.
type TokenType string

const (
	TokenTypeJWT     TokenType = "jwt"
	TokenTypeRefresh TokenType = "refresh"
	TokenTypeAccess  TokenType = "access"
	TokenTypeCookie  TokenType = "cookie"
)

// OAuthResult is the result of an OAuth login attempt.
type OAuthResult struct {
	Success      bool              `json:"success"`
	ProviderID   string            `json:"providerId,omitempty"`
	ProviderType ProviderType      `json:"providerType,omitempty"`
	Credentials  map[string]string `json:"credentials,omitempty"`
	AccountInfo  *OAuthAccountInfo `json:"accountInfo,omitempty"`
	Error        string            `json:"error,omitempty"`
}

// OAuthAccountInfo contains information about the authenticated account.
type OAuthAccountInfo struct {
	UserID    string `json:"userId,omitempty"`
	Email     string `json:"email,omitempty"`
	Name      string `json:"name,omitempty"`
	Avatar    string `json:"avatar,omitempty"`
	Quota     int64  `json:"quota,omitempty"`
	Used      int64  `json:"used,omitempty"`
	ExpiresAt int64  `json:"expiresAt,omitempty"`
}

// OAuthOptions contains options for starting an OAuth login flow.
type OAuthOptions struct {
	ProviderID   string       `json:"providerId"`
	ProviderType ProviderType `json:"providerType"`
	CallbackPort int          `json:"callbackPort,omitempty"`
	Timeout      int          `json:"timeout,omitempty"`
}

// OAuthCallbackData contains data from an OAuth callback redirect.
type OAuthCallbackData struct {
	Code             string `json:"code,omitempty"`
	Token            string `json:"token,omitempty"`
	State            string `json:"state,omitempty"`
	Error            string `json:"error,omitempty"`
	ErrorDescription string `json:"errorDescription,omitempty"`
}

// TokenValidationResult is the result of validating a token.
type TokenValidationResult struct {
	Valid       bool              `json:"valid"`
	TokenType   TokenType         `json:"tokenType,omitempty"`
	ExpiresAt   int64             `json:"expiresAt,omitempty"`
	AccountInfo *OAuthAccountInfo `json:"accountInfo,omitempty"`
	Error       string            `json:"error,omitempty"`
}

// CredentialInfo contains information about credentials for refresh token flow.
type CredentialInfo struct {
	Type         TokenType         `json:"type"`
	Value        string            `json:"value"`
	ExpiresAt    int64             `json:"expiresAt,omitempty"`
	RefreshToken string            `json:"refreshToken,omitempty"`
	Extra        map[string]string `json:"extra,omitempty"`
}

// AdapterConfig contains configuration for creating an adapter.
type AdapterConfig struct {
	ProviderID   string       `json:"providerId"`
	ProviderType ProviderType `json:"providerType"`
	AuthMethods  []AuthMethod `json:"authMethods"`
	CallbackPort int          `json:"callbackPort"`
	LoginURL     string       `json:"loginUrl,omitempty"`
	APIURL       string       `json:"apiUrl,omitempty"`
}

// OAuthProgressEvent represents a progress update during OAuth flow.
type OAuthProgressEvent struct {
	Status   OAuthStatus            `json:"status"`
	Message  string                 `json:"message"`
	Progress int                    `json:"progress,omitempty"`
	Data     map[string]interface{} `json:"data,omitempty"`
}

// ProgressCallback is a function that receives progress events.
type ProgressCallback func(event OAuthProgressEvent)

// ManualTokenConfig defines configuration for manual token input UI.
type ManualTokenConfig struct {
	ProviderType ProviderType `json:"providerType"`
	TokenType    TokenType    `json:"tokenType"`
	Label        string       `json:"label"`
	Placeholder  string       `json:"placeholder"`
	Description  string       `json:"description"`
	HelpURL      string       `json:"helpUrl,omitempty"`
}

// MANUAL_TOKEN_CONFIGS maps each provider to its manual token input configurations.
var MANUAL_TOKEN_CONFIGS = map[ProviderType][]ManualTokenConfig{
	ProviderDeepSeek: {
		{
			ProviderType: ProviderDeepSeek,
			TokenType:    TokenTypeAccess,
			Label:        "User Token",
			Placeholder:  "Enter the userToken obtained from browser LocalStorage",
			Description:  "Open Developer Tools on chat.deepseek.com, find userToken in Application > Local Storage",
			HelpURL:      "https://chat.deepseek.com",
		},
	},
	ProviderGLM: {
		{
			ProviderType: ProviderGLM,
			TokenType:    TokenTypeRefresh,
			Label:        "Refresh Token",
			Placeholder:  "Enter refresh_token",
			Description:  "After logging in to chatglm.cn, get refresh_token from Cookie or API response",
			HelpURL:      "https://chatglm.cn",
		},
	},
	ProviderKimi: {
		{
			ProviderType: ProviderKimi,
			TokenType:    TokenTypeJWT,
			Label:        "Access Token (JWT)",
			Placeholder:  "Enter JWT format access_token",
			Description:  "JWT Token starting with eyJ, obtained from browser Developer Tools",
			HelpURL:      "https://www.kimi.com",
		},
		{
			ProviderType: ProviderKimi,
			TokenType:    TokenTypeJWT,
			Label:        "JWT Token (kimi-auth)",
			Placeholder:  "Enter JWT token from kimi-auth cookie",
			Description:  "Get JWT token from kimi-auth cookie in browser DevTools",
			HelpURL:      "https://www.kimi.com",
		},
	},
	ProviderMiniMax: {
		{
			ProviderType: ProviderMiniMax,
			TokenType:    TokenTypeAccess,
			Label:        "Token (realUserID_token)",
			Placeholder:  "Format: realUserID_token",
			Description:  "Obtain after logging in to agent.minimaxi.com, format is realUserID + \"_\" + token",
			HelpURL:      "https://agent.minimaxi.com",
		},
	},
	ProviderQwen: {
		{
			ProviderType: ProviderQwen,
			TokenType:    TokenTypeCookie,
			Label:        "tongyi_sso_ticket",
			Placeholder:  "Enter tongyi_sso_ticket",
			Description:  "After logging in to www.qianwen.com, get tongyi_sso_ticket from Cookie",
			HelpURL:      "https://www.qianwen.com",
		},
	},
	ProviderQwenAI: {
		{
			ProviderType: ProviderQwenAI,
			TokenType:    TokenTypeJWT,
			Label:        "Auth Token",
			Placeholder:  "Enter JWT token from chat.qwen.ai",
			Description:  "JWT token obtained from chat.qwen.ai Local Storage (key: \"token\")",
			HelpURL:      "https://chat.qwen.ai",
		},
	},
	ProviderZai: {
		{
			ProviderType: ProviderZai,
			TokenType:    TokenTypeAccess,
			Label:        "Access Token",
			Placeholder:  "Enter JWT token from chat.z.ai",
			Description:  "JWT token obtained from chat.z.ai Local Storage or Cookie",
			HelpURL:      "https://chat.z.ai",
		},
	},
	ProviderPerplexity: {
		{
			ProviderType: ProviderPerplexity,
			TokenType:    TokenTypeCookie,
			Label:        "Cookies",
			Placeholder:  "Paste Perplexity cookies or import HAR file",
			Description:  "Get cookies from perplexity.ai browser DevTools or import HAR file",
			HelpURL:      "https://www.perplexity.ai",
		},
	},
	ProviderMimo: {
		{
			ProviderType: ProviderMimo,
			TokenType:    TokenTypeCookie,
			Label:        "Cookies (3-part)",
			Placeholder:  "Use manual input for serviceToken, userId, and xiaomichatbot_ph",
			Description:  "Mimo requires serviceToken, userId, and xiaomichatbot_ph cookies",
			HelpURL:      "https://aistudio.xiaomimimo.com",
		},
	},
}

// --- Browser Automation Types ---

// TokenSource defines where to extract tokens from.
type TokenSource struct {
	ProviderType       ProviderType
	LocalStorageKey    string   `json:"localStorageKey,omitempty"`    // e.g. "userToken"
	ExtraLocalStorage  []string `json:"extraLocalStorage,omitempty"`  // Additional localStorage keys (e.g. MiniMax: "user_detail_agent")
	CookieName         string   `json:"cookieName,omitempty"`         // e.g. "chatglm_refresh_token"
	ExtraCookies       []string `json:"extraCookies,omitempty"`       // Additional cookies for multi-token providers (e.g. Mimo: userId, ph_token)
	RequestHeader      string   `json:"requestHeader,omitempty"`      // e.g. "Authorization"
	URLPattern         string   `json:"urlPattern,omitempty"`         // e.g. "*://*.kimi.com/*"
	ExtractPattern     string   `json:"extractPattern,omitempty"`     // e.g. "^Bearer\\s+(.+)$"
	ResultKey          string   `json:"resultKey,omitempty"`          // key for credentials map, e.g. "token"
	JSONExtractField   string   `json:"jsonExtractField,omitempty"`   // Extract nested JSON field, e.g. "realUserID" from user_detail_agent
	JSONExtractField2  string   `json:"jsonExtractField2,omitempty"`  // Second fallback field, e.g. "id"
	MinLength          int      `json:"minLength,omitempty"`          // Minimum token length to be considered valid (e.g. 100 for refresh tokens)
	WindowTitle        string   `json:"windowTitle,omitempty"`        // Browser window title for login popup
	TargetDomains      []string `json:"targetDomains,omitempty"`      // Target domains for cookie scope
}

// InterceptedRequest captures request/response pairs.
type InterceptedRequest struct {
	URL       string
	Method    string
	Request   http.Header
	Response  http.Header
	Timestamp time.Time
}

// BrowserLoginResult wraps OAuthResult with browser-specific data.
type BrowserLoginResult struct {
	OAuthResult
	Cookies         []*http.Cookie       `json:"cookies,omitempty"`
	LocalStorage    map[string]string    `json:"localStorage,omitempty"`
	InterceptedReqs []InterceptedRequest `json:"interceptedRequests,omitempty"`
}

// BrowserConfig configures the browser automation.
type BrowserConfig struct {
	Headless       bool
	UserDataDir    string
	Proxy          string
	BrowserTimeout time.Duration
	Width          int
	Height         int
	WindowTitle    string
}
