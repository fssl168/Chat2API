package browser

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/playwright-community/playwright-go"

	"github.com/fssl168/chat2api-go/oauth/pkg/oauth"
)

type interceptedRequest struct {
	URL       string
	Method    string
	Request   map[string]string
	Response  map[string]string
	Timestamp time.Time
}

type PlaywrightController struct {
	pw          *playwright.Playwright
	browser     playwright.Browser
	context     playwright.BrowserContext
	page        playwright.Page
	flowLogger  *oauth.FlowLogger
	intercepts  []interceptedRequest
	interceptMu sync.RWMutex
	cancelWait  context.CancelFunc
}

func NewPlaywrightController(logger oauth.ProgressCallback) *PlaywrightController {
	fl := oauth.NewFlowLogger("browser-session")
	if logger != nil {
		fl.AddCallback(logger)
	}
	return &PlaywrightController{
		flowLogger: fl,
		intercepts: make([]interceptedRequest, 0),
	}
}

// GetFlowLogger returns the flow logger for this controller
func (p *PlaywrightController) GetFlowLogger() *oauth.FlowLogger {
	return p.flowLogger
}

// GetLogs returns all log entries
func (p *PlaywrightController) GetLogs() []oauth.LogEntry {
	return p.flowLogger.GetEntries()
}

func (p *PlaywrightController) Launch(cfg oauth.BrowserConfig) error {
	var err error

	p.flowLogger.Step(1, "🚀 Initializing Playwright browser automation",
		"headless", cfg.Headless,
		"proxy", cfg.Proxy)

	pw, err := playwright.Run()
	if err != nil {
		p.flowLogger.Error(fmt.Sprintf("Failed to initialize Playwright: %v", err))
		return fmt.Errorf("failed to initialize Playwright: %w", err)
	}
	p.pw = pw

	p.flowLogger.Debug("Playwright initialized successfully", "version", "latest")

	launchOpts := playwright.BrowserTypeLaunchOptions{
		Headless: playwright.Bool(cfg.Headless),
	}

	if cfg.Proxy != "" {
		launchOpts.Proxy = &playwright.Proxy{Server: cfg.Proxy}
		p.flowLogger.Debug("Proxy configured", "server", cfg.Proxy)
	}

	p.flowLogger.Step(2, "🌐 Launching Chromium browser...")

	startTime := time.Now()
	browser, err := pw.Chromium.Launch(launchOpts)
	duration := time.Since(startTime)

	if err != nil {
		p.flowLogger.Error(fmt.Sprintf("Failed to launch browser: %v", err),
			"duration", duration.String())
		return fmt.Errorf("failed to launch browser: %w", err)
	}
	p.browser = browser

	p.flowLogger.Info("Browser launched successfully",
		"headless", cfg.Headless,
		"duration", duration.Round(time.Millisecond).String())

	p.context, err = browser.NewContext()
	if err != nil {
		p.flowLogger.Error(fmt.Sprintf("Failed to create context: %v", err))
		return fmt.Errorf("failed to create browser context: %w", err)
	}

	p.page, err = p.context.NewPage()
	if err != nil {
		p.flowLogger.Error(fmt.Sprintf("Failed to create page: %v", err))
		return fmt.Errorf("failed to create page: %w", err)
	}

	p.flowLogger.Step(3, "✅ Browser environment ready",
		"contextID", "created",
		"pageID", "created")

	return nil
}

func (p *PlaywrightController) Navigate(navigateURL string) error {
	if p.page == nil {
		return fmt.Errorf("page not initialized")
	}

	parsedURL, _ := url.Parse(navigateURL)
	domain := ""
	if parsedURL != nil {
		domain = parsedURL.Hostname()
	}

	p.flowLogger.Step(4, "📍 Navigating to login page...",
		"url", navigateURL,
		"domain", domain)

	startTime := time.Now()
	resp, err := p.page.Goto(navigateURL)
	duration := time.Since(startTime)

	if err != nil {
		p.flowLogger.Error(fmt.Sprintf("Navigation failed: %v", err),
			"duration", duration.String())
		return fmt.Errorf("failed to navigate: %w", err)
	}

	statusCode := 0
	finalURL := ""
	if resp != nil {
		statusCode = resp.Status()
		finalURL = resp.URL()
	}

	p.flowLogger.Info("Page loaded successfully",
		"status", statusCode,
		"finalURL", finalURL,
		"duration", duration.Round(time.Millisecond).String())

	return nil
}

