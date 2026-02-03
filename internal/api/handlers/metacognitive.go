package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/Harshitk-cp/engram/internal/service"
	"github.com/google/uuid"
)

// MetacognitiveHandler handles metacognitive reflection endpoints.
type MetacognitiveHandler struct {
	metacognitiveService *service.MetacognitiveService
}

// NewMetacognitiveHandler creates a new metacognitive handler.
func NewMetacognitiveHandler(ms *service.MetacognitiveService) *MetacognitiveHandler {
	return &MetacognitiveHandler{metacognitiveService: ms}
}

type reflectRequest struct {
	AgentID string `json:"agent_id"`
	Focus   string `json:"focus,omitempty"` // "confidence", "uncertainty", "strategy", "all"
}

type confidenceAssessmentResponse struct {
	MemoryID           string             `json:"memory_id"`
	Content            string             `json:"content"`
	MemoryType         string             `json:"memory_type"`
	BaseConfidence     float32            `json:"base_confidence"`
	AdjustedConfidence float32            `json:"adjusted_confidence"`
	Factors            map[string]float32 `json:"factors"`
	Explanation        string             `json:"explanation"`
}

type memoryResponse struct {
	ID         string  `json:"id"`
	Type       string  `json:"type"`
	Content    string  `json:"content"`
	Confidence float32 `json:"confidence"`
}

type uncertaintyReportResponse struct {
	Topic               string           `json:"topic,omitempty"`
	UncertaintyLevel    float32          `json:"uncertainty_level"`
	ContradictedBeliefs []memoryResponse `json:"contradicted_beliefs"`
	LowConfidenceBeliefs []memoryResponse `json:"low_confidence_beliefs"`
	StaleBeliefs        []memoryResponse `json:"stale_beliefs"`
	Recommendation      string           `json:"recommendation"`
}

type metacogProcedureResponse struct {
	ID             string  `json:"id"`
	TriggerPattern string  `json:"trigger_pattern"`
	ActionTemplate string  `json:"action_template"`
	ActionType     string  `json:"action_type"`
	UseCount       int     `json:"use_count"`
	SuccessRate    float32 `json:"success_rate"`
}

type procedureAssessmentResponse struct {
	Procedure      metacogProcedureResponse `json:"procedure"`
	SuccessRate    float32           `json:"success_rate"`
	Recommendation string            `json:"recommendation"`
}

type failurePatternResponse struct {
	Pattern    string   `json:"pattern"`
	Frequency  int      `json:"frequency"`
	Topics     []string `json:"topics,omitempty"`
	Suggestion string   `json:"suggestion"`
}

type strategyReflectionResponse struct {
	EffectiveStrategies       []procedureAssessmentResponse `json:"effective_strategies"`
	UnderperformingStrategies []procedureAssessmentResponse `json:"underperforming_strategies"`
	FailurePatterns           []failurePatternResponse      `json:"failure_patterns"`
	Suggestions               []string                      `json:"suggestions"`
}

type reflectResponse struct {
	ConfidenceAssessments []confidenceAssessmentResponse `json:"confidence_assessments,omitempty"`
	UncertaintyReport     *uncertaintyReportResponse     `json:"uncertainty_report,omitempty"`
	StrategyReflection    *strategyReflectionResponse    `json:"strategy_reflection,omitempty"`
	OverallHealthScore    float32                        `json:"overall_health_score"`
	ActionItems           []string                       `json:"action_items"`
}

