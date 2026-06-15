package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Harshitk-cp/engram/internal/domain"
	"github.com/google/uuid"
	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"
)

var (
	ErrInvalidCredentials = errors.New("invalid email or password")
	ErrEmailTaken         = errors.New("an account with that email already exists")
	ErrNotAMember         = errors.New("not a member of that org")
)

// AuthService is the control plane: human accounts, sessions, orgs/memberships.
// It is deliberately separate from the data-plane API-key auth.
type AuthService struct {
	users           domain.UserStore
	oauth           domain.OAuthAccountStore
	memberships     domain.MembershipStore
	sessions        domain.ConsoleSessionStore
	tenants         domain.TenantStore
	defaultTenantID string
	defaultRole     string
	sessionTTL      time.Duration
	logger          *zap.Logger
}

func NewAuthService(users domain.UserStore, oauth domain.OAuthAccountStore, memberships domain.MembershipStore, sessions domain.ConsoleSessionStore, tenants domain.TenantStore, defaultTenantID, defaultRole string, sessionTTL time.Duration, logger *zap.Logger) *AuthService {
	if defaultRole == "" {
		defaultRole = "member"
	}
	return &AuthService{
		users: users, oauth: oauth, memberships: memberships, sessions: sessions,
		tenants: tenants, defaultTenantID: defaultTenantID, defaultRole: defaultRole,
		sessionTTL: sessionTTL, logger: logger,
	}
}

// roleScopes maps a membership role to data-plane scopes.
func roleScopes(role string) []string {
	switch role {
	case "owner", "admin":
		return []string{"admin", "read", "write"}
	default:
		return []string{"read", "write"}
	}
}

func hashToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

func newToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// Register creates a user, provisions an org, and starts a session. Returns the
// raw session token (to be set as the cookie).
func (s *AuthService) Register(ctx context.Context, email, password, name string) (*domain.User, string, error) {
	email = strings.TrimSpace(strings.ToLower(email))
	if email == "" || len(password) < 8 {
		return nil, "", errors.New("email and an 8+ character password are required")
	}
	if existing, err := s.users.GetByEmail(ctx, email); err == nil && existing != nil {
		return nil, "", ErrEmailTaken
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, "", err
	}
	hs := string(hash)
	if name == "" {
		name = strings.Split(email, "@")[0]
	}
	u := &domain.User{Email: email, PasswordHash: &hs, Name: name}
	if err := s.users.Create(ctx, u); err != nil {
		return nil, "", err
	}
	tenantID, err := s.provisionTenant(ctx, u)
	if err != nil {
		return nil, "", err
	}
	token, err := s.startSession(ctx, u.ID, &tenantID)
	if err != nil {
		return nil, "", err
	}
	return u, token, nil
}

// Login verifies credentials and starts a session.
func (s *AuthService) Login(ctx context.Context, email, password string) (string, error) {
	email = strings.TrimSpace(strings.ToLower(email))
	u, err := s.users.GetByEmail(ctx, email)
	if err != nil || u == nil || u.PasswordHash == nil {
		return "", ErrInvalidCredentials
	}
	if err := bcrypt.CompareHashAndPassword([]byte(*u.PasswordHash), []byte(password)); err != nil {
		return "", ErrInvalidCredentials
	}
	tenantID := s.activeTenantFor(ctx, u)
	token, err := s.startSession(ctx, u.ID, tenantID)
	if err != nil {
		return "", err
	}
	return token, nil
}

// OAuthLogin links/creates a user from a social identity and starts a session.
//
// SECURITY INVARIANT: `email` MUST be provider-verified (the caller passes "" when
// the provider has not verified it). Auto-linking to an existing account by email
// is only safe under that invariant — linking on an unverified email would allow
// account takeover (attacker registers a provider account with the victim's email).
func (s *AuthService) OAuthLogin(ctx context.Context, provider, providerUserID, email, name, avatar string) (string, error) {
	email = strings.TrimSpace(strings.ToLower(email))
	var user *domain.User

	if acct, err := s.oauth.GetByProvider(ctx, provider, providerUserID); err == nil && acct != nil {
		// Already-linked identity: always trust the stored oauth_accounts row.
		if user, err = s.users.GetByID(ctx, acct.UserID); err != nil {
			return "", err
		}
	} else if email != "" {
		// Link to an existing account only by a provider-VERIFIED email (see invariant).
		if existing, err := s.users.GetByEmail(ctx, email); err == nil && existing != nil {
			user = existing
			_ = s.oauth.Create(ctx, &domain.OAuthAccount{UserID: user.ID, Provider: provider, ProviderUserID: providerUserID})
		}
	}

	if user == nil {
		if email == "" {
			email = fmt.Sprintf("%s_%s@users.noreply.engram", provider, providerUserID)
		}
		if name == "" {
			name = strings.Split(email, "@")[0]
		}
		var av *string
		if avatar != "" {
			av = &avatar
		}
		user = &domain.User{Email: email, Name: name, AvatarURL: av}
		if err := s.users.Create(ctx, user); err != nil {
			return "", err
		}
		if err := s.oauth.Create(ctx, &domain.OAuthAccount{UserID: user.ID, Provider: provider, ProviderUserID: providerUserID}); err != nil {
			return "", err
		}
		if _, err := s.provisionTenant(ctx, user); err != nil {
			return "", err
		}
	}

	tenantID := s.activeTenantFor(ctx, user)
	return s.startSession(ctx, user.ID, tenantID)
}

func (s *AuthService) Logout(ctx context.Context, rawToken string) error {
	return s.sessions.Delete(ctx, hashToken(rawToken))
}

