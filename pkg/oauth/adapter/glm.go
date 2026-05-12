package adapter

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/fssl168/chat2api-go/oauth/pkg/oauth"
)

const glmAPIBase = "https://chatglm.cn"
const glmSignSecret = "8a1317a7468aa3ad86e997d08f3f31cb"

var glmFakeHeaders = map[string]string{
	"Accept":             "text/event-stream",
	"Accept-Encoding":    "gzip, deflate, br, zstd",
	"Accept-Language":    "zh-CN,zh;q=0.9,en;q=0.8,en-GB;q=0.7,en-US;q=0.6",
	"App-Name":           "chatglm",
	"Cache-Control":      "no-cache",
	"Content-Type":       "application/json",
	"Origin":             "https://chatglm.cn",
	"Pragma":             "no-cache",
	"Priority":           "u=1, i",
	"Referer":            "https://chatglm.cn/",
	"Sec-Ch-Ua":          `"Microsoft Edge";v="143", "Chromium";v="143", "Not A(Brand";v="24"`,
	"Sec-Ch-Ua-Mobile":   "?0",
	"Sec-Ch-Ua-Platform": `"Windows"`,
	"Sec-Fetch-Dest":     "empty",
	"Sec-Fetch-Mode":     "cors",
	"Sec-Fetch-Site":     "same-origin",
	"User-Agent":         "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/143.0.0.0 Safari/537.36 Edg/143.0.0.0",
	"X-App-Fr":           "browser_extension",
	"X-App-Platform":     "pc",
	"X-App-Version":      "0.0.1",
	"X-Device-Brand":     "",
	"X-Device-Model":     "",
	"X-Lang":             "zh",
}

// GLMAdapter implements OAuth for GLM (Zhipu).
type GLMAdapter struct {
	*BaseOAuthAdapter
}

// NewGLMAdapter creates a new GLM adapter.
func NewGLMAdapter(config oauth.AdapterConfig) *GLMAdapter {
	config.ProviderType = oauth.ProviderGLM
	config.AuthMethods = []oauth.AuthMethod{oauth.AuthMethodManual, oauth.AuthMethodToken}
	config.LoginURL = glmAPIBase
	config.APIURL = glmAPIBase
	return &GLMAdapter{
		BaseOAuthAdapter: NewBaseOAuthAdapter(config),
	}
}

// StartLogin opens the browser for manual login.
func (a *GLMAdapter) StartLogin(options oauth.OAuthOptions) (oauth.OAuthResult, error) {
	return a.DefaultStartLogin(options, oauth.ProviderGLM)
}

// LoginWithToken completes authentication with a manually entered token.
func (a *GLMAdapter) LoginWithToken(providerID string, token string, extras ...string) (oauth.OAuthResult, error) {
	a.EmitProgress(oauth.OAuthStatusPending, "Validating Token...", nil)

	validation, err := a.ValidateToken(map[string]string{"refresh_token": token})
	if err != nil {
		return oauth.OAuthResult{
			Success:      false,
			ProviderID:   providerID,
			ProviderType: oauth.ProviderGLM,
			Error:        err.Error(),
		}, nil
	}

	if !validation.Valid {
		return oauth.OAuthResult{
			Success:      false,
			ProviderID:   providerID,
			ProviderType: oauth.ProviderGLM,
			Error:        validation.Error,
		}, nil
	}

	a.EmitProgress(oauth.OAuthStatusSuccess, "Token validation successful", nil)
	return oauth.OAuthResult{
		Success:      true,
		ProviderID:   providerID,
		ProviderType: oauth.ProviderGLM,
		Credentials:  map[string]string{"refresh_token": token},
		AccountInfo:  validation.AccountInfo,
	}, nil
}

// generateGLMSign generates the custom signature for GLM API requests.
func generateGLMSign(timestamp, nonce string) string {
	digits := make([]int, len(timestamp))
	sum := 0
	for i, ch := range timestamp {
		digits[i] = int(ch - '0')
		sum += digits[i]
	}
	modified := timestamp[:len(timestamp)-2] + strconv.Itoa((sum-digits[len(digits)-2])%10) + timestamp[len(timestamp)-1:]
	return oauth.MD5(fmt.Sprintf("%s-%s-%s", modified, nonce, glmSignSecret))
}

// ValidateToken validates the refresh token via GLM API.
func (a *GLMAdapter) ValidateToken(credentials map[string]string) (oauth.TokenValidationResult, error) {
	refreshToken := credentials["refresh_token"]
	if refreshToken == "" {
		return oauth.TokenValidationResult{Valid: false, Error: "Refresh token cannot be empty"}, nil
	}

	timestamp := fmt.Sprintf("%d", oauth.GetTimestampMs())
	nonce := oauth.GenerateUUID()
	sign := generateGLMSign(timestamp, nonce)
	deviceID := oauth.GenerateUUID()
	requestID := oauth.GenerateUUID()

	client := &http.Client{Timeout: 15 * time.Second}
	req, err := http.NewRequest("POST", glmAPIBase+"/chatglm/user-api/user/refresh", bytes.NewReader([]byte("{}")))
	if err != nil {
		return oauth.TokenValidationResult{Valid: false, Error: err.Error()}, nil
	}

	req.Header.Set("Authorization", "Bearer "+refreshToken)
	req.Header.Set("X-Device-Id", deviceID)
	req.Header.Set("X-Nonce", nonce)
	req.Header.Set("X-Request-Id", requestID)
	req.Header.Set("X-Sign", sign)
	req.Header.Set("X-Timestamp", timestamp)
	for k, v := range glmFakeHeaders {
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
		Result struct {
			AccessToken  string `json:"access_token"`
			RefreshToken string `json:"refresh_token"`
			UserID       string `json:"user_id"`
			IsGuest      bool   `json:"is_guest"`
			Nickname     string `json:"nickname"`
			Email        string `json:"email"`
		} `json:"result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return oauth.TokenValidationResult{Valid: false, Error: err.Error()}, nil
	}

	// Guest check
	if result.Result.IsGuest || containsStr(result.Result.Nickname, "访客") || containsStr(result.Result.Email, "@guest") {
		return oauth.TokenValidationResult{Valid: false, Error: "Guest accounts are not allowed"}, nil
	}

	return oauth.TokenValidationResult{
		Valid:     true,
		TokenType: oauth.TokenTypeRefresh,
		AccountInfo: &oauth.OAuthAccountInfo{
			UserID: result.Result.UserID,
			Email:  result.Result.Email,
			Name:   result.Result.Nickname,
		},
	}, nil
}

// RefreshToken refreshes the token using GLM's refresh endpoint.
func (a *GLMAdapter) RefreshToken(credentials map[string]string) (*oauth.CredentialInfo, error) {
	validation, err := a.ValidateToken(credentials)
	if err != nil || !validation.Valid {
		return nil, fmt.Errorf("refresh failed: %v", validation.Error)
	}

	refreshToken := credentials["refresh_token"]
	if refreshToken == "" {
		return nil, fmt.Errorf("no refresh token available")
	}

	// The validateToken already refreshed and returned new tokens in the response
	// In a real implementation, you'd extract the new tokens from the validation response
	return &oauth.CredentialInfo{
		Type:  oauth.TokenTypeRefresh,
		Value: refreshToken,
	}, nil
}

func containsStr(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || indexOf(s, substr) >= 0)
}

func indexOf(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
