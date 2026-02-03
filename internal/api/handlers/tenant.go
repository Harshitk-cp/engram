package handlers

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"

	"github.com/Harshitk-cp/engram/internal/api/middleware"
	"github.com/Harshitk-cp/engram/internal/domain"
)

type TenantHandler struct {
	store domain.TenantStore
}

func NewTenantHandler(store domain.TenantStore) *TenantHandler {
	return &TenantHandler{store: store}
}

type createTenantRequest struct {
	Name string `json:"name"`
}

type createTenantResponse struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	APIKey string `json:"api_key"`
}

func (h *TenantHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req createTenantRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}

	apiKey, err := generateAPIKey()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to generate API key")
		return
	}

	tenant := &domain.Tenant{
		Name:       req.Name,
		APIKeyHash: middleware.HashAPIKey(apiKey),
	}

	if err := h.store.Create(r.Context(), tenant); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create tenant")
		return
	}

	writeJSON(w, http.StatusCreated, createTenantResponse{
		ID:     tenant.ID.String(),
		Name:   tenant.Name,
		APIKey: apiKey,
	})
}

func generateAPIKey() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return "mk_" + hex.EncodeToString(b), nil
}
