package adapter

import (
	"fmt"
	"runtime/debug"
	"strings"
	"time"

	"github.com/fssl168/chat2api-go/oauth/pkg/oauth"
	"github.com/fssl168/chat2api-go/oauth/pkg/oauth/browser"
)

// OAuthAdapter defines the interface for provider-specific OAuth adapters.
type OAuthAdapter interface {
	StartLogin(options oauth.OAuthOptions) (oauth.OAuthResult, error)
	ValidateToken(credentials map[string]string) (oauth.TokenValidationResult, error)
	RefreshToken(credentials map[string]string) (*oauth.CredentialInfo, error)
	LoginWithToken(providerID string, token string, extras ...string) (oauth.OAuthResult, error)
	SetProgressCallback(callback oauth.ProgressCallback)
	GetProviderType() oauth.ProviderType
	GetSupportedAuthMethods() []oauth.AuthMethod
	CancelLogin()
}

// BaseOAuthAdapter provides common functionality for all provider adapters.
type BaseOAuthAdapter struct {
	config           oauth.AdapterConfig
	state            string
	progressCallback oauth.ProgressCallback
	callbackServer   *oauth.CallbackServer
	validator        func(map[string]string) (oauth.TokenValidationResult, error)
}

// NewBaseOAuthAdapter creates a new base adapter with the given config.
func NewBaseOAuthAdapter(config oauth.AdapterConfig) *BaseOAuthAdapter {
	if config.CallbackPort <= 0 {
		config.CallbackPort = 8311
	}
	return &BaseOAuthAdapter{
		config: config,
	}
}

// GetProviderType returns the provider type.
func (b *BaseOAuthAdapter) GetProviderType() oauth.ProviderType {
	return b.config.ProviderType
}

// GetSupportedAuthMethods returns the supported authentication methods.
func (b *BaseOAuthAdapter) GetSupportedAuthMethods() []oauth.AuthMethod {
	return b.config.AuthMethods
}

// SetProgressCallback sets the progress callback function.
func (b *BaseOAuthAdapter) SetProgressCallback(callback oauth.ProgressCallback) {
	b.progressCallback = callback
}

// SetValidator sets the token validation function.
func (b *BaseOAuthAdapter) SetValidator(validator func(map[string]string) (oauth.TokenValidationResult, error)) {
	b.validator = validator
}

// EmitProgress emits a progress event.
func (b *BaseOAuthAdapter) EmitProgress(status oauth.OAuthStatus, message string, data map[string]interface{}) {
	if b.progressCallback != nil {
		b.progressCallback(oauth.OAuthProgressEvent{
			Status:  status,
			Message: message,
			Data:    data,
		})
	}
}

// GenerateState generates a random state string.
func (b *BaseOAuthAdapter) GenerateState() string {
	b.state = oauth.GenerateState()
	return b.state
}

// ValidateState validates the given state string.
func (b *BaseOAuthAdapter) ValidateState(state string) bool {
	return state == b.state
}

// OpenBrowser opens the login URL in the system default browser.
func (b *BaseOAuthAdapter) OpenBrowser(url string) error {
	return oauth.OpenBrowser(url)
}

// GetLoginURL returns the login URL from config.
func (b *BaseOAuthAdapter) GetLoginURL() string {
	return b.config.LoginURL
}

// GetAPIURL returns the API URL from config.
func (b *BaseOAuthAdapter) GetAPIURL() string {
	return b.config.APIURL
}

// StartCallbackServer starts the local HTTP callback server.
func (b *BaseOAuthAdapter) StartCallbackServer() (int, error) {
	b.callbackServer = oauth.NewCallbackServer(b.config.CallbackPort)
	b.callbackServer.SetState(b.state)
	return b.callbackServer.Start()
}

// StopCallbackServer stops the callback server.
func (b *BaseOAuthAdapter) StopCallbackServer() error {
	if b.callbackServer != nil {
		return b.callbackServer.Stop()
	}
	return nil
}

// GetCallbackServer returns the current callback server.
func (b *BaseOAuthAdapter) GetCallbackServer() *oauth.CallbackServer {
	return b.callbackServer
}

// CancelLogin cancels the current login flow.
func (b *BaseOAuthAdapter) CancelLogin() {
	b.StopCallbackServer()
	b.EmitProgress(oauth.OAuthStatusCancelled, "Login cancelled", nil)
}

