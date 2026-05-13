package oauth

import (
	"crypto/md5"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

// GenerateState generates a random state string for CSRF protection.
func GenerateState() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// ParseJWT parses a JWT token payload without verification.
func ParseJWT(token string) (map[string]interface{}, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, fmt.Errorf("invalid JWT format")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, err
	}
	var result map[string]interface{}
	if err := json.Unmarshal(payload, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// IsJWT checks if a token is in JWT format.
func IsJWT(token string) bool {
	return strings.HasPrefix(token, "eyJ") && len(strings.Split(token, ".")) == 3
}

// GenerateUUID generates a random UUID string.
func GenerateUUID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// MD5 returns the MD5 hash of the input string.
func MD5(input string) string {
	hash := md5.Sum([]byte(input))
	return hex.EncodeToString(hash[:])
}

// GetTimestamp returns the current Unix timestamp in seconds.
func GetTimestamp() int64 {
	return time.Now().Unix()
}

// GetTimestampMs returns the current Unix timestamp in milliseconds.
func GetTimestampMs() int64 {
	return time.Now().UnixMilli()
}

// OpenBrowser opens the given URL in the system default browser.
func OpenBrowser(url string) error {
	var cmd string
	var args []string

	switch runtime.GOOS {
	case "windows":
		cmd = "cmd"
		args = []string{"/c", "start", url}
	case "darwin":
		cmd = "open"
		args = []string{url}
	default:
		cmd = "xdg-open"
		args = []string{url}
	}

	return exec.Command(cmd, args...).Start()
}

// IsGuestEmail checks if the email indicates a guest account.
func IsGuestEmail(email string) bool {
	return strings.Contains(email, "@guest.com") || strings.Contains(email, "@guest")
}

// IsGuestNickname checks if the nickname indicates a guest account.
// Supports both Chinese "访客" and English "Guest" keywords.
func IsGuestNickname(nickname string) bool {
	if nickname == "" {
		return false
	}
	lower := strings.ToLower(nickname)
	return strings.Contains(nickname, "访客") ||
		strings.Contains(lower, "guest") ||
		strings.Contains(lower, "anonymous") ||
		strings.HasPrefix(lower, "user") && len(nickname) <= 10
}

// IsGuestAccount checks multiple indicators to determine if an account is a guest.
// This is the comprehensive check used by all providers (matching chat2api logic).
func IsGuestAccount(isGuest bool, nickname, email string, hasPhoneOrEmail bool) bool {
	if isGuest {
		return true
	}
	if IsGuestNickname(nickname) {
		return true
	}
	if IsGuestEmail(email) {
		return true
	}
	if !hasPhoneOrEmail {
		return true
	}
	return false
}

// CheckJWTPayloadForGuest checks JWT payload for guest account indicators.
// Returns (isGuest, reason) tuple.
func CheckJWTPayloadForGuest(payload map[string]interface{}) (bool, string) {
	if payload == nil {
		return false, ""
	}

	if email, ok := payload["email"].(string); ok && IsGuestEmail(email) {
		return true, fmt.Sprintf("email indicates guest: %s", email)
	}

	if name, ok := payload["name"].(string); ok && IsGuestNickname(name) {
		return true, fmt.Sprintf("nickname indicates guest: %s", name)
	}

	if sub, ok := payload["sub"].(string); ok && IsGuestEmail(sub) {
		return true, fmt.Sprintf("sub field indicates guest: %s", sub)
	}

	return false, ""
}

// GuestTokenInfo contains detailed information about why a token was rejected as guest.
type GuestTokenInfo struct {
	IsGuest bool
	Reason  string
	Details map[string]string
}

// ValidateTokenForGuest performs comprehensive guest token validation.
// Works with both JWT tokens (parses payload) and raw tokens (uses heuristics).
func ValidateTokenForGuest(token string, providerType ProviderType) GuestTokenInfo {
	result := GuestTokenInfo{
		Details: make(map[string]string),
	}

	if token == "" {
		result.IsGuest = true
		result.Reason = "token is empty"
		return result
	}

	if IsJWT(token) {
		payload, err := ParseJWT(token)
		if err != nil {
			result.Details["parseError"] = err.Error()
			return result
		}

		for k, v := range payload {
			if str, ok := v.(string); ok {
				if len(str) > 50 {
					result.Details[k] = str[:50] + "..."
				} else {
					result.Details[k] = str
				}
			} else if f, ok := v.(float64); ok {
				result.Details[k] = fmt.Sprintf("%.0f", f)
			} else if b, ok := v.(bool); ok {
				result.Details[k] = fmt.Sprintf("%v", b)
			}
		}

		isGuest, reason := CheckJWTPayloadForGuest(payload)
		if isGuest {
			result.IsGuest = true
			result.Reason = reason
			return result
		}

		if _, ok := payload["email"]; ok {
			result.Details["hasEmail"] = "true"
		}
		if _, ok := payload["phone"]; ok {
			result.Details["hasPhone"] = "true"
		}
		if _, ok := payload["sub"]; ok {
			result.Details["hasSub"] = "true"
		}
		if _, ok := payload["id"]; ok || payload["user_id"] != nil || payload["uid"] != nil {
			result.Details["hasIdentity"] = "true"
		}
	}

	result.Details["tokenLength"] = fmt.Sprintf("%d", len(token))
	result.Details["provider"] = string(providerType)

	return result
}
