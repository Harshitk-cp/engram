package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/Harshitk-cp/engram/internal/api/middleware"
	"github.com/Harshitk-cp/engram/internal/domain"
	"github.com/Harshitk-cp/engram/internal/service"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

type LearningHandler struct {
	learningSvc          *service.LearningService
	implicitFeedbackSvc  *service.ImplicitFeedbackDetector
	mutationLogStore     domain.MutationLogStore
}

func NewLearningHandler(
	learningSvc *service.LearningService,
	implicitFeedbackSvc *service.ImplicitFeedbackDetector,
	mutationLogStore domain.MutationLogStore,
) *LearningHandler {
	return &LearningHandler{
		learningSvc:         learningSvc,
		implicitFeedbackSvc: implicitFeedbackSvc,
		mutationLogStore:    mutationLogStore,
	}
}

type learningOutcomeRequest struct {
	EpisodeID    string   `json:"episode_id"`
	MemoriesUsed []string `json:"memories_used"`
	Outcome      string   `json:"outcome"`
}

// RecordOutcome handles POST /v1/learning/outcome
func (h *LearningHandler) RecordOutcome(w http.ResponseWriter, r *http.Request) {
	tenant := middleware.TenantFromContext(r.Context())
	if tenant == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req learningOutcomeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	episodeID, err := uuid.Parse(req.EpisodeID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid episode_id")
		return
	}

	var memoryIDs []uuid.UUID
	for _, idStr := range req.MemoriesUsed {
		id, err := uuid.Parse(idStr)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid memory_id: "+idStr)
			return
		}
		memoryIDs = append(memoryIDs, id)
	}

	var outcome domain.OutcomeType
	switch req.Outcome {
	case "success":
		outcome = domain.OutcomeSuccess
	case "failure":
		outcome = domain.OutcomeFailure
	case "neutral":
		outcome = domain.OutcomeNeutral
	default:
		writeError(w, http.StatusBadRequest, "invalid outcome: must be success, failure, or neutral")
		return
	}

	record := domain.OutcomeRecord{
		EpisodeID:    episodeID,
		MemoriesUsed: memoryIDs,
		Outcome:      outcome,
		OccurredAt:   time.Now(),
	}

	if err := h.learningSvc.RecordOutcome(r.Context(), record); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to record outcome")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"status":        "recorded",
		"episode_id":    episodeID.String(),
		"memories_count": len(memoryIDs),
		"outcome":       req.Outcome,
	})
}

type detectImplicitFeedbackRequest struct {
	AgentID      string           `json:"agent_id"`
	Memories     []memoryInput    `json:"memories"`
	Conversation []domain.Message `json:"conversation"`
}

type memoryInput struct {
	ID      string `json:"id"`
	Content string `json:"content"`
}

// DetectImplicitFeedback handles POST /v1/learning/detect-feedback
func (h *LearningHandler) DetectImplicitFeedback(w http.ResponseWriter, r *http.Request) {
	tenant := middleware.TenantFromContext(r.Context())
	if tenant == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req detectImplicitFeedbackRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	agentID, err := uuid.Parse(req.AgentID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid agent_id")
		return
	}

	var memories []domain.Memory
	for _, m := range req.Memories {
		id, err := uuid.Parse(m.ID)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid memory id: "+m.ID)
			return
		}
		memories = append(memories, domain.Memory{
			ID:      id,
			Content: m.Content,
		})
	}

	detectReq := service.DetectRequest{
		AgentID:      agentID,
		TenantID:     tenant.ID,
		Memories:     memories,
		Conversation: req.Conversation,
	}

	feedbacks, err := h.implicitFeedbackSvc.DetectAndApply(r.Context(), detectReq)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to detect implicit feedback")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"detected_count": len(feedbacks),
		"feedbacks":      feedbacks,
	})
}

// GetStats handles GET /v1/agents/:id/learning/stats
func (h *LearningHandler) GetStats(w http.ResponseWriter, r *http.Request) {
	tenant := middleware.TenantFromContext(r.Context())
	if tenant == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	agentIDStr := chi.URLParam(r, "id")
	agentID, err := uuid.Parse(agentIDStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid agent_id")
		return
	}

	stats, err := h.learningSvc.GetLearningStats(r.Context(), agentID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get learning stats")
		return
	}

	if stats == nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"message": "no learning stats available",
		})
		return
	}

	writeJSON(w, http.StatusOK, stats)
}

// GetMutationHistory handles GET /v1/memories/:id/mutations
func (h *LearningHandler) GetMutationHistory(w http.ResponseWriter, r *http.Request) {
	tenant := middleware.TenantFromContext(r.Context())
	if tenant == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	memoryIDStr := chi.URLParam(r, "id")
	memoryID, err := uuid.Parse(memoryIDStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid memory_id")
		return
	}

	limit := 50
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 100 {
			limit = l
		}
	}

	if h.mutationLogStore == nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"mutations": []any{},
			"message":   "mutation logging not enabled",
		})
		return
	}

	mutations, err := h.mutationLogStore.GetByMemoryID(r.Context(), memoryID, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get mutation history")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"memory_id": memoryID.String(),
		"count":     len(mutations),
		"mutations": mutations,
	})
}
