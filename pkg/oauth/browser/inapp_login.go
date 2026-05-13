package browser

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/fssl168/chat2api-go/oauth/pkg/oauth"
)

const (
	defaultInAppTimeout = 120 // seconds - 2 minutes is enough for login
	minLoginTime        = 5   // seconds before first token check
	stableCheckInterval = 1   // second between extraction attempts
	stableRequiredTime  = 3   // seconds token must be stable
	maxInvalidRetries   = 3   // max times to clear and retry after invalid token detection
)

// InAppLoginStatus represents the current status of the in-app login flow.
type InAppLoginStatus string

const (
	InAppStatusIdle       InAppLoginStatus = "idle"
	InAppStatusStarting   InAppLoginStatus = "starting"
	InAppStatusReady      InAppLoginStatus = "ready"
	InAppStatusExtracting InAppLoginStatus = "extracting"
	InAppStatusValidating InAppLoginStatus = "validating"
	InAppStatusCompleted  InAppLoginStatus = "completed"
	InAppStatusCancelled  InAppLoginStatus = "cancelled"
	InAppStatusError      InAppLoginStatus = "error"
)

// InAppLoginResult is the result of an in-app login attempt.
type InAppLoginResult struct {
	Success     bool                    `json:"success"`
	Credentials map[string]string       `json:"credentials,omitempty"`
	AccountInfo *oauth.OAuthAccountInfo `json:"accountInfo,omitempty"`
	Error       string                  `json:"error,omitempty"`
	Status      InAppLoginStatus        `json:"status"`
}

// StatusCallback is called when the login status changes.
type StatusCallback func(status InAppLoginStatus, message string)

// TokenFoundCallback is called when a token is found.
type TokenFoundCallback func(key, value string)

// InAppLoginManager manages in-app browser login and token extraction.
// Aligned with chat2api's TypeScript InAppLoginManager.
type InAppLoginManager struct {
	mu             sync.RWMutex
	isRunning      bool
	controller     *PlaywrightController
	extractor      *PlaywrightExtractor
	foundTokens    map[string]string
	tokenSource    oauth.TokenSource
	statusCallback StatusCallback
	tokenCallback  TokenFoundCallback
	flowLogger     *oauth.FlowLogger
	loginStartTime time.Time
	completeCh     chan InAppLoginResult
}

// NewInAppLoginManager creates a new in-app login manager.
func NewInAppLoginManager() *InAppLoginManager {
	return &InAppLoginManager{
		foundTokens: make(map[string]string),
		completeCh:  make(chan InAppLoginResult, 1),
	}
}

// SetStatusCallback sets the status callback function.
func (m *InAppLoginManager) SetStatusCallback(cb StatusCallback) {
	m.statusCallback = cb
}

// SetTokenFoundCallback sets the token found callback function.
func (m *InAppLoginManager) SetTokenFoundCallback(cb TokenFoundCallback) {
	m.tokenCallback = cb
}

// emitStatus emits a status update.
func (m *InAppLoginManager) emitStatus(status InAppLoginStatus, message string) {
	if m.statusCallback != nil {
		go m.statusCallback(status, message)
	}
	if m.flowLogger != nil {
		m.flowLogger.Info(fmt.Sprintf("[InAppLogin] %s: %s", status, message))
	}
}

// emitTokenFound emits a token found event.
func (m *InAppLoginManager) emitTokenFound(key, value string) {
	m.mu.Lock()
	m.foundTokens[key] = value
	m.mu.Unlock()

	if m.tokenCallback != nil {
		go m.tokenCallback(key, value)
	}
	if m.flowLogger != nil {
		m.flowLogger.Info("Token found",
			"key", key,
			"valueLength", len(value),
			"valuePreview", truncate(value, 50))
	}
}

