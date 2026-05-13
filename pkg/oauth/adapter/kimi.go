package adapter

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"time"

	"github.com/fssl168/chat2api-go/oauth/pkg/oauth"
)

const kimiAPIBase = "https://www.kimi.com"

var kimiFakeHeaders = map[string]string{
	"Accept":             "*/*",
	"Accept-Encoding":    "gzip, deflate, br, zstd",
	"Accept-Language":    "zh-CN,zh;q=0.9,en-US;q=0.8,en;q=0.7",
	"Cache-Control":      "no-cache",
	"Content-Type":       "application/json",
	"Origin":             "https://www.kimi.com",
	"Pragma":             "no-cache",
	"Priority":           "u=1, i",
	"Referer":            "https://www.kimi.com/",
	"R-Timezone":         "Asia/Shanghai",
	"Sec-Ch-Ua":          `"Google Chrome";v="131", "Chromium";v="131", "Not_A Brand";v="24"`,
	"Sec-Ch-Ua-Mobile":   "?0",
	"Sec-Ch-Ua-Platform": `"Windows"`,
	"Sec-Fetch-Dest":     "empty",
	"Sec-Fetch-Mode":     "cors",
	"Sec-Fetch-Site":     "same-origin",
	"User-Agent":         "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36",
	"X-Msh-Platform":     "web",
}

// KimiAdapter implements OAuth for Kimi (Moonshot).
type KimiAdapter struct {
	*BaseOAuthAdapter
}

// NewKimiAdapter creates a new Kimi adapter.
func NewKimiAdapter(config oauth.AdapterConfig) *KimiAdapter {
	config.ProviderType = oauth.ProviderKimi
	config.AuthMethods = []oauth.AuthMethod{oauth.AuthMethodManual, oauth.AuthMethodToken}
	config.LoginURL = kimiAPIBase
	config.APIURL = kimiAPIBase
	return &KimiAdapter{
		BaseOAuthAdapter: NewBaseOAuthAdapter(config),
	}
}

// StartLogin opens the browser for manual login.
func (a *KimiAdapter) StartLogin(options oauth.OAuthOptions) (oauth.OAuthResult, error) {
	return a.DefaultStartLogin(options, oauth.ProviderKimi)
}

// LoginWithToken completes authentication with a manually entered token.
func (a *KimiAdapter) LoginWithToken(providerID string, token string, extras ...string) (oauth.OAuthResult, error) {
	a.EmitProgress(oauth.OAuthStatusPending, "Validating Token...", nil)

	validation, err := a.ValidateToken(map[string]string{"token": token})
	if err != nil {
		return oauth.OAuthResult{
			Success:      false,
			ProviderID:   providerID,
			ProviderType: oauth.ProviderKimi,
			Error:        err.Error(),
		}, nil
	}

	if !validation.Valid {
		return oauth.OAuthResult{
			Success:      false,
			ProviderID:   providerID,
			ProviderType: oauth.ProviderKimi,
			Error:        validation.Error,
		}, nil
	}

	a.EmitProgress(oauth.OAuthStatusSuccess, "Token validation successful", nil)
	return oauth.OAuthResult{
		Success:      true,
		ProviderID:   providerID,
		ProviderType: oauth.ProviderKimi,
		Credentials:  map[string]string{"token": token},
		AccountInfo:  validation.AccountInfo,
	}, nil
}

// generateKimiDeviceID generates a random 19-digit device ID starting with 7.
func generateKimiDeviceID() string {
	return "7" + randDigits(18)
}

// generateKimiSessionID generates a random 18-digit session ID starting with 17.
func generateKimiSessionID() string {
	return "17" + randDigits(16)
}

func randDigits(n int) string {
	digits := make([]byte, n)
	for i := range digits {
		digits[i] = byte('0' + rand.Intn(10))
	}
	return string(digits)
}

