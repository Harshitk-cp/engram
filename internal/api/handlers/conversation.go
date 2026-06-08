package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/Harshitk-cp/engram/internal/api/middleware"
	"github.com/Harshitk-cp/engram/internal/domain"
	"github.com/Harshitk-cp/engram/internal/service"
	"github.com/Harshitk-cp/engram/internal/store"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

type ConversationHandler struct {
	svc      *service.ConversationService
	anchors  *store.EntityStore
	sessions *store.SessionStore
}

func NewConversationHandler(svc *service.ConversationService, anchors *store.EntityStore, sessions *store.SessionStore) *ConversationHandler {
	return &ConversationHandler{svc: svc, anchors: anchors, sessions: sessions}
}

type ingestRequest struct {
	Messages []domain.Message `json:"messages"`
	EventDate string `json:"event_date,omitempty"`
	Metadata map[string]any `json:"metadata,omitempty"`
	Sync bool `json:"sync"`
	// Bind extracted traces to a subject and/or a conversation.
	AnchorID         string `json:"anchor_id,omitempty"`
	AnchorExternalID string `json:"anchor_external_id,omitempty"`
	SessionID        string `json:"session_id,omitempty"`
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

	anchorID, sessionID, err := h.resolveBindings(r, tenant.ID, req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	domainReq := &domain.ConversationIngestRequest{
		AgentID:   agentID,
		TenantID:  tenant.ID,
		Messages:  req.Messages,
		EventDate: eventDate,
		Metadata:  req.Metadata,
		Sync:      req.Sync,
		AnchorID:  anchorID,
		SessionID: sessionID,
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

func (h *ConversationHandler) resolveBindings(r *http.Request, tenantID uuid.UUID, req ingestRequest) (anchorID, sessionID *uuid.UUID, err error) {
	if req.SessionID != "" {
		sid, perr := uuid.Parse(req.SessionID)
		if perr != nil {
			return nil, nil, errors.New("invalid session_id")
		}
		sess, gerr := h.sessions.GetByID(r.Context(), sid, tenantID)
		if gerr != nil {
			return nil, nil, errors.New("session not found")
		}
		sessionID = &sess.ID
		anchorID = sess.AnchorID
	}
	switch {
	case req.AnchorID != "":
		id, perr := uuid.Parse(req.AnchorID)
		if perr != nil {
			return nil, nil, errors.New("invalid anchor_id")
		}
		if _, gerr := h.anchors.GetAnchor(r.Context(), id, tenantID); gerr != nil {
			return nil, nil, errors.New("anchor not found")
		}
		anchorID = &id
	case req.AnchorExternalID != "":
		a, ferr := h.anchors.FindAnchorByExternalID(r.Context(), tenantID, req.AnchorExternalID)
		if ferr == nil {
			anchorID = &a.ID
			break
		}
		anchor := &domain.Entity{TenantID: tenantID, Name: req.AnchorExternalID, ExternalID: req.AnchorExternalID}
		if cerr := h.anchors.CreateAnchor(r.Context(), anchor, nil); cerr != nil {
			return nil, nil, errors.New("failed to resolve or create anchor")
		}
		anchorID = &anchor.ID
	}
	return anchorID, sessionID, nil
}