// StartLogin starts the in-app browser login flow.
func (m *InAppLoginManager) StartLogin(providerType oauth.ProviderType, validator func(map[string]string) (oauth.TokenValidationResult, error)) (InAppLoginResult, error) {
	m.mu.Lock()
	if m.isRunning {
		m.mu.Unlock()
		return InAppLoginResult{
			Success: false,
			Error:   "A login process is already in progress",
			Status:  InAppStatusError,
		}, nil
	}
	m.isRunning = true
	m.foundTokens = make(map[string]string)
	m.loginStartTime = time.Now()
	m.mu.Unlock()

	cfg, ok := TokenSources[providerType]
	if !ok {
		m.cleanup()
		return InAppLoginResult{
			Success: false,
			Error:   fmt.Sprintf("No token extraction config found for provider: %s", providerType),
			Status:  InAppStatusError,
		}, nil
	}
	m.tokenSource = cfg

	m.flowLogger = oauth.NewFlowLogger("inapp-login-" + string(providerType))

	m.emitStatus(InAppStatusStarting, "Opening login window...")

	// Start login in background with timeout
	go m.runLoginFlow(providerType, cfg, validator)

	// Wait for completion or timeout
	select {
	case result := <-m.completeCh:
		return result, nil
	case <-time.After(time.Duration(defaultInAppTimeout) * time.Second):
		// Race-condition-safe: give completeCh a moment to receive a just-sent result
		select {
		case result := <-m.completeCh:
			return result, nil
		default:
		}
		m.flowLogger.Error("Login timeout - no valid token detected within timeout period")
		return InAppLoginResult{
			Success: false,
			Error:   "Login timeout - no valid token detected. Please ensure you log in completely before the browser closes.",
			Status:  InAppStatusError,
		}, nil
	}
}

func (m *InAppLoginManager) runLoginFlow(providerType oauth.ProviderType, cfg oauth.TokenSource, validator func(map[string]string) (oauth.TokenValidationResult, error)) {
	defer m.cleanup()

	loginURL := ProviderLoginURL[providerType]
	if loginURL == "" {
		m.sendResult(InAppLoginResult{
			Success: false,
			Error:   fmt.Sprintf("Login URL not configured for provider: %s", providerType),
			Status:  InAppStatusError,
		})
		return
	}

	m.flowLogger.Info("Starting in-app login flow", "provider", providerType, "loginURL", loginURL)

	// Launch browser
	m.controller = NewPlaywrightController(nil)
	browserCfg := oauth.BrowserConfig{
		Headless:    false,
		Width:       1400,
		Height:      900,
		WindowTitle: cfg.WindowTitle,
	}

	if err := m.controller.Launch(browserCfg); err != nil {
		m.sendResult(InAppLoginResult{
			Success: false,
			Error:   fmt.Sprintf("Failed to launch browser: %v", err),
			Status:  InAppStatusError,
		})
		return
	}
	defer m.controller.Close()

	m.extractor = NewPlaywrightExtractor(m.controller)

	// Enable web request interception
	if err := m.extractor.EnableWebRequestIntercept(); err != nil {
		m.flowLogger.Warn("Failed to enable webRequest interception, continuing...", "error", err.Error())
	}

	// Navigate to login page
	if err := m.controller.Navigate(loginURL); err != nil {
		m.sendResult(InAppLoginResult{
			Success: false,
			Error:   fmt.Sprintf("Failed to navigate to login page: %v", err),
			Status:  InAppStatusError,
		})
		return
	}

	m.emitStatus(InAppStatusReady, "Login window ready - please log in")
	m.flowLogger.Step(6, "Browser is now open! Waiting for user to complete login...",
		"note", "Please login in the browser window. DO NOT close it!",
		"timeout", fmt.Sprintf("%ds", defaultInAppTimeout))

	// Start token extraction with smart completion detection
	result := m.extractTokensWithSmartCompletion(cfg, validator)
	m.sendResult(result)
}