// ValidateToken validates the token via Kimi API.
// Supports multiple credential field aliases: accessToken, token, access_token, apiKey, api_key.
func (a *KimiAdapter) ValidateToken(credentials map[string]string) (oauth.TokenValidationResult, error) {
	// Try multiple possible credential keys (matching chat2api implementation)
	token := credentials["accessToken"]
	if token == "" {
		token = credentials["token"]
	}
	if token == "" {
		token = credentials["access_token"]
	}
	if token == "" {
		token = credentials["apiKey"]
	}
	if token == "" {
		token = credentials["api_key"]
	}
	if token == "" {
		return oauth.TokenValidationResult{Valid: false, Error: "Token cannot be empty"}, nil
	}

	deviceID := generateKimiDeviceID()
	sessionID := generateKimiSessionID()

	// Try to extract device_id and ssid from JWT payload
	if oauth.IsJWT(token) {
		payload, err := oauth.ParseJWT(token)
		if err == nil {
			if did, ok := payload["device_id"].(string); ok && did != "" {
				deviceID = did
			}
			if sid, ok := payload["ssid"].(string); ok && sid != "" {
				sessionID = sid
			}
		}
	}

	// Try 1: Bearer token authentication (for API key / JWT)
	result, bearerErr := a.doValidateRequest(token, deviceID, sessionID, true)
	if bearerErr == nil && result.Valid {
		return result, nil
	}

	// Try 2: Cookie authentication (for kimi-auth cookie extracted from browser)
	result, cookieErr := a.doValidateRequest(token, deviceID, sessionID, false)
	if cookieErr == nil && result.Valid {
		return result, nil
	}

	// Both failed - return the error from Bearer attempt (more common case)
	if bearerErr != nil {
		return oauth.TokenValidationResult{Valid: false, Error: bearerErr.Error()}, nil
	}
	return oauth.TokenValidationResult{Valid: false, Error: "Token is invalid or expired"}, nil
}

// doValidateRequest performs the actual validation request.
// If useBearer is true, sends token as Authorization: Bearer header.
// If useBearer is false, sends token as Cookie: kimi-auth header.
func (a *KimiAdapter) doValidateRequest(token, deviceID, sessionID string, useBearer bool) (oauth.TokenValidationResult, error) {
	client := &http.Client{Timeout: 15 * time.Second}
	body := map[string]interface{}{}
	bodyBytes, _ := json.Marshal(body)

	req, err := http.NewRequest("POST", kimiAPIBase+"/apiv2/kimi.gateway.order.v1.SubscriptionService/GetSubscription", bytes.NewReader(bodyBytes))
	if err != nil {
		return oauth.TokenValidationResult{}, err
	}

	if useBearer {
		req.Header.Set("Authorization", "Bearer "+token)
	} else {
		req.Header.Set("Cookie", "kimi-auth="+token)
	}
	req.Header.Set("X-Msh-Device-Id", deviceID)
	req.Header.Set("X-Msh-Session-Id", sessionID)
	for k, v := range kimiFakeHeaders {
		req.Header.Set(k, v)
	}

	resp, err := client.Do(req)
	if err != nil {
		return oauth.TokenValidationResult{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return oauth.TokenValidationResult{}, fmt.Errorf("HTTP %d: Token is invalid or expired", resp.StatusCode)
	}

	var result struct {
		Subscription struct {
			UserID   string `json:"userId"`
			UserName string `json:"userName"`
		} `json:"subscription"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return oauth.TokenValidationResult{}, err
	}

	fmt.Println("[Kimi] API response received:",
		"userID=", result.Subscription.UserID,
		"userName=", result.Subscription.UserName)

	if result.Subscription.UserID == "" || result.Subscription.UserID == "0" {
		fmt.Println("[Kimi] Guest account detected: empty or zero userID")
		return oauth.TokenValidationResult{
			Valid: false,
			Error: "Guest accounts are not allowed (invalid userID)",
		}, nil
	}
	if oauth.IsGuestNickname(result.Subscription.UserName) {
		fmt.Println("[Kimi] Guest account detected: nickname indicates guest")
		return oauth.TokenValidationResult{
			Valid: false,
			Error: "Guest accounts are not allowed (nickname indicates guest)",
		}, nil
	}

	return oauth.TokenValidationResult{
		Valid:     true,
		TokenType: oauth.TokenTypeJWT,
		AccountInfo: &oauth.OAuthAccountInfo{
			UserID: result.Subscription.UserID,
			Name:   result.Subscription.UserName,
		},
	}, nil
}

// RefreshToken attempts to refresh the token.
func (a *KimiAdapter) RefreshToken(credentials map[string]string) (*oauth.CredentialInfo, error) {
	return nil, fmt.Errorf("token refresh not supported for Kimi")
}
