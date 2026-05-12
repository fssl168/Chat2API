package adapter

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/fssl168/chat2api-go/oauth/pkg/oauth"
)

const qwenAPIBase = "https://chat2-api.qianwen.com"
const qwenLoginURL = "https://www.qianwen.com"

var qwenFakeHeaders = map[string]string{
	"Accept":             "*/*",
	"Accept-Encoding":    "gzip, deflate, br, zstd",
	"Accept-Language":    "zh-CN,zh;q=0.9,en;q=0.8",
	"Cache-Control":      "no-cache",
	"Content-Type":       "application/json",
	"Origin":             "https://www.qianwen.com",
	"Pragma":             "no-cache",
	"Referer":            "https://www.qianwen.com/",
	"Sec-Ch-Ua":          `"Chromium";v="134", "Not:A-Brand";v="24", "Google Chrome";v="134"`,
	"Sec-Ch-Ua-Mobile":   "?0",
	"Sec-Ch-Ua-Platform": `"Windows"`,
	"Sec-Fetch-Dest":     "empty",
	"Sec-Fetch-Mode":     "cors",
	"Sec-Fetch-Site":     "same-origin",
	"User-Agent":         "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/134.0.0.0 Safari/537.36",
	"X-Platform":         "pc_tongyi",
}

// QwenAdapter implements OAuth for Qwen (Domestic).
type QwenAdapter struct {
	*BaseOAuthAdapter
}

// NewQwenAdapter creates a new Qwen adapter.
func NewQwenAdapter(config oauth.AdapterConfig) *QwenAdapter {
	config.ProviderType = oauth.ProviderQwen
	config.AuthMethods = []oauth.AuthMethod{oauth.AuthMethodManual, oauth.AuthMethodCookie}
	config.LoginURL = qwenLoginURL
	config.APIURL = qwenAPIBase
	return &QwenAdapter{
		BaseOAuthAdapter: NewBaseOAuthAdapter(config),
	}
}

// StartLogin opens the browser for manual login.
func (a *QwenAdapter) StartLogin(options oauth.OAuthOptions) (oauth.OAuthResult, error) {
	return a.DefaultStartLogin(options, oauth.ProviderQwen)
}

// LoginWithToken completes authentication with a manually entered ticket.
func (a *QwenAdapter) LoginWithToken(providerID string, token string, extras ...string) (oauth.OAuthResult, error) {
	a.EmitProgress(oauth.OAuthStatusPending, "Validating Token...", nil)

	validation, err := a.ValidateToken(map[string]string{"ticket": token})
	if err != nil {
		return oauth.OAuthResult{
			Success:      false,
			ProviderID:   providerID,
			ProviderType: oauth.ProviderQwen,
			Error:        err.Error(),
		}, nil
	}

	if !validation.Valid {
		return oauth.OAuthResult{
			Success:      false,
			ProviderID:   providerID,
			ProviderType: oauth.ProviderQwen,
			Error:        validation.Error,
		}, nil
	}

	a.EmitProgress(oauth.OAuthStatusSuccess, "Token validation successful", nil)
	return oauth.OAuthResult{
		Success:      true,
		ProviderID:   providerID,
		ProviderType: oauth.ProviderQwen,
		Credentials:  map[string]string{"ticket": token},
		AccountInfo:  validation.AccountInfo,
	}, nil
}

// ValidateToken validates the tongyi_sso_ticket.
// Supports multiple credential field aliases: ticket, tongyi_sso_ticket.
func (a *QwenAdapter) ValidateToken(credentials map[string]string) (oauth.TokenValidationResult, error) {
	ticket := credentials["ticket"]
	if ticket == "" {
		ticket = credentials["tongyi_sso_ticket"]
	}
	if ticket == "" {
		return oauth.TokenValidationResult{Valid: false, Error: "tongyi_sso_ticket cannot be empty"}, nil
	}

	deviceID := oauth.GenerateUUID()

	queryParams := url.Values{}
	queryParams.Set("biz_id", "ai_qwen")
	queryParams.Set("chat_client", "h5")
	queryParams.Set("device", "pc")
	queryParams.Set("fr", "pc")
	queryParams.Set("pr", "qwen")

	apiURL := qwenAPIBase + "/api/v2/session/page/list?" + queryParams.Encode()

	client := &http.Client{Timeout: 15 * time.Second}
	req, err := http.NewRequest("POST", apiURL, bytes.NewReader([]byte("{}")))
	if err != nil {
		return oauth.TokenValidationResult{Valid: false, Error: err.Error()}, nil
	}

	req.Header.Set("Cookie", "tongyi_sso_ticket="+ticket)
	req.Header.Set("X-DeviceId", deviceID)
	for k, v := range qwenFakeHeaders {
		req.Header.Set(k, v)
	}

	resp, err := client.Do(req)
	if err != nil {
		return oauth.TokenValidationResult{Valid: false, Error: err.Error()}, nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return oauth.TokenValidationResult{Valid: false, Error: "Ticket is invalid or expired"}, nil
	}

	var result struct {
		Success bool `json:"success"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return oauth.TokenValidationResult{Valid: false, Error: err.Error()}, nil
	}

	if !result.Success {
		return oauth.TokenValidationResult{Valid: false, Error: "Ticket validation failed"}, nil
	}

	return oauth.TokenValidationResult{
		Valid:     true,
		TokenType: oauth.TokenTypeCookie,
	}, nil
}

// RefreshToken attempts to refresh the token.
func (a *QwenAdapter) RefreshToken(credentials map[string]string) (*oauth.CredentialInfo, error) {
	return nil, fmt.Errorf("token refresh not supported for Qwen")
}
