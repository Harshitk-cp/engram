package handlers

import (
	"net/http"
	"strconv"

	"github.com/Harshitk-cp/engram/internal/api/middleware"
	"github.com/Harshitk-cp/engram/internal/domain"
	"github.com/Harshitk-cp/engram/internal/service"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

type TierHandler struct {
	memorySvc *service.MemoryService
}

func NewTierHandler(memorySvc *service.MemoryService) *TierHandler {
	return &TierHandler{memorySvc: memorySvc}
}

type tierStatsResponse struct {
	HotCount     int `json:"hot_count"`
	WarmCount    int `json:"warm_count"`
	ColdCount    int `json:"cold_count"`
	ArchiveCount int `json:"archive_count"`
}

func (h *TierHandler) GetTierStats(w http.ResponseWriter, r *http.Request) {
	tenant := middleware.TenantFromContext(r.Context())
	if tenant == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	idStr := chi.URLParam(r, "id")
	agentID, err := uuid.Parse(idStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid agent id")
		return
	}

	stats, err := h.memorySvc.GetTierStats(r.Context(), agentID, tenant.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get tier stats")
		return
	}

	writeJSON(w, http.StatusOK, tierStatsResponse{
		HotCount:     stats.HotCount,
		WarmCount:    stats.WarmCount,
		ColdCount:    stats.ColdCount,
		ArchiveCount: stats.ArchiveCount,
	})
}

type hotMemoriesResponse struct {
	Memories []memoryWithTier `json:"memories"`
	Count    int              `json:"count"`
}

type memoryWithTier struct {
	*domain.Memory
	Tier       domain.MemoryTier `json:"tier"`
	TierReason string            `json:"tier_reason"`
}

func (h *TierHandler) GetHotMemories(w http.ResponseWriter, r *http.Request) {
	tenant := middleware.TenantFromContext(r.Context())
	if tenant == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	idStr := chi.URLParam(r, "id")
	agentID, err := uuid.Parse(idStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid agent id")
		return
	}

	limit := 20
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			limit = l
		}
	}

	memories, err := h.memorySvc.GetHotMemories(r.Context(), agentID, tenant.ID, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get hot memories")
		return
	}

	result := make([]memoryWithTier, 0, len(memories))
	for _, m := range memories {
		mem := m
		result = append(result, memoryWithTier{
			Memory:     &mem,
			Tier:       domain.TierHot,
			TierReason: domain.TierReason(float64(m.Confidence)),
		})
	}

	writeJSON(w, http.StatusOK, hotMemoriesResponse{
		Memories: result,
		Count:    len(result),
	})
}