func (m *InAppLoginManager) extractTokensWithSmartCompletion(cfg oauth.TokenSource, validator func(map[string]string) (oauth.TokenValidationResult, error)) InAppLoginResult {
	m.emitStatus(InAppStatusExtracting, "Monitoring for login tokens...")

	// Wait minimum login time before first check
	elapsed := time.Since(m.loginStartTime)
	if elapsed < time.Duration(minLoginTime)*time.Second {
		waitTime := time.Duration(minLoginTime)*time.Second - elapsed
		m.flowLogger.Info(fmt.Sprintf("Waiting minimum %ds before first token check...", minLoginTime))
		select {
		case <-m.controller.WaitForClose():
			return InAppLoginResult{Success: false, Error: "Login window was closed", Status: InAppStatusCancelled}
		case <-time.After(waitTime):
		}
	}

	m.flowLogger.Info("Starting token extraction loop",
		"localStorageKey", cfg.LocalStorageKey,
		"cookieName", cfg.CookieName,
		"resultKey", cfg.ResultKey,
		"minLength", cfg.MinLength)

	var lastValues map[string]string
	stableSince := time.Time{}
	validationPassed := false
	var validationResult oauth.TokenValidationResult
	attempt := 0
	invalidRetryCount := 0

	for {
		// Check if browser was closed by user
		if m.controller.IsClosed() {
			return InAppLoginResult{Success: false, Error: "Login window was closed", Status: InAppStatusCancelled}
		}

		attempt++
		extracted := m.extractor.extractOnce(cfg)

		if len(extracted) > 0 {
			m.flowLogger.Info(fmt.Sprintf("Extraction attempt #%d: found %d token(s)", attempt, len(extracted)),
				"keys", strings.Join(getMapKeys(extracted), ", "))

			// Log each found token for debugging (with preview)
			for k, v := range extracted {
				m.flowLogger.Debug("Extracted token detail",
					"key", k,
					"valueLength", len(v),
					"valuePreview", truncate(v, 50))
				m.mu.RLock()
				existing, hasExisting := m.foundTokens[k]
				m.mu.RUnlock()
				if !hasExisting || existing != v {
					m.emitTokenFound(k, v)
				}
			}

			// Check if we have all required tokens
			hasAll := m.hasAllRequiredTokens(cfg, extracted)
			m.flowLogger.Debug(fmt.Sprintf("Attempt #%d: hasAllRequiredTokens=%v", attempt, hasAll),
				"extractedKeys", strings.Join(getMapKeys(extracted), ", "))

			if hasAll {
				// Check stability
				if lastValues == nil || !m.valuesEqual(lastValues, extracted) {
					lastValues = make(map[string]string)
					for k, v := range extracted {
						lastValues[k] = v
					}
					stableSince = time.Now()
					m.flowLogger.Info("Token values detected, waiting for stability...",
						"keys", strings.Join(getMapKeys(extracted), ", "))
				} else {
					// Values unchanged - check stability duration
					stableDuration := time.Since(stableSince)
					m.flowLogger.Debug(fmt.Sprintf("Tokens stable for %.1fs (need %ds)", stableDuration.Seconds(), stableRequiredTime))

					if stableDuration >= time.Duration(stableRequiredTime)*time.Second {
						// Stable enough - validate
						if validator != nil && !validationPassed {
							m.emitStatus(InAppStatusValidating, "Validating extracted tokens...")
							m.flowLogger.Info("Running token validation...",
								"credentialsKeys", strings.Join(getMapKeys(extracted), ", "))
							valResult, valErr := validator(extracted)
							if valErr != nil || !valResult.Valid {
								errMsg := "Token validation failed"
								if valErr != nil {
									errMsg = valErr.Error()
								} else if valResult.Error != "" {
									errMsg = valResult.Error
								}

								// Check if the error indicates the token is definitively invalid
								// (e.g. guest account, HTTP 400 bad request, expired token)
								if isTokenDefinitivelyInvalid(errMsg) {
									invalidRetryCount++
									m.flowLogger.Warn("Token is definitively invalid",
										"error", errMsg,
										"retryCount", invalidRetryCount,
										"maxRetries", maxInvalidRetries)

									if invalidRetryCount >= maxInvalidRetries {
										m.flowLogger.Error("Max invalid token retries reached, aborting login")
										return InAppLoginResult{
											Success: false,
											Error:   fmt.Sprintf("Login failed after %d attempts: %s. Please ensure you are using a valid (non-guest) account.", maxInvalidRetries, errMsg),
											Status:  InAppStatusError,
										}
									}

									// Clear the invalid token from browser and show a warning on the page
									// Then continue waiting for the user to re-login with a valid account
									m.flowLogger.Info("Clearing invalid token from browser and showing warning to user...")
									m.clearInvalidTokenAndWarn(cfg, errMsg, invalidRetryCount)

									// Reset state to wait for a new (hopefully valid) token
									stableSince = time.Time{}
									lastValues = nil
									validationPassed = false
									continue
								}

								m.flowLogger.Warn("Validation failed (non-definitive), will continue waiting for new token",
									"error", errMsg,
									"valid", valResult.Valid)
								// Reset to wait for a different token
								stableSince = time.Time{}
								lastValues = nil
								validationPassed = false
							} else {
								validationPassed = true
								validationResult = valResult
								m.flowLogger.Info("Validation passed",
									"accountInfo", fmt.Sprintf("%+v", valResult.AccountInfo))
							}
						}

						if validationPassed || validator == nil {
							m.flowLogger.Info("All tokens extracted, stable, and validated - completing login")
							return InAppLoginResult{
								Success:     true,
								Credentials: extracted,
								AccountInfo: validationResult.AccountInfo,
								Status:      InAppStatusCompleted,
							}
						}
					}
				}
			} else {
				m.flowLogger.Debug("Not all required tokens present yet",
					"haveKeys", strings.Join(getMapKeys(extracted), ", "))
			}
		} else if attempt%5 == 0 {
			// Log every 5th empty attempt to show we're still alive
			m.flowLogger.Debug(fmt.Sprintf("Extraction attempt #%d: no tokens found yet", attempt))
		}

		// Wait before next check
		select {
		case <-m.controller.WaitForClose():
			return InAppLoginResult{Success: false, Error: "Login window was closed", Status: InAppStatusCancelled}
		case <-time.After(time.Duration(stableCheckInterval) * time.Second):
		}
	}
}

