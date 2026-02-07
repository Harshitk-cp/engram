package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/Harshitk-cp/engram/internal/api/middleware"
	"github.com/Harshitk-cp/engram/internal/domain"
	"github.com/Harshitk-cp/engram/internal/service"
	"github.com/google/uuid"
)

type GraphHandler struct {
	hybridSvc *service.HybridRecallService
	graphSvc  *service.GraphBuilderService
	graphStore domain.GraphStore
	entityStore domain.EntityStore
}

func NewGraphHandler(
	hybridSvc *service.HybridRecallService,
	graphSvc *service.GraphBuilderService,
	graphStore domain.GraphStore,
	entityStore domain.EntityStore,
) *GraphHandler {
	return &GraphHandler{
		hybridSvc:   hybridSvc,
		graphSvc:    graphSvc,
		graphStore:  graphStore,
		entityStore: entityStore,
	}
}

type listEntitiesResponse struct {
	Entities []entityResponse `json:"entities"`
	Count    int              `json:"count"`
}

type entityResponse struct {
	ID          uuid.UUID `json:"id"`
	Name        string    `json:"name"`
	EntityType  string    `json:"entity_type"`
	Aliases     []string  `json:"aliases,omitempty"`
	MemoryCount int       `json:"memory_count"`
}

func (h *GraphHandler) ListEntities(w http.ResponseWriter, r *http.Request) {
	tenant := middleware.TenantFromContext(r.Context())
	if tenant == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	agentIDStr := r.URL.Query().Get("agent_id")
	if agentIDStr == "" {
		writeError(w, http.StatusBadRequest, "agent_id is required")
		return
	}

	agentID, err := uuid.Parse(agentIDStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid agent_id")
		return
	}

	entities, err := h.entityStore.GetByAgent(r.Context(), agentID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get entities")
		return
	}

	responses := make([]entityResponse, len(entities))
	for i, e := range entities {
		mentions, _ := h.entityStore.GetMentionsByEntity(r.Context(), e.ID)
		responses[i] = entityResponse{
			ID:          e.ID,
			Name:        e.Name,
			EntityType:  string(e.EntityType),
			Aliases:     e.Aliases,
			MemoryCount: len(mentions),
		}
	}

	writeJSON(w, http.StatusOK, listEntitiesResponse{
		Entities: responses,
		Count:    len(responses),
	})
}

type getRelationshipsResponse struct {
	Relationships []relationshipResponse `json:"relationships"`
	Count         int                    `json:"count"`
}

type relationshipResponse struct {
	SourceID       uuid.UUID `json:"source_id"`
	TargetID       uuid.UUID `json:"target_id"`
	RelationType   string    `json:"relation_type"`
	Strength       float32   `json:"strength"`
	TraversalCount int       `json:"traversal_count"`
}

func (h *GraphHandler) GetRelationships(w http.ResponseWriter, r *http.Request) {
	tenant := middleware.TenantFromContext(r.Context())
	if tenant == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	memoryIDStr := r.URL.Query().Get("memory_id")
	if memoryIDStr == "" {
		writeError(w, http.StatusBadRequest, "memory_id is required")
		return
	}

	memoryID, err := uuid.Parse(memoryIDStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid memory_id")
		return
	}

	depthStr := r.URL.Query().Get("depth")
	depth := 1
	if depthStr != "" {
		if d, err := strconv.Atoi(depthStr); err == nil && d > 0 && d <= 3 {
			depth = d
		}
	}

	// Get direct relationships
	edges, err := h.graphStore.GetNeighbors(r.Context(), memoryID, "both", nil)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get relationships")
		return
	}

	responses := make([]relationshipResponse, len(edges))
	for i, edge := range edges {
		responses[i] = relationshipResponse{
			SourceID:       edge.SourceID,
			TargetID:       edge.TargetID,
			RelationType:   string(edge.RelationType),
			Strength:       edge.Strength,
			TraversalCount: edge.TraversalCount,
		}
	}

	// If depth > 1, recursively get more relationships
	if depth > 1 {
		visited := make(map[uuid.UUID]bool)
		visited[memoryID] = true
		for _, edge := range edges {
			visited[edge.SourceID] = true
			visited[edge.TargetID] = true
		}

		for currentDepth := 2; currentDepth <= depth; currentDepth++ {
			var newEdges []domain.GraphEdge
			for id := range visited {
				if id == memoryID {
					continue
				}
				neighbors, err := h.graphStore.GetNeighbors(r.Context(), id, "both", nil)
				if err != nil {
					continue
				}
				for _, edge := range neighbors {
					if !visited[edge.SourceID] || !visited[edge.TargetID] {
						newEdges = append(newEdges, edge)
						visited[edge.SourceID] = true
						visited[edge.TargetID] = true
					}
				}
			}
			for _, edge := range newEdges {
				responses = append(responses, relationshipResponse{
					SourceID:       edge.SourceID,
					TargetID:       edge.TargetID,
					RelationType:   string(edge.RelationType),
					Strength:       edge.Strength,
					TraversalCount: edge.TraversalCount,
				})
			}
		}
	}

	writeJSON(w, http.StatusOK, getRelationshipsResponse{
		Relationships: responses,
		Count:         len(responses),
	})
}

type traverseRequest struct {
	StartIDs      []string `json:"start_ids"`
	RelationTypes []string `json:"relation_types,omitempty"`
	MaxDepth      int      `json:"max_depth"`
}

