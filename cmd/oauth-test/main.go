package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/fssl168/chat2api-go/oauth/pkg/oauth"
	"github.com/fssl168/chat2api-go/oauth/pkg/oauth/adapter"
)

func main() {
	fmt.Println("========================================")
	fmt.Println("  AI Provider OAuth Test Tool (Go)")
	fmt.Println("========================================")
	fmt.Println()

	manager := adapter.NewOAuthManager()
	manager.SetProgressCallback(func(event oauth.OAuthProgressEvent) {
		fmt.Printf("[Progress] %s: %s\n", event.Status, event.Message)
	})

	for {
		showMainMenu()
		choice := readInput("Select option: ")

		switch strings.TrimSpace(choice) {
		case "1":
			testManualToken(manager)
		case "2":
			testOAuthCallback(manager)
		case "3":
			testBrowserAutomation(manager)
		case "4":
			testTokenRefresh(manager)
		case "5":
			listProviders()
		case "6":
			testJWTValidation()
		case "0", "q", "quit", "exit":
			fmt.Println("Goodbye!")
			return
		default:
			fmt.Println("Invalid option, please try again.")
		}

		fmt.Println()
		fmt.Println("Press Enter to continue...")
		_ = readInput("")
	}
}

func showMainMenu() {
	fmt.Println("-------- Main Menu --------")
	fmt.Println("1. Test Manual Token Login")
	fmt.Println("2. Test OAuth Callback Login")
	fmt.Println("3. Test Browser Automation (Playwright)")
	fmt.Println("4. Test Token Refresh")
	fmt.Println("5. List Supported Providers")
	fmt.Println("6. Test JWT Validation")
	fmt.Println("0. Quit")
	fmt.Println("---------------------------")
}

func listProviders() {
	fmt.Println("\nSupported Providers:")
	fmt.Println("--------------------")
	providers := adapter.GetSupportedProviders()
	for i, p := range providers {
		configs := oauth.MANUAL_TOKEN_CONFIGS[p]
		label := string(p)
		if len(configs) > 0 {
			label = configs[0].Label
		}
		fmt.Printf("%d. %s (%s)\n", i+1, p, label)
	}
}

func selectProvider() (string, oauth.ProviderType, bool) {
	listProviders()
	fmt.Println()
	choice := readInput("Select provider (number or ID): ")
	choice = strings.TrimSpace(choice)

	providers := adapter.GetSupportedProviders()

	// Try numeric selection
	for i, p := range providers {
		if choice == fmt.Sprintf("%d", i+1) {
			return fmt.Sprintf("test-%s", p), p, true
		}
	}

	// Try provider ID
	for _, p := range providers {
		if choice == string(p) {
			return fmt.Sprintf("test-%s", p), p, true
		}
	}

	fmt.Println("Invalid provider selection.")
	return "", "", false
}

func testManualToken(manager *adapter.OAuthManager) {
	fmt.Println("\n--- Manual Token Test ---")
	providerID, providerType, ok := selectProvider()
	if !ok {
		return
	}

	configs := oauth.MANUAL_TOKEN_CONFIGS[providerType]
	if len(configs) == 0 {
		fmt.Println("No manual token config found for this provider.")
		return
	}

	fmt.Printf("\nProvider: %s\n", providerType)
	fmt.Printf("Description: %s\n", configs[0].Description)
	if configs[0].HelpURL != "" {
		fmt.Printf("Help URL: %s\n", configs[0].HelpURL)
	}
	fmt.Println()

	token := readInput(fmt.Sprintf("Enter %s: ", configs[0].Label))
	token = strings.TrimSpace(token)

	if token == "" {
		fmt.Println("Token cannot be empty.")
		return
	}

	var extras []string

	// Handle multi-credential providers
	switch providerType {
	case oauth.ProviderMiniMax:
		realUserID := readInput("Enter realUserID (optional, press Enter to skip): ")
		extras = append(extras, strings.TrimSpace(realUserID))
	case oauth.ProviderMimo:
		userID := readInput("Enter userId: ")
		phToken := readInput("Enter ph_token (xiaomichatbot_ph): ")
		extras = append(extras, strings.TrimSpace(userID), strings.TrimSpace(phToken))
	}

	fmt.Println("\nValidating token...")

	result, err := manager.LoginWithToken(providerID, providerType, token, extras...)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	printResult(result)
}

func testOAuthCallback(manager *adapter.OAuthManager) {
	fmt.Println("\n--- OAuth Callback Test ---")
	providerID, providerType, ok := selectProvider()
	if !ok {
		return
	}

	fmt.Printf("\nStarting OAuth login for %s...\n", providerType)
	fmt.Println("This will open your system browser.")
	fmt.Println("Note: Most AI providers do not support standard OAuth redirect.")
	fmt.Println("For manual token extraction, use option 1 instead.")
	fmt.Println()

	result, err := manager.StartLogin(oauth.OAuthOptions{
		ProviderID:   providerID,
		ProviderType: providerType,
		Timeout:      300000,
	})
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	printResult(result)
}

