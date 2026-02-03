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

type ProcedureHandler struct {
	svc *service.ProceduralService
}

func NewProcedureHandler(svc *service.ProceduralService) *ProcedureHandler {
	return &ProcedureHandler{svc: svc}
}

type matchProceduresRequest struct {
	AgentID        string  `json:"agent_id"`
	Situation      string  `json:"situation"`
	MinSuccessRate float32 `json:"min_success_rate,omitempty"`
	MinConfidence  float32 `json:"min_confidence,omitempty"`
	Limit          int     `json:"limit,omitempty"`
}

type matchProceduresResponse struct {
	Procedures []procedureResponse `json:"procedures"`
	Count      int                 `json:"count"`
}

type procedureResponse struct {
	ID              string                  `json:"id"`
	TriggerPattern  string                  `json:"trigger_pattern"`
	TriggerKeywords []string                `json:"trigger_keywords,omitempty"`
	ActionTemplate  string                  `json:"action_template"`
	ActionType      string                  `json:"action_type"`
	UseCount        int                     `json:"use_count"`
	SuccessCount    int                     `json:"success_count"`
	FailureCount    int                     `json:"failure_count"`
	SuccessRate     float32                 `json:"success_rate"`
	Confidence      float32                 `json:"confidence"`
	Version         int                     `json:"version"`
	Score           float32                 `json:"score,omitempty"`
	Examples        []domain.ExampleExchange `json:"examples,omitempty"`
	CreatedAt       string                  `json:"created_at"`
	UpdatedAt       string                  `json:"updated_at"`
}

// Match finds procedures applicable to the current situation.
// POST /v1/procedures/match
func (h *ProcedureHandler) Match(w http.ResponseWriter, r *http.Request) {
	tenant := middleware.TenantFromContext(r.Context())
	if tenant == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req matchProceduresRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Situation == "" {
		writeError(w, http.StatusBadRequest, "situation is required")
		return
	}

	agentID, err := uuid.Parse(req.AgentID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid agent_id")
		return
	}

	input := service.ProcedureMatchInput{
		AgentID:        agentID,
		TenantID:       tenant.ID,
		Situation:      req.Situation,
		MinSuccessRate: req.MinSuccessRate,
		MinConfidence:  req.MinConfidence,
		Limit:          req.Limit,
	}

	procedures, err := h.svc.GetApplicableProcedures(r.Context(), input)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to match procedures")
		return
	}

	response := matchProceduresResponse{
		Procedures: make([]procedureResponse, len(procedures)),
		Count:      len(procedures),
	}

	for i, p := range procedures {
		response.Procedures[i] = toProcedureResponse(&p.Procedure)
		response.Procedures[i].Score = p.Score
	}

	writeJSON(w, http.StatusOK, response)
}

// GetByID retrieves a specific procedure.
// GET /v1/procedures/:id
func (h *ProcedureHandler) GetByID(w http.ResponseWriter, r *http.Request) {
	tenant := middleware.TenantFromContext(r.Context())
	if tenant == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	idStr := chi.URLParam(r, "id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid procedure id")
		return
	}

	procedure, err := h.svc.GetByID(r.Context(), id, tenant.ID)
	if err != nil {
		if errors.Is(err, service.ErrProcedureNotFound) {
			writeError(w, http.StatusNotFound, "procedure not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to get procedure")
		return
	}

	writeJSON(w, http.StatusOK, toProcedureResponse(procedure))
}

type procedureOutcomeRequest struct {
	Success bool `json:"success"`
}

// RecordOutcome records the success/failure of using a procedure.
// POST /v1/procedures/:id/outcome
func (h *ProcedureHandler) RecordOutcome(w http.ResponseWriter, r *http.Request) {
	tenant := middleware.TenantFromContext(r.Context())
	if tenant == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	idStr := chi.URLParam(r, "id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid procedure id")
		return
	}

	var req procedureOutcomeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if err := h.svc.RecordProcedureOutcome(r.Context(), id, tenant.ID, req.Success); err != nil {
		if errors.Is(err, service.ErrProcedureNotFound) {
			writeError(w, http.StatusNotFound, "procedure not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to record outcome")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "recorded"})
}

type learnFromEpisodeRequest struct {
	EpisodeID string `json:"episode_id"`
	Outcome   string `json:"outcome"` // success, failure, neutral, unknown
}

// LearnFromEpisode extracts procedures from an episode outcome.
// POST /v1/procedures/learn
func (h *ProcedureHandler) LearnFromEpisode(w http.ResponseWriter, r *http.Request) {
	tenant := middleware.TenantFromContext(r.Context())
	if tenant == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req learnFromEpisodeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	episodeID, err := uuid.Parse(req.EpisodeID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid episode_id")
		return
	}

	if !domain.ValidOutcomeType(req.Outcome) {
		writeError(w, http.StatusBadRequest, "invalid outcome type (must be: success, failure, neutral, unknown)")
		return
	}

	outcome := domain.OutcomeType(req.Outcome)

	if err := h.svc.LearnFromOutcome(r.Context(), episodeID, tenant.ID, outcome); err != nil {
		if errors.Is(err, service.ErrEpisodeNotFound) {
			writeError(w, http.StatusNotFound, "episode not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to learn from episode")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "learned"})
}

// Helper to convert domain.Procedure to API response.
func toProcedureResponse(p *domain.Procedure) procedureResponse {
	resp := procedureResponse{
		ID:              p.ID.String(),
		TriggerPattern:  p.TriggerPattern,
		TriggerKeywords: p.TriggerKeywords,
		ActionTemplate:  p.ActionTemplate,
		ActionType:      string(p.ActionType),
		UseCount:        p.UseCount,
		SuccessCount:    p.SuccessCount,
		FailureCount:    p.FailureCount,
		SuccessRate:     p.SuccessRate,
		Confidence:      p.Confidence,
		Version:         p.Version,
		Examples:        p.ExampleExchanges,
		CreatedAt:       p.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		UpdatedAt:       p.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}

	// Ensure slices aren't nil for JSON
	if resp.TriggerKeywords == nil {
		resp.TriggerKeywords = []string{}
	}
	if resp.Examples == nil {
		resp.Examples = []domain.ExampleExchange{}
	}

	return resp
}

