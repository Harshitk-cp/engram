package domain

import (
	"time"

	"github.com/google/uuid"
)

// Scope values for API keys.
const (
	ScopeRead  = "read"
	ScopeWrite = "write"
	ScopeAdmin = "admin"
)

// MasterKeyScopes are granted to keys created via the setup or legacy tenant endpoint.
var MasterKeyScopes = []string{ScopeAdmin, ScopeRead, ScopeWrite}

// DefaultKeyScopes are granted to user-created restricted keys unless overridden.
var DefaultKeyScopes = []string{ScopeRead, ScopeWrite}

type Tenant struct {
	ID        uuid.UUID `json:"id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// APIKey represents a tenant API key stored in the api_keys table.
// The full key is returned only once at creation. Listings return KeyPrefix only.
type APIKey struct {
	ID         uuid.UUID  `json:"id"`
	TenantID   uuid.UUID  `json:"tenant_id"`
	Name       string     `json:"name"`
	KeyPrefix  string     `json:"key_prefix"`
	Scopes     []string   `json:"scopes"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
	ExpiresAt  *time.Time `json:"expires_at,omitempty"`
	RevokedAt  *time.Time `json:"revoked_at,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`

	// KeyHash is only populated internally for storage; never serialised.
	KeyHash string `json:"-"`
}

// APIKeyAuth is the result of a successful authentication lookup.
// It combines key metadata with the owning tenant.
type APIKeyAuth struct {
	KeyID  uuid.UUID
	Scopes []string
	Tenant *Tenant
}

// HasScope reports whether the auth context includes the requested scope.
// Keys with admin scope implicitly satisfy any scope check.
func (a *APIKeyAuth) HasScope(scope string) bool {
	for _, s := range a.Scopes {
		if s == ScopeAdmin || s == scope {
			return true
		}
	}
	return false
}
