package adapter

import (
	"fmt"
	"time"

	"github.com/fssl168/chat2api-go/oauth/pkg/oauth"
	"github.com/fssl168/chat2api-go/oauth/pkg/oauth/browser"
)

const (
	defaultCallbackPort = 8311
	defaultTimeout      = 300000 // 5 minutes in milliseconds
)

// OAuthManager manages authentication flows for providers.
type OAuthManager struct {
	adapters         map[string]OAuthAdapter
	progressCallback oauth.ProgressCallback
	currentLogin     *currentLoginState
}

type currentLoginState struct {
	providerID string
	adapter    OAuthAdapter
	timeout    *time.Timer
}

// NewOAuthManager creates a new OAuth manager.
func NewOAuthManager() *OAuthManager {
	return &OAuthManager{
		adapters: make(map[string]OAuthAdapter),
	}
}

// SetProgressCallback sets the progress callback function.
func (m *OAuthManager) SetProgressCallback(callback oauth.ProgressCallback) {
	m.progressCallback = callback
}

// getAdapter returns or creates an adapter for the given provider.
func (m *OAuthManager) getAdapter(providerID string, providerType oauth.ProviderType) (OAuthAdapter, error) {
	key := fmt.Sprintf("%s_%s", providerID, providerType)

	if adapter, ok := m.adapters[key]; ok {
		return adapter, nil
	}

	adpt, err := CreateAdapter(providerType, oauth.AdapterConfig{
		ProviderID:   providerID,
		ProviderType: providerType,
		AuthMethods:  []oauth.AuthMethod{},
		CallbackPort: defaultCallbackPort,
		LoginURL:     browser.ProviderLoginURL[providerType],
	})
	if err != nil {
		return nil, err
	}

	if m.progressCallback != nil {
		adpt.SetProgressCallback(m.progressCallback)
	}

	m.adapters[key] = adpt
	return adpt, nil
}

// StartLogin starts a standard OAuth login flow (opens system browser).
func (m *OAuthManager) StartLogin(options oauth.OAuthOptions) (oauth.OAuthResult, error) {
	if m.currentLogin != nil {
		return oauth.OAuthResult{
			Success:      false,
			ProviderID:   options.ProviderID,
			ProviderType: options.ProviderType,
			Error:        "A login process is already in progress",
		}, nil
	}

	adpt, err := m.getAdapter(options.ProviderID, options.ProviderType)
	if err != nil {
		return oauth.OAuthResult{
			Success:      false,
			ProviderID:   options.ProviderID,
			ProviderType: options.ProviderType,
			Error:        err.Error(),
		}, nil
	}

	timeout := options.Timeout
	if timeout <= 0 {
		timeout = defaultTimeout
	}

	timer := time.AfterFunc(time.Duration(timeout)*time.Millisecond, func() {
		m.CancelLogin()
		if m.progressCallback != nil {
			m.progressCallback(oauth.OAuthProgressEvent{
				Status:  oauth.OAuthStatusError,
				Message: "Login timeout",
			})
		}
	})

	m.currentLogin = &currentLoginState{
		providerID: options.ProviderID,
		adapter:    adpt,
		timeout:    timer,
	}

	if m.progressCallback != nil {
		m.progressCallback(oauth.OAuthProgressEvent{
			Status:  oauth.OAuthStatusPending,
			Message: "Opening browser...",
		})
	}

	result, err := adpt.StartLogin(options)
	m.cleanupLogin()
	return result, err
}

// StartLoginWithBrowser uses Playwright for automated login and token extraction.
// It now uses InAppLoginManager for a refined popup login experience.
func (m *OAuthManager) StartLoginWithBrowser(options oauth.OAuthOptions) (oauth.OAuthResult, error) {
	if m.currentLogin != nil {
		return oauth.OAuthResult{
			Success:      false,
			ProviderID:   options.ProviderID,
			ProviderType: options.ProviderType,
			Error:        "A login process is already in progress",
		}, nil
	}

	adpt, err := m.getAdapter(options.ProviderID, options.ProviderType)
	if err != nil {
		return oauth.OAuthResult{
			Success:      false,
			ProviderID:   options.ProviderID,
			ProviderType: options.ProviderType,
			Error:        err.Error(),
		}, nil
	}

	// Try InAppLogin first (new refined popup login)
	if extAdpt, ok := interface{}(adpt).(interface {
		StartInAppLogin(options oauth.OAuthOptions) (oauth.OAuthResult, error)
	}); ok {
		return extAdpt.StartInAppLogin(options)
	}

	// Fallback to old StartLoginWithBrowser
	if extAdpt, ok := interface{}(adpt).(interface {
		StartLoginWithBrowser(options oauth.OAuthOptions) (oauth.OAuthResult, error)
	}); ok {
		return extAdpt.StartLoginWithBrowser(options)
	}

	// 如果不支持，回退到默认方法
	if m.progressCallback != nil {
		m.progressCallback(oauth.OAuthProgressEvent{
			Status:  oauth.OAuthStatusPending,
			Message: "Browser automation not supported for this adapter, using manual login",
		})
	}

	return m.StartLogin(options)
}