func testBrowserAutomation(manager *adapter.OAuthManager) {
	fmt.Println("\n--- Browser Automation Test (Playwright) ---")
	fmt.Println("This requires Playwright to be installed:")
	fmt.Println("  go install github.com/playwright-community/playwright-go/cmd/playwright@latest")
	fmt.Println("  playwright install chromium")
	fmt.Println()

	providerID, providerType, ok := selectProvider()
	if !ok {
		return
	}

	fmt.Printf("\nStarting browser automation for %s...\n", providerType)
	fmt.Println("This will launch a browser window.")
	fmt.Println("Please log in manually in the browser.")
	fmt.Println("The tool will attempt to extract tokens automatically.")
	fmt.Println()

	result, err := manager.StartLoginWithBrowser(oauth.OAuthOptions{
		ProviderID:   providerID,
		ProviderType: providerType,
		Timeout:      300000,
	})
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	printResult(result)

	if !result.Success {
		fmt.Println("\nBrowser automation failed or is currently in stub mode.")
		fmt.Println("To use full automation, install Playwright and implement the driver.")
	}
}

func testTokenRefresh(manager *adapter.OAuthManager) {
	fmt.Println("\n--- Token Refresh Test ---")
	providerID, providerType, ok := selectProvider()
	if !ok {
		return
	}

	fmt.Println("\nEnter existing credentials for refresh test:")
	credentials := make(map[string]string)

	switch providerType {
	case oauth.ProviderGLM:
		token := readInput("Enter refresh_token: ")
		credentials["refresh_token"] = strings.TrimSpace(token)
	case oauth.ProviderQwen:
		ticket := readInput("Enter tongyi_sso_ticket: ")
		credentials["ticket"] = strings.TrimSpace(ticket)
	case oauth.ProviderMiniMax:
		token := readInput("Enter token: ")
		credentials["token"] = strings.TrimSpace(token)
		realUserID := readInput("Enter realUserID: ")
		credentials["realUserID"] = strings.TrimSpace(realUserID)
	case oauth.ProviderMimo:
		serviceToken := readInput("Enter serviceToken: ")
		credentials["service_token"] = strings.TrimSpace(serviceToken)
		userID := readInput("Enter userId: ")
		credentials["user_id"] = strings.TrimSpace(userID)
		phToken := readInput("Enter ph_token: ")
		credentials["ph_token"] = strings.TrimSpace(phToken)
	default:
		token := readInput("Enter token: ")
		credentials["token"] = strings.TrimSpace(token)
	}

	fmt.Println("\nAttempting token refresh...")

	credInfo, err := manager.RefreshToken(providerID, providerType, credentials)
	if err != nil {
		fmt.Printf("Refresh failed: %v\n", err)
		return
	}

	if credInfo == nil {
		fmt.Println("Token refresh not supported for this provider or no new token returned.")
		return
	}

	fmt.Println("Token refresh successful!")
	fmt.Printf("Token Type: %s\n", credInfo.Type)
	fmt.Printf("Value: %s...\n", truncate(credInfo.Value, 20))
	if credInfo.ExpiresAt > 0 {
		fmt.Printf("Expires At: %d\n", credInfo.ExpiresAt)
	}
}

func testJWTValidation() {
	fmt.Println("\n--- JWT Validation Test ---")
	token := readInput("Enter JWT token to parse: ")
	token = strings.TrimSpace(token)

	if token == "" {
		fmt.Println("Token cannot be empty.")
		return
	}

	if !oauth.IsJWT(token) {
		fmt.Println("Token is not in JWT format (should start with 'eyJ' and have 3 parts).")
		return
	}

	payload, err := oauth.ParseJWT(token)
	if err != nil {
		fmt.Printf("Failed to parse JWT: %v\n", err)
		return
	}

	fmt.Println("\nJWT Payload:")
	prettyJSON, _ := json.MarshalIndent(payload, "", "  ")
	fmt.Println(string(prettyJSON))

	fmt.Println("\nDetected Fields:")
	for _, key := range []string{"sub", "id", "user_id", "uid", "email", "name", "exp", "app_id", "typ"} {
		if val, ok := payload[key]; ok {
			fmt.Printf("  %s: %v\n", key, val)
		}
	}
}

func printResult(result oauth.OAuthResult) {
	fmt.Println("\n--- Result ---")
	fmt.Printf("Success: %v\n", result.Success)
	fmt.Printf("Provider: %s\n", result.ProviderType)

	if result.Success {
		fmt.Println("Status: SUCCESS")
		if result.AccountInfo != nil {
			fmt.Printf("User ID: %s\n", result.AccountInfo.UserID)
			fmt.Printf("Email: %s\n", result.AccountInfo.Email)
			fmt.Printf("Name: %s\n", result.AccountInfo.Name)
		}
		if len(result.Credentials) > 0 {
			fmt.Println("Credentials:")
			for k, v := range result.Credentials {
				fmt.Printf("  %s: %s...\n", k, truncate(v, 20))
			}
		}
	} else {
		fmt.Println("Status: FAILED")
		fmt.Printf("Error: %s\n", result.Error)
	}
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen]
}

func readInput(prompt string) string {
	fmt.Print(prompt)
	reader := bufio.NewReader(os.Stdin)
	input, _ := reader.ReadString('\n')
	return strings.TrimSpace(input)
}
