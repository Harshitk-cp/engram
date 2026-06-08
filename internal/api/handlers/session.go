package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/Harshitk-cp/engram/internal/api/middleware"
	"github.com/Harshitk-cp/engram/internal/domain"
	"github.com/Harshitk-cp/engram/internal/store"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

type SessionHandler struct {
	sessions *store.SessionStore
	anchors  *store.EntityStore
	ttl      time.Duration
}

func NewSessionHandler(sessions *store.SessionStore, anchors *store.EntityStore, ttl time.Duration) *SessionHandler {
	if ttl <= 0 {
		ttl = 30 * 24 * time.Hour // default: session memory lingers ~30 days after end
	}
	return &SessionHandler{sessions: sessions, anchors: anchors, ttl: ttl}
}

type createSessionRequest struct {
	AgentID          string         `json:"agent_id"`
	AnchorID         string         `json:"anchor_id,omitempty"`
	AnchorExternalID string         `json:"anchor_external_id,omitempty"`
	ExternalID       string         `json:"external_id,omitempty"`
	Metadata         map[string]any `json:"metadata,omitempty"`
}

func (h *SessionHandler) Create(w http.ResponseWriter, r *http.Request) {
	tenant := middleware.TenantFromContext(r.Context())
	if tenant == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	var req createSessionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	agentID, err := uuid.Parse(req.AgentID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid or missing agent_id")
		return
	}

	anchorID, err := h.resolveAnchor(r, tenant.ID, req.AnchorID, req.AnchorExternalID)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	sess := &domain.Session{
		TenantID:   tenant.ID,
		AgentID:    agentID,
		AnchorID:   anchorID,
		ExternalID: req.ExternalID,
		Metadata:   req.Metadata,
	}
	if err := h.sessions.Create(r.Context(), sess); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create session")
		return
	}
	writeJSON(w, http.StatusCreated, sess)
}

func (h *SessionHandler) GetByID(w http.ResponseWriter, r *http.Request) {
	tenant := middleware.TenantFromContext(r.Context())
	if tenant == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid session id")
		return
	}
	sess, err := h.sessions.GetByID(r.Context(), id, tenant.ID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "session not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to get session")
		return
	}
	writeJSON(w, http.StatusOK, sess)
}

// End closes a session and sets when its memory should be swept.
func (h *SessionHandler) End(w http.ResponseWriter, r *http.Request) {
	tenant := middleware.TenantFromContext(r.Context())
	if tenant == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid session id")
		return
	}
	expiresAt := time.Now().Add(h.ttl)
	if err := h.sessions.End(r.Context(), id, tenant.ID, expiresAt); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "session not found or already ended")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to end session")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ended": true, "expires_at": expiresAt})
}

func (h *SessionHandler) resolveAnchor(r *http.Request, tenantID uuid.UUID, anchorID, externalID string) (*uuid.UUID, error) {
	switch {
	case anchorID != "":
		id, err := uuid.Parse(anchorID)
		if err != nil {
			return nil, errors.New("invalid anchor_id")
		}
		if _, err := h.anchors.GetAnchor(r.Context(), id, tenantID); err != nil {
			return nil, errors.New("anchor not found")
		}
		return &id, nil
	case externalID != "":
		a, err := h.anchors.FindAnchorByExternalID(r.Context(), tenantID, externalID)
		if err != nil {
			return nil, errors.New("anchor not found for anchor_external_id")
		}
		return &a.ID, nil
	default:
		return nil, nil
	}
}
