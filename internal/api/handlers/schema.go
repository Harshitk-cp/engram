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

type SchemaHandler struct {
	svc *service.SchemaService
}

func NewSchemaHandler(svc *service.SchemaService) *SchemaHandler {
	return &SchemaHandler{svc: svc}
}

type matchSchemasRequest struct {
	AgentID       string   `json:"agent_id"`
	Query         string   `json:"query,omitempty"`
	Contexts      []string `json:"contexts,omitempty"`
	TimeOfDay     string   `json:"time_of_day,omitempty"`
	MinMatchScore float32  `json:"min_match_score,omitempty"`
	Limit         int      `json:"limit,omitempty"`
}

type matchSchemasResponse struct {
	Schemas []schemaMatchResponse `json:"schemas"`
	Count   int                   `json:"count"`
}

type schemaMatchResponse struct {
	Schema      schemaResponse `json:"schema"`
	MatchScore  float32        `json:"match_score"`
	MatchReason string         `json:"match_reason,omitempty"`
}

type schemaResponse struct {
	ID                 string         `json:"id"`
	AgentID            string         `json:"agent_id"`
	SchemaType         string         `json:"schema_type"`
	Name               string         `json:"name"`
	Description        string         `json:"description,omitempty"`
	Attributes         map[string]any `json:"attributes"`
	EvidenceCount      int            `json:"evidence_count"`
	Confidence         float32        `json:"confidence"`
	ContradictionCount int            `json:"contradiction_count"`
	ApplicableContexts []string       `json:"applicable_contexts,omitempty"`
	CreatedAt          string         `json:"created_at"`
	UpdatedAt          string         `json:"updated_at"`
}

type listSchemasResponse struct {
	Schemas []schemaResponse `json:"schemas"`
	Count   int              `json:"count"`
}

type detectSchemasRequest struct {
	AgentID string `json:"agent_id"`
}

type detectSchemasResponse struct {
	DetectedSchemas []schemaResponse `json:"detected_schemas"`
	Count           int              `json:"count"`
}

// Match finds schemas that apply to the current situation.
// POST /v1/schemas/match
func (h *SchemaHandler) Match(w http.ResponseWriter, r *http.Request) {
	tenant := middleware.TenantFromContext(r.Context())
	if tenant == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req matchSchemasRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	agentID, err := uuid.Parse(req.AgentID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid agent_id")
		return
	}

	input := service.SchemaMatchInput{
		AgentID:       agentID,
		TenantID:      tenant.ID,
		Query:         req.Query,
		Contexts:      req.Contexts,
		TimeOfDay:     req.TimeOfDay,
		MinMatchScore: req.MinMatchScore,
		Limit:         req.Limit,
	}

	matches, err := h.svc.MatchSchemas(r.Context(), input)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to match schemas")
		return
	}

	response := matchSchemasResponse{
		Schemas: make([]schemaMatchResponse, len(matches)),
		Count:   len(matches),
	}

	for i, m := range matches {
		response.Schemas[i] = schemaMatchResponse{
			Schema:      toSchemaResponse(&m.Schema),
			MatchScore:  m.MatchScore,
			MatchReason: m.MatchReason,
		}
	}

	writeJSON(w, http.StatusOK, response)
}

// List retrieves all schemas for an agent.
// GET /v1/schemas?agent_id=...
func (h *SchemaHandler) List(w http.ResponseWriter, r *http.Request) {
	tenant := middleware.TenantFromContext(r.Context())
	if tenant == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	agentIDStr := r.URL.Query().Get("agent_id")
	if agentIDStr == "" {
		writeError(w, http.StatusBadRequest, "agent_id query parameter is required")
		return
	}

	agentID, err := uuid.Parse(agentIDStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid agent_id")
		return
	}

	schemas, err := h.svc.GetByAgent(r.Context(), agentID, tenant.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list schemas")
		return
	}

	response := listSchemasResponse{
		Schemas: make([]schemaResponse, len(schemas)),
		Count:   len(schemas),
	}

	for i, s := range schemas {
		response.Schemas[i] = toSchemaResponse(&s)
	}

	writeJSON(w, http.StatusOK, response)
}