func (p *PlaywrightController) WaitForURL(contains string, timeoutSec int) error {
	if p.page == nil {
		return fmt.Errorf("page not initialized")
	}

	p.flowLogger.Step(5, fmt.Sprintf("⏳ Waiting for user login (timeout: %ds)...", timeoutSec),
		"urlContains", contains)

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeoutSec)*time.Second)
	p.cancelWait = cancel
	defer cancel()

	timeout := float64(timeoutSec) * 1000

	currentURL := p.page.URL()
	if strings.Contains(currentURL, contains) {
		p.flowLogger.Info("URL condition already met", "currentURL", currentURL)
		return nil
	}

	waitOpts := playwright.PageWaitForURLOptions{Timeout: &timeout}
	startTime := time.Now()

	err := p.page.WaitForURL(func(u *string) bool {
		if u == nil {
			return false
		}
		if contains == "" {
			return true
		}
		return strings.Contains(*u, contains)
	}, waitOpts)

	duration := time.Since(startTime)

	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			p.flowLogger.Warn("Wait timeout reached",
				"timeout", fmt.Sprintf("%ds", timeoutSec),
				"duration", duration.Round(time.Millisecond).String())
			return fmt.Errorf("timeout waiting for URL containing '%s'", contains)
		}
		p.flowLogger.Error(fmt.Sprintf("WaitForURL error: %v", err))
		return fmt.Errorf("failed to wait for URL: %w", err)
	}

	finalURL := p.page.URL()
	p.flowLogger.Info("URL condition met - user has logged in!",
		"targetPattern", contains,
		"finalURL", finalURL,
		"waitDuration", duration.Round(time.Millisecond).String())

	return nil
}

func (p *PlaywrightController) Close() error {
	p.flowLogger.Step(99, "🔒 Closing browser and cleaning up resources...")

	if p.cancelWait != nil {
		p.cancelWait()
	}

	closeStartTime := time.Now()

	closeWithTimeout := func(name string, closeFn func() error) {
		done := make(chan error, 1)
		go func() {
			done <- closeFn()
		}()
		select {
		case <-done:
			p.flowLogger.Debug(name + " closed")
		case <-time.After(3 * time.Second):
			p.flowLogger.Warn(name+" close timed out, forcing continue", "timeout", "3s")
		}
	}

	if p.page != nil {
		closeWithTimeout("Page", func() error { return p.page.Close() })
	}
	if p.context != nil {
		closeWithTimeout("Context", func() error { return p.context.Close() })
	}
	if p.browser != nil {
		closeWithTimeout("Browser", func() error { return p.browser.Close() })
	}
	if p.pw != nil {
		closeWithTimeout("Playwright", func() error { p.pw.Stop(); return nil })
	}

	duration := time.Since(closeStartTime)
	totalIntercepts := len(p.intercepts)

	p.flowLogger.Info("Browser cleanup complete",
		"closeDuration", duration.Round(time.Millisecond).String(),
		"totalRequestsIntercepted", totalIntercepts)

	return nil
}

type PlaywrightExtractor struct {
	controller     *PlaywrightController
	extractLog     *oauth.FlowLogger
	extractAttempt int
}

func NewPlaywrightExtractor(ctrl *PlaywrightController) *PlaywrightExtractor {
	el := oauth.NewFlowLogger("extractor")
	return &PlaywrightExtractor{
		controller: ctrl,
		extractLog: el,
	}
}

// GetExtractLogs returns extraction-specific logs
func (e *PlaywrightExtractor) GetExtractLogs() []oauth.LogEntry {
	return e.extractLog.GetEntries()
}

func (e *PlaywrightExtractor) EnableWebRequestIntercept() error {
	if e.controller.page == nil {
		return fmt.Errorf("page not initialized")
	}

	e.extractLog.Step(10, "🔍 Enabling webRequest interception...")

	e.controller.page.OnRequest(func(req playwright.Request) {
		intercepted := interceptedRequest{
			URL:       req.URL(),
			Method:    req.Method(),
			Request:   req.Headers(),
			Timestamp: time.Now(),
		}

		e.controller.interceptMu.Lock()
		e.controller.intercepts = append(e.controller.intercepts, intercepted)
		e.controller.interceptMu.Unlock()

		e.extractLog.Debug("▶️ Request intercepted",
			"method", req.Method(),
			"url", truncateURL(req.URL(), 80),
			"resourceType", req.ResourceType())
	})

	e.controller.page.OnResponse(func(resp playwright.Response) {
		intercepted := interceptedRequest{
			URL:       resp.URL(),
			Method:    resp.Request().Method(),
			Response:  resp.Headers(),
			Timestamp: time.Now(),
		}

		e.controller.interceptMu.Lock()
		e.controller.intercepts = append(e.controller.intercepts, intercepted)
		e.controller.interceptMu.Unlock()

		status := resp.Status()
		e.extractLog.Debug("◀️ Response received",
			"status", status,
			"url", truncateURL(resp.URL(), 80))
	})

	e.extractLog.Info("webRequest interception enabled - monitoring all network traffic",
		"interceptors", "request+response")

	return nil
}

