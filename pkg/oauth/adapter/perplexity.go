package adapter

import (
	"fmt"

	"github.com/fssl168/chat2api-go/oauth/pkg/oauth"
)

const perplexityAPIBase = "https://www.perplexity.ai"

var perplexityFakeHeaders = map[string]string{
	"Accept":             "text/event-stream",
	"Accept-Encoding":    "gzip, deflate, br, zstd",
	"Accept-Language":    "en-US,en;q=0.9",
	"Cache-Control":      "no-cache",
	"Origin":             "https://www.perplexity.ai",
	"Pragma":             "no-cache",
	"Referer":            "https://www.perplexity.ai/",
	"Sec-Ch-Ua":          `"Chromium";v="134", "Not:A-Brand";v="24", "Google Chrome";v="134"`,
	"Sec-Ch-Ua-Mobile":   "?0",
	"Sec-Ch-Ua-Platform": `"macOS"`,
	"Sec-Fetch-Dest":     "empty",
	"Sec-Fetch-Mode":     "cors",
	"Sec-Fetch-Site":     "same-origin",
	"User-Agent":         "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/134.0.0.0 Safari/537.36",
}

var perplexityStreamHeaders = map[string]string{
	"Accept":             "text/event-stream",
	"Accept-Encoding":    "gzip, deflate, br, zstd",
	"Accept-Language":    "en-US,en;q=0.9",
	"Cache-Control":      "no-cache",
	"Origin":             "https://www.perplexity.ai",
	"Pragma":             "no-cache",
	"Referer":            "https://www.perplexity.ai/",
	"Sec-Ch-Ua":          `"Chromium";v="134", "Not:A-Brand";v="24", "Google Chrome";v="134"`,
	"Sec-Ch-Ua-Mobile":   "?0",
	"Sec-Ch-Ua-Platform": `"macOS"`,
	"Sec-Fetch-Dest":     "empty",
	"Sec-Fetch-Mode":     "cors",
	"Sec-Fetch-Site":     "same-origin",
	"User-Agent":         "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/134.0.0.0 Safari/537.36",
}

// PerplexityAdapter implements OAuth for Perplexity.
type PerplexityAdapter struct {
	*BaseOAuthAdapter
}

// NewPerplexityAdapter creates a new Perplexity adapter.
func NewPerplexityAdapter(config oauth.AdapterConfig) *PerplexityAdapter {
	config.ProviderType = oauth.ProviderPerplexity
	config.AuthMethods = []oauth.AuthMethod{oauth.AuthMethodManual, oauth.AuthMethodCookie}
	config.LoginURL = perplexityAPIBase
	config.APIURL = perplexityAPIBase
	return &PerplexityAdapter{
		BaseOAuthAdapter: NewBaseOAuthAdapter(config),
	}
}

// StartLogin opens the browser for manual login.
func (a *PerplexityAdapter) StartLogin(options oauth.OAuthOptions) (oauth.OAuthResult, error) {
	return a.DefaultStartLogin(options, oauth.ProviderPerplexity)
}

// LoginWithToken completes authentication with cookies.
func (a *PerplexityAdapter) LoginWithToken(providerID string, token string, extras ...string) (oauth.OAuthResult, error) {
	a.EmitProgress(oauth.OAuthStatusPending, "Validating session...", nil)

	credentials := map[string]string{
		"sessionToken": token,
	}

	if len(extras) >= 1 {
		credentials["email"] = extras[0]
	}

	validation, err := a.ValidateToken(credentials)
	if err != nil {
		return oauth.OAuthResult{
			Success:      false,
			ProviderID:   providerID,
			ProviderType: oauth.ProviderPerplexity,
			Error:        err.Error(),
		}, nil
	}

	if !validation.Valid {
		return oauth.OAuthResult{
			Success:      false,
			ProviderID:   providerID,
			ProviderType: oauth.ProviderPerplexity,
			Error:        validation.Error,
		}, nil
	}

	a.EmitProgress(oauth.OAuthStatusSuccess, "Session validation successful", nil)
	return oauth.OAuthResult{
		Success:      true,
		ProviderID:   providerID,
		ProviderType: oauth.ProviderPerplexity,
		Credentials:  credentials,
		AccountInfo:  validation.AccountInfo,
	}, nil
}

// ValidateToken validates the Perplexity session token (Cookie-based auth).
func (a *PerplexityAdapter) ValidateToken(credentials map[string]string) (oauth.TokenValidationResult, error) {
	sessionToken := credentials["sessionToken"]
	if sessionToken == "" {
		return oauth.TokenValidationResult{Valid: false, Error: "sessionToken cannot be empty"}, nil
	}

	if len(sessionToken) < 10 {
		return oauth.TokenValidationResult{Valid: false, Error: "Invalid session token format"}, nil
	}

	return oauth.TokenValidationResult{
		Valid:     true,
		TokenType: oauth.TokenTypeCookie,
		AccountInfo: &oauth.OAuthAccountInfo{
			UserID: "perplexity-user",
		},
	}, nil
}

// RefreshToken attempts to refresh the token.
func (a *PerplexityAdapter) RefreshToken(credentials map[string]string) (*oauth.CredentialInfo, error) {
	return nil, fmt.Errorf("token refresh not supported for Perplexity (requires re-login)")
}

// GetHeaders returns the headers to use for Perplexity API requests.
func GetPerplexityHeaders() map[string]string {
	headers := make(map[string]string)
	for k, v := range perplexityFakeHeaders {
		headers[k] = v
	}
	return headers
}

// GetStreamHeaders returns the headers to use for Perplexity streaming API requests.
func GetPerplexityStreamHeaders() map[string]string {
	headers := make(map[string]string)
	for k, v := range perplexityStreamHeaders {
		headers[k] = v
	}
	return headers
}
