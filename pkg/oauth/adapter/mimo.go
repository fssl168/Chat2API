package adapter

import (
	"fmt"
	"net/http"
	"time"

	"github.com/fssl168/chat2api-go/oauth/pkg/oauth"
)

const mimoAPIBase = "https://aistudio.xiaomimimo.com"

var mimoFakeHeaders = map[string]string{
	"Accept":             "application/json, text/plain, */*",
	"Accept-Encoding":    "gzip, deflate, br, zstd",
	"Accept-Language":    "zh-CN,zh;q=0.9,en;q=0.8",
	"Cache-Control":      "no-cache",
	"Content-Type":       "application/json",
	"Origin":             mimoAPIBase,
	"Pragma":             "no-cache",
	"Referer":            mimoAPIBase + "/",
	"Sec-Ch-Ua":          `"Chromium";v="134", "Not:A-Brand";v="24", "Google Chrome";v="134"`,
	"Sec-Ch-Ua-Mobile":   "?0",
	"Sec-Ch-Ua-Platform": `"Windows"`,
	"Sec-Fetch-Dest":     "empty",
	"Sec-Fetch-Mode":     "cors",
	"Sec-Fetch-Site":     "same-origin",
	"User-Agent":         "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/134.0.0.0 Safari/537.36",
}

type MimoAdapter struct {
	*BaseOAuthAdapter
}

func NewMimoAdapter(config oauth.AdapterConfig) *MimoAdapter {
	config.ProviderType = oauth.ProviderMimo
	config.AuthMethods = []oauth.AuthMethod{oauth.AuthMethodManual, oauth.AuthMethodCookie}
	config.LoginURL = mimoAPIBase
	config.APIURL = mimoAPIBase
	return &MimoAdapter{
		BaseOAuthAdapter: NewBaseOAuthAdapter(config),
	}
}

func (a *MimoAdapter) StartLogin(options oauth.OAuthOptions) (oauth.OAuthResult, error) {
	return a.DefaultStartLogin(options, oauth.ProviderMimo)
}

func (a *MimoAdapter) LoginWithToken(providerID string, token string, extras ...string) (oauth.OAuthResult, error) {
	a.EmitProgress(oauth.OAuthStatusPending, "Validating Mimo credentials...", nil)

	credentials := map[string]string{
		"service_token": token,
	}

	if len(extras) >= 2 {
		credentials["user_id"] = extras[0]
		credentials["ph_token"] = extras[1]
	}

	validation, err := a.ValidateToken(credentials)
	if err != nil {
		return oauth.OAuthResult{
			Success:      false,
			ProviderID:   providerID,
			ProviderType: oauth.ProviderMimo,
			Error:        err.Error(),
		}, nil
	}

	if !validation.Valid {
		return oauth.OAuthResult{
			Success:      false,
			ProviderID:   providerID,
			ProviderType: oauth.ProviderMimo,
			Error:        validation.Error,
		}, nil
	}

	a.EmitProgress(oauth.OAuthStatusSuccess, "Token validation successful", nil)
	return oauth.OAuthResult{
		Success:      true,
		ProviderID:   providerID,
		ProviderType: oauth.ProviderMimo,
		Credentials:  credentials,
		AccountInfo:  validation.AccountInfo,
	}, nil
}

func (a *MimoAdapter) ValidateToken(credentials map[string]string) (oauth.TokenValidationResult, error) {
	serviceToken := credentials["service_token"]
	userID := credentials["user_id"]
	phToken := credentials["ph_token"]

	if serviceToken == "" {
		return oauth.TokenValidationResult{Valid: false, Error: "serviceToken cannot be empty"}, nil
	}

	if userID == "" || phToken == "" {
		return oauth.TokenValidationResult{Valid: false, Error: "Mimo requires user_id and ph_token (xiaomichatbot_ph) in addition to serviceToken"}, nil
	}

	// Validate credentials via API using fake headers
	client := &http.Client{Timeout: 15 * time.Second}
	req, err := http.NewRequest("GET", mimoAPIBase+"/api/user/info", nil)
	if err != nil {
		return oauth.TokenValidationResult{Valid: false, Error: err.Error()}, nil
	}

	// Set auth cookies
	req.Header.Set("Cookie", fmt.Sprintf("serviceToken=%s; userId=%s; xiaomichatbot_ph=%s",
		serviceToken, userID, phToken))
	for k, v := range mimoFakeHeaders {
		req.Header.Set(k, v)
	}

	resp, err := client.Do(req)
	if err != nil {
		return oauth.TokenValidationResult{Valid: false, Error: err.Error()}, nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return oauth.TokenValidationResult{Valid: false, Error: fmt.Sprintf("HTTP %d: Token validation failed", resp.StatusCode)}, nil
	}

	return oauth.TokenValidationResult{
		Valid:     true,
		TokenType: oauth.TokenTypeCookie,
		AccountInfo: &oauth.OAuthAccountInfo{
			UserID: userID,
		},
	}, nil
}

func (a *MimoAdapter) RefreshToken(credentials map[string]string) (*oauth.CredentialInfo, error) {
	return nil, fmt.Errorf("token refresh not supported for Mimo")
}
