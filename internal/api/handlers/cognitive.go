package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/Harshitk-cp/engram/internal/api/middleware"
	"github.com/Harshitk-cp/engram/internal/service"
	"github.com/google/uuid"
)

type CognitiveHandler struct {
	decayService         *service.DecayService
	consolidationService *service.ConsolidationService
	confidenceService    *service.ConfidenceService
}

func NewCognitiveHandler(ds *service.DecayService, cs *service.ConsolidationService) *CognitiveHandler {
	return &CognitiveHandler{decayService: ds, consolidationService: cs}
}

func (h *CognitiveHandler) SetConfidenceService(cs *service.ConfidenceService) {
	h.confidenceService = cs
}

type triggerDecayRequest struct {
	AgentID string `json:"agent_id"`
}

type triggerDecayResponse struct {
	MemoriesDecayed  int `json:"memories_decayed"`
	MemoriesArchived int `json:"memories_archived"`
	EpisodesDecayed  int `json:"episodes_decayed"`
	EpisodesArchived int `json:"episodes_archived"`
}

func (h *CognitiveHandler) TriggerDecay(w http.ResponseWriter, r *http.Request) {
	var req triggerDecayRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.AgentID == "" {
		writeError(w, http.StatusBadRequest, "agent_id is required")
		return
	}

	agentID, err := uuid.Parse(req.AgentID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid agent_id format")
		return
	}

	result, err := h.decayService.RunDecayForAgent(r.Context(), agentID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	resp := triggerDecayResponse{
		MemoriesDecayed:  result.Decayed,
		MemoriesArchived: result.Archived,
		EpisodesDecayed:  result.EpisodesDecayed,
		EpisodesArchived: result.EpisodesArchived,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}

type triggerConsolidationRequest struct {
	AgentID string `json:"agent_id"`
	Scope   string `json:"scope"` // "recent" or "full"
}

type triggerConsolidationResponse struct {
	EpisodesProcessed    int `json:"episodes_processed"`
	SemanticExtracted    int `json:"semantic_extracted"`
	SemanticReinforced   int `json:"semantic_reinforced"`
	ProceduresLearned    int `json:"procedures_learned"`
	ProceduresReinforced int `json:"procedures_reinforced"`
	SchemasDetected      int `json:"schemas_detected"`
	SchemasUpdated       int `json:"schemas_updated"`
	MemoriesDecayed      int `json:"memories_decayed"`
	MemoriesArchived     int `json:"memories_archived"`
	MemoriesMerged       int `json:"memories_merged"`
	AssociationsCreated  int `json:"associations_created"`
}

// TriggerConsolidation manually triggers the consolidation pipeline for an agent.
func (h *CognitiveHandler) TriggerConsolidation(w http.ResponseWriter, r *http.Request) {
	if h.consolidationService == nil {
		writeError(w, http.StatusServiceUnavailable, "consolidation service not available")
		return
	}

	var req triggerConsolidationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.AgentID == "" {
		writeError(w, http.StatusBadRequest, "agent_id is required")
		return
	}

	agentID, err := uuid.Parse(req.AgentID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid agent_id format")
		return
	}

	// Get tenant from context
	tenant := middleware.TenantFromContext(r.Context())
	if tenant == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	tenantID := tenant.ID

	scope := service.ConsolidationScopeRecent
	if req.Scope == "full" {
		scope = service.ConsolidationScopeFull
	}

	result, err := h.consolidationService.Consolidate(r.Context(), agentID, tenantID, scope)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	resp := triggerConsolidationResponse{
		EpisodesProcessed:    result.EpisodesProcessed,
		SemanticExtracted:    result.SemanticExtracted,
		SemanticReinforced:   result.SemanticReinforced,
		ProceduresLearned:    result.ProceduresLearned,
		ProceduresReinforced: result.ProceduresReinforced,
		SchemasDetected:      result.SchemasDetected,
		SchemasUpdated:       result.SchemasUpdated,
		MemoriesDecayed:      result.MemoriesDecayed,
		MemoriesArchived:     result.MemoriesArchived,
		MemoriesMerged:       result.MemoriesMerged,
		AssociationsCreated:  result.AssociationsCreated,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}

type memoryHealthResponse struct {
	EpisodicCount      int      `json:"episodic_count"`
	SemanticCount      int      `json:"semantic_count"`
	ProceduralCount    int      `json:"procedural_count"`
	SchemaCount        int      `json:"schema_count"`
	MemoriesAtRisk     int      `json:"memories_at_risk"`
	RecentlyReinforced int      `json:"recently_reinforced"`
	ContradictionCount int      `json:"contradiction_count"`
	UncertaintyAreas   []string `json:"uncertainty_areas"`
	AverageConfidence  float32  `json:"average_confidence"`
	OldestUnprocessed  *string  `json:"oldest_unprocessed,omitempty"`
}

// GetMemoryHealth returns statistics about the memory system health for an agent.
func (h *CognitiveHandler) GetMemoryHealth(w http.ResponseWriter, r *http.Request) {
	if h.consolidationService == nil {
		writeError(w, http.StatusServiceUnavailable, "consolidation service not available")
		return
	}

	agentIDStr := r.URL.Query().Get("agent_id")
	if agentIDStr == "" {
		writeError(w, http.StatusBadRequest, "agent_id query parameter is required")
		return
	}

	agentID, err := uuid.Parse(agentIDStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid agent_id format")
		return
	}

	// Get tenant from context
	tenant := middleware.TenantFromContext(r.Context())
	if tenant == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	tenantID := tenant.ID

	stats, err := h.consolidationService.GetMemoryHealth(r.Context(), agentID, tenantID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	resp := memoryHealthResponse{
		EpisodicCount:      stats.EpisodicCount,
		SemanticCount:      stats.SemanticCount,
		ProceduralCount:    stats.ProceduralCount,
		SchemaCount:        stats.SchemaCount,
		MemoriesAtRisk:     stats.MemoriesAtRisk,
		RecentlyReinforced: stats.RecentlyReinforced,
		ContradictionCount: stats.ContradictionCount,
		UncertaintyAreas:   stats.UncertaintyAreas,
		AverageConfidence:  stats.AverageConfidence,
	}

	if stats.OldestUnprocessed != nil {
		s := stats.OldestUnprocessed.Format("2006-01-02T15:04:05Z")
		resp.OldestUnprocessed = &s
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}

type confidenceStatsResponse struct {
	MemoryID           string  `json:"memory_id"`
	RawConfidence      float32 `json:"raw_confidence"`
	DecayedConfidence  float64 `json:"decayed_confidence"`
	ReinforcementCount int     `json:"reinforcement_count"`
	Provenance         string  `json:"provenance"`
	HoursSinceAccess   float64 `json:"hours_since_access"`
	DecayFactor        float64 `json:"decay_factor"`
}

func (h *CognitiveHandler) GetConfidenceStats(w http.ResponseWriter, r *http.Request) {
	if h.confidenceService == nil {
		writeError(w, http.StatusServiceUnavailable, "confidence service not available")
		return
	}

	memoryIDStr := r.URL.Query().Get("memory_id")
	if memoryIDStr == "" {
		writeError(w, http.StatusBadRequest, "memory_id query parameter is required")
		return
	}

	memoryID, err := uuid.Parse(memoryIDStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid memory_id format")
		return
	}

	tenant := middleware.TenantFromContext(r.Context())
	if tenant == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	stats, err := h.confidenceService.GetStats(r.Context(), memoryID, tenant.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	resp := confidenceStatsResponse{
		MemoryID:           stats.MemoryID.String(),
		RawConfidence:      stats.RawConfidence,
		DecayedConfidence:  stats.DecayedConfidence,
		ReinforcementCount: stats.ReinforcementCount,
		Provenance:         stats.Provenance,
		HoursSinceAccess:   stats.HoursSinceAccess,
		DecayFactor:        stats.DecayFactor,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}

type reinforceRequest struct {
	MemoryID string `json:"memory_id"`
}

func (h *CognitiveHandler) ReinforceMemory(w http.ResponseWriter, r *http.Request) {
	if h.confidenceService == nil {
		writeError(w, http.StatusServiceUnavailable, "confidence service not available")
		return
	}

	var req reinforceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.MemoryID == "" {
		writeError(w, http.StatusBadRequest, "memory_id is required")
		return
	}

	memoryID, err := uuid.Parse(req.MemoryID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid memory_id format")
		return
	}

	tenant := middleware.TenantFromContext(r.Context())
	if tenant == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	if err := h.confidenceService.Reinforce(r.Context(), memoryID, tenant.ID); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "reinforced"})
}

type penalizeRequest struct {
	MemoryID string `json:"memory_id"`
}

func (h *CognitiveHandler) PenalizeMemory(w http.ResponseWriter, r *http.Request) {
	if h.confidenceService == nil {
		writeError(w, http.StatusServiceUnavailable, "confidence service not available")
		return
	}

	var req penalizeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.MemoryID == "" {
		writeError(w, http.StatusBadRequest, "memory_id is required")
		return
	}

	memoryID, err := uuid.Parse(req.MemoryID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid memory_id format")
		return
	}

	tenant := middleware.TenantFromContext(r.Context())
	if tenant == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	if err := h.confidenceService.Penalize(r.Context(), memoryID, tenant.ID); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "penalized"})
}
