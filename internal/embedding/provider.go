package embedding

import (
	"fmt"

	"github.com/Harshitk-cp/engram/internal/domain"
)

// Provider constants
const (
	ProviderOpenAI     = "openai"
	ProviderCompatible = "openai-compatible" // any OpenAI-format /embeddings endpoint
	ProviderLocal      = "local"             // alias for openai-compatible (self-hosted)
	ProviderMock       = "mock"
)

const openAIBaseURL = "https://api.openai.com/v1"

// Config selects and configures an embedding provider.
type Config struct {
	Provider   string
	APIKey     string
	BaseURL    string // required for openai-compatible / local
	Model      string // optional; provider default when empty
	Dimensions int    // optional; request a specific output width (Matryoshka models)
}

// NewClient creates an embedding client from cfg. OpenAI is routed through the
// OpenAI-compatible client (its API is the reference format), so EMBEDDING_MODEL
// and a requested dimension work for it too.
func NewClient(cfg Config) (domain.EmbeddingClient, error) {
	switch cfg.Provider {
	case ProviderOpenAI, "":
		if cfg.APIKey == "" {
			return nil, fmt.Errorf("OPENAI_API_KEY (or EMBEDDING_API_KEY) is required for the OpenAI embedding provider")
		}
		return NewCompatibleClient(openAIBaseURL, cfg.APIKey, cfg.Model, cfg.Dimensions), nil

	case ProviderCompatible, ProviderLocal:
		if cfg.BaseURL == "" {
			return nil, fmt.Errorf("EMBEDDING_BASE_URL is required for the %q embedding provider", cfg.Provider)
		}
		return NewCompatibleClient(cfg.BaseURL, cfg.APIKey, cfg.Model, cfg.Dimensions), nil

	case ProviderMock:
		return NewMockClientDim(cfg.Dimensions), nil

	default:
		return nil, fmt.Errorf("unknown embedding provider: %s (valid: openai, openai-compatible, local, mock)", cfg.Provider)
	}
}
