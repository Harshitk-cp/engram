package handlers

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/Harshitk-cp/engram/internal/api/middleware"
	"github.com/Harshitk-cp/engram/internal/domain"
	"github.com/Harshitk-cp/engram/internal/service"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

type AdminHandler struct {
	svc *service.AdminService
}

func NewAdminHandler(svc *service.AdminService) *AdminHandler {
	return &AdminHandler{svc: svc}
}

type updateMemoryRequest struct {
	Confidence *float32 `json:"confidence,omitempty"`
	Content    *string  `json:"content,omitempty"`
	Reason     string   `json:"reason"`
}

// UpdateMemory handles PATCH /v1/memories/{id} — an audited admin correction of a
// memory's confidence and/or content.
func (h *AdminHandler) UpdateMemory(w http.ResponseWriter, r *http.Request) {
	tenant := middleware.TenantFromContext(r.Context())
	auth := middleware.AuthFromContext(r.Context())
	if tenant == nil || auth == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid memory id")
		return
	}
	var req updateMemoryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Reason == "" {
		writeError(w, http.StatusBadRequest, "reason is required")
		return
	}
	if req.Confidence == nil && req.Content == nil {
		writeError(w, http.StatusBadRequest, "confidence or content is required")
		return
	}

	var mem *domain.Memory
	if req.Content != nil {
		mem, err = h.svc.UpdateContent(r.Context(), id, tenant.ID, *req.Content, req.Reason, auth.KeyID)
		if err != nil {
			h.writeServiceErr(w, err)
			return
		}
	}
	if req.Confidence != nil {
		mem, err = h.svc.UpdateConfidence(r.Context(), id, tenant.ID, *req.Confidence, req.Reason, auth.KeyID)
		if err != nil {
			h.writeServiceErr(w, err)
			return
		}
	}
	writeJSON(w, http.StatusOK, mem)
}

type reasonRequest struct {
	Reason string `json:"reason"`
}

// Redact handles POST /v1/admin/memories/{id}/redact — GDPR redaction.
func (h *AdminHandler) Redact(w http.ResponseWriter, r *http.Request) {
	tenant := middleware.TenantFromContext(r.Context())
	auth := middleware.AuthFromContext(r.Context())
	if tenant == nil || auth == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid memory id")
		return
	}
	var req reasonRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := h.svc.RedactMemory(r.Context(), id, tenant.ID, req.Reason, auth.KeyID); err != nil {
		h.writeServiceErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type resolveContradictionRequest struct {
	KeepID   string `json:"keep_id"`
	DemoteID string `json:"demote_id"`
	Reason   string `json:"reason"`
}

// ResolveContradiction handles POST /v1/admin/contradictions/resolve.
func (h *AdminHandler) ResolveContradiction(w http.ResponseWriter, r *http.Request) {
	tenant := middleware.TenantFromContext(r.Context())
	auth := middleware.AuthFromContext(r.Context())
	if tenant == nil || auth == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	var req resolveContradictionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	keepID, err := uuid.Parse(req.KeepID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid keep_id")
		return
	}
	demoteID, err := uuid.Parse(req.DemoteID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid demote_id")
		return
	}
	if err := h.svc.ResolveContradiction(r.Context(), tenant.ID, keepID, demoteID, req.Reason, auth.KeyID); err != nil {
		h.writeServiceErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// CryptoShredAnchor handles POST /v1/admin/anchors/{id}/shred — per-subject
// cryptographic erasure.
func (h *AdminHandler) CryptoShredAnchor(w http.ResponseWriter, r *http.Request) {
	tenant := middleware.TenantFromContext(r.Context())
	auth := middleware.AuthFromContext(r.Context())
	if tenant == nil || auth == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	anchorID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid anchor id")
		return
	}
	var req reasonRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	n, err := h.svc.CryptoShredAnchor(r.Context(), anchorID, tenant.ID, req.Reason, auth.KeyID)
	if err != nil {
		h.writeServiceErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"shredded": n})
}

func (h *AdminHandler) writeServiceErr(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, service.ErrReasonRequired):
		writeError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, service.ErrMemoryNotFound):
		writeError(w, http.StatusNotFound, err.Error())
	default:
		writeError(w, http.StatusInternalServerError, "admin operation failed")
	}
}