// evaluateWithTimeout wraps page.Evaluate with a timeout to prevent blocking.
func (e *PlaywrightExtractor) evaluateWithTimeout(script string, timeout time.Duration) (interface{}, error) {
	type evalResult struct {
		result interface{}
		err    error
	}

	done := make(chan evalResult, 1)
	go func() {
		r, err := e.controller.page.Evaluate(script)
		done <- evalResult{result: r, err: err}
	}()

	select {
	case res := <-done:
		return res.result, res.err
	case <-time.After(timeout):
		return nil, fmt.Errorf("page.Evaluate timed out after %v", timeout)
	}
}

func (e *PlaywrightExtractor) ExtractFromLocalStorage(key string) (string, error) {
	if e.controller.page == nil {
		return "", fmt.Errorf("page not initialized")
	}

	e.extractLog.Step(11, fmt.Sprintf("📦 Reading LocalStorage['%s']...", key))

	result, err := e.evaluateWithTimeout(fmt.Sprintf(`
		(function() {
			try {
				var value = window.localStorage.getItem('%s');
				var allKeys = [];
				for (var i = 0; i < localStorage.length; i++) {
					allKeys.push(localStorage.key(i));
				}
				return { value: value, allKeys: allKeys };
			} catch(e) {
				return { value: null, error: e.message, allKeys: [] };
			}
		})()
	`, key), 5*time.Second)

	if err != nil {
		e.extractLog.Error("LocalStorage read failed", "error", err.Error())
		return "", fmt.Errorf("failed to read localStorage: %w", err)
	}

	if result == nil {
		e.extractLog.Warn("LocalStorage returned null", "key", key)
		return "", nil
	}

	resultMap, ok := result.(map[string]interface{})
	if !ok {
		e.extractLog.Warn("Unexpected LocalStorage format", "type", fmt.Sprintf("%T", result))
		return "", fmt.Errorf("unexpected localStorage value type: %T", result)
	}

	value, _ := resultMap["value"].(string)
	allKeysRaw, _ := resultMap["allKeys"].([]interface{})
	allKeys := make([]string, 0, len(allKeysRaw))
	for _, k := range allKeysRaw {
		if s, ok := k.(string); ok {
			allKeys = append(allKeys, s)
		}
	}

	if value != "" {
		value = unwrapJSONValue(value, key)
	}

	if value != "" {
		e.extractLog.Info("✅ LocalStorage value found",
			"key", key,
			"valueLength", len(value),
			"valuePreview", truncate(value, 60),
			"allKeysCount", len(allKeys))
	} else {
		e.extractLog.Warn("LocalStorage key not found or empty",
			"key", key,
			"availableKeys", strings.Join(allKeys, ", "))
	}

	return value, nil
}

func unwrapJSONValue(value string, key string) string {
	if value == "" || len(value) < 2 || value[0] != '{' {
		return value
	}
	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(value), &parsed); err != nil {
		return value
	}
	if innerVal, ok := parsed["value"].(string); ok && innerVal != "" {
		return innerVal
	}
	return value
}

func (e *PlaywrightExtractor) ExtractAllLocalStorage() (map[string]string, error) {
	if e.controller.page == nil {
		return nil, fmt.Errorf("page not initialized")
	}

	e.extractLog.Step(12, "📦 Extracting ALL LocalStorage items...")

	result, err := e.controller.page.Evaluate(`
		(function() {
			try {
				var items = {};
				var keys = [];
				for (var i = 0; i < localStorage.length; i++) {
					var key = localStorage.key(i);
					keys.push(key);
					items[key] = localStorage.getItem(key);
				}
				return { items: items, keys: keys, count: keys.length };
			} catch(e) {
				return { items: {}, keys: [], count: 0, error: e.message };
			}
		})()
	`)

	if err != nil {
		e.extractLog.Error("Failed to extract all localStorage", "error", err.Error())
		return nil, fmt.Errorf("failed to read localStorage: %w", err)
	}

	if result == nil {
		return nil, nil
	}

	resultMap, ok := result.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("unexpected localStorage result type: %T", result)
	}

	itemsRaw, _ := resultMap["items"].(map[string]interface{})
	items := make(map[string]string)
	for k, v := range itemsRaw {
		if str, ok := v.(string); ok {
			items[k] = str
		}
	}

	e.extractLog.Info("LocalStorage extraction complete",
		"itemCount", len(items),
		"keys", strings.Join(getMapKeys(items), ", "))

	return items, nil
}

func (e *PlaywrightExtractor) ExtractAllCookies() (map[string]string, error) {
	if e.controller.context == nil {
		return nil, fmt.Errorf("context not initialized")
	}

	e.extractLog.Step(13, "🍪 Extracting ALL cookies...")

	cookies, err := e.controller.context.Cookies()
	if err != nil {
		e.extractLog.Error("Cookie extraction failed", "error", err.Error())
		return nil, fmt.Errorf("failed to get cookies: %w", err)
	}

	result := make(map[string]string)
	for _, c := range cookies {
		result[c.Name] = c.Value
		e.extractLog.Debug("Cookie extracted",
			"name", c.Name,
			"domain", c.Domain,
			"path", c.Path,
			"httpOnly", c.HttpOnly,
			"secure", c.Secure,
			"valueLength", len(c.Value))
	}

	e.extractLog.Info("Cookie extraction complete",
		"cookieCount", len(result))

	return result, nil
}

