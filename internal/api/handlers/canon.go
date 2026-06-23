package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/Harshitk-cp/engram/internal/api/middleware"
	"github.com/Harshitk-cp/engram/internal/domain"
	"github.com/Harshitk-cp/engram/internal/service"
	"github.com/Harshitk-cp/engram/internal/store"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

type CanonHandler struct {
	svc      *service.MemoryService
	memories *store.MemoryStore
}

func NewCanonHandler(svc *service.MemoryService, memories *store.MemoryStore) *CanonHandler {
	return &CanonHandler{svc: svc, memories: memories}
}

type createCanonRequest struct {
	AgentID    string         `json:"agent_id"`
	Content    string         `json:"content"`
	Type       string         `json:"type,omitempty"`
	Source     string         `json:"source,omitempty"`
	Confidence float32        `json:"confidence,omitempty"`
	Metadata   map[string]any `json:"metadata,omitempty"`
	EventDate  string         `json:"event_date,omitempty"`
}

func (h *CanonHandler) Create(w http.ResponseWriter, r *http.Request) {
	tenant := middleware.TenantFromContext(r.Context())
	if tenant == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	var req createCanonRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	agentID, err := uuid.Parse(req.AgentID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid or missing agent_id")
		return
	}

	if req.Content != "" {
		if existing, ferr := h.memories.FindCanonByContent(r.Context(), tenant.ID, req.Content); ferr == nil && existing != nil {
			tier := domain.ComputeTier(float64(existing.Confidence))
			writeJSON(w, http.StatusOK, createMemoryResponse{
				Memory:     existing,
				Tier:       tier,
				TierReason: domain.TierReason(float64(existing.Confidence)),
			})
			return
		}
	}

	memory := &domain.Memory{
		AgentID:    agentID,
		TenantID:   tenant.ID,
		Type:       domain.MemoryType(req.Type),
		Content:    req.Content,
		Source:     req.Source,
		Confidence: req.Confidence,
		Metadata:   req.Metadata,
	}
	if req.EventDate != "" {
		for _, layout := range []string{time.RFC3339, "2006-01-02T15:04:05", "2006-01-02"} {
			if t, err := time.Parse(layout, req.EventDate); err == nil {
				memory.EventDate = &t
				break
			}
		}
	}

	if _, err := h.svc.CreateCanon(r.Context(), memory); err != nil {
		switch {
		case errors.Is(err, service.ErrMemoryContentEmpty),
			errors.Is(err, service.ErrInvalidMemoryType):
			writeError(w, http.StatusBadRequest, err.Error())
		case errors.Is(err, service.ErrAgentNotFound):
			writeError(w, http.StatusBadRequest, "agent not found")
		default:
			writeError(w, http.StatusInternalServerError, "failed to create canon memory")
		}
		return
	}

	tier := domain.ComputeTier(float64(memory.Confidence))
	writeJSON(w, http.StatusCreated, createMemoryResponse{
		Memory:     memory,
		Tier:       tier,
		TierReason: domain.TierReason(float64(memory.Confidence)),
	})
}

func (h *CanonHandler) List(w http.ResponseWriter, r *http.Request) {
	tenant := middleware.TenantFromContext(r.Context())
	if tenant == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	limit := 100
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 {
			limit = clampLimit(n)
		}
	}
	memories, err := h.memories.ListCanon(r.Context(), tenant.ID, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list canon")
		return
	}
	domain.AnnotateTiers(memories)
	writeJSON(w, http.StatusOK, map[string]any{"memories": memories, "count": len(memories)})
}

func (h *CanonHandler) Delete(w http.ResponseWriter, r *http.Request) {
	tenant := middleware.TenantFromContext(r.Context())
	if tenant == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid memory id")
		return
	}
	if err := h.memories.Delete(r.Context(), id, tenant.ID); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "canon memory not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to delete canon memory")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"deleted": true})
}
