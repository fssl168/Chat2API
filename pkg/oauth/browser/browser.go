package browser

import "github.com/fssl168/chat2api-go/oauth/pkg/oauth"

// BrowserController defines the interface for browser automation.
type BrowserController interface {
	Launch(cfg oauth.BrowserConfig) error
	Navigate(url string) error
	WaitForURL(contains string, timeout int) error
	Close() error
}
