package adapter

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/fssl168/chat2api-go/oauth/pkg/oauth"
)

const minimaxAPIBase = "https://agent.minimaxi.com"

var minimaxFakeHeaders = map[string]string{
	"Accept":             "application/json, text/plain, */*",
	"Accept-Encoding":    "gzip, deflate, br, zstd",
	"Accept-Language":    "zh-CN,zh;q=0.9",
	"Cache-Control":      "no-cache",
	"Content-Type":       "application/json",
	"Origin":             "https://agent.minimaxi.com",
	"Pragma":             "no-cache",
	"Referer":            "https://agent.minimaxi.com/",
	"Sec-Ch-Ua":          `"Not:A-Brand";v="99", "Google Chrome";v="145", "Chromium";v="145"`,
	"Sec-Ch-Ua-Mobile":   "?0",
	"Sec-Ch-Ua-Platform": `"macOS"`,
	"Sec-Fetch-Dest":     "empty",
	"Sec-Fetch-Mode":     "cors",
	"Sec-Fetch-Site":     "same-origin",
	"User-Agent":         "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/145.0.0.0 Safari/537.36",
}

// MiniMaxAdapter implements OAuth for MiniMax (Hailuo).
type MiniMaxAdapter struct {
	*BaseOAuthAdapter
}

// NewMiniMaxAdapter creates a new MiniMax adapter.
func NewMiniMaxAdapter(config oauth.AdapterConfig) *MiniMaxAdapter {
	config.ProviderType = oauth.ProviderMiniMax
	config.AuthMethods = []oauth.AuthMethod{oauth.AuthMethodManual, oauth.AuthMethodToken}
	config.LoginURL = minimaxAPIBase
	config.APIURL = minimaxAPIBase
	return &MiniMaxAdapter{
		BaseOAuthAdapter: NewBaseOAuthAdapter(config),
	}
}

// StartLogin opens the browser for manual login.
func (a *MiniMaxAdapter) StartLogin(options oauth.OAuthOptions) (oauth.OAuthResult, error) {
	return a.DefaultStartLogin(options, oauth.ProviderMiniMax)
}

// LoginWithToken completes authentication with manually entered tokens.
// For MiniMax, token format should be "realUserID_token" or provided as separate fields.
func (a *MiniMaxAdapter) LoginWithToken(providerID string, token string, extras ...string) (oauth.OAuthResult, error) {
	a.EmitProgress(oauth.OAuthStatusPending, "Validating Token...", nil)

	credentials := map[string]string{"token": token}
	// If extras[0] is provided, it's the realUserID
	if len(extras) > 0 && extras[0] != "" {
		credentials["realUserID"] = extras[0]
	}

	validation, err := a.ValidateToken(credentials)
	if err != nil {
		return oauth.OAuthResult{
			Success:      false,
			ProviderID:   providerID,
			ProviderType: oauth.ProviderMiniMax,
			Error:        err.Error(),
		}, nil
	}

	if !validation.Valid {
		return oauth.OAuthResult{
			Success:      false,
			ProviderID:   providerID,
			ProviderType: oauth.ProviderMiniMax,
			Error:        validation.Error,
		}, nil
	}

	a.EmitProgress(oauth.OAuthStatusSuccess, "Token validation successful", nil)

	resultCreds := map[string]string{"token": token}
	if len(extras) > 0 && extras[0] != "" {
		resultCreds["realUserID"] = extras[0]
	}

	return oauth.OAuthResult{
		Success:      true,
		ProviderID:   providerID,
		ProviderType: oauth.ProviderMiniMax,
		Credentials:  resultCreds,
		AccountInfo:  validation.AccountInfo,
	}, nil
}