func (e *PlaywrightExtractor) ExtractCookie(name string) (string, error) {
	cookies, err := e.ExtractAllCookies()
	if err != nil {
		return "", err
	}

	if value, ok := cookies[name]; ok {
		e.extractLog.Info("Target cookie found",
			"name", name,
			"valueLength", len(value),
			"value", value)
		return value, nil
	}

	e.extractLog.Warn("Cookie not found", "name", name)
	return "", nil
}

// extractCookieViaJS reads a specific cookie using JavaScript document.cookie.
// This is more reliable than Playwright's API for httpOnly cookies or
// cookies set by JavaScript after page load.
func (e *PlaywrightExtractor) extractCookieViaJS(name string) string {
	if e.controller.page == nil {
		return ""
	}

	result, err := e.evaluateWithTimeout(fmt.Sprintf(`
		(function() {
			var cookies = document.cookie.split(';');
			for (var i = 0; i < cookies.length; i++) {
				var cookie = cookies[i].trim();
				if (cookie.indexOf('%s=') === 0) {
					return decodeURIComponent(cookie.substring('%s='.length));
				}
			}
			return '';
		})()
	`, name, name), 3*time.Second)

	if err != nil {
		e.extractLog.Debug("JS cookie read failed", "name", name, "error", err.Error())
		return ""
	}

	if result == nil {
		return ""
	}

	if str, ok := result.(string); ok {
		return str
	}

	return ""
}

func (e *PlaywrightExtractor) GetInterceptedRequests() []interceptedRequest {
	e.controller.interceptMu.RLock()
	defer e.controller.interceptMu.RUnlock()
	return e.controller.intercepts
}

func (e *PlaywrightExtractor) FindInIntercepted(headerName, headerValue string) *interceptedRequest {
	e.controller.interceptMu.RLock()
	defer e.controller.interceptMu.RUnlock()

	e.extractLog.Debug("Searching intercepted requests",
		"headerName", headerName,
		"headerValue", headerValue,
		"totalIntercepts", len(e.controller.intercepts))

	for i := len(e.controller.intercepts) - 1; i >= 0; i-- {
		req := e.controller.intercepts[i]
		if val, ok := req.Request[headerName]; ok && strings.Contains(val, headerValue) {
			e.extractLog.Info("Matching request found in intercepts",
				"index", i,
				"method", req.Method,
				"url", truncateURL(req.URL, 80),
				"headerName", headerName,
				"headerValuePreview", truncate(val, 50))
			return &req
		}
	}

	e.extractLog.Debug("No matching request found",
		"headerName", headerName)
	return nil
}

// FindAllInIntercepted returns all intercepted requests that match the given header and URL pattern.
// If urlPattern is empty, only header matching is applied.
func (e *PlaywrightExtractor) FindAllInIntercepted(headerName, urlPattern string) []interceptedRequest {
	e.controller.interceptMu.RLock()
	defer e.controller.interceptMu.RUnlock()

	var results []interceptedRequest
	for i := len(e.controller.intercepts) - 1; i >= 0; i-- {
		req := e.controller.intercepts[i]
		if _, ok := req.Request[headerName]; !ok {
			continue
		}
		// URL pattern filter: simple wildcard match (e.g. "*.kimi.com")
		if urlPattern != "" && !matchURLPattern(req.URL, urlPattern) {
			continue
		}
		results = append(results, req)
	}
	return results
}

// matchURLPattern does simple wildcard matching for URL patterns.
// Supports patterns like "*.kimi.com" (matches any subdomain).
func matchURLPattern(urlStr, pattern string) bool {
	if pattern == "" || pattern == "*" {
		return true
	}
	// Parse URL to get hostname
	parsed, err := url.Parse(urlStr)
	if err != nil {
		// Try with scheme prefix
		parsed, err = url.Parse("https://" + urlStr)
		if err != nil {
			return strings.Contains(urlStr, pattern)
		}
	}
	host := parsed.Hostname()
	// Simple wildcard: "*.kimi.com" matches "api.kimi.com", "www.kimi.com", etc.
	if strings.HasPrefix(pattern, "*.") {
		suffix := pattern[2:] // remove "*."
		return host == suffix || strings.HasSuffix(host, "."+suffix)
	}
	return strings.Contains(host, pattern) || strings.Contains(urlStr, pattern)
}

