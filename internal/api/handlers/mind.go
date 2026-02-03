package handlers

import (
	"net/http"
	"sort"

	"github.com/Harshitk-cp/engram/internal/api/middleware"
	"github.com/Harshitk-cp/engram/internal/domain"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// MindHandler provides observability into an agent's cognitive state.
type MindHandler struct {
	memoryStore    domain.MemoryStore
	episodeStore   domain.EpisodeStore
	procedureStore domain.ProcedureStore
	schemaStore    domain.SchemaStore
}

func NewMindHandler(
	ms domain.MemoryStore,
	es domain.EpisodeStore,
	ps domain.ProcedureStore,
	ss domain.SchemaStore,
) *MindHandler {
	return &MindHandler{
		memoryStore:    ms,
		episodeStore:   es,
		procedureStore: ps,
		schemaStore:    ss,
	}
}

type beliefResponse struct {
	ID                 string  `json:"id"`
	Type               string  `json:"type"`
	Content            string  `json:"content"`
	Confidence         float32 `json:"confidence"`
	ReinforcementCount int     `json:"reinforcement_count"`
	DecayStatus        string  `json:"decay_status"`
}

type procedureMindResponse struct {
	ID             string  `json:"id"`
	TriggerPattern string  `json:"trigger_pattern"`
	ActionTemplate string  `json:"action_template"`
	ActionType     string  `json:"action_type"`
	SuccessRate    float32 `json:"success_rate"`
	UseCount       int     `json:"use_count"`
}

type schemaMindResponse struct {
	ID          string         `json:"id"`
	SchemaType  string         `json:"schema_type"`
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Confidence  float32        `json:"confidence"`
	Attributes  map[string]any `json:"attributes,omitempty"`
}

type episodeMindResponse struct {
	ID              string   `json:"id"`
	RawContent      string   `json:"raw_content"`
	OccurredAt      string   `json:"occurred_at"`
	ImportanceScore float32  `json:"importance_score"`
	Outcome         string   `json:"outcome,omitempty"`
	Topics          []string `json:"topics,omitempty"`
}

type mindStatsResponse struct {
	TotalMemories   int     `json:"total_memories"`
	TotalEpisodes   int     `json:"total_episodes"`
	TotalProcedures int     `json:"total_procedures"`
	TotalSchemas    int     `json:"total_schemas"`
	Archived        int     `json:"archived"`
	AvgConfidence   float32 `json:"avg_confidence"`
	AtRisk          int     `json:"at_risk"`
}

type mindResponse struct {
	AgentID        string                  `json:"agent_id"`
	Beliefs        []beliefResponse        `json:"beliefs"`
	Procedures     []procedureMindResponse `json:"procedures"`
	Schemas        []schemaMindResponse    `json:"schemas"`
	RecentEpisodes []episodeMindResponse   `json:"recent_episodes"`
	Stats          mindStatsResponse       `json:"stats"`
}

// GetMind returns a comprehensive view of an agent's cognitive state.
// GET /v1/agents/{id}/mind
func (h *MindHandler) GetMind(w http.ResponseWriter, r *http.Request) {
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

	ctx := r.Context()
	resp := mindResponse{
		AgentID:        agentID.String(),
		Beliefs:        []beliefResponse{},
		Procedures:     []procedureMindResponse{},
		Schemas:        []schemaMindResponse{},
		RecentEpisodes: []episodeMindResponse{},
	}

	var totalConfidence float32
	var confidenceCount int
	var archivedCount int

	// Get memories (beliefs) - we need to get all and sort by confidence
	if h.memoryStore != nil {
		memories, err := h.memoryStore.GetByAgentForDecay(ctx, agentID)
		if err == nil {
			// Sort by confidence descending
			sort.Slice(memories, func(i, j int) bool {
				return memories[i].Confidence > memories[j].Confidence
			})

			// Take top 20
			limit := 20
			if len(memories) < limit {
				limit = len(memories)
			}

			for i := 0; i < limit; i++ {
				m := memories[i]
				resp.Beliefs = append(resp.Beliefs, beliefResponse{
					ID:                 m.ID.String(),
					Type:               string(m.Type),
					Content:            m.Content,
					Confidence:         m.Confidence,
					ReinforcementCount: m.ReinforcementCount,
					DecayStatus:        calculateDecayStatus(m.Confidence),
				})
			}

			// Calculate stats from all memories
			for _, m := range memories {
				totalConfidence += m.Confidence
				confidenceCount++
			}

			resp.Stats.TotalMemories = len(memories)

			// Count at-risk (confidence < 0.4)
			for _, m := range memories {
				if m.Confidence < 0.4 {
					resp.Stats.AtRisk++
				}
			}
		}
	}

	// Get procedures
	if h.procedureStore != nil {
		procedures, err := h.procedureStore.GetByAgent(ctx, agentID, tenant.ID)
		if err == nil {
			// Sort by success rate * use count (most effective and used)
			sort.Slice(procedures, func(i, j int) bool {
				scoreI := procedures[i].SuccessRate * float32(procedures[i].UseCount+1)
				scoreJ := procedures[j].SuccessRate * float32(procedures[j].UseCount+1)
				return scoreI > scoreJ
			})

			limit := 10
			if len(procedures) < limit {
				limit = len(procedures)
			}

			for i := 0; i < limit; i++ {
				p := procedures[i]
				resp.Procedures = append(resp.Procedures, procedureMindResponse{
					ID:             p.ID.String(),
					TriggerPattern: p.TriggerPattern,
					ActionTemplate: truncate(p.ActionTemplate, 200),
					ActionType:     string(p.ActionType),
					SuccessRate:    p.SuccessRate,
					UseCount:       p.UseCount,
				})
			}

			resp.Stats.TotalProcedures = len(procedures)
		}
	}

	// Get schemas
	if h.schemaStore != nil {
		schemas, err := h.schemaStore.GetByAgent(ctx, agentID, tenant.ID)
		if err == nil {
			// Sort by confidence
			sort.Slice(schemas, func(i, j int) bool {
				return schemas[i].Confidence > schemas[j].Confidence
			})

			limit := 10
			if len(schemas) < limit {
				limit = len(schemas)
			}

			for i := 0; i < limit; i++ {
				s := schemas[i]
				resp.Schemas = append(resp.Schemas, schemaMindResponse{
					ID:          s.ID.String(),
					SchemaType:  string(s.SchemaType),
					Name:        s.Name,
					Description: s.Description,
					Confidence:  s.Confidence,
					Attributes:  s.Attributes,
				})
			}

			resp.Stats.TotalSchemas = len(schemas)
		}
	}

	// Get recent episodes
	if h.episodeStore != nil {
		// Get episodes from last 7 days, sorted by recency
		episodes, err := h.episodeStore.GetByAgentForDecay(ctx, agentID)
		if err == nil {
			// Sort by occurred_at descending (most recent first)
			sort.Slice(episodes, func(i, j int) bool {
				return episodes[i].OccurredAt.After(episodes[j].OccurredAt)
			})

			limit := 10
			if len(episodes) < limit {
				limit = len(episodes)
			}

			for i := 0; i < limit; i++ {
				e := episodes[i]
				ep := episodeMindResponse{
					ID:              e.ID.String(),
					RawContent:      truncate(e.RawContent, 300),
					OccurredAt:      e.OccurredAt.Format("2006-01-02T15:04:05Z07:00"),
					ImportanceScore: e.ImportanceScore,
					Topics:          e.Topics,
				}
				if e.Outcome != "" {
					ep.Outcome = string(e.Outcome)
				}
				resp.RecentEpisodes = append(resp.RecentEpisodes, ep)
			}

			resp.Stats.TotalEpisodes = len(episodes)

			// Count archived episodes
			for _, e := range episodes {
				if e.ConsolidationStatus == domain.ConsolidationArchived {
					archivedCount++
				}
			}
		}
	}

	// Calculate average confidence
	if confidenceCount > 0 {
		resp.Stats.AvgConfidence = totalConfidence / float32(confidenceCount)
	}
	resp.Stats.Archived = archivedCount

	writeJSON(w, http.StatusOK, resp)
}

// truncate shortens a string to maxLen characters, adding "..." if truncated.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return s[:maxLen]
	}
	return s[:maxLen-3] + "..."
}
