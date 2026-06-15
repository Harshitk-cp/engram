package handlers

import (
	"crypto/subtle"
	"encoding/json"
	"net/http"

	"github.com/Harshitk-cp/engram/internal/domain"
)

// TenantHandler exposes the legacy POST /v1/tenants endpoint.
// Deprecated: use POST /v1/setup instead.
type TenantHandler struct {
	tenantStore domain.TenantStore
	apiKeyStore domain.APIKeyStore
	setupToken  string
}

func NewTenantHandler(ts domain.TenantStore, aks domain.APIKeyStore, setupToken string) *TenantHandler {
	return &TenantHandler{tenantStore: ts, apiKeyStore: aks, setupToken: setupToken}
}

type createTenantRequest struct {
	Name string `json:"name"`
}

type createTenantResponse struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	APIKey string `json:"api_key"`
}

// Create is the legacy tenant bootstrap endpoint. It mints a tenant plus a
// full-scope master key, so it carries the same X-Setup-Token gate as
// /v1/setup: with no token configured it is disabled entirely.
// Deprecated — callers receive a Deprecation header.
func (h *TenantHandler) Create(w http.ResponseWriter, r *http.Request) {
	if h.setupToken == "" {
		writeError(w, http.StatusServiceUnavailable, "tenant creation is not configured (ENGRAM_SETUP_TOKEN not set); use POST /v1/setup")
		return
	}
	if !tokenMatches(r.Header.Get("X-Setup-Token"), h.setupToken) {
		writeError(w, http.StatusForbidden, "invalid setup token")
		return
	}

	var req createTenantRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}

	tenant := &domain.Tenant{Name: req.Name}
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

	w.Header().Set("Deprecation", "true")
	w.Header().Set("Link", `</v1/setup>; rel="successor-version"`)
	writeJSON(w, http.StatusCreated, createTenantResponse{
		ID:     tenant.ID.String(),
		Name:   tenant.Name,
		APIKey: rawKey,
	})
}

// tokenMatches compares a presented secret against the configured one in
// constant time.
func tokenMatches(presented, configured string) bool {
	return subtle.ConstantTimeCompare([]byte(presented), []byte(configured)) == 1
}