func (e *PlaywrightExtractor) WaitForAndExtract(cfg oauth.TokenSource, timeoutSec int) (map[string]string, error) {
	return e.WaitForAndExtractWithValidator(cfg, timeoutSec, nil)
}

// WaitForAndExtractWithValidator waits for token extraction with optional validation.
// If validator is provided and returns false, extraction continues waiting for a new valid token.
func (e *PlaywrightExtractor) WaitForAndExtractWithValidator(cfg oauth.TokenSource, timeoutSec int, validator func(map[string]string) bool) (map[string]string, error) {
	e.extractLog.Step(20, fmt.Sprintf("⏳ Waiting for and extracting token (source: %s, timeout: %ds)...", cfg.ProviderType, timeoutSec),
		"localStorageKey", cfg.LocalStorageKey,
		"cookieName", cfg.CookieName,
		"requestHeader", cfg.RequestHeader)

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeoutSec)*time.Second)
	defer cancel()

	result := make(map[string]string)
	done := make(chan struct{})

	minWaitTime := 15 * time.Second
	stableWaitTime := 5 * time.Second
	maxExtractionTime := 60 * time.Second
	startTime := time.Now()
	foundTokenAt := time.Time{}
	lastStableCheck := time.Time{}
	lastValue := ""

	go func() {
		attempt := 0
		for {
			select {
			case <-ctx.Done():
				return
			case <-done:
				return
			default:
				attempt++
				e.extractLog.Debug(fmt.Sprintf("Extraction attempt #%d (elapsed: %v)", attempt, time.Since(startTime).Round(time.Second)))

				if extracted := e.extractOnce(cfg); len(extracted) > 0 {
					if validator != nil {
						if !validator(extracted) {
							e.extractLog.Info("🔄 Extracted token failed validation, continuing to wait...",
								"attempt", attempt,
								"keysFound", strings.Join(getMapKeys(extracted), ", "))
							foundTokenAt = time.Time{}
							lastStableCheck = time.Time{}
							lastValue = ""
							result = make(map[string]string)
							time.Sleep(500 * time.Millisecond)
							continue
						}
					}

					for k, v := range extracted {
						result[k] = v
					}

					if foundTokenAt.IsZero() {
						foundTokenAt = time.Now()
						lastStableCheck = foundTokenAt
						for _, v := range extracted {
							lastValue = v
							break
						}
						e.extractLog.Info("✅ Token detected! Waiting for minimum hold + stable time...",
							"attempt", attempt,
							"keysFound", strings.Join(getMapKeys(extracted), ", "),
							"firstValuePreview", truncate(lastValue, 40))
					}

					elapsedSinceFound := time.Since(foundTokenAt)
					elapsedTotal := time.Since(startTime)

					if elapsedTotal >= maxExtractionTime {
						e.extractLog.Info("⏰ Max extraction time reached, completing with current value",
							"totalWaitTime", elapsedTotal.Round(time.Second))
						close(done)
						return
					}

					if time.Since(lastStableCheck) >= stableWaitTime {
						currentValue := ""
						for _, v := range extracted {
							currentValue = v
							break
						}
						if currentValue == lastValue {
							if elapsedSinceFound >= minWaitTime {
								e.extractLog.Info("✅ Token value is stable and minimum wait time elapsed. Completing extraction.",
									"totalWaitTime", time.Since(startTime).Round(time.Second),
									"stableChecks", int(elapsedSinceFound/stableWaitTime))
								close(done)
								return
							}
						} else {
							e.extractLog.Info("🔄 Token value changed during wait, resetting stable timer",
								"oldPreview", truncate(lastValue, 30),
								"newPreview", truncate(currentValue, 30))
							lastValue = currentValue
						}
						lastStableCheck = time.Now()
					}
				}

				time.Sleep(500 * time.Millisecond)
			}
		}
	}()

	select {
	case <-ctx.Done():
		e.extractLog.Warn("Token extraction timed out",
			"timeout", fmt.Sprintf("%ds", timeoutSec),
			"attempts", "max")
		return result, fmt.Errorf("timeout waiting for token extraction")
	case <-done:
		e.extractLog.Info("Token extraction completed",
			"keys", strings.Join(getMapKeys(result), ", "),
			"totalDuration", time.Since(startTime).Round(time.Second))
		return result, nil
	}
}