// ValidateToken validates the MiniMax token.
func (a *MiniMaxAdapter) ValidateToken(credentials map[string]string) (oauth.TokenValidationResult, error) {
	token := credentials["token"]
	realUserID := credentials["realUserID"]

	// If token contains plus sign, split it (matching chat2api: realUserID+jwtToken)
	if realUserID == "" && strings.Contains(token, "+") {
		parts := strings.SplitN(token, "+", 2)
		realUserID = parts[0]
		token = parts[1]
	}

	// Fallback: underscore separator (legacy format)
	if realUserID == "" && strings.Contains(token, "_") {
		parts := strings.SplitN(token, "_", 2)
		realUserID = parts[0]
		token = parts[1]
	}

	// Fallback: extract realUserID from JWT payload (matching chat2api: user.id or sub)
	if realUserID == "" && oauth.IsJWT(token) {
		if payload, err := oauth.ParseJWT(token); err == nil {
			if userObj, ok := payload["user"].(map[string]interface{}); ok {
				if id, ok := userObj["id"].(string); ok && id != "" {
					realUserID = id
				} else if id, ok := userObj["id"].(float64); ok {
					realUserID = fmt.Sprintf("%.0f", id)
				}
			}
			if realUserID == "" {
				if sub, ok := payload["sub"].(string); ok && sub != "" {
					realUserID = sub
				}
			}
		}
	}

	if token == "" {
		return oauth.TokenValidationResult{Valid: false, Error: "Token cannot be empty"}, nil
	}

	timestamp := fmt.Sprintf("%d", oauth.GetTimestampMs())
	signature := oauth.MD5(timestamp + token + "{}")

	// Build query parameters
	queryParams := url.Values{}
	queryParams.Set("device_platform", "web")
	queryParams.Set("biz_id", "3")
	queryParams.Set("app_id", "3001")
	queryParams.Set("version_code", "22201")
	queryParams.Set("unix", timestamp)
	queryParams.Set("lang", "zh")
	queryParams.Set("token", token)

	if realUserID != "" {
		queryParams.Set("uuid", realUserID)
		queryParams.Set("user_id", realUserID)
	}

	apiURL := minimaxAPIBase + "/v1/api/user/info?" + queryParams.Encode()

	// YY header: md5(encodeURIComponent(fullUri) + '_' + '{}' + md5(unix) + 'ooui')
	fullURI := apiURL
	yyHeader := oauth.MD5(url.QueryEscape(fullURI) + "_{}" + oauth.MD5(timestamp) + "ooui")

	client := &http.Client{Timeout: 15 * time.Second}
	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return oauth.TokenValidationResult{Valid: false, Error: err.Error()}, nil
	}

	req.Header.Set("token", token)
	req.Header.Set("x-timestamp", timestamp)
	req.Header.Set("x-signature", signature)
	req.Header.Set("yy", yyHeader)
	for k, v := range minimaxFakeHeaders {
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
		StatusInfo struct {
			Code int `json:"code"`
		} `json:"statusInfo"`
		Data struct {
			UserInfo struct {
				UserID   string `json:"userId"`
				NickName string `json:"nickName"`
			} `json:"userInfo"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return oauth.TokenValidationResult{Valid: false, Error: err.Error()}, nil
	}

	fmt.Println("[MiniMax] API response received:",
		"userID=", result.Data.UserInfo.UserID,
		"nickName=", result.Data.UserInfo.NickName,
		"statusCode=", result.StatusInfo.Code)

	if result.StatusInfo.Code != 0 {
		return oauth.TokenValidationResult{Valid: false, Error: fmt.Sprintf("API error code: %d", result.StatusInfo.Code)}, nil
	}

	if result.Data.UserInfo.UserID == "" || result.Data.UserInfo.UserID == "0" {
		fmt.Println("[MiniMax] Guest account detected: empty or zero userID")
		return oauth.TokenValidationResult{Valid: false, Error: "Guest accounts are not allowed (invalid userID)"}, nil
	}
	if oauth.IsGuestNickname(result.Data.UserInfo.NickName) {
		fmt.Println("[MiniMax] Guest account detected: nickname indicates guest")
		return oauth.TokenValidationResult{Valid: false, Error: "Guest accounts are not allowed (nickname indicates guest)"}, nil
	}

	return oauth.TokenValidationResult{
		Valid:     true,
		TokenType: oauth.TokenTypeJWT,
		AccountInfo: &oauth.OAuthAccountInfo{
			UserID: result.Data.UserInfo.UserID,
			Name:   result.Data.UserInfo.NickName,
		},
	}, nil
}

// RefreshToken attempts to refresh the token.
func (a *MiniMaxAdapter) RefreshToken(credentials map[string]string) (*oauth.CredentialInfo, error) {
	return nil, fmt.Errorf("token refresh not supported for MiniMax")
}
