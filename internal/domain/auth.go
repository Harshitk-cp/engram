package domain

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// User is a human account in the control plane (distinct from a tenant/API key).
type User struct {
	ID           uuid.UUID `json:"id"`
	Email        string    `json:"email"`
	PasswordHash *string   `json:"-"`
	Name         string    `json:"name"`
	AvatarURL    *string   `json:"avatar_url,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// OAuthAccount links a social identity (google/github) to a user.
type OAuthAccount struct {
	ID             uuid.UUID
	UserID         uuid.UUID
	Provider       string
	ProviderUserID string
	CreatedAt      time.Time
}

// Membership ties a user to a tenant (org) with a role.
type Membership struct {
	ID        uuid.UUID `json:"id"`
	UserID    uuid.UUID `json:"user_id"`
	TenantID  uuid.UUID `json:"tenant_id"`
	Role      string    `json:"role"`
	CreatedAt time.Time `json:"created_at"`
}

// MembershipWithTenant includes the org name for the tenant switcher.
type MembershipWithTenant struct {
	Membership
	TenantName string `json:"tenant_name"`
}

// ConsoleSession is a human login session (distinct from a conversation Session);
// the cookie holds an opaque token whose sha256 is stored as token_hash.
type ConsoleSession struct {
	ID             uuid.UUID
	TokenHash      string
	UserID         uuid.UUID
	ActiveTenantID *uuid.UUID
	ExpiresAt      time.Time
	CreatedAt      time.Time
}

type UserStore interface {
	Create(ctx context.Context, u *User) error
	GetByEmail(ctx context.Context, email string) (*User, error)
	GetByID(ctx context.Context, id uuid.UUID) (*User, error)
}

type OAuthAccountStore interface {
	Create(ctx context.Context, a *OAuthAccount) error
	GetByProvider(ctx context.Context, provider, providerUserID string) (*OAuthAccount, error)
}

type MembershipStore interface {
	Create(ctx context.Context, m *Membership) error
	Get(ctx context.Context, userID, tenantID uuid.UUID) (*Membership, error)
	ListByUser(ctx context.Context, userID uuid.UUID) ([]MembershipWithTenant, error)
}

type ConsoleSessionStore interface {
	Create(ctx context.Context, s *ConsoleSession) error
	GetByTokenHash(ctx context.Context, tokenHash string) (*ConsoleSession, error)
	UpdateActiveTenant(ctx context.Context, sessionID, tenantID uuid.UUID) error
	Delete(ctx context.Context, tokenHash string) error
	DeleteExpired(ctx context.Context) error
}
