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

type PolicyHandler struct {
	svc *service.PolicyService
}

func NewPolicyHandler(svc *service.PolicyService) *PolicyHandler {
	return &PolicyHandler{svc: svc}
}

type policyRequest struct {
	MemoryType     string   `json:"memory_type"`
	MaxMemories    int      `json:"max_memories"`
	RetentionDays  *int     `json:"retention_days"`
	PriorityWeight float64  `json:"priority_weight"`
	AutoSummarize  bool     `json:"auto_summarize"`
}

type upsertPoliciesRequest struct {
	Policies []policyRequest `json:"policies"`
}

type policyResponse struct {
	MemoryType     string   `json:"memory_type"`
	MaxMemories    int      `json:"max_memories"`
	RetentionDays  *int     `json:"retention_days,omitempty"`
	PriorityWeight float64  `json:"priority_weight"`
	AutoSummarize  bool     `json:"auto_summarize"`
}

type policiesResponse struct {
	Policies []policyResponse `json:"policies"`
}

func toPolicyResponse(p domain.Policy) policyResponse {
	return policyResponse{
		MemoryType:     string(p.MemoryType),
		MaxMemories:    p.MaxMemories,
		RetentionDays:  p.RetentionDays,
		PriorityWeight: p.PriorityWeight,
		AutoSummarize:  p.AutoSummarize,
	}
}

func (h *PolicyHandler) Get(w http.ResponseWriter, r *http.Request) {
	tenant := middleware.TenantFromContext(r.Context())
	if tenant == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	agentID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid agent id")
		return
	}

	policies, err := h.svc.GetPolicies(r.Context(), agentID, tenant.ID)
	if err != nil {
		if errors.Is(err, service.ErrAgentNotFound) {
			writeError(w, http.StatusNotFound, "agent not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to get policies")
		return
	}

	resp := policiesResponse{Policies: make([]policyResponse, 0, len(policies))}
	for _, p := range policies {
		resp.Policies = append(resp.Policies, toPolicyResponse(p))
	}

	writeJSON(w, http.StatusOK, resp)
}

func (h *PolicyHandler) Upsert(w http.ResponseWriter, r *http.Request) {
	tenant := middleware.TenantFromContext(r.Context())
	if tenant == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	agentID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid agent id")
		return
	}

	var req upsertPoliciesRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if len(req.Policies) == 0 {
		writeError(w, http.StatusBadRequest, "at least one policy is required")
		return
	}

	var domainPolicies []domain.Policy
	for _, p := range req.Policies {
		domainPolicies = append(domainPolicies, domain.Policy{
			MemoryType:     domain.MemoryType(p.MemoryType),
			MaxMemories:    p.MaxMemories,
			RetentionDays:  p.RetentionDays,
			PriorityWeight: p.PriorityWeight,
			AutoSummarize:  p.AutoSummarize,
		})
	}

	result, err := h.svc.UpsertPolicies(r.Context(), agentID, tenant.ID, domainPolicies)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrAgentNotFound):
			writeError(w, http.StatusNotFound, "agent not found")
		case errors.Is(err, service.ErrPolicyInvalidType):
			writeError(w, http.StatusBadRequest, err.Error())
		case errors.Is(err, service.ErrPolicyMaxMemories):
			writeError(w, http.StatusBadRequest, err.Error())
		case errors.Is(err, service.ErrPolicyPriorityWeight):
			writeError(w, http.StatusBadRequest, err.Error())
		default:
			writeError(w, http.StatusInternalServerError, "failed to upsert policies")
		}
		return
	}

	resp := policiesResponse{Policies: make([]policyResponse, 0, len(result))}
	for _, p := range result {
		resp.Policies = append(resp.Policies, toPolicyResponse(p))
	}

	writeJSON(w, http.StatusOK, resp)
}
