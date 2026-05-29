package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/Harshitk-cp/engram/internal/domain"
)

// TenantHandler exposes the legacy POST /v1/tenants endpoint.
// Deprecated: use POST /v1/setup instead.
type TenantHandler struct {
	tenantStore domain.TenantStore
	apiKeyStore domain.APIKeyStore
}

func NewTenantHandler(ts domain.TenantStore, aks domain.APIKeyStore) *TenantHandler {
	return &TenantHandler{tenantStore: ts, apiKeyStore: aks}
}

type createTenantRequest struct {
	Name string `json:"name"`
}

type createTenantResponse struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	APIKey string `json:"api_key"`
}

// Create is the legacy unauthenticated tenant bootstrap endpoint.
// It still functions but is deprecated — callers receive a Deprecation header.
func (h *TenantHandler) Create(w http.ResponseWriter, r *http.Request) {
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

