package handlers

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/Harshitk-cp/engram/internal/api/middleware"
	"github.com/Harshitk-cp/engram/internal/domain"
	"github.com/Harshitk-cp/engram/internal/service"
	"github.com/google/uuid"
)

type FeedbackHandler struct {
	svc *service.FeedbackService
}

func NewFeedbackHandler(svc *service.FeedbackService) *FeedbackHandler {
	return &FeedbackHandler{svc: svc}
}

type createFeedbackRequest struct {
	MemoryID   string         `json:"memory_id"`
	AgentID    string         `json:"agent_id"`
	SignalType string         `json:"signal_type"`
	Context    map[string]any `json:"context,omitempty"`
}

func (h *FeedbackHandler) Create(w http.ResponseWriter, r *http.Request) {
	tenant := middleware.TenantFromContext(r.Context())
	if tenant == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req createFeedbackRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	memoryID, err := uuid.Parse(req.MemoryID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid memory_id")
		return
	}

	agentID, err := uuid.Parse(req.AgentID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid agent_id")
		return
	}

	feedback := &domain.Feedback{
		MemoryID:   memoryID,
		AgentID:    agentID,
		SignalType: domain.FeedbackType(req.SignalType),
		Context:    req.Context,
	}

	if err := h.svc.Create(r.Context(), feedback, tenant.ID); err != nil {
		switch {
		case errors.Is(err, service.ErrFeedbackMemoryIDMissing),
			errors.Is(err, service.ErrFeedbackAgentIDMissing),
			errors.Is(err, service.ErrFeedbackInvalidSignal):
			writeError(w, http.StatusBadRequest, err.Error())
		case errors.Is(err, service.ErrAgentNotFound):
			writeError(w, http.StatusBadRequest, "agent not found")
		case errors.Is(err, service.ErrMemoryNotFound):
			writeError(w, http.StatusBadRequest, "memory not found")
		default:
			writeError(w, http.StatusInternalServerError, "failed to create feedback")
		}
		return
	}

	writeJSON(w, http.StatusCreated, feedback)
}
