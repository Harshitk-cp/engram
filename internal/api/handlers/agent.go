package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/Harshitk-cp/engram/internal/api/middleware"
	"github.com/Harshitk-cp/engram/internal/domain"
	"github.com/Harshitk-cp/engram/internal/service"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

type AgentHandler struct {
	svc *service.AgentService
}

func NewAgentHandler(svc *service.AgentService) *AgentHandler {
	return &AgentHandler{svc: svc}
}

type createAgentRequest struct {
	ExternalID string         `json:"external_id"`
	Name       string         `json:"name"`
	Metadata   map[string]any `json:"metadata"`
}

func (h *AgentHandler) Create(w http.ResponseWriter, r *http.Request) {
	tenant := middleware.TenantFromContext(r.Context())
	if tenant == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req createAgentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.ExternalID == "" {
		writeError(w, http.StatusBadRequest, "external_id is required")
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}

	agent := &domain.Agent{
		TenantID:   tenant.ID,
		ExternalID: req.ExternalID,
		Name:       req.Name,
		Metadata:   req.Metadata,
	}

	if err := h.svc.Create(r.Context(), agent); err != nil {
		if errors.Is(err, service.ErrAgentConflict) {
			writeError(w, http.StatusConflict, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to create agent")
		return
	}

	writeJSON(w, http.StatusCreated, agent)
}

func (h *AgentHandler) List(w http.ResponseWriter, r *http.Request) {
	tenant := middleware.TenantFromContext(r.Context())
	if tenant == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	limit := 50
	offset := 0
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 200 {
			limit = n
		}
	}
	if v := r.URL.Query().Get("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			offset = n
		}
	}

	agents, err := h.svc.List(r.Context(), tenant.ID, limit, offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list agents")
		return
	}
	if agents == nil {
		agents = []domain.Agent{}
	}
	total, err := h.svc.Count(r.Context(), tenant.ID)
	if err != nil {
		total = len(agents)
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"agents": agents,
		"total":  total,
		"count":  len(agents),
		"limit":  limit,
		"offset": offset,
	})
}

func (h *AgentHandler) Delete(w http.ResponseWriter, r *http.Request) {
	tenant := middleware.TenantFromContext(r.Context())
	if tenant == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	idStr := chi.URLParam(r, "id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid agent id")
		return
	}

	if err := h.svc.Delete(r.Context(), id, tenant.ID); err != nil {
		if errors.Is(err, service.ErrAgentNotFound) {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to delete agent")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *AgentHandler) GetByID(w http.ResponseWriter, r *http.Request) {
	tenant := middleware.TenantFromContext(r.Context())
	if tenant == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	idStr := chi.URLParam(r, "id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid agent id")
		return
	}

	agent, err := h.svc.GetByID(r.Context(), id, tenant.ID)
	if err != nil {
		if errors.Is(err, service.ErrAgentNotFound) {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to get agent")
		return
	}

	writeJSON(w, http.StatusOK, agent)
}
