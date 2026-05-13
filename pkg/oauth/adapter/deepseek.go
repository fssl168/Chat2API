package adapter

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/fssl168/chat2api-go/oauth/pkg/oauth"
)

const deepseekAPIBase = "https://chat.deepseek.com"

var deepseekFakeHeaders = map[string]string{
	"Accept":                   "*/*",
	"Accept-Encoding":          "gzip, deflate, br, zstd",
	"Accept-Language":          "zh-CN,zh;q=0.9,en;q=0.8,en-GB;q=0.7,en-US;q=0.6",
	"Origin":                   "https://chat.deepseek.com",
	"Pragma":                   "no-cache",
	"Priority":                 "u=1, i",
	"Referer":                  "https://chat.deepseek.com/",
	"Sec-Ch-Ua":                `"Chromium";v="134", "Not:A-Brand";v="24", "Google Chrome";v="134"`,
	"Sec-Ch-Ua-Mobile":         "?0",
	"Sec-Ch-Ua-Platform":       `"macOS"`,
	"Sec-Fetch-Dest":           "empty",
	"Sec-Fetch-Mode":           "cors",
	"Sec-Fetch-Site":           "same-origin",
	"User-Agent":               "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/145.0.0.0 Safari/537.36",
	"X-App-Version":            "20241129.1",
	"X-Client-Locale":          "zh-CN",
	"X-Client-Platform":        "web",
	"X-Client-Version":         "1.8.0",
	"x-Client-Timezone-Offset": "28800",
}

// DeepSeekAdapter implements OAuth for DeepSeek.
type DeepSeekAdapter struct {
	*BaseOAuthAdapter
}

// NewDeepSeekAdapter creates a new DeepSeek adapter.
func NewDeepSeekAdapter(config oauth.AdapterConfig) *DeepSeekAdapter {
	config.ProviderType = oauth.ProviderDeepSeek
	config.AuthMethods = []oauth.AuthMethod{oauth.AuthMethodManual, oauth.AuthMethodToken}
	config.LoginURL = deepseekAPIBase
	config.APIURL = deepseekAPIBase
	base := NewBaseOAuthAdapter(config)
	base.SetValidator(func(credentials map[string]string) (oauth.TokenValidationResult, error) {
		adapter := &DeepSeekAdapter{BaseOAuthAdapter: base}
		return adapter.ValidateToken(credentials)
	})
	return &DeepSeekAdapter{
		BaseOAuthAdapter: base,
	}
}

// StartLogin opens the browser for manual login.
func (a *DeepSeekAdapter) StartLogin(options oauth.OAuthOptions) (oauth.OAuthResult, error) {
	return a.DefaultStartLogin(options, oauth.ProviderDeepSeek)
}

// LoginWithToken completes authentication with a manually entered token.
func (a *DeepSeekAdapter) LoginWithToken(providerID string, token string, extras ...string) (oauth.OAuthResult, error) {
	a.EmitProgress(oauth.OAuthStatusPending, "Validating Token...", nil)

	validation, err := a.ValidateToken(map[string]string{"token": token})
	if err != nil {
		return oauth.OAuthResult{
			Success:      false,
			ProviderID:   providerID,
			ProviderType: oauth.ProviderDeepSeek,
			Error:        err.Error(),
		}, nil
	}

	if !validation.Valid {
		return oauth.OAuthResult{
			Success:      false,
			ProviderID:   providerID,
			ProviderType: oauth.ProviderDeepSeek,
			Error:        validation.Error,
		}, nil
	}

	a.EmitProgress(oauth.OAuthStatusSuccess, "Token validation successful", nil)
	return oauth.OAuthResult{
		Success:      true,
		ProviderID:   providerID,
		ProviderType: oauth.ProviderDeepSeek,
		Credentials:  map[string]string{"token": token},
		AccountInfo:  validation.AccountInfo,
	}, nil
}

// ValidateToken validates the token via DeepSeek API.
func (a *DeepSeekAdapter) ValidateToken(credentials map[string]string) (oauth.TokenValidationResult, error) {
	token := credentials["token"]
	if token == "" {
		token = credentials["userToken"]
	}
	if token == "" {
		return oauth.TokenValidationResult{Valid: false, Error: "Token cannot be empty"}, nil
	}

	client := &http.Client{Timeout: 15 * time.Second}
	req, err := http.NewRequest("GET", deepseekAPIBase+"/api/v0/users/current", nil)
	if err != nil {
		return oauth.TokenValidationResult{Valid: false, Error: err.Error()}, nil
	}

	req.Header.Set("Authorization", "Bearer "+token)
	for k, v := range deepseekFakeHeaders {
		req.Header.Set(k, v)
	}

	resp, err := client.Do(req)
	if err != nil {
		return oauth.TokenValidationResult{Valid: false, Error: err.Error()}, nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return oauth.TokenValidationResult{Valid: false, Error: "Token is invalid or expired"}, nil
	}

	var result struct {
		Data struct {
			BizData struct {
				ID    string `json:"id"`
				Email string `json:"email"`
				Name  string `json:"name"`
			} `json:"biz_data"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return oauth.TokenValidationResult{Valid: false, Error: err.Error()}, nil
	}

	fmt.Println("[DeepSeek] API response received:",
		"userID=", result.Data.BizData.ID,
		"email=", result.Data.BizData.Email,
		"name=", result.Data.BizData.Name)

	if oauth.IsGuestEmail(result.Data.BizData.Email) {
		fmt.Println("[DeepSeek] Guest account detected: email contains @guest")
		return oauth.TokenValidationResult{Valid: false, Error: "Guest accounts are not allowed (email indicates guest)"}, nil
	}
	if oauth.IsGuestNickname(result.Data.BizData.Name) {
		fmt.Println("[DeepSeek] Guest account detected: nickname indicates guest")
		return oauth.TokenValidationResult{Valid: false, Error: "Guest accounts are not allowed (nickname indicates guest)"}, nil
	}

	return oauth.TokenValidationResult{
		Valid:     true,
		TokenType: oauth.TokenTypeAccess,
		AccountInfo: &oauth.OAuthAccountInfo{
			UserID: result.Data.BizData.ID,
			Email:  result.Data.BizData.Email,
			Name:   result.Data.BizData.Name,
		},
	}, nil
}

// RefreshToken attempts to refresh the token.
func (a *DeepSeekAdapter) RefreshToken(credentials map[string]string) (*oauth.CredentialInfo, error) {
	validation, err := a.ValidateToken(credentials)
	if err != nil || !validation.Valid {
		return nil, fmt.Errorf("token refresh not supported or validation failed")
	}
	return nil, nil
}
