package embedding

import (
	"fmt"

	"github.com/Harshitk-cp/engram/internal/domain"
)

// Provider constants
const (
	ProviderOpenAI = "openai"
	ProviderMock   = "mock"
)

// NewClient creates an embedding client based on the provider name.
// Returns an error if the provider is unknown or the API key is empty (except for mock).
func NewClient(provider, apiKey string) (domain.EmbeddingClient, error) {
	switch provider {
	case ProviderOpenAI:
		if apiKey == "" {
			return nil, fmt.Errorf("OPENAI_API_KEY is required for OpenAI embedding provider")
		}
		return NewOpenAIClient(apiKey), nil

	case ProviderMock:
		return NewMockClient(), nil

	default:
		return nil, fmt.Errorf("unknown embedding provider: %s (valid options: openai, mock)", provider)
	}
}
