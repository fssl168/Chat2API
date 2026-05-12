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
	return strings.Contains(email, "@guest.com")
}