// Reflect handles POST /v1/cognitive/reflect.
func (h *MetacognitiveHandler) Reflect(w http.ResponseWriter, r *http.Request) {
	if h.metacognitiveService == nil {
		writeError(w, http.StatusServiceUnavailable, "metacognitive service not available")
		return
	}

	var req reflectRequest
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

	// Validate focus parameter
	if req.Focus != "" && req.Focus != "all" && req.Focus != "confidence" && req.Focus != "uncertainty" && req.Focus != "strategy" {
		writeError(w, http.StatusBadRequest, "focus must be one of: confidence, uncertainty, strategy, all")
		return
	}

	// Get tenant ID from context
	tenantID, ok := r.Context().Value("tenant_id").(uuid.UUID)
	if !ok {
		writeError(w, http.StatusUnauthorized, "tenant_id not found in context")
		return
	}

	result, err := h.metacognitiveService.Reflect(r.Context(), agentID, tenantID, req.Focus)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Convert to response format
	resp := reflectResponse{
		OverallHealthScore: result.OverallHealthScore,
		ActionItems:        result.ActionItems,
	}

	// Convert confidence assessments
	if len(result.ConfidenceAssessments) > 0 {
		resp.ConfidenceAssessments = make([]confidenceAssessmentResponse, len(result.ConfidenceAssessments))
		for i, ca := range result.ConfidenceAssessments {
			resp.ConfidenceAssessments[i] = confidenceAssessmentResponse{
				MemoryID:           ca.MemoryID.String(),
				Content:            ca.Content,
				MemoryType:         string(ca.MemoryType),
				BaseConfidence:     ca.BaseConfidence,
				AdjustedConfidence: ca.AdjustedConfidence,
				Factors:            ca.Factors,
				Explanation:        ca.Explanation,
			}
		}
	}

	// Convert uncertainty report
	if result.UncertaintyReport != nil {
		ur := result.UncertaintyReport
		resp.UncertaintyReport = &uncertaintyReportResponse{
			Topic:               ur.Topic,
			UncertaintyLevel:    ur.UncertaintyLevel,
			ContradictedBeliefs: make([]memoryResponse, len(ur.ContradictedBeliefs)),
			LowConfidenceBeliefs: make([]memoryResponse, len(ur.LowConfidenceBeliefs)),
			StaleBeliefs:        make([]memoryResponse, len(ur.StaleBeliefs)),
			Recommendation:      ur.Recommendation,
		}

		for i, m := range ur.ContradictedBeliefs {
			resp.UncertaintyReport.ContradictedBeliefs[i] = memoryResponse{
				ID:         m.ID.String(),
				Type:       string(m.Type),
				Content:    m.Content,
				Confidence: m.Confidence,
			}
		}

		for i, m := range ur.LowConfidenceBeliefs {
			resp.UncertaintyReport.LowConfidenceBeliefs[i] = memoryResponse{
				ID:         m.ID.String(),
				Type:       string(m.Type),
				Content:    m.Content,
				Confidence: m.Confidence,
			}
		}

		for i, m := range ur.StaleBeliefs {
			resp.UncertaintyReport.StaleBeliefs[i] = memoryResponse{
				ID:         m.ID.String(),
				Type:       string(m.Type),
				Content:    m.Content,
				Confidence: m.Confidence,
			}
		}
	}

	// Convert strategy reflection
	if result.StrategyReflection != nil {
		sr := result.StrategyReflection
		resp.StrategyReflection = &strategyReflectionResponse{
			EffectiveStrategies:       make([]procedureAssessmentResponse, len(sr.EffectiveStrategies)),
			UnderperformingStrategies: make([]procedureAssessmentResponse, len(sr.UnderperformingStrategies)),
			FailurePatterns:           make([]failurePatternResponse, len(sr.FailurePatterns)),
			Suggestions:               sr.Suggestions,
		}

		for i, pa := range sr.EffectiveStrategies {
			resp.StrategyReflection.EffectiveStrategies[i] = procedureAssessmentResponse{
				Procedure: metacogProcedureResponse{
					ID:             pa.Procedure.ID.String(),
					TriggerPattern: pa.Procedure.TriggerPattern,
					ActionTemplate: pa.Procedure.ActionTemplate,
					ActionType:     string(pa.Procedure.ActionType),
					UseCount:       pa.Procedure.UseCount,
					SuccessRate:    pa.Procedure.SuccessRate,
				},
				SuccessRate:    pa.SuccessRate,
				Recommendation: pa.Recommendation,
			}
		}

		for i, pa := range sr.UnderperformingStrategies {
			resp.StrategyReflection.UnderperformingStrategies[i] = procedureAssessmentResponse{
				Procedure: metacogProcedureResponse{
					ID:             pa.Procedure.ID.String(),
					TriggerPattern: pa.Procedure.TriggerPattern,
					ActionTemplate: pa.Procedure.ActionTemplate,
					ActionType:     string(pa.Procedure.ActionType),
					UseCount:       pa.Procedure.UseCount,
					SuccessRate:    pa.Procedure.SuccessRate,
				},
				SuccessRate:    pa.SuccessRate,
				Recommendation: pa.Recommendation,
			}
		}

		for i, fp := range sr.FailurePatterns {
			resp.StrategyReflection.FailurePatterns[i] = failurePatternResponse{
				Pattern:    fp.Pattern,
				Frequency:  fp.Frequency,
				Topics:     fp.Topics,
				Suggestion: fp.Suggestion,
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}

// AssessConfidence handles GET /v1/cognitive/confidence for assessing a single memory.
func (h *MetacognitiveHandler) AssessConfidence(w http.ResponseWriter, r *http.Request) {
	if h.metacognitiveService == nil {
		writeError(w, http.StatusServiceUnavailable, "metacognitive service not available")
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

	// Get tenant ID from context
	tenantID, ok := r.Context().Value("tenant_id").(uuid.UUID)
	if !ok {
		writeError(w, http.StatusUnauthorized, "tenant_id not found in context")
		return
	}

	explanation, adjustedConfidence, err := h.metacognitiveService.GetConfidenceExplanationForMemory(r.Context(), memoryID, tenantID)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	resp := map[string]any{
		"memory_id":           memoryID.String(),
		"adjusted_confidence": adjustedConfidence,
		"explanation":         explanation,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}

// DetectUncertainty handles GET /v1/cognitive/uncertainty for detecting uncertainty areas.
func (h *MetacognitiveHandler) DetectUncertainty(w http.ResponseWriter, r *http.Request) {
	if h.metacognitiveService == nil {
		writeError(w, http.StatusServiceUnavailable, "metacognitive service not available")
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

	topic := r.URL.Query().Get("topic")

	// Get tenant ID from context
	tenantID, ok := r.Context().Value("tenant_id").(uuid.UUID)
	if !ok {
		writeError(w, http.StatusUnauthorized, "tenant_id not found in context")
		return
	}

	result, err := h.metacognitiveService.DetectUncertainty(r.Context(), agentID, tenantID, topic)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Convert to response
	resp := uncertaintyReportResponse{
		Topic:               result.Topic,
		UncertaintyLevel:    result.UncertaintyLevel,
		ContradictedBeliefs: make([]memoryResponse, len(result.ContradictedBeliefs)),
		LowConfidenceBeliefs: make([]memoryResponse, len(result.LowConfidenceBeliefs)),
		StaleBeliefs:        make([]memoryResponse, len(result.StaleBeliefs)),
		Recommendation:      result.Recommendation,
	}

	for i, m := range result.ContradictedBeliefs {
		resp.ContradictedBeliefs[i] = memoryResponse{
			ID:         m.ID.String(),
			Type:       string(m.Type),
			Content:    m.Content,
			Confidence: m.Confidence,
		}
	}

	for i, m := range result.LowConfidenceBeliefs {
		resp.LowConfidenceBeliefs[i] = memoryResponse{
			ID:         m.ID.String(),
			Type:       string(m.Type),
			Content:    m.Content,
			Confidence: m.Confidence,
		}
	}

	for i, m := range result.StaleBeliefs {
		resp.StaleBeliefs[i] = memoryResponse{
			ID:         m.ID.String(),
			Type:       string(m.Type),
			Content:    m.Content,
			Confidence: m.Confidence,
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}