func (e *PlaywrightExtractor) extractOnce(cfg oauth.TokenSource) map[string]string {
	candidates := make(map[string]string) // key -> value from all sources

	// 检查page是否仍然有效
	if e.controller.page == nil {
		e.extractLog.Warn("Page is nil, skipping extraction")
		return nil
	}

	e.extractLog.Debug("Starting extraction attempt",
		"localStorageKey", cfg.LocalStorageKey,
		"cookieName", cfg.CookieName,
		"requestHeader", cfg.RequestHeader)

	e.extractAttempt++
	// 诊断dump暂时禁用，避免page.Evaluate阻塞导致提取卡住
	// if e.extractAttempt%10 == 1 {
	// 	e.dumpAllLocalStorage()
	// 	e.dumpAllCookies()
	// }

	// === Source 1: LocalStorage ===
	if cfg.LocalStorageKey != "" {
		e.extractLog.Step(12, fmt.Sprintf("📦 Checking LocalStorage['%s']...", cfg.LocalStorageKey))
		val, err := e.ExtractFromLocalStorage(cfg.LocalStorageKey)
		if err != nil {
			e.extractLog.Error("LocalStorage extraction error", "error", err.Error())
			if strings.Contains(err.Error(), "closed") || strings.Contains(err.Error(), "been closed") {
				e.extractLog.Warn("Browser page appears to be closed by user")
			}
		} else if val != "" {
			// Use ResultKey if configured (e.g. "_token" -> "token")
			resultKey := cfg.ResultKey
			if resultKey == "" {
				resultKey = cfg.LocalStorageKey
			}
			candidates[resultKey] = val
			e.extractLog.Info("Found value in LocalStorage",
				"key", cfg.LocalStorageKey,
				"resultKey", resultKey,
				"valueLength", len(val),
				"isJWT", isJWT(val))
		}

		// Extract extra localStorage keys (e.g. MiniMax: user_detail_agent -> realUserID)
		for _, extraKey := range cfg.ExtraLocalStorage {
			extraVal, err := e.ExtractFromLocalStorage(extraKey)
			if err != nil || extraVal == "" {
				continue
			}
			// If JSONExtractField is configured, parse JSON and extract nested field
			if cfg.JSONExtractField != "" {
				var parsed map[string]interface{}
				if err := json.Unmarshal([]byte(extraVal), &parsed); err == nil {
					extractedValue := ""
					if v, ok := parsed[cfg.JSONExtractField].(string); ok && v != "" {
						extractedValue = v
					} else if cfg.JSONExtractField2 != "" {
						if v, ok := parsed[cfg.JSONExtractField2].(string); ok && v != "" {
							extractedValue = v
						} else if v, ok := parsed[cfg.JSONExtractField2].(float64); ok {
							extractedValue = fmt.Sprintf("%.0f", v)
						}
					}
					if extractedValue != "" {
						candidates[cfg.JSONExtractField] = extractedValue
						e.extractLog.Info("Extracted field from localStorage JSON",
							"localStorageKey", extraKey,
							"field", cfg.JSONExtractField,
							"valueLength", len(extractedValue))
					}
				}
			} else {
				candidates[extraKey] = extraVal
				e.extractLog.Info("Found extra localStorage value",
					"key", extraKey,
					"valueLength", len(extraVal))
			}
		}
	}

	// === Source 2: Cookie ===
	if cfg.CookieName != "" {
		e.extractLog.Step(13, fmt.Sprintf("🍪 Checking Cookie '%s'...", cfg.CookieName))

		allCookies, cookieErr := e.ExtractAllCookies()
		if cookieErr != nil {
			e.extractLog.Error("ExtractAllCookies failed", "error", cookieErr.Error())
		}

		jsCookieValue := e.extractCookieViaJS(cfg.CookieName)

		var finalValue string
		if jsCookieValue != "" {
			finalValue = jsCookieValue
			if apiVal, ok := allCookies[cfg.CookieName]; ok && apiVal != jsCookieValue {
				e.extractLog.Warn("Cookie value mismatch between Playwright API and JS",
					"cookieName", cfg.CookieName,
					"using", "JS (more reliable)")
			}
		} else if apiVal, ok := allCookies[cfg.CookieName]; ok && apiVal != "" {
			finalValue = apiVal
		}

		if finalValue != "" {
			resultKey := cfg.ResultKey
			if resultKey == "" {
				resultKey = cfg.CookieName
			}
			candidates[resultKey] = finalValue
			e.extractLog.Info("Found value in Cookie",
				"name", cfg.CookieName,
				"resultKey", resultKey,
				"valueLength", len(finalValue),
				"isJWT", isJWT(finalValue))
		}

		// Extract extra cookies for multi-token providers (e.g. Mimo)
		for _, extraCookieName := range cfg.ExtraCookies {
			extraValue := e.extractCookieViaJS(extraCookieName)
			if extraValue == "" {
				if apiVal, ok := allCookies[extraCookieName]; ok && apiVal != "" {
					extraValue = apiVal
				}
			}
			if extraValue != "" {
				candidates[extraCookieName] = extraValue
				e.extractLog.Info("Found extra cookie",
					"name", extraCookieName,
					"valueLength", len(extraValue))
			}
		}
	}

	// === Source 3: Request Header (with URL pattern filter and regex extract) ===
	if cfg.RequestHeader != "" {
		e.extractLog.Step(14, fmt.Sprintf("🔍 Checking intercepted requests for header '%s'...", cfg.RequestHeader))

		// Find all intercepted requests matching URL pattern and header
		matchedReqs := e.FindAllInIntercepted(cfg.RequestHeader, cfg.URLPattern)
		if len(matchedReqs) > 0 {
			for _, req := range matchedReqs {
				if val, ok := req.Request[cfg.RequestHeader]; ok && val != "" {
					extractedValue := val

					// Apply extract pattern if configured
					if cfg.ExtractPattern != "" {
						re, err := regexp.Compile(cfg.ExtractPattern)
						if err == nil {
							matches := re.FindStringSubmatch(val)
							if len(matches) > 1 {
								extractedValue = matches[1]
								e.extractLog.Debug("Extracted value using regex",
									"pattern", cfg.ExtractPattern,
									"originalLength", len(val),
									"extractedLength", len(extractedValue))
							} else {
								e.extractLog.Debug("Regex did not match",
									"pattern", cfg.ExtractPattern,
									"value", truncate(val, 60))
								continue
							}
						} else {
							e.extractLog.Warn("Invalid extract pattern",
								"pattern", cfg.ExtractPattern,
								"error", err.Error())
						}
					} else {
						// No extract pattern: strip "Bearer " prefix if present
						extractedValue = strings.TrimPrefix(val, "Bearer ")
					}

					// Use ResultKey as the credentials key (fallback to RequestHeader name)
					resultKey := cfg.ResultKey
					if resultKey == "" {
						resultKey = cfg.RequestHeader
					}

					candidates[resultKey] = extractedValue
					e.extractLog.Info("Found value in intercepted request header",
						"header", cfg.RequestHeader,
						"resultKey", resultKey,
						"valueLength", len(extractedValue),
						"isJWT", isJWT(extractedValue),
						"url", truncateURL(req.URL, 80))
				}
			}
		} else {
			interceptedCount := len(e.GetInterceptedRequests())
			e.extractLog.Debug("No matching intercepted request found",
				"header", cfg.RequestHeader,
				"urlPattern", cfg.URLPattern,
				"totalInterceptedRequests", interceptedCount)
		}
	}

	// === 优先选择 JWT 格式的值 ===
	if len(candidates) == 0 {
		e.extractLog.Debug("Extraction attempt completed - no token found from any source")
		return nil
	}

	// 记录所有候选值
	for k, v := range candidates {
		e.extractLog.Debug("Candidate token",
			"source", k,
			"valueLength", len(v),
			"isJWT", isJWT(v),
			"valuePreview", truncate(v, 60))
	}

	// 优先返回 JWT 格式的值
	for sourceKey, value := range candidates {
		if isJWT(value) {
			e.extractLog.Info("✅ Selected JWT token (preferred)",
				"source", sourceKey,
				"valueLength", len(value))
			return map[string]string{sourceKey: value}
		}
	}

	// Filter out invalid tokens using isValidToken pre-validation
	validCandidates := make(map[string]string)
	for sourceKey, value := range candidates {
		if isValidToken(value) {
			validCandidates[sourceKey] = value
		} else {
			e.extractLog.Debug("Token failed pre-validation, skipping",
				"source", sourceKey,
				"valueLength", len(value),
				"valuePreview", truncate(value, 40))
		}
	}

	// 没有有效 JWT，返回第一个有效的值
	for sourceKey, value := range validCandidates {
		e.extractLog.Info("✅ Selected token (no JWT found)",
			"source", sourceKey,
			"valueLength", len(value))
		return map[string]string{sourceKey: value}
	}

	e.extractLog.Debug("All candidates failed pre-validation")
	return nil
}

