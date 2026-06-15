package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/Harshitk-cp/engram/internal/api/middleware"
	"github.com/Harshitk-cp/engram/internal/domain"
)

// SettingsHandler exposes per-tenant engine tuning (decay, confidence deltas,
// competition) so operators can adjust the cognitive engine without a redeploy.
type SettingsHandler struct {
	store domain.TenantSettingsStore
}

func NewSettingsHandler(store domain.TenantSettingsStore) *SettingsHandler {
	return &SettingsHandler{store: store}
}

// Get returns the tenant's effective engine settings (defaults if never set).
func (h *SettingsHandler) Get(w http.ResponseWriter, r *http.Request) {
	tenant := middleware.TenantFromContext(r.Context())
	if tenant == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	es, err := h.store.Get(r.Context(), tenant.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"settings": es,
		"defaults": domain.DefaultEngineSettings(),
	})
}

// Update upserts the tenant's engine settings. Values are sanitized (clamped to
// safe ranges) before persistence so a bad write can't destabilize the engine.
func (h *SettingsHandler) Update(w http.ResponseWriter, r *http.Request) {
	tenant := middleware.TenantFromContext(r.Context())
	if tenant == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var es domain.EngineSettings
	if err := json.NewDecoder(r.Body).Decode(&es); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	es = es.Sanitize()
	if err := h.store.Upsert(r.Context(), tenant.ID, es); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"settings": es,
		"defaults": domain.DefaultEngineSettings(),
	})
}
