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
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

type EpisodeHandler struct {
	svc *service.EpisodeService
}

func NewEpisodeHandler(svc *service.EpisodeService) *EpisodeHandler {
	return &EpisodeHandler{svc: svc}
}

type createEpisodeRequest struct {
	AgentID        string `json:"agent_id"`
	RawContent     string `json:"raw_content"`
	ConversationID string `json:"conversation_id,omitempty"`
	OccurredAt     string `json:"occurred_at,omitempty"` // RFC3339 format
	Outcome        string `json:"outcome,omitempty"`     // success, failure, neutral, unknown
}

func (h *EpisodeHandler) Create(w http.ResponseWriter, r *http.Request) {
	tenant := middleware.TenantFromContext(r.Context())
	if tenant == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req createEpisodeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	agentID, err := uuid.Parse(req.AgentID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid agent_id")
		return
	}

	input := service.EncodeInput{
		AgentID:    agentID,
		TenantID:   tenant.ID,
		RawContent: req.RawContent,
	}

	// Parse optional conversation ID
	if req.ConversationID != "" {
		convID, err := uuid.Parse(req.ConversationID)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid conversation_id")
			return
		}
		input.ConversationID = &convID
	}

	// Parse optional occurred_at
	if req.OccurredAt != "" {
		t, err := time.Parse(time.RFC3339, req.OccurredAt)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid occurred_at format (use RFC3339)")
			return
		}
		input.OccurredAt = t
	}

	// Parse optional outcome
	if req.Outcome != "" {
		if !domain.ValidOutcomeType(req.Outcome) {
			writeError(w, http.StatusBadRequest, "invalid outcome type")
			return
		}
		outcome := domain.OutcomeType(req.Outcome)
		input.Outcome = &outcome
	}

	episode, err := h.svc.Encode(r.Context(), input)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrEpisodeContentEmpty),
			errors.Is(err, service.ErrEpisodeAgentIDMissing):
			writeError(w, http.StatusBadRequest, err.Error())
		case errors.Is(err, service.ErrAgentNotFound):
			writeError(w, http.StatusBadRequest, "agent not found")
		default:
			writeError(w, http.StatusInternalServerError, "failed to create episode")
		}
		return
	}

	writeJSON(w, http.StatusCreated, episode)
}

func (h *EpisodeHandler) GetByID(w http.ResponseWriter, r *http.Request) {
	tenant := middleware.TenantFromContext(r.Context())
	if tenant == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid episode id")
		return
	}

	episode, err := h.svc.GetByID(r.Context(), id, tenant.ID)
	if err != nil {
		if errors.Is(err, service.ErrEpisodeNotFound) {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to get episode")
		return
	}

	writeJSON(w, http.StatusOK, episode)
}

type recallEpisodesResponse struct {
	Episodes []domain.EpisodeWithScore `json:"episodes"`
	Count    int                       `json:"count"`
}

func (h *EpisodeHandler) Recall(w http.ResponseWriter, r *http.Request) {
	tenant := middleware.TenantFromContext(r.Context())
	if tenant == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	agentIDStr := r.URL.Query().Get("agent_id")
	if agentIDStr == "" {
		writeError(w, http.StatusBadRequest, "agent_id is required")
		return
	}

	agentID, err := uuid.Parse(agentIDStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid agent_id")
		return
	}

	opts := service.EpisodeRecallOpts{
		Query: r.URL.Query().Get("query"),
	}

	// Parse optional limit
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		limit, err := strconv.Atoi(limitStr)
		if err != nil || limit <= 0 {
			writeError(w, http.StatusBadRequest, "invalid limit")
			return
		}
		opts.Limit = limit
	}

	// Parse optional min_importance
	if minImpStr := r.URL.Query().Get("min_importance"); minImpStr != "" {
		minImp, err := strconv.ParseFloat(minImpStr, 32)
		if err != nil || minImp < 0 || minImp > 1 {
			writeError(w, http.StatusBadRequest, "invalid min_importance (0-1)")
			return
		}
		opts.MinImportance = float32(minImp)
	}

	// Parse optional time range
	if startStr := r.URL.Query().Get("start_time"); startStr != "" {
		start, err := time.Parse(time.RFC3339, startStr)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid start_time format (use RFC3339)")
			return
		}
		opts.StartTime = &start
	}

	if endStr := r.URL.Query().Get("end_time"); endStr != "" {
		end, err := time.Parse(time.RFC3339, endStr)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid end_time format (use RFC3339)")
			return
		}
		opts.EndTime = &end
	}

	episodes, err := h.svc.Recall(r.Context(), agentID, tenant.ID, opts)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrAgentNotFound):
			writeError(w, http.StatusBadRequest, "agent not found")
		default:
			writeError(w, http.StatusBadRequest, err.Error())
		}
		return
	}

	resp := recallEpisodesResponse{
		Episodes: episodes,
		Count:    len(episodes),
	}

	writeJSON(w, http.StatusOK, resp)
}

type recordOutcomeRequest struct {
	Outcome     string `json:"outcome"`
	Description string `json:"description,omitempty"`
}

func (h *EpisodeHandler) RecordOutcome(w http.ResponseWriter, r *http.Request) {
	tenant := middleware.TenantFromContext(r.Context())
	if tenant == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid episode id")
		return
	}

	var req recordOutcomeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Outcome == "" {
		writeError(w, http.StatusBadRequest, "outcome is required")
		return
	}

	if !domain.ValidOutcomeType(req.Outcome) {
		writeError(w, http.StatusBadRequest, "invalid outcome type (success, failure, neutral, unknown)")
		return
	}

	err = h.svc.RecordOutcome(r.Context(), id, tenant.ID, domain.OutcomeType(req.Outcome), req.Description)
	if err != nil {
		if errors.Is(err, service.ErrEpisodeNotFound) {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		if errors.Is(err, service.ErrInvalidOutcomeType) {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to record outcome")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *EpisodeHandler) GetAssociations(w http.ResponseWriter, r *http.Request) {
	tenant := middleware.TenantFromContext(r.Context())
	if tenant == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid episode id")
		return
	}

	// Verify episode exists and belongs to tenant
	_, err = h.svc.GetByID(r.Context(), id, tenant.ID)
	if err != nil {
		if errors.Is(err, service.ErrEpisodeNotFound) {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to get episode")
		return
	}

	associations, err := h.svc.GetAssociations(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get associations")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"associations": associations,
		"count":        len(associations),
	})
}