// isJWT checks if a string looks like a JWT token (base64-encoded header starting with {"alg").
func isJWT(s string) bool {
	if s == "" {
		return false
	}
	// JWT tokens start with "eyJ" (base64 of '{"')
	return strings.HasPrefix(s, "eyJ")
}

// isValidToken performs comprehensive token validation matching chat2api's TypeScript implementation.
// Validates: JWT format, JWE format, length checks, base64 encoding, guest account filtering.
func isValidToken(value string) bool {
	if value == "" || len(value) < 5 {
		return false
	}

	if strings.HasPrefix(value, "eyJ") {
		parts := strings.Split(value, ".")
		if len(parts) == 5 && len(value) >= 100 {
			return true
		}
		if len(parts) == 3 {
			if payload, err := decodeJWTPayload(value); err == nil {
				if email, ok := payload["email"].(string); ok && strings.Contains(email, "@guest.com") {
					return false
				}
				if hasAnyField(payload, "app_id", "sub", "exp", "id", "user_id", "uid", "email") {
					return true
				}
			}
		}
	}

	if len(value) >= 20 && !strings.Contains(value, " ") && !strings.Contains(value, "\n") && !strings.Contains(value, "\r") {
		return true
	}

	if len(value) >= 5 && !strings.Contains(value, " ") && !strings.Contains(value, "\n") {
		return true
	}

	return false
}

