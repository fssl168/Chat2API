package adapter

import (
	"fmt"

	"github.com/fssl168/chat2api-go/oauth/pkg/oauth"
)

// CreateAdapter creates a provider-specific OAuth adapter.
func CreateAdapter(providerType oauth.ProviderType, config oauth.AdapterConfig) (OAuthAdapter, error) {
	switch providerType {
	case oauth.ProviderDeepSeek:
		return NewDeepSeekAdapter(config), nil
	case oauth.ProviderGLM:
		return NewGLMAdapter(config), nil
	case oauth.ProviderKimi:
		return NewKimiAdapter(config), nil
	case oauth.ProviderMiniMax:
		return NewMiniMaxAdapter(config), nil
	case oauth.ProviderQwen:
		return NewQwenAdapter(config), nil
	case oauth.ProviderQwenAI:
		return NewQwenAIAdapter(config), nil
	case oauth.ProviderZai:
		return NewZaiAdapter(config), nil
	case oauth.ProviderPerplexity:
		return NewPerplexityAdapter(config), nil
	case oauth.ProviderMimo:
		return NewMimoAdapter(config), nil
	default:
		return nil, fmt.Errorf("unsupported provider type: %s", providerType)
	}
}

// GetSupportedAuthMethods returns the supported authentication methods for a provider.
func GetSupportedAuthMethods(providerType oauth.ProviderType) []string {
	switch providerType {
	case oauth.ProviderDeepSeek:
		return []string{"manual", "token"}
	case oauth.ProviderGLM:
		return []string{"manual", "token"}
	case oauth.ProviderKimi:
		return []string{"manual", "token"}
	case oauth.ProviderMiniMax:
		return []string{"manual", "token"}
	case oauth.ProviderQwen:
		return []string{"manual", "cookie"}
	case oauth.ProviderQwenAI:
		return []string{"manual", "token"}
	case oauth.ProviderZai:
		return []string{"manual", "token"}
	case oauth.ProviderPerplexity:
		return []string{"manual", "cookie"}
	case oauth.ProviderMimo:
		return []string{"manual", "cookie"}
	default:
		return []string{"manual"}
	}
}

// IsProviderType checks if the given string matches a known provider type.
func IsProviderType(s string) bool {
	switch oauth.ProviderType(s) {
	case oauth.ProviderDeepSeek, oauth.ProviderGLM, oauth.ProviderKimi,
		oauth.ProviderMiniMax, oauth.ProviderQwen, oauth.ProviderQwenAI,
		oauth.ProviderZai, oauth.ProviderPerplexity, oauth.ProviderMimo:
		return true
	default:
		return false
	}
}
