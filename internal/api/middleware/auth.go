package middleware

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/Harshitk-cp/engram/internal/domain"
)

type contextKey string

const (
	tenantContextKey contextKey = "tenant"
	authContextKey   contextKey = "auth"
)

// SessionCookieName is the console session cookie.
const SessionCookieName = "engram_session"

// SessionResolver turns a raw session token into a data-plane auth context.
// Implemented by the control-plane AuthService.
type SessionResolver interface {
	ResolveSessionAuth(ctx context.Context, rawToken string) (*domain.APIKeyAuth, error)
}

func TenantFromContext(ctx context.Context) *domain.Tenant {
	if a, ok := ctx.Value(authContextKey).(*domain.APIKeyAuth); ok && a != nil {
		return a.Tenant
	}
	return nil
}

func AuthFromContext(ctx context.Context) *domain.APIKeyAuth {
	a, _ := ctx.Value(authContextKey).(*domain.APIKeyAuth)
	return a
}

// APIKeyAuth authenticates requests using the api_keys table.
// On success it stores *domain.APIKeyAuth in the request context and
// fires a non-blocking last_used_at update.
func APIKeyAuth(apiKeyStore domain.APIKeyStore) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				writeError(w, http.StatusUnauthorized, "missing authorization header")
				return
			}

			parts := strings.SplitN(authHeader, " ", 2)
			if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
				writeError(w, http.StatusUnauthorized, "invalid authorization header format")
				return
			}

			hash := HashAPIKey(parts[1])

			auth, err := apiKeyStore.GetAuthByHash(r.Context(), hash)
			if err != nil {
				writeError(w, http.StatusUnauthorized, "invalid API key")
				return
			}

			// Non-blocking last_used_at update — don't slow down the request path.
			go func(id interface{ String() string }) {
				_ = apiKeyStore.UpdateLastUsed(context.Background(), auth.KeyID)
			}(auth.KeyID)

			ctx := context.WithValue(r.Context(), authContextKey, auth)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// SessionOrAPIKey authenticates a request via the console session cookie first,
// then falls back to a Bearer API key. Both resolve to a *domain.APIKeyAuth in
// the request context, so all /v1 handlers work for the browser console and for
// programmatic clients without any per-handler changes.
func SessionOrAPIKey(apiKeyStore domain.APIKeyStore, resolver SessionResolver) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if resolver != nil {
				if c, err := r.Cookie(SessionCookieName); err == nil && c.Value != "" {
					if auth, err := resolver.ResolveSessionAuth(r.Context(), c.Value); err == nil && auth != nil {
						next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), authContextKey, auth)))
						return
					}
				}
			}

			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				writeError(w, http.StatusUnauthorized, "missing authorization")
				return
			}
			parts := strings.SplitN(authHeader, " ", 2)
			if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
				writeError(w, http.StatusUnauthorized, "invalid authorization header format")
				return
			}
			auth, err := apiKeyStore.GetAuthByHash(r.Context(), HashAPIKey(parts[1]))
			if err != nil {
				writeError(w, http.StatusUnauthorized, "invalid API key")
				return
			}
			go func() { _ = apiKeyStore.UpdateLastUsed(context.Background(), auth.KeyID) }()
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), authContextKey, auth)))
		})
	}
}

// RequireScope returns middleware that rejects requests whose API key lacks the given scope.
// Keys with the "admin" scope pass all scope checks.
func RequireScope(scope string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			auth := AuthFromContext(r.Context())
			if auth == nil || !auth.HasScope(scope) {
				writeError(w, http.StatusForbidden, "insufficient scope: "+scope+" required")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// RequireWriteForMutations enforces the "write" scope on any state-changing
// request (POST/PUT/PATCH/DELETE) while leaving safe, read-only methods
// (GET/HEAD/OPTIONS) open to read-scoped keys.
func RequireWriteForMutations(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
			next.ServeHTTP(w, r)
			return
		}
		auth := AuthFromContext(r.Context())
		if auth == nil || !auth.HasScope("write") {
			writeError(w, http.StatusForbidden, "insufficient scope: write required")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// HashAPIKey returns the SHA-256 hex digest of the given key.
func HashAPIKey(key string) string {
	h := sha256.Sum256([]byte(key))
	return hex.EncodeToString(h[:])
}

func writeError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