// decodeJWTPayload extracts and decodes the payload section of a JWT token.
func decodeJWTPayload(token string) (map[string]interface{}, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, fmt.Errorf("invalid JWT format")
	}
	// Add padding if needed
	payload := parts[1]
	padding := 4 - len(payload)%4
	if padding != 4 {
		payload += strings.Repeat("=", padding)
	}
	decoded, err := base64.StdEncoding.DecodeString(payload)
	if err != nil {
		// Try URL-safe base64
		decoded, err = base64.URLEncoding.DecodeString(payload)
		if err != nil {
			return nil, err
		}
	}
	var result map[string]interface{}
	if err := json.Unmarshal(decoded, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// hasAnyField checks if the map contains any of the specified keys.
func hasAnyField(m map[string]interface{}, keys ...string) bool {
	for _, k := range keys {
		if _, ok := m[k]; ok {
			return true
		}
	}
	return false
}

func (e *PlaywrightExtractor) dumpAllLocalStorage() {
	e.extractLog.Info("📋 Starting LocalStorage Diagnostic Dump...")

	if e.controller.page == nil {
		e.extractLog.Warn("Cannot dump LocalStorage: page is nil")
		return
	}

	result, err := e.evaluateWithTimeout(`
		(function() {
			try {
				var items = {};
				for (var i = 0; i < localStorage.length; i++) {
					var key = localStorage.key(i);
					var value = localStorage.getItem(key);
					items[key] = value ? value.substring(0, 50) + '...' : '(empty)';
				}
				return { count: localStorage.length, items: items };
			} catch(e) {
				return { count: 0, items: {}, error: e.message };
			}
		})()
	`, 5*time.Second)

	if err != nil {
		e.extractLog.Error("Failed to dump LocalStorage", "error", err.Error())
		return
	}

	if result == nil {
		e.extractLog.Warn("LocalStorage Evaluate returned nil")
		return
	}

	e.extractLog.Debug("LocalStorage raw result", "type", fmt.Sprintf("%T", result), "value", fmt.Sprintf("%+v", result))

	if resultMap, ok := result.(map[string]interface{}); ok {
		keys := make([]string, 0, len(resultMap))
		for k := range resultMap {
			keys = append(keys, k)
		}
		e.extractLog.Debug("Parsed as map[string]interface{}", "keys", strings.Join(keys, ", "))

		// 尝试解析count字段（可能是int或float64）
		var count int
		countVal := resultMap["count"]
		switch v := countVal.(type) {
		case int:
			count = v
		case float64:
			count = int(v)
		case int64:
			count = int(v)
		default:
			e.extractLog.Error("Failed to parse count field",
				"type", fmt.Sprintf("%T", countVal),
				"value", fmt.Sprintf("%+v", countVal))
			return
		}

		if items, ok := resultMap["items"].(map[string]interface{}); ok {
			e.extractLog.Info("📋 LocalStorage Diagnostic Dump",
				"totalCount", count)

			for key, value := range items {
				if valueStr, ok := value.(string); ok {
					e.extractLog.Debug("  LocalStorage item",
						"key", key,
						"valuePreview", valueStr)
				} else {
					e.extractLog.Debug("  LocalStorage item (non-string)",
						"key", key,
						"valueType", fmt.Sprintf("%T", value),
						"value", fmt.Sprintf("%+v", value))
				}
			}
		} else {
			e.extractLog.Error("Failed to parse items field",
				"type", fmt.Sprintf("%T", resultMap["items"]))
		}
	} else {
		e.extractLog.Error("Unexpected result format",
			"expectedType", "map[string]interface{}",
			"actualType", fmt.Sprintf("%T", result))
	}
}

// dumpAllCookies 诊断函数：输出所有cookie信息
func (e *PlaywrightExtractor) dumpAllCookies() {
	if e.controller.page == nil {
		return
	}

	cookies, err := e.controller.page.Context().Cookies()
	if err != nil {
		e.extractLog.Error("Failed to dump Cookies", "error", err.Error())
		return
	}

	e.extractLog.Info("🍪 Cookies Diagnostic Dump",
		"totalCount", len(cookies))

	for _, cookie := range cookies {
		valuePreview := cookie.Value
		if len(valuePreview) > 30 {
			valuePreview = valuePreview[:30] + "..."
		}

		e.extractLog.Debug("  Cookie item",
			"name", cookie.Name,
			"valuePreview", valuePreview,
			"domain", cookie.Domain,
			"path", cookie.Path)
	}
}

func truncateURL(u string, maxLen int) string {
	if len(u) <= maxLen {
		return u
	}
	return u[:maxLen-3] + "..."
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

var _ BrowserController = (*PlaywrightController)(nil)
var _ TokenExtractor = (*PlaywrightExtractor)(nil)