// DefaultStartLogin opens the browser and returns a manual input prompt result.
func (b *BaseOAuthAdapter) DefaultStartLogin(options oauth.OAuthOptions, providerType oauth.ProviderType) (oauth.OAuthResult, error) {
	b.EmitProgress(oauth.OAuthStatusPending, "Opening browser...", nil)

	loginURL := b.GetLoginURL()
	if loginURL == "" {
		return oauth.OAuthResult{
			Success:      false,
			ProviderID:   options.ProviderID,
			ProviderType: providerType,
			Error:        "Login URL not configured",
		}, nil
	}

	if err := b.OpenBrowser(loginURL); err != nil {
		b.EmitProgress(oauth.OAuthStatusError, "Failed to open browser: "+err.Error(), nil)
		return oauth.OAuthResult{
			Success:      false,
			ProviderID:   options.ProviderID,
			ProviderType: providerType,
			Error:        err.Error(),
		}, nil
	}

	b.EmitProgress(oauth.OAuthStatusPending, "Please log in via browser and enter Token manually", nil)

	return oauth.OAuthResult{
		Success:      false,
		ProviderID:   options.ProviderID,
		ProviderType: providerType,
		Error:        "Please log in via browser, extract Token from Developer Tools and enter manually",
	}, nil
}

// StartLoginWithBrowser uses Playwright to automate browser login with webRequest interception and LocalStorage extraction.
func (b *BaseOAuthAdapter) StartLoginWithBrowser(options oauth.OAuthOptions) (oauth.OAuthResult, error) {
	flowLog := oauth.NewFlowLogger("browser-login-" + string(options.ProviderType))
	if b.progressCallback != nil {
		flowLog.AddCallback(b.progressCallback)
	}

	return b.doStartLoginWithBrowser(options, flowLog)
}

// StartLoginWithBrowserAndLogs uses Playwright for automated login and returns both result and flow logs.
func (b *BaseOAuthAdapter) StartLoginWithBrowserAndLogs(options oauth.OAuthOptions) (oauth.OAuthResult, *oauth.FlowLogger, error) {
	flowLog := oauth.NewFlowLogger("browser-login-" + string(options.ProviderType))
	if b.progressCallback != nil {
		flowLog.AddCallback(b.progressCallback)
	}

	result, err := b.doStartLoginWithBrowser(options, flowLog)

	return result, flowLog, err
}

// StartInAppLogin starts a new in-app browser login using InAppLoginManager.
// This provides a more refined popup login experience aligned with chat2api's InAppLoginManager.
func (b *BaseOAuthAdapter) StartInAppLogin(options oauth.OAuthOptions) (oauth.OAuthResult, error) {
	manager := browser.NewInAppLoginManager()

	// Wire up progress callbacks
	manager.SetStatusCallback(func(status browser.InAppLoginStatus, message string) {
		var oauthStatus oauth.OAuthStatus
		switch status {
		case browser.InAppStatusStarting:
			oauthStatus = oauth.OAuthStatusPending
		case browser.InAppStatusReady:
			oauthStatus = oauth.OAuthStatusPending
		case browser.InAppStatusExtracting:
			oauthStatus = oauth.OAuthStatusPending
		case browser.InAppStatusValidating:
			oauthStatus = oauth.OAuthStatusPending
		case browser.InAppStatusCompleted:
			oauthStatus = oauth.OAuthStatusSuccess
		case browser.InAppStatusCancelled:
			oauthStatus = oauth.OAuthStatusCancelled
		case browser.InAppStatusError:
			oauthStatus = oauth.OAuthStatusError
		default:
			oauthStatus = oauth.OAuthStatusPending
		}
		b.EmitProgress(oauthStatus, message, nil)
	})

	result, err := manager.StartLogin(options.ProviderType, b.getValidatorFunc())
	if err != nil {
		return oauth.OAuthResult{
			Success:      false,
			ProviderID:   options.ProviderID,
			ProviderType: options.ProviderType,
			Error:        err.Error(),
		}, nil
	}

	return oauth.OAuthResult{
		Success:      result.Success,
		ProviderID:   options.ProviderID,
		ProviderType: options.ProviderType,
		Credentials:  result.Credentials,
		AccountInfo:  result.AccountInfo,
		Error:        result.Error,
	}, nil
}

// getValidatorFunc returns the validator function for InAppLoginManager.
func (b *BaseOAuthAdapter) getValidatorFunc() func(map[string]string) (oauth.TokenValidationResult, error) {
	if b.validator == nil {
		return nil
	}
	return func(creds map[string]string) (oauth.TokenValidationResult, error) {
		return b.validator(creds)
	}
}

