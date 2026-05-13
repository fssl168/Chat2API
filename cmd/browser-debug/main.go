package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/fssl168/chat2api-go/oauth/pkg/oauth"
	"github.com/fssl168/chat2api-go/oauth/pkg/oauth/browser"
)

func main() {
	fmt.Println("=== Browser Debug Tool ===")
	fmt.Println()

	// Step 1: Launch browser
	fmt.Println("[Step 1] Launching browser...")
	ctrl := browser.NewPlaywrightController(nil)

	cfg := oauth.BrowserConfig{
		Headless:    false,
		Width:       1400,
		Height:      900,
		WindowTitle: "Debug Login - GLM",
	}

	if err := ctrl.Launch(cfg); err != nil {
		fmt.Printf("[ERROR] Failed to launch browser: %v\n", err)
		return
	}
	defer ctrl.Close()
	fmt.Println("[OK] Browser launched successfully")

	// Step 2: Navigate to GLM login page
	fmt.Println("\n[Step 2] Navigating to GLM login page...")
	loginURL := "https://chatglm.cn"
	if err := ctrl.Navigate(loginURL); err != nil {
		fmt.Printf("[ERROR] Failed to navigate: %v\n", err)
		return
	}
	fmt.Println("[OK] Navigation completed")

	// Step 3: Wait for user to see the page
	fmt.Println("\n[Step 3] Waiting 5 seconds for page to load...")
	time.Sleep(5 * time.Second)
	fmt.Println("[OK] Page should be visible now")

	// Step 4: Check if browser is still open
	if ctrl.IsClosed() {
		fmt.Println("\n[ERROR] Browser was closed!")
		return
	}
	fmt.Println("\n[OK] Browser is still open")

	// Step 5: Create extractor and check cookies
	extractor := browser.NewPlaywrightExtractor(ctrl)

	fmt.Println("\n[Step 6] Checking cookies (every 5 seconds)...")
	fmt.Println("Please try to log in now. Press Ctrl+C to stop.\n")

	attempt := 0
	for {
		if ctrl.IsClosed() {
			fmt.Println("\n[ERROR] Browser was closed by user")
			break
		}

		attempt++
		allCookies, _ := extractor.ExtractAllCookies()

		fmt.Printf("\n--- Attempt %d (%s) ---\n", attempt, time.Now().Format("15:04:05"))
		fmt.Printf("Total cookies: %d\n", len(allCookies))

		for name, value := range allCookies {
			valuePreview := value
			if len(value) > 40 {
				valuePreview = value[:40] + "..."
			}
			fmt.Printf("  %-30s = %s (len=%d)\n", name, valuePreview, len(value))
		}

		// Check specifically for GLM cookie
		glmCookie, ok := allCookies["chatglm_refresh_token"]
		if ok {
			fmt.Printf("\n*** FOUND chatglm_refresh_token! Length=%d ***\n", len(glmCookie))
			// Decode and print JWT payload for analysis
			if payload, err := decodeJWTPayload(glmCookie); err == nil {
				payloadJSON, _ := json.MarshalIndent(payload, "", "  ")
				fmt.Printf("*** JWT PAYLOAD (please share this):\n%s\n", string(payloadJSON))
				// Check for guest indicators
				isGuest := false
				if v, ok := payload["is_guest"].(bool); ok && v {
					isGuest = true
				}
				if v, ok := payload["isGuest"].(bool); ok && v {
					isGuest = true
				}
				if v, ok := payload["email"].(string); ok && strings.Contains(v, "@guest") {
					isGuest = true
				}
				if isGuest {
					fmt.Printf("*** DETECTED: GUEST TOKEN ***\n")
				} else {
					fmt.Printf("*** This token PASSES guest pre-check ***\n")
				}
			} else {
				fmt.Printf("*** Not a valid JWT: %v ***\n", err)
			}
		} else {
			fmt.Println("\nNo chatglm_refresh_token found yet")
		}

		// Check localStorage
		lsValue, lsErr := extractor.ExtractFromLocalStorage("userToken")
		if lsErr == nil && lsValue != "" {
			fmt.Printf("*** Found localStorage 'userToken': length=%d ***\n", len(lsValue))
		}

		// Wait 5 seconds before next check
		select {
		case <-ctrl.WaitForClose():
			fmt.Println("\nBrowser closed, exiting debug loop")
			return
		case <-time.After(5 * time.Second):
		}
	}

	fmt.Println("\n=== Debug Complete ===")
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

func getMapKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

func containsStr(s, substr string) bool {
	return strings.Contains(s, substr)
}

func decodeJWTPayload(token string) (map[string]interface{}, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, fmt.Errorf("invalid JWT format: expected 3 parts, got %d", len(parts))
	}
	payload := parts[1]
	padding := 4 - len(payload)%4
	if padding != 4 {
		payload += strings.Repeat("=", padding)
	}
	decoded, err := base64.StdEncoding.DecodeString(payload)
	if err != nil {
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
