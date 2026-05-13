package adapter

import (
	"fmt"

	"github.com/fssl168/chat2api-go/oauth/pkg/oauth"
)

const zaiAPIBase = "https://chat.z.ai"

var zaiFakeHeaders = map[string]string{
	"Accept":             "*/*",
	"Accept-Encoding":    "gzip, deflate, br, zstd",
	"Accept-Language":    "zh-CN,zh;q=0.9",
	"Cache-Control":      "no-cache",
	"Content-Type":       "application/json",
	"Origin":             "https://chat.z.ai",
	"Pragma":             "no-cache",
	"Referer":            "https://chat.z.ai/",
	"Sec-Ch-Ua":          `"Chromium";v="144", "Not(A:Brand";v="8", "Google Chrome";v="144"`,
	"Sec-Ch-Ua-Mobile":   "?0",
	"Sec-Ch-Ua-Platform": `"Windows"`,
	"Sec-Fetch-Dest":     "empty",
	"Sec-Fetch-Mode":     "cors",
	"Sec-Fetch-Site":     "same-origin",
	"User-Agent":         "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/144.0.0.0 Safari/537.36",
	"X-FE-Version":       "0.0.1",
}

var zaiStreamHeaders = map[string]string{
	"Accept":             "text/event-stream",
	"Accept-Encoding":    "gzip, deflate, br, zstd",
	"Accept-Language":    "zh-CN,zh;q=0.9",
	"Cache-Control":      "no-cache",
	"Content-Type":       "application/json",
	"Origin":             "https://chat.z.ai",
	"Pragma":             "no-cache",
	"Referer":            "https://chat.z.ai/",
	"Sec-Ch-Ua":          `"Chromium";v="144", "Not(A:Brand";v="8", "Google Chrome";v="144"`,
	"Sec-Ch-Ua-Mobile":   "?0",
	"Sec-Ch-Ua-Platform": `"Windows"`,
	"Sec-Fetch-Dest":     "empty",
	"Sec-Fetch-Mode":     "cors",
	"Sec-Fetch-Site":     "same-origin",
	"User-Agent":         "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/144.0.0.0 Safari/537.36",
	"X-FE-Version":       "0.0.1",
}

// ZaiAdapter implements OAuth for Z.ai (GLM International).
type ZaiAdapter struct {
	*BaseOAuthAdapter
}

// NewZaiAdapter creates a new Z.ai adapter.
func NewZaiAdapter(config oauth.AdapterConfig) *ZaiAdapter {
	config.ProviderType = oauth.ProviderZai
	config.AuthMethods = []oauth.AuthMethod{oauth.AuthMethodManual, oauth.AuthMethodToken}
	config.LoginURL = zaiAPIBase
	config.APIURL = zaiAPIBase
	return &ZaiAdapter{
		BaseOAuthAdapter: NewBaseOAuthAdapter(config),
	}
}

// StartLogin opens the browser for manual login.
func (a *ZaiAdapter) StartLogin(options oauth.OAuthOptions) (oauth.OAuthResult, error) {
	return a.DefaultStartLogin(options, oauth.ProviderZai)
}

// LoginWithToken completes authentication with a manually entered token.
func (a *ZaiAdapter) LoginWithToken(providerID string, token string, extras ...string) (oauth.OAuthResult, error) {
	a.EmitProgress(oauth.OAuthStatusPending, "Validating Token...", nil)

	validation, err := a.ValidateToken(map[string]string{"token": token})
	if err != nil {
		return oauth.OAuthResult{
			Success:      false,
			ProviderID:   providerID,
			ProviderType: oauth.ProviderZai,
			Error:        err.Error(),
		}, nil
	}

	if !validation.Valid {
		return oauth.OAuthResult{
			Success:      false,
			ProviderID:   providerID,
			ProviderType: oauth.ProviderZai,
			Error:        validation.Error,
		}, nil
	}

	a.EmitProgress(oauth.OAuthStatusSuccess, "Token validation successful", nil)
	return oauth.OAuthResult{
		Success:      true,
		ProviderID:   providerID,
		ProviderType: oauth.ProviderZai,
		Credentials:  map[string]string{"token": token},
		AccountInfo:  validation.AccountInfo,
	}, nil
}

// ValidateToken validates the Z.ai token via JWT parsing only (no API validation).
func (a *ZaiAdapter) ValidateToken(credentials map[string]string) (oauth.TokenValidationResult, error) {
	token := credentials["token"]
	if token == "" {
		return oauth.TokenValidationResult{Valid: false, Error: "Token cannot be empty"}, nil
	}

	if !oauth.IsJWT(token) {
		return oauth.TokenValidationResult{Valid: false, Error: "Token must be in JWT format"}, nil
	}

	payload, err := oauth.ParseJWT(token)
	if err != nil {
		return oauth.TokenValidationResult{Valid: false, Error: "Failed to parse JWT: " + err.Error()}, nil
	}

	// Check for id field
	id, hasID := payload["id"]
	if !hasID {
		return oauth.TokenValidationResult{Valid: false, Error: "JWT missing required 'id' field"}, nil
	}

	// Guest check (comprehensive, matching chat2api)
	fmt.Println("[Z.ai] JWT payload parsed:",
		"id=", id,
		"email=", payload["email"],
		"name=", payload["name"])

	if email, ok := payload["email"].(string); ok && oauth.IsGuestEmail(email) {
		fmt.Println("[Z.ai] Guest account detected: email indicates guest")
		return oauth.TokenValidationResult{Valid: false, Error: "Guest accounts are not allowed (email indicates guest)"}, nil
	}
	if name, ok := payload["name"].(string); ok && oauth.IsGuestNickname(name) {
		fmt.Println("[Z.ai] Guest account detected: nickname indicates guest")
		return oauth.TokenValidationResult{Valid: false, Error: "Guest accounts are not allowed (nickname indicates guest)"}, nil
	}

	accountInfo := &oauth.OAuthAccountInfo{}
	if idStr, ok := id.(string); ok {
		accountInfo.UserID = idStr
	}
	if email, ok := payload["email"].(string); ok {
		accountInfo.Email = email
	}
	if name, ok := payload["name"].(string); ok {
		accountInfo.Name = name
	}

	return oauth.TokenValidationResult{
		Valid:       true,
		TokenType:   oauth.TokenTypeAccess,
		AccountInfo: accountInfo,
	}, nil
}

// RefreshToken attempts to refresh the token.
func (a *ZaiAdapter) RefreshToken(credentials map[string]string) (*oauth.CredentialInfo, error) {
	return nil, fmt.Errorf("token refresh not supported for Z.ai")
}

// GetHeaders returns the headers to use for Z.ai API requests.
func GetZaiHeaders() map[string]string {
	headers := make(map[string]string)
	for k, v := range zaiFakeHeaders {
		headers[k] = v
	}
	return headers
}

// GetStreamHeaders returns the headers to use for Z.ai streaming API requests.
func GetZaiStreamHeaders() map[string]string {
	headers := make(map[string]string)
	for k, v := range zaiStreamHeaders {
		headers[k] = v
	}
	return headers
}