// clearInvalidTokenAndWarn clears the invalid token from browser storage and shows
// a prominent warning to the user, prompting them to re-login with a valid account.
func (m *InAppLoginManager) clearInvalidTokenAndWarn(cfg oauth.TokenSource, errMsg string, retryCount int) {
	if m.controller == nil || m.controller.page == nil {
		return
	}

	page := m.controller.page

	// 1. Clear the specific cookie
	if cfg.CookieName != "" {
		_, _ = page.Evaluate(fmt.Sprintf(`
			(function() {
				document.cookie = '%s=; expires=Thu, 01 Jan 1970 00:00:00 GMT; path=/;';
				document.cookie = '%s=; expires=Thu, 01 Jan 1970 00:00:00 GMT; path=/; domain=.chatglm.cn;';
				document.cookie = '%s=; expires=Thu, 01 Jan 1970 00:00:00 GMT; path=/; domain=chatglm.cn;';
				return 'cookie cleared';
			})()
		`, cfg.CookieName, cfg.CookieName, cfg.CookieName))
		m.flowLogger.Info("Cleared invalid cookie", "cookieName", cfg.CookieName)
	}

	// 2. Clear the specific localStorage key
	if cfg.LocalStorageKey != "" {
		_, _ = page.Evaluate(fmt.Sprintf(`
			(function() {
				localStorage.removeItem('%s');
				return 'localStorage cleared';
			})()
		`, cfg.LocalStorageKey))
		m.flowLogger.Info("Cleared invalid localStorage", "key", cfg.LocalStorageKey)
	}

	// 3. Show a prominent warning overlay on the page
	warningHTML := fmt.Sprintf(`
	<div id="chat2api-login-warning" style="
		position: fixed;
		top: 0; left: 0; right: 0; bottom: 0;
		background: rgba(0,0,0,0.8);
		z-index: 999999;
		display: flex;
		align-items: center;
		justify-content: center;
		font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
	">
		<div style="
			background: #fff;
			padding: 32px;
			border-radius: 12px;
			max-width: 420px;
			width: 90%%;
			text-align: center;
			box-shadow: 0 20px 60px rgba(0,0,0,0.3);
		">
			<div style="font-size: 48px; margin-bottom: 16px;">⚠️</div>
			<h2 style="margin: 0 0 12px; color: #dc3545; font-size: 20px;">Login Failed</h2>
			<p style="margin: 0 0 16px; color: #333; line-height: 1.5; font-size: 14px;">
				<b>%s</b>
			</p>
			<p style="margin: 0 0 20px; color: #666; line-height: 1.5; font-size: 13px;">
				Please log out and log in again with a <b>registered (non-guest) account</b>.<br>
				Do not close this browser window.
			</p>
			<div style="background: #f8f9fa; padding: 12px; border-radius: 6px; margin-bottom: 20px;">
				<span style="color: #666; font-size: 12px;">Attempt </span>
				<span style="color: #dc3545; font-weight: bold; font-size: 14px;">%d/%d</span>
			</div>
			<button onclick="document.getElementById('chat2api-login-warning').remove(); location.reload();" style="
				background: #dc3545;
				color: #fff;
				border: none;
				padding: 12px 28px;
				border-radius: 6px;
				font-size: 14px;
				cursor: pointer;
				font-weight: 500;
			">
				I've re-logged in, Continue
			</button>
		</div>
	</div>
	`, errMsg, retryCount, maxInvalidRetries)

	_, evalErr := page.Evaluate(fmt.Sprintf(`
		(function() {
			// Remove any existing warning
			var existing = document.getElementById('chat2api-login-warning');
			if (existing) existing.remove();
			// Insert new warning
			var div = document.createElement('div');
			div.innerHTML = %q;
			document.body.appendChild(div.firstElementChild);
			return 'warning shown';
		})()
	`, warningHTML))

	if evalErr != nil {
		m.flowLogger.Warn("Failed to show warning overlay on page", "error", evalErr.Error())
	} else {
		m.flowLogger.Info("Warning overlay shown on page",
			"retryCount", retryCount,
			"maxRetries", maxInvalidRetries)
	}
}