// ResolveSessionAuth turns a session token into a data-plane auth context so the
// combined middleware can authorize /v1 calls from the console.
func (s *AuthService) ResolveSessionAuth(ctx context.Context, rawToken string) (*domain.APIKeyAuth, error) {
	sess, err := s.sessions.GetByTokenHash(ctx, hashToken(rawToken))
	if err != nil {
		return nil, err
	}
	if time.Now().After(sess.ExpiresAt) {
		_ = s.sessions.Delete(ctx, sess.TokenHash)
		return nil, errors.New("session expired")
	}
	if sess.ActiveTenantID == nil {
		return nil, errors.New("no active org")
	}
	tenant, err := s.tenants.GetByID(ctx, *sess.ActiveTenantID)
	if err != nil {
		return nil, err
	}
	m, err := s.memberships.Get(ctx, sess.UserID, *sess.ActiveTenantID)
	if err != nil {
		return nil, ErrNotAMember
	}
	uid := sess.UserID
	return &domain.APIKeyAuth{KeyID: sess.UserID, UserID: &uid, Scopes: roleScopes(m.Role), Tenant: tenant}, nil
}

// CurrentUser returns the session's user, the session, and the user's orgs.
func (s *AuthService) CurrentUser(ctx context.Context, rawToken string) (*domain.User, *domain.ConsoleSession, []domain.MembershipWithTenant, error) {
	sess, err := s.sessions.GetByTokenHash(ctx, hashToken(rawToken))
	if err != nil {
		return nil, nil, nil, err
	}
	if time.Now().After(sess.ExpiresAt) {
		return nil, nil, nil, errors.New("session expired")
	}
	u, err := s.users.GetByID(ctx, sess.UserID)
	if err != nil {
		return nil, nil, nil, err
	}
	orgs, err := s.memberships.ListByUser(ctx, sess.UserID)
	if err != nil {
		return nil, nil, nil, err
	}
	return u, sess, orgs, nil
}

// SwitchTenant changes the session's active org after verifying membership.
func (s *AuthService) CreateOrg(ctx context.Context, rawToken, name string) (*domain.MembershipWithTenant, error) {
	sess, err := s.sessions.GetByTokenHash(ctx, hashToken(rawToken))
	if err != nil {
		return nil, err
	}
	if time.Now().After(sess.ExpiresAt) {
		return nil, errors.New("session expired")
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, errors.New("organization name is required")
	}
	t := &domain.Tenant{Name: name}
	if err := s.tenants.Create(ctx, t); err != nil {
		return nil, err
	}
	if err := s.memberships.Create(ctx, &domain.Membership{UserID: sess.UserID, TenantID: t.ID, Role: "owner"}); err != nil {
		return nil, err
	}
	if err := s.sessions.UpdateActiveTenant(ctx, sess.ID, t.ID); err != nil {
		return nil, err
	}
	return &domain.MembershipWithTenant{
		Membership: domain.Membership{UserID: sess.UserID, TenantID: t.ID, Role: "owner"},
		TenantName: t.Name,
	}, nil
}

func (s *AuthService) SwitchTenant(ctx context.Context, rawToken string, tenantID uuid.UUID) error {
	sess, err := s.sessions.GetByTokenHash(ctx, hashToken(rawToken))
	if err != nil {
		return err
	}
	if _, err := s.memberships.Get(ctx, sess.UserID, tenantID); err != nil {
		return ErrNotAMember
	}
	return s.sessions.UpdateActiveTenant(ctx, sess.ID, tenantID)
}

// provisionTenant joins the default org (if configured) or creates a personal org.
func (s *AuthService) provisionTenant(ctx context.Context, u *domain.User) (uuid.UUID, error) {
	if s.defaultTenantID != "" {
		if tid, err := uuid.Parse(s.defaultTenantID); err == nil {
			if _, err := s.tenants.GetByID(ctx, tid); err == nil {
				if err := s.memberships.Create(ctx, &domain.Membership{UserID: u.ID, TenantID: tid, Role: s.defaultRole}); err != nil {
					return uuid.Nil, err
				}
				return tid, nil
			}
		}
		s.logger.Warn("ENGRAM_DEFAULT_TENANT_ID invalid; creating a personal org instead")
	}
	t := &domain.Tenant{Name: u.Name + "'s Org"}
	if err := s.tenants.Create(ctx, t); err != nil {
		return uuid.Nil, err
	}
	if err := s.memberships.Create(ctx, &domain.Membership{UserID: u.ID, TenantID: t.ID, Role: "owner"}); err != nil {
		return uuid.Nil, err
	}
	return t.ID, nil
}

// activeTenantFor returns the user's first org (or provisions one if none).
func (s *AuthService) activeTenantFor(ctx context.Context, u *domain.User) *uuid.UUID {
	orgs, err := s.memberships.ListByUser(ctx, u.ID)
	if err == nil && len(orgs) > 0 {
		id := orgs[0].TenantID
		return &id
	}
	if id, err := s.provisionTenant(ctx, u); err == nil {
		return &id
	}
	return nil
}

func (s *AuthService) startSession(ctx context.Context, userID uuid.UUID, tenantID *uuid.UUID) (string, error) {
	raw, err := newToken()
	if err != nil {
		return "", err
	}
	sess := &domain.ConsoleSession{
		TokenHash:      hashToken(raw),
		UserID:         userID,
		ActiveTenantID: tenantID,
		ExpiresAt:      time.Now().Add(s.sessionTTL),
	}
	if err := s.sessions.Create(ctx, sess); err != nil {
		return "", err
	}
	return raw, nil
}
