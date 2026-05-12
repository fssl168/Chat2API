package browser

import (
	"net/http"

	"github.com/fssl168/chat2api-go/oauth/pkg/oauth"
)

// TokenExtractor defines the interface for token extraction.
type TokenExtractor interface {
	EnableWebRequestIntercept() error
	ExtractFromLocalStorage(key string) (string, error)
	ExtractAllCookies() (map[string]string, error)
	ExtractCookie(name string) (string, error)
	WaitForAndExtract(cfg oauth.TokenSource, timeout int) (map[string]string, error)
}

// DefaultExtractor is a no-op implementation (for manual input fallback).
type DefaultExtractor struct{}

func NewDefaultExtractor() *DefaultExtractor { return &DefaultExtractor{} }

func (d *DefaultExtractor) EnableWebRequestIntercept() error { return nil }

func (d *DefaultExtractor) ExtractFromLocalStorage(key string) (string, error) {
	return "", nil
}

func (d *DefaultExtractor) ExtractAllCookies() (map[string]string, error) {
	return map[string]string{}, nil
}

func (d *DefaultExtractor) ExtractCookie(name string) (string, error) {
	return "", nil
}

func (d *DefaultExtractor) WaitForAndExtract(cfg oauth.TokenSource, timeout int) (map[string]string, error) {
	return map[string]string{}, nil
}

// CookieToMap converts http.Cookie slice to name-value map.
func CookieToMap(cookies []*http.Cookie) map[string]string {
	res := map[string]string{}
	for _, c := range cookies {
		res[c.Name] = c.Value
	}
	return res
}
