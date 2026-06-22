package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/Harshitk-cp/engram/internal/api/middleware"
	"github.com/Harshitk-cp/engram/internal/domain"
	"github.com/Harshitk-cp/engram/internal/store"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// AnchorHandler exposes anchors — the tenant-scoped referents that memory traces
type AnchorHandler struct {
	anchors  *store.EntityStore
	memories *store.MemoryStore
}

func NewAnchorHandler(anchors *store.EntityStore, memories *store.MemoryStore) *AnchorHandler {
	return &AnchorHandler{anchors: anchors, memories: memories}
}

type createAnchorRequest struct {
	Name       string         `json:"name"`
	EntityType string         `json:"entity_type,omitempty"`
	ExternalID string         `json:"external_id,omitempty"`
	Aliases    []string       `json:"aliases,omitempty"`
	Metadata   map[string]any `json:"metadata,omitempty"`
	AgentID    string         `json:"agent_id,omitempty"`
}

func (h *AnchorHandler) Create(w http.ResponseWriter, r *http.Request) {
	tenant := middleware.TenantFromContext(r.Context())
	if tenant == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req createAnchorRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	if req.EntityType != "" && !domain.ValidEntityType(req.EntityType) {
		writeError(w, http.StatusBadRequest, "invalid entity_type")
		return
	}

	var agentID *uuid.UUID
	if req.AgentID != "" {
		id, err := uuid.Parse(req.AgentID)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid agent_id")
			return
		}
		agentID = &id
	}

	anchor := &domain.Entity{
		TenantID:   tenant.ID,
		Name:       req.Name,
		EntityType: domain.EntityType(req.EntityType),
		Aliases:    req.Aliases,
		Metadata:   req.Metadata,
		ExternalID: req.ExternalID,
	}
	if err := h.anchors.CreateAnchor(r.Context(), anchor, agentID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create anchor")
		return
	}
	writeJSON(w, http.StatusCreated, anchor)
}

func (h *AnchorHandler) List(w http.ResponseWriter, r *http.Request) {
	tenant := middleware.TenantFromContext(r.Context())
	if tenant == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var entityType domain.EntityType
	if t := r.URL.Query().Get("entity_type"); t != "" {
		if !domain.ValidEntityType(t) {
			writeError(w, http.StatusBadRequest, "invalid entity_type")
			return
		}
		entityType = domain.EntityType(t)
	}

	limit := 100
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 {
			limit = clampLimit(n)
		}
	}

	anchors, err := h.anchors.ListAnchors(r.Context(), tenant.ID, entityType, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list anchors")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"anchors": anchors, "count": len(anchors)})
}

func (h *AnchorHandler) GetByID(w http.ResponseWriter, r *http.Request) {
	tenant := middleware.TenantFromContext(r.Context())
	if tenant == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid anchor id")
		return
	}
	anchor, err := h.anchors.GetAnchor(r.Context(), id, tenant.ID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "anchor not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to get anchor")
		return
	}
	writeJSON(w, http.StatusOK, anchor)
}

// ListMemories returns an anchor's durable profile — the traces bound to it.
func (h *AnchorHandler) ListMemories(w http.ResponseWriter, r *http.Request) {
	tenant := middleware.TenantFromContext(r.Context())
	if tenant == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid anchor id")
		return
	}
	limit := 100
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 {
			limit = clampLimit(n)
		}
	}
	memories, err := h.memories.ListByAnchor(r.Context(), id, tenant.ID, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list anchor memories")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"memories": memories, "count": len(memories)})
}

// Delete unlinks (default) or purges (?purge=true) an anchor. Purge is the
// GDPR/HIPAA erasure path: it hard-deletes the anchor's traces, then the anchor.
func (h *AnchorHandler) Delete(w http.ResponseWriter, r *http.Request) {
	tenant := middleware.TenantFromContext(r.Context())
	if tenant == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid anchor id")
		return
	}
	// Confirm the anchor belongs to this tenant before touching anything.
	if _, err := h.anchors.GetAnchor(r.Context(), id, tenant.ID); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "anchor not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to load anchor")
		return
	}

	purge := r.URL.Query().Get("purge") == "true"
	var purged int64
	if purge {
		purged, err = h.memories.PurgeByAnchor(r.Context(), id, tenant.ID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to purge anchor memories")
			return
		}
	}

	if err := h.anchors.Delete(r.Context(), id); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete anchor")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"deleted": true, "purged": purge, "memories_purged": purged})
}
