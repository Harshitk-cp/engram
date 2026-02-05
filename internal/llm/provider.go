package llm

import (
	"fmt"

	"github.com/Harshitk-cp/engram/internal/domain"
)

func provenanceTag(p domain.Provenance) string {
	switch p {
	case domain.ProvenanceUser:
		return "USER"
	case domain.ProvenanceTool:
		return "TOOL"
	case domain.ProvenanceAgent:
		return "AGENT"
	case domain.ProvenanceDerived:
		return "DERIVED"
	case domain.ProvenanceInferred:
		return "INFERRED"
	default:
		return "UNKNOWN"
	}
}

// Provider constants
const (
	ProviderOpenAI    = "openai"
	ProviderAnthropic = "anthropic"
	ProviderGemini    = "gemini"
	ProviderCerebras  = "cerebras"
	ProviderMock      = "mock"
)

// NewClient creates an LLM client based on the provider name.
// Returns an error if the provider is unknown or the API key is empty (except for mock).
func NewClient(provider, apiKey string) (domain.LLMClient, error) {
	switch provider {
	case ProviderOpenAI:
		if apiKey == "" {
			return nil, fmt.Errorf("OPENAI_API_KEY is required for OpenAI provider")
		}
		return NewOpenAIClient(apiKey), nil

	case ProviderAnthropic:
		if apiKey == "" {
			return nil, fmt.Errorf("ANTHROPIC_API_KEY is required for Anthropic provider")
		}
		return NewAnthropicClient(apiKey), nil

	case ProviderGemini:
		if apiKey == "" {
			return nil, fmt.Errorf("GEMINI_API_KEY is required for Gemini provider")
		}
		return NewGeminiClient(apiKey), nil

	case ProviderCerebras:
		if apiKey == "" {
			return nil, fmt.Errorf("CEREBRAS_API_KEY is required for Cerebras provider")
		}
		return NewCerebrasClient(apiKey), nil

	case ProviderMock:
		return NewMockClient(), nil

	default:
		return nil, fmt.Errorf("unknown LLM provider: %s (valid options: openai, anthropic, gemini, cerebras, mock)", provider)
	}
}