type traverseResponse struct {
	Paths    []pathResponse         `json:"paths"`
	Memories []graphMemoryResponse  `json:"memories"`
}

type pathResponse struct {
	Path         []uuid.UUID `json:"path"`
	PathLength   int         `json:"path_length"`
	TotalStrength float32    `json:"total_strength"`
}

type graphMemoryResponse struct {
	ID         uuid.UUID `json:"id"`
	Content    string    `json:"content"`
	Type       string    `json:"type"`
	Confidence float32   `json:"confidence"`
}

func (h *GraphHandler) Traverse(w http.ResponseWriter, r *http.Request) {
	tenant := middleware.TenantFromContext(r.Context())
	if tenant == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req traverseRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if len(req.StartIDs) == 0 {
		writeError(w, http.StatusBadRequest, "start_ids is required")
		return
	}

	if req.MaxDepth <= 0 || req.MaxDepth > 5 {
		req.MaxDepth = 2
	}

	startIDs := make([]uuid.UUID, 0, len(req.StartIDs))
	for _, idStr := range req.StartIDs {
		id, err := uuid.Parse(idStr)
		if err != nil {
			continue
		}
		startIDs = append(startIDs, id)
	}

	var relationTypes []domain.RelationType
	for _, rt := range req.RelationTypes {
		if domain.ValidRelationType(rt) {
			relationTypes = append(relationTypes, domain.RelationType(rt))
		}
	}

	// Simple BFS traversal
	visited := make(map[uuid.UUID]bool)
	paths := []pathResponse{}
	memoryIDs := make(map[uuid.UUID]bool)

	type queueItem struct {
		path     []uuid.UUID
		strength float32
	}

	queue := make([]queueItem, 0)
	for _, id := range startIDs {
		queue = append(queue, queueItem{
			path:     []uuid.UUID{id},
			strength: 1.0,
		})
		memoryIDs[id] = true
	}

	for depth := 0; depth < req.MaxDepth && len(queue) > 0; depth++ {
		nextQueue := []queueItem{}

		for _, item := range queue {
			lastID := item.path[len(item.path)-1]
			if visited[lastID] {
				continue
			}
			visited[lastID] = true

			edges, err := h.graphStore.GetNeighbors(r.Context(), lastID, "both", relationTypes)
			if err != nil {
				continue
			}

			for _, edge := range edges {
				targetID := edge.TargetID
				if targetID == lastID {
					targetID = edge.SourceID
				}

				if visited[targetID] {
					continue
				}

				newPath := make([]uuid.UUID, len(item.path)+1)
				copy(newPath, item.path)
				newPath[len(item.path)] = targetID

				newStrength := item.strength * edge.Strength

				nextQueue = append(nextQueue, queueItem{
					path:     newPath,
					strength: newStrength,
				})

				memoryIDs[targetID] = true

				paths = append(paths, pathResponse{
					Path:          newPath,
					PathLength:    len(newPath),
					TotalStrength: newStrength,
				})
			}
		}

		queue = nextQueue
	}

	writeJSON(w, http.StatusOK, traverseResponse{
		Paths:    paths,
		Memories: []graphMemoryResponse{}, // Caller can fetch memories by ID if needed
	})
}

func (h *GraphHandler) HybridRecall(w http.ResponseWriter, r *http.Request) {
	tenant := middleware.TenantFromContext(r.Context())
	if tenant == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	query := r.URL.Query().Get("query")
	if query == "" {
		writeError(w, http.StatusBadRequest, "query is required")
		return
	}

	agentIDStr := r.URL.Query().Get("agent_id")
	if agentIDStr == "" {
		writeError(w, http.StatusBadRequest, "agent_id is required")
		return
	}

	agentID, err := uuid.Parse(agentIDStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid agent_id")
		return
	}

	topK := 10
	if topKStr := r.URL.Query().Get("top_k"); topKStr != "" {
		if k, err := strconv.Atoi(topKStr); err == nil && k > 0 && k <= 100 {
			topK = k
		}
	}

	useGraph := true
	if useGraphStr := r.URL.Query().Get("use_graph"); useGraphStr == "false" {
		useGraph = false
	}

	graphWeight := 0.4
	if gwStr := r.URL.Query().Get("graph_weight"); gwStr != "" {
		if gw, err := strconv.ParseFloat(gwStr, 64); err == nil && gw >= 0 && gw <= 1 {
			graphWeight = gw
		}
	}

	maxHops := 2
	if mhStr := r.URL.Query().Get("max_hops"); mhStr != "" {
		if mh, err := strconv.Atoi(mhStr); err == nil && mh > 0 && mh <= 5 {
			maxHops = mh
		}
	}

	req := domain.HybridRecallRequest{
		Query:        query,
		AgentID:      agentID,
		TenantID:     tenant.ID,
		TopK:         topK,
		VectorWeight: 1 - graphWeight,
		GraphWeight:  graphWeight,
		MaxGraphHops: maxHops,
		UseGraph:     useGraph,
	}

	results, err := h.hybridSvc.Recall(r.Context(), req)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to perform hybrid recall")
		return
	}

	type recallResponse struct {
		Memories []domain.ScoredMemory `json:"memories"`
		Query    string                `json:"query"`
		Count    int                   `json:"count"`
	}

	writeJSON(w, http.StatusOK, recallResponse{
		Memories: results,
		Query:    query,
		Count:    len(results),
	})
}
