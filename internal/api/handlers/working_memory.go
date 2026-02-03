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

type WorkingMemoryHandler struct {
	svc *service.WorkingMemoryService
}

func NewWorkingMemoryHandler(svc *service.WorkingMemoryService) *WorkingMemoryHandler {
	return &WorkingMemoryHandler{svc: svc}
}

type activateRequest struct {
	AgentID string           `json:"agent_id"`
	Goal    string           `json:"goal,omitempty"`
	Cues    []string         `json:"cues"`
	Context []domain.Message `json:"context,omitempty"`
}

type activateResponse struct {
	WorkingMemory workingMemoryResponse `json:"working_memory"`
	AssembledContext string             `json:"assembled_context"`
}

type workingMemoryResponse struct {
	SessionID     string               `json:"session_id"`
	CurrentGoal   string               `json:"current_goal,omitempty"`
	Activations   []activationResponse `json:"activations"`
	ActiveSchemas []schemaMatchResp    `json:"active_schemas,omitempty"`
	SlotUsage     int                  `json:"slot_usage"`
	MaxSlots      int                  `json:"max_slots"`
}

type activationResponse struct {
	MemoryType string  `json:"memory_type"`
	MemoryID   string  `json:"memory_id"`
	Content    string  `json:"content"`
	Confidence float32 `json:"confidence"`
	Score      float32 `json:"score"`
}

type schemaMatchResp struct {
	SchemaID    string  `json:"schema_id"`
	Name        string  `json:"name"`
	Description string  `json:"description,omitempty"`
	MatchScore  float32 `json:"match_score"`
}

type getSessionResponse struct {
	SessionID      string               `json:"session_id"`
	AgentID        string               `json:"agent_id"`
	CurrentGoal    string               `json:"current_goal,omitempty"`
	ActiveContext  []domain.Message     `json:"active_context,omitempty"`
	ReasoningState map[string]any       `json:"reasoning_state,omitempty"`
	Activations    []activationResponse `json:"activations,omitempty"`
	ActiveSchemas  []schemaMatchResp    `json:"active_schemas,omitempty"`
	SlotUsage      int                  `json:"slot_usage"`
	MaxSlots       int                  `json:"max_slots"`
	StartedAt      string               `json:"started_at"`
	LastActivityAt string               `json:"last_activity_at"`
}

type updateGoalRequest struct {
	AgentID string `json:"agent_id"`
	Goal    string `json:"goal"`
}

type clearSessionRequest struct {
	AgentID string `json:"agent_id"`
}

// Activate performs intelligent memory activation for a task.
// POST /v1/cognitive/activate
func (h *WorkingMemoryHandler) Activate(w http.ResponseWriter, r *http.Request) {
	tenant := middleware.TenantFromContext(r.Context())
	if tenant == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req activateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	agentID, err := uuid.Parse(req.AgentID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid agent_id")
		return
	}

	if len(req.Cues) == 0 && req.Goal == "" {
		writeError(w, http.StatusBadRequest, "at least one cue or goal is required")
		return
	}

	input := domain.ActivationInput{
		AgentID:  agentID,
		TenantID: tenant.ID,
		Goal:     req.Goal,
		Cues:     req.Cues,
		Context:  req.Context,
	}

	result, err := h.svc.Activate(r.Context(), input)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to activate memories")
		return
	}

	response := activateResponse{
		WorkingMemory: workingMemoryResponse{
			SessionID:   result.Session.ID.String(),
			CurrentGoal: result.Session.CurrentGoal,
			SlotUsage:   result.SlotUsage,
			MaxSlots:    result.MaxSlots,
		},
		AssembledContext: result.AssembledContext,
	}

	for _, act := range result.Activations {
		response.WorkingMemory.Activations = append(response.WorkingMemory.Activations, activationResponse{
			MemoryType: string(act.Type),
			MemoryID:   act.ID.String(),
			Content:    act.Content,
			Confidence: act.Confidence,
			Score:      act.Score,
		})
	}

	for _, sm := range result.ActiveSchemas {
		response.WorkingMemory.ActiveSchemas = append(response.WorkingMemory.ActiveSchemas, schemaMatchResp{
			SchemaID:    sm.Schema.ID.String(),
			Name:        sm.Schema.Name,
			Description: sm.Schema.Description,
			MatchScore:  sm.MatchScore,
		})
	}

	writeJSON(w, http.StatusOK, response)
}

// GetSession retrieves the current working memory session.
// GET /v1/cognitive/session?agent_id=...
func (h *WorkingMemoryHandler) GetSession(w http.ResponseWriter, r *http.Request) {
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

	session, err := h.svc.GetSession(r.Context(), agentID, tenant.ID)
	if err != nil {
		if errors.Is(err, service.ErrSessionNotFound) {
			writeError(w, http.StatusNotFound, "no active session for this agent")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to get session")
		return
	}

	response := getSessionResponse{
		SessionID:      session.ID.String(),
		AgentID:        session.AgentID.String(),
		CurrentGoal:    session.CurrentGoal,
		ActiveContext:  session.ActiveContext,
		ReasoningState: session.ReasoningState,
		SlotUsage:      len(session.Activations),
		MaxSlots:       session.MaxSlots,
		StartedAt:      session.StartedAt.Format("2006-01-02T15:04:05Z07:00"),
		LastActivityAt: session.LastActivityAt.Format("2006-01-02T15:04:05Z07:00"),
	}

	for _, act := range session.Activations {
		response.Activations = append(response.Activations, activationResponse{
			MemoryType: string(act.MemoryType),
			MemoryID:   act.MemoryID.String(),
			Content:    act.Content,
			Confidence: act.MemoryConfidence,
			Score:      act.ActivationLevel,
		})
	}

	writeJSON(w, http.StatusOK, response)
}

// UpdateGoal updates the current goal in working memory.
// PUT /v1/cognitive/goal
func (h *WorkingMemoryHandler) UpdateGoal(w http.ResponseWriter, r *http.Request) {
	tenant := middleware.TenantFromContext(r.Context())
	if tenant == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req updateGoalRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	agentID, err := uuid.Parse(req.AgentID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid agent_id")
		return
	}

	if err := h.svc.UpdateGoal(r.Context(), agentID, tenant.ID, req.Goal); err != nil {
		if errors.Is(err, service.ErrSessionNotFound) {
			writeError(w, http.StatusNotFound, "no active session for this agent")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to update goal")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

// ClearSession clears the working memory session.
// DELETE /v1/cognitive/session
func (h *WorkingMemoryHandler) ClearSession(w http.ResponseWriter, r *http.Request) {
	tenant := middleware.TenantFromContext(r.Context())
	if tenant == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req clearSessionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	agentID, err := uuid.Parse(req.AgentID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid agent_id")
		return
	}

	if err := h.svc.ClearSession(r.Context(), agentID, tenant.ID); err != nil {
		if errors.Is(err, service.ErrSessionNotFound) {
			writeError(w, http.StatusNotFound, "no active session for this agent")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to clear session")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "cleared"})
}
