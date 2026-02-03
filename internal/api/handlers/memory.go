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

type MemoryHandler struct {
	svc *service.MemoryService
}

func NewMemoryHandler(svc *service.MemoryService) *MemoryHandler {
	return &MemoryHandler{svc: svc}
}

type createMemoryRequest struct {
	AgentID    string         `json:"agent_id"`
	Content    string         `json:"content"`
	Type       string         `json:"type,omitempty"`
	Source     string         `json:"source,omitempty"`
	Confidence float32        `json:"confidence,omitempty"`
	Metadata   map[string]any `json:"metadata,omitempty"`
}

type createMemoryResponse struct {
	*domain.Memory
	Reinforced bool `json:"reinforced"`
}

func (h *MemoryHandler) Create(w http.ResponseWriter, r *http.Request) {
	tenant := middleware.TenantFromContext(r.Context())
	if tenant == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req createMemoryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	agentID, err := uuid.Parse(req.AgentID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid agent_id")
		return
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

	result, err := h.svc.Create(r.Context(), memory)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrMemoryContentEmpty),
			errors.Is(err, service.ErrMemoryAgentIDMissing),
			errors.Is(err, service.ErrInvalidMemoryType):
			writeError(w, http.StatusBadRequest, err.Error())
		case errors.Is(err, service.ErrAgentNotFound):
			writeError(w, http.StatusBadRequest, "agent not found")
		default:
			writeError(w, http.StatusInternalServerError, "failed to create memory")
		}
		return
	}

	resp := createMemoryResponse{
		Memory:     memory,
		Reinforced: result != nil && result.Reinforced,
	}

	writeJSON(w, http.StatusCreated, resp)
}

func (h *MemoryHandler) GetByID(w http.ResponseWriter, r *http.Request) {
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

	memory, err := h.svc.GetByID(r.Context(), id, tenant.ID)
	if err != nil {
		if errors.Is(err, service.ErrMemoryNotFound) {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to get memory")
		return
	}

	writeJSON(w, http.StatusOK, memory)
}

func (h *MemoryHandler) Delete(w http.ResponseWriter, r *http.Request) {
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

	if err := h.svc.Delete(r.Context(), id, tenant.ID); err != nil {
		if errors.Is(err, service.ErrMemoryNotFound) {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to delete memory")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

type memoryWithDecayStatus struct {
	domain.MemoryWithScore
	DecayStatus string `json:"decay_status"`
}

type recallResponse struct {
	Memories []memoryWithDecayStatus `json:"memories"`
	Query    string                  `json:"query"`
	Count    int                     `json:"count"`
}

func calculateDecayStatus(confidence float32) string {
	switch {
	case confidence >= 0.7:
		return "healthy"
	case confidence >= 0.4:
		return "decaying"
	default:
		return "at_risk"
	}
}

func (h *MemoryHandler) Recall(w http.ResponseWriter, r *http.Request) {
	tenant := middleware.TenantFromContext(r.Context())
	if tenant == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	query := r.URL.Query().Get("query")
	if query == "" {
		writeError(w, http.StatusBadRequest, "query parameter is required")
		return
	}

	agentIDStr := r.URL.Query().Get("agent_id")
	agentID, err := uuid.Parse(agentIDStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid or missing agent_id parameter")
		return
	}

	opts := domain.RecallOpts{
		TopK: 10,
	}

	if topKStr := r.URL.Query().Get("top_k"); topKStr != "" {
		if topK, err := strconv.Atoi(topKStr); err == nil && topK > 0 {
			opts.TopK = topK
		}
	}

	if typeStr := r.URL.Query().Get("type"); typeStr != "" {
		mt := domain.MemoryType(typeStr)
		if !domain.ValidMemoryType(typeStr) {
			writeError(w, http.StatusBadRequest, "invalid type parameter")
			return
		}
		opts.MemoryType = &mt
	}

	if minConfStr := r.URL.Query().Get("min_confidence"); minConfStr != "" {
		if mc, err := strconv.ParseFloat(minConfStr, 32); err == nil {
			opts.MinConfidence = float32(mc)
		}
	}

	memories, err := h.svc.Recall(r.Context(), query, agentID, tenant.ID, opts)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrRecallQueryEmpty),
			errors.Is(err, service.ErrRecallAgentIDMissing):
			writeError(w, http.StatusBadRequest, err.Error())
		default:
			writeError(w, http.StatusInternalServerError, "failed to recall memories")
		}
		return
	}

	var memoriesWithStatus []memoryWithDecayStatus
	for _, mem := range memories {
		memoriesWithStatus = append(memoriesWithStatus, memoryWithDecayStatus{
			MemoryWithScore: mem,
			DecayStatus:     calculateDecayStatus(mem.Confidence),
		})
	}

	if memoriesWithStatus == nil {
		memoriesWithStatus = []memoryWithDecayStatus{}
	}

	writeJSON(w, http.StatusOK, recallResponse{
		Memories: memoriesWithStatus,
		Query:    query,
		Count:    len(memoriesWithStatus),
	})
}

type extractRequest struct {
	AgentID      string           `json:"agent_id"`
	Conversation []domain.Message `json:"conversation"`
	AutoStore    bool             `json:"auto_store"`
}

type extractResponse struct {
	Extracted []service.ExtractResult `json:"extracted"`
	Count     int                     `json:"count"`
}

func (h *MemoryHandler) Extract(w http.ResponseWriter, r *http.Request) {
	tenant := middleware.TenantFromContext(r.Context())
	if tenant == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req extractRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	agentID, err := uuid.Parse(req.AgentID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid agent_id")
		return
	}

	if len(req.Conversation) == 0 {
		writeError(w, http.StatusBadRequest, "conversation is required")
		return
	}

	results, err := h.svc.Extract(r.Context(), agentID, tenant.ID, req.Conversation, req.AutoStore)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to extract memories")
		return
	}

	if results == nil {
		results = []service.ExtractResult{}
	}

	writeJSON(w, http.StatusOK, extractResponse{
		Extracted: results,
		Count:     len(results),
	})
}