// GetByID retrieves a specific schema.
// GET /v1/schemas/:id
func (h *SchemaHandler) GetByID(w http.ResponseWriter, r *http.Request) {
	tenant := middleware.TenantFromContext(r.Context())
	if tenant == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	idStr := chi.URLParam(r, "id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid schema id")
		return
	}

	schema, err := h.svc.GetByID(r.Context(), id, tenant.ID)
	if err != nil {
		if errors.Is(err, service.ErrSchemaNotFound) {
			writeError(w, http.StatusNotFound, "schema not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to get schema")
		return
	}

	writeJSON(w, http.StatusOK, toSchemaResponse(schema))
}

// Delete removes a schema.
// DELETE /v1/schemas/:id
func (h *SchemaHandler) Delete(w http.ResponseWriter, r *http.Request) {
	tenant := middleware.TenantFromContext(r.Context())
	if tenant == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	idStr := chi.URLParam(r, "id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid schema id")
		return
	}

	if err := h.svc.Delete(r.Context(), id, tenant.ID); err != nil {
		if errors.Is(err, service.ErrSchemaNotFound) {
			writeError(w, http.StatusNotFound, "schema not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to delete schema")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// Detect triggers schema detection for an agent.
// POST /v1/schemas/detect
func (h *SchemaHandler) Detect(w http.ResponseWriter, r *http.Request) {
	tenant := middleware.TenantFromContext(r.Context())
	if tenant == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req detectSchemasRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	agentID, err := uuid.Parse(req.AgentID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid agent_id")
		return
	}

	schemas, err := h.svc.DetectSchemas(r.Context(), agentID, tenant.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to detect schemas")
		return
	}

	response := detectSchemasResponse{
		DetectedSchemas: make([]schemaResponse, len(schemas)),
		Count:           len(schemas),
	}

	for i, s := range schemas {
		response.DetectedSchemas[i] = toSchemaResponse(&s)
	}

	writeJSON(w, http.StatusOK, response)
}

// Contradict records a contradiction for a schema.
// POST /v1/schemas/:id/contradict
func (h *SchemaHandler) Contradict(w http.ResponseWriter, r *http.Request) {
	tenant := middleware.TenantFromContext(r.Context())
	if tenant == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	idStr := chi.URLParam(r, "id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid schema id")
		return
	}

	if err := h.svc.RecordContradiction(r.Context(), id, tenant.ID); err != nil {
		if errors.Is(err, service.ErrSchemaNotFound) {
			writeError(w, http.StatusNotFound, "schema not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to record contradiction")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "recorded"})
}

// Validate validates a schema.
// POST /v1/schemas/:id/validate
func (h *SchemaHandler) Validate(w http.ResponseWriter, r *http.Request) {
	tenant := middleware.TenantFromContext(r.Context())
	if tenant == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	idStr := chi.URLParam(r, "id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid schema id")
		return
	}

	if err := h.svc.ValidateSchema(r.Context(), id, tenant.ID); err != nil {
		if errors.Is(err, service.ErrSchemaNotFound) {
			writeError(w, http.StatusNotFound, "schema not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to validate schema")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "validated"})
}

// Helper to convert domain.Schema to API response.
func toSchemaResponse(s *domain.Schema) schemaResponse {
	resp := schemaResponse{
		ID:                 s.ID.String(),
		AgentID:            s.AgentID.String(),
		SchemaType:         string(s.SchemaType),
		Name:               s.Name,
		Description:        s.Description,
		Attributes:         s.Attributes,
		EvidenceCount:      s.EvidenceCount,
		Confidence:         s.Confidence,
		ContradictionCount: s.ContradictionCount,
		ApplicableContexts: s.ApplicableContexts,
		CreatedAt:          s.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		UpdatedAt:          s.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}

	// Ensure slices and maps aren't nil for JSON
	if resp.Attributes == nil {
		resp.Attributes = map[string]any{}
	}
	if resp.ApplicableContexts == nil {
		resp.ApplicableContexts = []string{}
	}

	return resp
}