// hasAllRequiredTokens checks if all required tokens for the provider are present.
func (m *InAppLoginManager) hasAllRequiredTokens(cfg oauth.TokenSource, extracted map[string]string) bool {
	if len(extracted) == 0 {
		return false
	}

	// Check primary token (use ResultKey if configured, otherwise use source key)
	primaryKey := cfg.ResultKey
	if primaryKey == "" {
		if cfg.LocalStorageKey != "" {
			primaryKey = cfg.LocalStorageKey
		} else if cfg.CookieName != "" {
			primaryKey = cfg.CookieName
		} else if cfg.RequestHeader != "" {
			primaryKey = cfg.RequestHeader
		}
	}

	if primaryKey != "" {
		if _, ok := extracted[primaryKey]; !ok {
			m.flowLogger.Debug("Primary token missing", "expectedKey", primaryKey, "availableKeys", strings.Join(getMapKeys(extracted), ", "))
			return false
		}
	}

	// Check extra localStorage keys
	for _, key := range cfg.ExtraLocalStorage {
		if cfg.JSONExtractField != "" {
			if _, ok := extracted[cfg.JSONExtractField]; !ok {
				m.flowLogger.Debug("Extra JSON field missing", "expectedField", cfg.JSONExtractField)
				return false
			}
		} else {
			if _, ok := extracted[key]; !ok {
				m.flowLogger.Debug("Extra localStorage key missing", "expectedKey", key)
				return false
			}
		}
	}

	// Check extra cookies
	for _, key := range cfg.ExtraCookies {
		if _, ok := extracted[key]; !ok {
			m.flowLogger.Debug("Extra cookie missing", "expectedCookie", key)
			return false
		}
	}

	return true
}

func (m *InAppLoginManager) valuesEqual(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

// sendResult sends the result to the completeCh safely.
// Does NOT cancel any context to avoid race conditions.
func (m *InAppLoginManager) sendResult(result InAppLoginResult) {
	if result.Success {
		m.emitStatus(InAppStatusCompleted, "Login completed successfully")
	} else if result.Status == InAppStatusCancelled {
		m.emitStatus(InAppStatusCancelled, result.Error)
	} else {
		m.emitStatus(InAppStatusError, result.Error)
	}

	select {
	case m.completeCh <- result:
		m.flowLogger.Info("Result sent to complete channel", "success", result.Success)
	default:
		m.flowLogger.Warn("completeCh is full, result dropped")
	}
}

// Cancel cancels the current login flow.
func (m *InAppLoginManager) Cancel() {
	m.sendResult(InAppLoginResult{
		Success: false,
		Error:   "Login cancelled by user",
		Status:  InAppStatusCancelled,
	})
}

// IsRunning returns true if a login is in progress.
func (m *InAppLoginManager) IsRunning() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.isRunning
}

// GetFoundTokens returns the currently found tokens.
func (m *InAppLoginManager) GetFoundTokens() map[string]string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make(map[string]string)
	for k, v := range m.foundTokens {
		result[k] = v
	}
	return result
}

// GetFlowLogger returns the flow logger for debugging.
func (m *InAppLoginManager) GetFlowLogger() *oauth.FlowLogger {
	return m.flowLogger
}

func (m *InAppLoginManager) cleanup() {
	m.mu.Lock()
	m.isRunning = false
	m.mu.Unlock()

	if m.controller != nil {
		m.controller.Close()
	}
}

// isTokenDefinitivelyInvalid checks if a validation error indicates the token
// is permanently invalid and will never become valid by waiting longer.
// These errors should cause immediate abort rather than continuing to wait.
func isTokenDefinitivelyInvalid(errMsg string) bool {
	if errMsg == "" {
		return false
	}
	lower := strings.ToLower(errMsg)
	definitiveIndicators := []string{
		"guest",             // Guest accounts
		"bad request",       // HTTP 400 - malformed token
		"invalid",           // Token invalid or expired
		"expired",           // Token expired
		"unauthorized",      // HTTP 401
		"not allowed",       // Permission denied
		"cannot be empty",   // Empty token
		"http 400",          // HTTP 400
		"http 401",          // HTTP 401
		"http 403",          // HTTP 403
		"status:400",        // GLM specific error format
		"status:401",        // GLM specific error format
		"status:403",        // GLM specific error format
	}
	for _, indicator := range definitiveIndicators {
		if strings.Contains(lower, indicator) {
			return true
		}
	}
	return false
}
