package handlers

import (
	"net/http"
	"strconv"
	"time"

	"github.com/Harshitk-cp/engram/internal/api/middleware"
	"github.com/Harshitk-cp/engram/internal/domain"
	"github.com/Harshitk-cp/engram/internal/service"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

type ConsoleHandler struct {
	svc *service.ConsoleService
}

func NewConsoleHandler(svc *service.ConsoleService) *ConsoleHandler {
	return &ConsoleHandler{svc: svc}
}

// Dashboard handles GET /v1/agents/{id}/dashboard.
func (h *ConsoleHandler) Dashboard(w http.ResponseWriter, r *http.Request) {
	tenant := middleware.TenantFromContext(r.Context())
	if tenant == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	agentID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid agent id")
		return
	}

	summary, err := h.svc.Dashboard(r.Context(), agentID, tenant.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to build dashboard")
		return
	}
	writeJSON(w, http.StatusOK, summary)
}

// Memories handles GET /v1/agents/{id}/memories?tier=&type=&limit=&offset=.
func (h *ConsoleHandler) Memories(w http.ResponseWriter, r *http.Request) {
	tenant := middleware.TenantFromContext(r.Context())
	if tenant == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	agentID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid agent id")
		return
	}
	q := r.URL.Query()
	limit := 50
	if v := q.Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}
	offset := 0
	if v := q.Get("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			offset = n
		}
	}
	filter := domain.MemoryFilter{
		Tier:       q.Get("tier"),
		Type:       q.Get("type"),
		Provenance: q.Get("provenance"),
		Binding:    q.Get("binding"),
	}
	page, err := h.svc.Memories(r.Context(), agentID, tenant.ID, filter, limit, offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list memories")
		return
	}
	writeJSON(w, http.StatusOK, page)
}

// Snapshot handles GET /v1/agents/{id}/snapshot?at=<RFC3339> — beliefs as of a
// past instant (defaults to now).
func (h *ConsoleHandler) Snapshot(w http.ResponseWriter, r *http.Request) {
	tenant := middleware.TenantFromContext(r.Context())
	if tenant == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	agentID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid agent id")
		return
	}
	at := time.Now()
	if v := r.URL.Query().Get("at"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			at = t
		} else {
			writeError(w, http.StatusBadRequest, "invalid 'at' (use RFC3339)")
			return
		}
	}
	limit := 100
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}
	snap, err := h.svc.SnapshotAsOf(r.Context(), agentID, tenant.ID, at, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to reconstruct snapshot")
		return
	}
	writeJSON(w, http.StatusOK, snap)
}

// Contradictions handles GET /v1/agents/{id}/contradictions.
func (h *ConsoleHandler) Contradictions(w http.ResponseWriter, r *http.Request) {
	tenant := middleware.TenantFromContext(r.Context())
	if tenant == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	agentID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid agent id")
		return
	}
	pairs, err := h.svc.Contradictions(r.Context(), agentID, tenant.ID, 200)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list contradictions")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"pairs": pairs, "count": len(pairs)})
}

// ReviewQueue handles GET /v1/agents/{id}/review-queue.
func (h *ConsoleHandler) ReviewQueue(w http.ResponseWriter, r *http.Request) {
	tenant := middleware.TenantFromContext(r.Context())
	if tenant == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	agentID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid agent id")
		return
	}

	limit := 50
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}

	items, err := h.svc.ReviewQueue(r.Context(), agentID, tenant.ID, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to build review queue")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"items": items,
		"count": len(items),
	})
}