// doStartLoginWithBrowser contains the actual implementation of browser automation login.
func (b *BaseOAuthAdapter) doStartLoginWithBrowser(options oauth.OAuthOptions, flowLog *oauth.FlowLogger) (oauth.OAuthResult, error) {
	// 使用recover防止panic导致浏览器意外关闭
	defer func() {
		if r := recover(); r != nil {
			flowLog.Error("PANIC recovered in browser login",
				"reason", fmt.Sprintf("%v", r),
				"stack", string(debug.Stack()))
		}
	}()

	flowLog.Info("═════════════════════════════════════════")
	flowLog.Info("🌐 BROWSER AUTOMATION LOGIN FLOW STARTED")
	flowLog.Info("═════════════════════════════════════════")

	flowLog.Step(0, "Initializing browser automation login",
		"providerType", string(options.ProviderType),
		"providerId", options.ProviderID)

	loginURL := browser.ProviderLoginURL[options.ProviderType]
	if loginURL == "" {
		flowLog.Error("Login URL not configured", "providerType", string(options.ProviderType))
		return oauth.OAuthResult{
			Success:      false,
			ProviderID:   options.ProviderID,
			ProviderType: options.ProviderType,
			Error:        "Login URL not configured for " + string(options.ProviderType),
		}, nil
	}

	flowLog.Info("Configuration loaded",
		"loginURL", loginURL,
		"tokenSource", fmt.Sprintf("%+v", browser.TokenSources[options.ProviderType]))

	ctrl := browser.NewPlaywrightController(b.progressCallback)
	// 移除defer ctrl.Close()，改为手动控制浏览器关闭时机

	cfg := oauth.BrowserConfig{
			Headless:    false,
			Width:       1400,
			Height:      900,
			WindowTitle: browser.TokenSources[options.ProviderType].WindowTitle,
		}

	if err := flowLog.TimedAction("Browser Launch", func() error { return ctrl.Launch(cfg) }); err != nil {
		ctrl.Close() // 启动失败立即关闭
		return oauth.OAuthResult{
			Success:      false,
			ProviderID:   options.ProviderID,
			ProviderType: options.ProviderType,
			Error:        err.Error(),
		}, nil
	}

	extractor := browser.NewPlaywrightExtractor(ctrl)

	if err := flowLog.TimedAction("Enable webRequest Interception", func() error {
		return extractor.EnableWebRequestIntercept()
	}); err != nil {
		flowLog.Error("Failed to enable webRequest interception, but continuing...", "error", err.Error())
		// 不返回错误，继续执行
	}

	if err := flowLog.TimedAction("Navigate to Login Page", func() error {
		return ctrl.Navigate(loginURL)
	}); err != nil {
		flowLog.Error("Navigation failed", "error", err.Error())
		ctrl.Close() // 导航失败才关闭浏览器
		return oauth.OAuthResult{
			Success:      false,
			ProviderID:   options.ProviderID,
			ProviderType: options.ProviderType,
			Error:        "Failed to navigate to login page: " + err.Error(),
		}, nil
	}

	flowLog.Step(6, "⏳ Browser is now open! Waiting for you to complete login...",
		"note", "Please login in the browser window. DO NOT close it!",
		"note2", "We will automatically detect when you're logged in.",
		"timeout", "300s")

	timeout := 300
	if options.Timeout > 0 && options.Timeout < 3600 { // 最大1小时
		timeout = options.Timeout
	}

	tokenSource := browser.TokenSources[options.ProviderType]
	flowLog.Step(7, "🔍 Starting token extraction (will wait up to %ds for user to complete login)...",
		"extractionConfig", fmt.Sprintf("%+v", tokenSource),
		"timeout", timeout)

	// Build validator function that continues waiting on validation failure
	var extractValidator func(map[string]string) bool
	if b.validator != nil {
		validationFailCount := 0
		lastValidatedValue := ""
		extractValidator = func(creds map[string]string) bool {
			flowLog.Debug("Running inline validation during extraction...",
				"keys", strings.Join(getMapKeys(creds), ", "))
			valResult, valErr := b.validator(creds)
			if valErr != nil || !valResult.Valid {
				errMsg := "Token validation failed"
				isNetworkError := false
				if valErr != nil {
					errMsg = valErr.Error()
					isNetworkError = true
				} else if valResult.Error != "" {
					errMsg = valResult.Error
				}

				currentValue := ""
				for _, v := range creds {
					currentValue = v
					break
				}

				if currentValue == lastValidatedValue {
					validationFailCount++
				} else {
					validationFailCount = 1
					lastValidatedValue = currentValue
				}

				if isNetworkError && validationFailCount >= 2 {
					flowLog.Info("🔄 Validation failed due to network error multiple times for same token, accepting token anyway",
						"error", errMsg,
						"failCount", validationFailCount)
					return true
				}

				if validationFailCount >= 3 {
					flowLog.Info("🔄 Validation failed 3+ times for same token, accepting token anyway",
						"error", errMsg,
						"failCount", validationFailCount)
					return true
				}

				flowLog.Info("🔄 Validation failed, continuing to wait for new token...",
					"error", errMsg,
					"failCount", validationFailCount)
				return false
			}
			flowLog.Info("✅ Inline validation passed")
			return true
		}
	}

	creds, extractionErr := extractor.WaitForAndExtractWithValidator(tokenSource, timeout, extractValidator)

	// 记录提取结果
	flowLog.Info("Token extraction phase completed",
		"error", extractionErr,
		"credsCount", len(creds))

	// 延迟关闭浏览器，让用户看到结果
	time.Sleep(2 * time.Second)

	// 关闭浏览器（无论成功与否）
	flowLog.Step(99, "🔒 Closing browser...")
	ctrl.Close()

	if extractionErr != nil {
		flowLog.Error("Token extraction phase failed", "error", extractionErr.Error())

		extractLogs := extractor.GetExtractLogs()
		flowLog.Debug("Extraction logs available",
			"logCount", len(extractLogs))

		return oauth.OAuthResult{
			Success:      false,
			ProviderID:   options.ProviderID,
			ProviderType: options.ProviderType,
			Error:        extractionErr.Error(),
		}, nil
	}

	if len(creds) == 0 {
		flowLog.Warn("No credentials extracted - user may not be fully logged in or browser was closed early",
			"attemptedSources", fmt.Sprintf("localStorage=%s, cookie=%s, header=%s",
				tokenSource.LocalStorageKey, tokenSource.CookieName, tokenSource.RequestHeader))
		return oauth.OAuthResult{
			Success:      false,
			ProviderID:   options.ProviderID,
			ProviderType: options.ProviderType,
			Error:        "No credentials found. Please ensure you are fully logged in before the browser closes.",
		}, nil
	}

	flowLog.Step(8, "📋 Credentials extracted successfully!",
		"keys", strings.Join(getMapKeys(creds), ", "),
		"keyCount", len(creds))

	for k, v := range creds {
		flowLog.Debug("Extracted credential",
			"key", k,
			"valueLength", len(v),
			"valuePreview", truncate(v, 30))
	}

	flowLog.Step(9, "🔍 Validating extracted credentials...")

	if b.validator == nil {
		flowLog.Warn("No validator set, skipping validation")
		flowLog.Info("═════════════════════════════════════════")
		flowLog.Info("🎉 BROWSER LOGIN COMPLETED SUCCESSFULLY!")
		flowLog.Info("═════════════════════════════════════════")
		return oauth.OAuthResult{
			Success:      true,
			ProviderID:   options.ProviderID,
			ProviderType: options.ProviderType,
			Credentials:  creds,
		}, nil
	}

	// Validation already passed during extraction if validator was set
	valResult, valErr := b.validator(creds)
	if valErr != nil || !valResult.Valid {
		errMsg := "Token validation failed"
		if valErr != nil {
			errMsg = valErr.Error()
		} else if valResult.Error != "" {
			errMsg = valResult.Error
		}

		flowLog.Error("Validation failed",
			"error", errMsg,
			"valid", false)

		flowLog.Info("═════════════════════════════════════════")
		flowLog.Info("❌ BROWSER LOGIN FAILED (validation)")
		flowLog.Info("═════════════════════════════════════════")

		allLogs := ctrl.GetLogs()
		flowLog.Debug("Total log entries for session",
			"count", len(allLogs))

		return oauth.OAuthResult{
			Success:      false,
			ProviderID:   options.ProviderID,
			ProviderType: options.ProviderType,
			Error:        errMsg,
		}, nil
	}

	flowLog.Info("═════════════════════════════════════════")
	flowLog.Info("✅ BROWSER LOGIN COMPLETED AND VALIDATED!")
	flowLog.Info("═════════════════════════════════════════")

	return oauth.OAuthResult{
		Success:      true,
		ProviderID:   options.ProviderID,
		ProviderType: options.ProviderType,
		Credentials:  creds,
		AccountInfo:  valResult.AccountInfo, // 已经是指针类型
	}, nil
}

func getMapKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
