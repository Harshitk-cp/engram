package handlers

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/Harshitk-cp/engram/internal/api/middleware"
	"github.com/Harshitk-cp/engram/internal/domain"
	"github.com/Harshitk-cp/engram/internal/service"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

type ConversationHandler struct {
	svc *service.ConversationService
}

func NewConversationHandler(svc *service.ConversationService) *ConversationHandler {
	return &ConversationHandler{svc: svc}
}

type ingestRequest struct {
	Messages []domain.Message `json:"messages"`
	EventDate string `json:"event_date,omitempty"`
	Metadata map[string]any `json:"metadata,omitempty"`
	Sync bool `json:"sync"`
}

type ingestResponse struct {
	Stored     int    `json:"stored"`
	Skipped    int    `json:"skipped"`
	DurationMs int64  `json:"duration_ms"`
}

// Ingest handles POST /v1/agents/{id}/conversations/ingest.
func (h *ConversationHandler) Ingest(w http.ResponseWriter, r *http.Request) {
	tenant := middleware.TenantFromContext(r.Context())
	if tenant == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	agentIDStr := chi.URLParam(r, "id")
	agentID, err := uuid.Parse(agentIDStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid agent id")
		return
	}

	var req ingestRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}

	if len(req.Messages) == 0 {
		writeError(w, http.StatusBadRequest, "messages is required and must not be empty")
		return
	}

	for i, msg := range req.Messages {
		if msg.Role != "user" && msg.Role != "assistant" && msg.Role != "system" {
			writeError(w, http.StatusBadRequest, "message["+string(rune('0'+i))+"]: role must be 'user', 'assistant', or 'system'")
			return
		}
		if msg.Content == "" {
			writeError(w, http.StatusBadRequest, "message content must not be empty")
			return
		}
	}

	var eventDate *time.Time
	if req.EventDate != "" {
		formats := []string{
			time.RFC3339,
			"2006-01-02T15:04:05",
			"2006-01-02",
		}
		for _, fmt := range formats {
			if t, err := time.Parse(fmt, req.EventDate); err == nil {
				eventDate = &t
				break
			}
		}
		if eventDate == nil {
			writeError(w, http.StatusBadRequest, "event_date must be ISO 8601 (e.g. '2023-03-15' or '2023-03-15T14:30:00Z')")
			return
		}
	}

	domainReq := &domain.ConversationIngestRequest{
		AgentID:   agentID,
		TenantID:  tenant.ID,
		Messages:  req.Messages,
		EventDate: eventDate,
		Metadata:  req.Metadata,
		Sync:      req.Sync,
	}

	result, err := h.svc.Ingest(r.Context(), domainReq)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "ingest failed: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, ingestResponse{
		Stored:     len(result.Stored),
		Skipped:    result.Skipped,
		DurationMs: result.Duration,
	})
}
