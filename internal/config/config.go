package config

import (
	"fmt"
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

// Load reads the .env file specified by ENGRAM_ENV (or .env by default),
// then loads the corresponding .secret file if it exists.
// All config is flat env vars read via os.Getenv after loading.
func Load() error {
	envFile := os.Getenv("ENGRAM_ENV")
	if envFile == "" {
		envFile = ".env"
	}

	// Load main env file (ignore error if file doesn't exist)
	_ = godotenv.Load(envFile)

	// Load secret sidecar if it exists
	_ = godotenv.Load(envFile + ".secret")

	return nil
}

func ServerPort() int {
	port, err := strconv.Atoi(os.Getenv("SERVER_PORT"))
	if err != nil {
		return 8080
	}
	return port
}

func DatabaseURL() string {
	return os.Getenv("DATABASE_URL")
}

func OpenAIAPIKey() string {
	return os.Getenv("OPENAI_API_KEY")
}

func AnthropicAPIKey() string {
	return os.Getenv("ANTHROPIC_API_KEY")
}

func GeminiAPIKey() string {
	return os.Getenv("GEMINI_API_KEY")
}

func CerebrasAPIKey() string {
	return os.Getenv("CEREBRAS_API_KEY")
}

// LLMProvider returns the configured LLM provider.
// Defaults to "openai" if not set.
// Valid values: openai, anthropic, gemini, cerebras, mock
func LLMProvider() string {
	p := os.Getenv("LLM_PROVIDER")
	if p == "" {
		return "openai"
	}
	return p
}

// EmbeddingProvider returns the configured embedding provider.
// Defaults to "openai" if not set.
// Valid values: openai, mock
func EmbeddingProvider() string {
	p := os.Getenv("EMBEDDING_PROVIDER")
	if p == "" {
		return "openai"
	}
	return p
}

// LLMAPIKey returns the API key for the configured LLM provider.
func LLMAPIKey() string {
	switch LLMProvider() {
	case "anthropic":
		return AnthropicAPIKey()
	case "gemini":
		return GeminiAPIKey()
	case "cerebras":
		return CerebrasAPIKey()
	case "mock":
		return ""
	default:
		return OpenAIAPIKey()
	}
}

// EmbeddingAPIKey returns the API key for the configured embedding provider.
func EmbeddingAPIKey() string {
	switch EmbeddingProvider() {
	case "mock":
		return ""
	default:
		return OpenAIAPIKey()
	}
}

func MigrationsPath() string {
	p := os.Getenv("MIGRATIONS_PATH")
	if p == "" {
		return "migrations"
	}
	return p
}

func ServerAddr() string {
	return fmt.Sprintf(":%d", ServerPort())
}

// RateLimitRPS returns requests per second limit.
// Defaults to 100 if not set.
func RateLimitRPS() float64 {
	rps, err := strconv.ParseFloat(os.Getenv("RATE_LIMIT_RPS"), 64)
	if err != nil || rps <= 0 {
		return 100
	}
	return rps
}

// RateLimitBurst returns the burst size for rate limiting.
// Defaults to 20 if not set.
func RateLimitBurst() int {
	burst, err := strconv.Atoi(os.Getenv("RATE_LIMIT_BURST"))
	if err != nil || burst <= 0 {
		return 20
	}
	return burst
}

// LogLevel returns the log level (debug, info, warn, error).
// Defaults to "info" if not set.
func LogLevel() string {
	level := os.Getenv("LOG_LEVEL")
	if level == "" {
		return "info"
	}
	return level
}
