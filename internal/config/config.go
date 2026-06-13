package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"

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

// SetupToken returns ENGRAM_SETUP_TOKEN. If empty, POST /v1/setup is disabled.
func SetupToken() string {
	return os.Getenv("ENGRAM_SETUP_TOKEN")
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

// CORSAllowedOrigins returns the browser origins permitted to call the API,
// parsed from the comma-separated CORS_ALLOWED_ORIGINS env var. An empty result
// disables CORS; a single "*" allows any origin. Used by the console frontend.
func CORSAllowedOrigins() []string {
	raw := strings.TrimSpace(os.Getenv("CORS_ALLOWED_ORIGINS"))
	if raw == "" {
		return nil
	}
	var origins []string
	for _, o := range strings.Split(raw, ",") {
		if o = strings.TrimSpace(o); o != "" {
			origins = append(origins, o)
		}
	}
	return origins
}

// ---- Control plane (console auth) ----

// DefaultTenantID, if set, is the org that newly-signed-up users auto-join
// (instead of getting a fresh org). Used to point demo signups at seeded data.
func DefaultTenantID() string { return strings.TrimSpace(os.Getenv("ENGRAM_DEFAULT_TENANT_ID")) }

// DefaultTenantRole is the role new users get when auto-joining the default org.
// Product default is "member"; set to "admin" in a demo so signups can manage
// keys and resolve contradictions on the shared seeded org.
func DefaultTenantRole() string {
	r := strings.ToLower(strings.TrimSpace(os.Getenv("ENGRAM_DEFAULT_TENANT_ROLE")))
	switch r {
	case "owner", "admin", "member":
		return r
	default:
		return "member"
	}
}

// SessionTTLHours is how long a console session lasts. Default 30 days.
func SessionTTLHours() int {
	if n, err := strconv.Atoi(os.Getenv("SESSION_TTL_HOURS")); err == nil && n > 0 {
		return n
	}
	return 720
}

// CookieSecure controls the Secure flag on the session cookie. Default false so
// it works on http://localhost; set COOKIE_SECURE=true behind HTTPS.
func CookieSecure() bool { return strings.EqualFold(os.Getenv("COOKIE_SECURE"), "true") }

// TrustProxyHeaders controls whether client-IP headers (X-Real-IP /
// X-Forwarded-For) are honored. Default false: on a directly-exposed server
// those headers are attacker-controlled and would let anyone spoof the IP the
// rate limiter keys on. Set TRUST_PROXY_HEADERS=true only behind a proxy that
// strips and re-sets them.
func TrustProxyHeaders() bool { return strings.EqualFold(os.Getenv("TRUST_PROXY_HEADERS"), "true") }

// AppBaseURL is the externally-reachable base URL, used to build OAuth redirect
// URIs. Defaults to http://localhost:<port>.
func AppBaseURL() string {
	if v := strings.TrimSpace(os.Getenv("APP_BASE_URL")); v != "" {
		return strings.TrimRight(v, "/")
	}
	return fmt.Sprintf("http://localhost:%d", ServerPort())
}

// GoogleOAuth returns (clientID, clientSecret); empty when not configured.
func GoogleOAuth() (string, string) {
	return os.Getenv("GOOGLE_OAUTH_CLIENT_ID"), os.Getenv("GOOGLE_OAUTH_CLIENT_SECRET")
}

// GitHubOAuth returns (clientID, clientSecret); empty when not configured.
func GitHubOAuth() (string, string) {
	return os.Getenv("GITHUB_OAUTH_CLIENT_ID"), os.Getenv("GITHUB_OAUTH_CLIENT_SECRET")
}

// WorkOSAuth returns (clientID, apiKey) for WorkOS enterprise SSO (AuthKit);
// empty when not configured. The API key is the client secret for token exchange.
func WorkOSAuth() (string, string) {
	return os.Getenv("WORKOS_CLIENT_ID"), os.Getenv("WORKOS_API_KEY")
}

// AuditSigningKey is an optional HMAC secret used to sign audit exports so a
// recipient can confirm an export came from this server. Empty = unsigned export.
func AuditSigningKey() string { return os.Getenv("AUDIT_SIGNING_KEY") }

// ---- Billing / managed cloud (Stripe) ----

// StripeSecretKey is the Stripe API secret. When empty, billing is disabled:
// the checkout/portal/webhook endpoints report "not configured" and quota
// enforcement is a no-op, so self-hosted/OSS deployments run unmetered.
func StripeSecretKey() string { return strings.TrimSpace(os.Getenv("STRIPE_SECRET_KEY")) }

// StripeWebhookSecret verifies the signature on incoming Stripe webhook events.
func StripeWebhookSecret() string { return strings.TrimSpace(os.Getenv("STRIPE_WEBHOOK_SECRET")) }

// BillingEnabled reports whether managed-cloud billing + quota enforcement is on.
// Gated solely on a Stripe secret being configured.
func BillingEnabled() bool { return StripeSecretKey() != "" }

// StripePriceIDs maps the self-serve plan names to their Stripe Price IDs,
// created in the Stripe dashboard. Plans without a configured price can't be
// purchased via checkout.
func StripePriceIDs() map[string]string {
	m := map[string]string{}
	if v := strings.TrimSpace(os.Getenv("STRIPE_PRICE_DEVELOPER")); v != "" {
		m["developer"] = v
	}
	if v := strings.TrimSpace(os.Getenv("STRIPE_PRICE_TEAM")); v != "" {
		m["team"] = v
	}
	if v := strings.TrimSpace(os.Getenv("STRIPE_PRICE_GROWTH")); v != "" {
		m["growth"] = v
	}
	return m
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
