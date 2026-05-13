package adapter

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/fssl168/chat2api-go/oauth/pkg/oauth"
)

const qwenAIAPIBase = "https://chat.qwen.ai"

var qwenAIFakeHeaders = map[string]string{
	"Accept":             "application/json",
	"Accept-Encoding":    "gzip, deflate, br, zstd",
	"Accept-Language":    "zh-CN,zh;q=0.9,en;q=0.8",
	"Cache-Control":      "no-cache",
	"Content-Type":       "application/json",
	"Origin":             "https://chat.qwen.ai",
	"Pragma":             "no-cache",
	"Referer":            "https://chat.qwen.ai/",
	"Sec-Ch-Ua":          `"Chromium";v="144", "Not(A:Brand";v="8", "Google Chrome";v="144"`,
	"Sec-Ch-Ua-Mobile":   "?0",
	"Sec-Ch-Ua-Platform": `"Windows"`,
	"Sec-Fetch-Dest":     "empty",
	"Sec-Fetch-Mode":     "cors",
	"Sec-Fetch-Site":     "same-origin",
	"User-Agent":         "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/144.0.0.0 Safari/537.36",
	"source":             "web",
}

// QwenAIAdapter implements OAuth for Qwen AI (International).
type QwenAIAdapter struct {
	*BaseOAuthAdapter
}

// NewQwenAIAdapter creates a new Qwen AI adapter.
func NewQwenAIAdapter(config oauth.AdapterConfig) *QwenAIAdapter {
	config.ProviderType = oauth.ProviderQwenAI
	config.AuthMethods = []oauth.AuthMethod{oauth.AuthMethodManual, oauth.AuthMethodToken}
	config.LoginURL = qwenAIAPIBase
	config.APIURL = qwenAIAPIBase
	return &QwenAIAdapter{
		BaseOAuthAdapter: NewBaseOAuthAdapter(config),
	}
}

// StartLogin opens the browser for manual login.
func (a *QwenAIAdapter) StartLogin(options oauth.OAuthOptions) (oauth.OAuthResult, error) {
	return a.DefaultStartLogin(options, oauth.ProviderQwenAI)
}

// LoginWithToken completes authentication with a manually entered token.
func (a *QwenAIAdapter) LoginWithToken(providerID string, token string, extras ...string) (oauth.OAuthResult, error) {
	a.EmitProgress(oauth.OAuthStatusPending, "Validating Token...", nil)

	validation, err := a.ValidateToken(map[string]string{"token": token})
	if err != nil {
		return oauth.OAuthResult{
			Success:      false,
			ProviderID:   providerID,
			ProviderType: oauth.ProviderQwenAI,
			Error:        err.Error(),
		}, nil
	}

	if !validation.Valid {
		return oauth.OAuthResult{
			Success:      false,
			ProviderID:   providerID,
			ProviderType: oauth.ProviderQwenAI,
			Error:        validation.Error,
		}, nil
	}

	a.EmitProgress(oauth.OAuthStatusSuccess, "Token validation successful", nil)
	return oauth.OAuthResult{
		Success:      true,
		ProviderID:   providerID,
		ProviderType: oauth.ProviderQwenAI,
		Credentials:  map[string]string{"token": token},
		AccountInfo:  validation.AccountInfo,
	}, nil
}

// ValidateToken validates the Qwen AI token.
func (a *QwenAIAdapter) ValidateToken(credentials map[string]string) (oauth.TokenValidationResult, error) {
	token := credentials["token"]
	if token == "" {
		return oauth.TokenValidationResult{Valid: false, Error: "Token cannot be empty"}, nil
	}

	// Primary validation: parse JWT payload
	if oauth.IsJWT(token) {
		payload, err := oauth.ParseJWT(token)
		if err == nil {
			// Check for required fields
			hasID := false
			for _, key := range []string{"sub", "id", "user_id", "uid"} {
				if _, ok := payload[key]; ok {
					hasID = true
					break
				}
			}
			if !hasID {
				return oauth.TokenValidationResult{Valid: false, Error: "JWT missing required identity fields"}, nil
			}

			// Guest check
			if email, ok := payload["email"].(string); ok && oauth.IsGuestEmail(email) {
				return oauth.TokenValidationResult{Valid: false, Error: "Guest accounts are not allowed"}, nil
			}

			accountInfo := &oauth.OAuthAccountInfo{}
			if id, ok := payload["sub"].(string); ok {
				accountInfo.UserID = id
			} else if id, ok := payload["id"].(string); ok {
				accountInfo.UserID = id
			} else if id, ok := payload["user_id"].(string); ok {
				accountInfo.UserID = id
			} else if id, ok := payload["uid"].(string); ok {
				accountInfo.UserID = id
			}
			if email, ok := payload["email"].(string); ok {
				accountInfo.Email = email
			}
			if name, ok := payload["name"].(string); ok {
				accountInfo.Name = name
			}

			// Secondary: API verification
			apiValidation := a.validateTokenViaAPI(token)
			if apiValidation != nil {
				return *apiValidation, nil
			}

			return oauth.TokenValidationResult{
				Valid:       true,
				TokenType:   oauth.TokenTypeJWT,
				AccountInfo: accountInfo,
			}, nil
		}
	}

	return oauth.TokenValidationResult{Valid: false, Error: "Invalid token format"}, nil
}

// validateTokenViaAPI performs API-based token validation.
func (a *QwenAIAdapter) validateTokenViaAPI(token string) *oauth.TokenValidationResult {
	client := &http.Client{Timeout: 15 * time.Second}
	req, err := http.NewRequest("GET", qwenAIAPIBase+"/api/v2/user/info", nil)
	if err != nil {
		return nil
	}

	req.Header.Set("Authorization", "Bearer "+token)
	for k, v := range qwenAIFakeHeaders {
		req.Header.Set(k, v)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil
	}

	var result struct {
		ID      string `json:"id"`
		Email   string `json:"email"`
		Name    string `json:"name"`
		IsGuest bool   `json:"is_guest"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil
	}

	if result.IsGuest || oauth.IsGuestEmail(result.Email) {
		fmt.Println("[QwenAI] Guest account detected:",
			"isGuest=", result.IsGuest,
			"email=", result.Email)
		return &oauth.TokenValidationResult{Valid: false, Error: "Guest accounts are not allowed"}
	}

	fmt.Println("[QwenAI] API validation passed:",
		"id=", result.ID,
		"email=", result.Email,
		"name=", result.Name)

	return &oauth.TokenValidationResult{
		Valid:     true,
		TokenType: oauth.TokenTypeJWT,
		AccountInfo: &oauth.OAuthAccountInfo{
			UserID: result.ID,
			Email:  result.Email,
			Name:   result.Name,
		},
	}
}

// RefreshToken attempts to refresh the token.
func (a *QwenAIAdapter) RefreshToken(credentials map[string]string) (*oauth.CredentialInfo, error) {
	return nil, fmt.Errorf("token refresh not supported for Qwen AI")
}
