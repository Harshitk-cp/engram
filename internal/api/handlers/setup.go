package handlers

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/Harshitk-cp/engram/internal/api/middleware"
	"github.com/Harshitk-cp/engram/internal/domain"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// SetupHandler handles bootstrap and API key management endpoints.
type SetupHandler struct {
	tenantStore domain.TenantStore
	apiKeyStore domain.APIKeyStore
	setupToken  string // value of ENGRAM_SETUP_TOKEN; empty means setup is disabled
}

func NewSetupHandler(ts domain.TenantStore, aks domain.APIKeyStore, setupToken string) *SetupHandler {
	return &SetupHandler{tenantStore: ts, apiKeyStore: aks, setupToken: setupToken}
}

// --- Bootstrap ---

type bootstrapRequest struct {
	OrgName string `json:"org_name"`
}

type bootstrapResponse struct {
	TenantID   string    `json:"tenant_id"`
	TenantName string    `json:"tenant_name"`
	KeyID      string    `json:"key_id"`
	KeyPrefix  string    `json:"key_prefix"`
	APIKey     string    `json:"api_key"` // shown only once
	Scopes     []string  `json:"scopes"`
	CreatedAt  time.Time `json:"created_at"`
}

// Bootstrap creates a new tenant and its first master key atomically.
// Requires the X-Setup-Token header to match ENGRAM_SETUP_TOKEN.
func (h *SetupHandler) Bootstrap(w http.ResponseWriter, r *http.Request) {
	if h.setupToken == "" {
		writeError(w, http.StatusServiceUnavailable, "setup endpoint is not configured (ENGRAM_SETUP_TOKEN not set)")
		return
	}

	if !tokenMatches(r.Header.Get("X-Setup-Token"), h.setupToken) {
		writeError(w, http.StatusForbidden, "invalid setup token")
		return
	}

	var req bootstrapRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.OrgName) == "" {
		writeError(w, http.StatusBadRequest, "org_name is required")
		return
	}

	tenant := &domain.Tenant{Name: req.OrgName}
	if err := h.tenantStore.Create(r.Context(), tenant); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create tenant")
		return
	}

	rawKey, apiKey, err := buildMasterKey(tenant.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to generate API key")
		return
	}

	if err := h.apiKeyStore.Create(r.Context(), apiKey); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to store API key")
		return
	}

	writeJSON(w, http.StatusCreated, bootstrapResponse{
		TenantID:   tenant.ID.String(),
		TenantName: tenant.Name,
		KeyID:      apiKey.ID.String(),
		KeyPrefix:  apiKey.KeyPrefix,
		APIKey:     rawKey,
		Scopes:     apiKey.Scopes,
		CreatedAt:  apiKey.CreatedAt,
	})
}

// --- Key management (require admin scope) ---

type createKeyRequest struct {
	Name      string     `json:"name"`
	Scopes    []string   `json:"scopes"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
}

type createKeyResponse struct {
	KeyID     string     `json:"key_id"`
	KeyPrefix string     `json:"key_prefix"`
	APIKey    string     `json:"api_key"` // shown only once
	Name      string     `json:"name"`
	Scopes    []string   `json:"scopes"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
}

// CreateKey creates a new API key for the authenticated tenant.
func (h *SetupHandler) CreateKey(w http.ResponseWriter, r *http.Request) {
	tenant := middleware.TenantFromContext(r.Context())
	if tenant == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req createKeyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if strings.TrimSpace(req.Name) == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}

	scopes := req.Scopes
	if len(scopes) == 0 {
		scopes = domain.DefaultKeyScopes
	}
	if !validScopes(scopes) {
		writeError(w, http.StatusBadRequest, "invalid scopes: allowed values are read, write, admin")
		return
	}

	rawKey, err := generateRawKey("rk")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to generate API key")
		return
	}

	apiKey := &domain.APIKey{
		TenantID:  tenant.ID,
		Name:      req.Name,
		KeyHash:   middleware.HashAPIKey(rawKey),
		KeyPrefix: rawKey[:12],
		Scopes:    scopes,
		ExpiresAt: req.ExpiresAt,
	}
	// Attribute the key to the console user who created it (nil for API-key callers).
	if auth := middleware.AuthFromContext(r.Context()); auth != nil {
		apiKey.CreatedBy = auth.UserID
	}

	if err := h.apiKeyStore.Create(r.Context(), apiKey); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create key")
		return
	}

	writeJSON(w, http.StatusCreated, createKeyResponse{
		KeyID:     apiKey.ID.String(),
		KeyPrefix: apiKey.KeyPrefix,
		APIKey:    rawKey,
		Name:      apiKey.Name,
		Scopes:    apiKey.Scopes,
		ExpiresAt: apiKey.ExpiresAt,
		CreatedAt: apiKey.CreatedAt,
	})
}

// ListKeys returns all active keys for the authenticated tenant.
func (h *SetupHandler) ListKeys(w http.ResponseWriter, r *http.Request) {
	tenant := middleware.TenantFromContext(r.Context())
	if tenant == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	keys, err := h.apiKeyStore.ListByTenantID(r.Context(), tenant.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list keys")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"keys": keys, "count": len(keys)})
}

// RevokeKey revokes a key by ID. Immediate effect on the next request using that key.
func (h *SetupHandler) RevokeKey(w http.ResponseWriter, r *http.Request) {
	tenant := middleware.TenantFromContext(r.Context())
	if tenant == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid key id")
		return
	}

	if err := h.apiKeyStore.Revoke(r.Context(), id, tenant.ID); err != nil {
		writeError(w, http.StatusNotFound, "key not found or already revoked")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// --- Helpers ---

func buildMasterKey(tenantID uuid.UUID) (rawKey string, k *domain.APIKey, err error) {
	rawKey, err = generateRawKey("mk")
	if err != nil {
		return "", nil, err
	}
	k = &domain.APIKey{
		TenantID:  tenantID,
		Name:      "default",
		KeyHash:   middleware.HashAPIKey(rawKey),
		KeyPrefix: rawKey[:12],
		Scopes:    domain.MasterKeyScopes,
	}
	return rawKey, k, nil
}

// generateRawKey returns a key with the given prefix: e.g. "mk_<64 hex chars>".
func generateRawKey(prefix string) (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return prefix + "_" + hex.EncodeToString(b), nil
}

func validScopes(scopes []string) bool {
	allowed := map[string]bool{domain.ScopeRead: true, domain.ScopeWrite: true, domain.ScopeAdmin: true}
	for _, s := range scopes {
		if !allowed[s] {
			return false
		}
	}
	return len(scopes) > 0
}