// StartLoginWithBrowserAndLogs uses Playwright for automated login and returns both result and flow logs.
func (m *OAuthManager) StartLoginWithBrowserAndLogs(options oauth.OAuthOptions) (oauth.OAuthResult, *oauth.FlowLogger, error) {
	if m.currentLogin != nil {
		return oauth.OAuthResult{
			Success:      false,
			ProviderID:   options.ProviderID,
			ProviderType: options.ProviderType,
			Error:        "A login process is already in progress",
		}, nil, nil
	}

	adpt, err := m.getAdapter(options.ProviderID, options.ProviderType)
	if err != nil {
		return oauth.OAuthResult{
			Success:      false,
			ProviderID:   options.ProviderID,
			ProviderType: options.ProviderType,
			Error:        err.Error(),
		}, nil, nil
	}

	// Try InAppLogin first (new refined popup login)
	if extAdpt, ok := interface{}(adpt).(interface {
		StartInAppLogin(options oauth.OAuthOptions) (oauth.OAuthResult, error)
	}); ok {
		result, err := extAdpt.StartInAppLogin(options)
		return result, nil, err
	}

	// Fallback to old StartLoginWithBrowserAndLogs
	if extAdpt, ok := interface{}(adpt).(interface {
		StartLoginWithBrowserAndLogs(options oauth.OAuthOptions) (oauth.OAuthResult, *oauth.FlowLogger, error)
	}); ok {
		return extAdpt.StartLoginWithBrowserAndLogs(options)
	}

	// 如果不支持，回退到默认方法
	result, err := m.StartLogin(options)
	return result, nil, err
}

// LoginWithToken completes authentication with a manually entered token.
func (m *OAuthManager) LoginWithToken(providerID string, providerType oauth.ProviderType, token string, extras ...string) (oauth.OAuthResult, error) {
	adpt, err := m.getAdapter(providerID, providerType)
	if err != nil {
		return oauth.OAuthResult{
			Success:      false,
			ProviderID:   providerID,
			ProviderType: providerType,
			Error:        err.Error(),
		}, nil
	}

	return adpt.LoginWithToken(providerID, token, extras...)
}

// ValidateToken validates the given credentials.
func (m *OAuthManager) ValidateToken(providerID string, providerType oauth.ProviderType, credentials map[string]string) (oauth.TokenValidationResult, error) {
	adpt, err := m.getAdapter(providerID, providerType)
	if err != nil {
		return oauth.TokenValidationResult{Valid: false, Error: err.Error()}, nil
	}

	return adpt.ValidateToken(credentials)
}

// RefreshToken attempts to refresh the token.
func (m *OAuthManager) RefreshToken(providerID string, providerType oauth.ProviderType, credentials map[string]string) (*oauth.CredentialInfo, error) {
	adpt, err := m.getAdapter(providerID, providerType)
	if err != nil {
		return nil, err
	}

	return adpt.RefreshToken(credentials)
}

// CancelLogin cancels the current login process.
func (m *OAuthManager) CancelLogin() {
	if m.currentLogin != nil {
		m.currentLogin.adapter.CancelLogin()
		m.cleanupLogin()
	}
}

// cleanupLogin cleans up the current login state.
func (m *OAuthManager) cleanupLogin() {
	if m.currentLogin != nil {
		if m.currentLogin.timeout != nil {
			m.currentLogin.timeout.Stop()
		}
		m.currentLogin = nil
	}
}

// GetStatus returns the current login status.
func (m *OAuthManager) GetStatus() oauth.OAuthStatus {
	if m.currentLogin != nil {
		return oauth.OAuthStatusPending
	}
	return oauth.OAuthStatusIdle
}

// Destroy cleans up all resources.
func (m *OAuthManager) Destroy() {
	m.CancelLogin()
	for _, adpt := range m.adapters {
		adpt.CancelLogin()
	}
	m.adapters = make(map[string]OAuthAdapter)
}

// GetSupportedProviders returns a list of all supported provider types.
func GetSupportedProviders() []oauth.ProviderType {
	return []oauth.ProviderType{
		oauth.ProviderDeepSeek,
		oauth.ProviderGLM,
		oauth.ProviderKimi,
		oauth.ProviderMiniMax,
		oauth.ProviderQwen,
		oauth.ProviderQwenAI,
		oauth.ProviderZai,
		oauth.ProviderPerplexity,
		oauth.ProviderMimo,
	}
}
