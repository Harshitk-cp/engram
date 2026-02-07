package service

import (
	"context"
	"sort"
	"time"

	"github.com/Harshitk-cp/engram/internal/domain"
	"github.com/google/uuid"
)

type HybridRecallService struct {
	memoryStore    domain.MemoryStore
	graphStore     domain.GraphStore
	entityStore    domain.EntityStore
	embeddingClient domain.EmbeddingClient
	llmClient      domain.LLMClient
}

func NewHybridRecallService(
	memoryStore domain.MemoryStore,
	graphStore domain.GraphStore,
	entityStore domain.EntityStore,
	embeddingClient domain.EmbeddingClient,
	llmClient domain.LLMClient,
) *HybridRecallService {
	return &HybridRecallService{
		memoryStore:    memoryStore,
		graphStore:     graphStore,
		entityStore:    entityStore,
		embeddingClient: embeddingClient,
		llmClient:      llmClient,
	}
}

const (
	defaultVectorWeight    = 0.6
	defaultGraphWeight     = 0.4
	defaultMaxHops         = 2
	defaultTopK            = 10
	minActivation          = 0.1
	hopDecay               = 0.7
	traversalStrengthBoost = 0.03
)

func (s *HybridRecallService) Recall(ctx context.Context, req domain.HybridRecallRequest) ([]domain.ScoredMemory, error) {
	// Set defaults
	if req.TopK <= 0 {
		req.TopK = defaultTopK
	}
	if req.VectorWeight == 0 {
		req.VectorWeight = defaultVectorWeight
	}
	if req.GraphWeight == 0 {
		req.GraphWeight = defaultGraphWeight
	}
	if req.MaxGraphHops <= 0 {
		req.MaxGraphHops = defaultMaxHops
	}

	// Step 1: Vector retrieval
	embedding, err := s.embeddingClient.Embed(ctx, req.Query)
	if err != nil {
		return nil, err
	}

	recallOpts := domain.RecallOpts{
		TopK:          req.TopK * 2, // Get more for merging
		MemoryType:    req.MemoryType,
		MinConfidence: req.MinConfidence,
	}
	vectorResults, err := s.memoryStore.Recall(ctx, embedding, req.AgentID, req.TenantID, recallOpts)
	if err != nil {
		return nil, err
	}

	// Convert to scored memories
	scoredResults := make(map[uuid.UUID]*domain.ScoredMemory)
	for _, vr := range vectorResults {
		scoredResults[vr.ID] = &domain.ScoredMemory{
			Memory:      vr.Memory,
			VectorScore: vr.Score,
		}
	}

	// Step 2: Graph traversal (if enabled)
	if req.UseGraph && s.graphStore != nil {
		graphResults := s.traverseGraph(ctx, vectorResults, req.MaxGraphHops)

		for _, gr := range graphResults {
			if existing, ok := scoredResults[gr.MemoryID]; ok {
				existing.GraphScore = gr.GraphRelevance
				existing.PathLength = gr.PathLength
				existing.GraphPath = gr.Path
			} else {
				// Fetch the memory if not in vector results
				mem, err := s.memoryStore.GetByID(ctx, gr.MemoryID, req.TenantID)
				if err == nil && mem != nil {
					scoredResults[gr.MemoryID] = &domain.ScoredMemory{
						Memory:     *mem,
						GraphScore: gr.GraphRelevance,
						PathLength: gr.PathLength,
						GraphPath:  gr.Path,
					}
				}
			}
		}
	}

	// Step 3: Compute final scores and rank
	results := make([]domain.ScoredMemory, 0, len(scoredResults))
	for _, sm := range scoredResults {
		sm.FinalScore = float32(float64(sm.VectorScore)*req.VectorWeight + float64(sm.GraphScore)*req.GraphWeight)
		results = append(results, *sm)
	}

	// Sort by final score descending
	sort.Slice(results, func(i, j int) bool {
		return results[i].FinalScore > results[j].FinalScore
	})

	// Limit to topK
	if len(results) > req.TopK {
		results = results[:req.TopK]
	}

	return results, nil
}

type queueItem struct {
	memoryID   uuid.UUID
	activation float32
	path       []uuid.UUID
	createdAt  *time.Time
}

func (s *HybridRecallService) traverseGraph(ctx context.Context, seeds []domain.MemoryWithScore, maxHops int) []domain.GraphTraversalResult {
	return s.traverseGraphWithConstraints(ctx, seeds, maxHops, domain.TraversalConstraints{})
}

func (s *HybridRecallService) traverseGraphWithConstraints(ctx context.Context, seeds []domain.MemoryWithScore, maxHops int, constraints domain.TraversalConstraints) []domain.GraphTraversalResult {
	visited := make(map[uuid.UUID]bool)
	results := []domain.GraphTraversalResult{}

	// Initialize queue with seed memories
	queue := make([]queueItem, 0, len(seeds))
	for _, seed := range seeds {
		if seed.Score > 0.5 { // Only use high-quality seeds
			queue = append(queue, queueItem{
				memoryID:   seed.ID,
				activation: seed.Score,
				path:       []uuid.UUID{seed.ID},
				createdAt:  &seed.CreatedAt,
			})
		}
	}

	// BFS with activation decay
	for hop := 0; hop < maxHops && len(queue) > 0; hop++ {
		nextQueue := []queueItem{}

		for _, item := range queue {
			if visited[item.memoryID] {
				continue
			}
			visited[item.memoryID] = true

			// Get neighbors (with optional relation filter)
			neighbors, err := s.graphStore.GetNeighbors(ctx, item.memoryID, "both", constraints.RelationFilter)
			if err != nil {
				continue
			}

			for _, edge := range neighbors {
				// Skip weak edges
				if constraints.MinEdgeStrength > 0 && edge.Strength < constraints.MinEdgeStrength {
					continue
				}

				targetID := edge.TargetID
				if targetID == item.memoryID {
					targetID = edge.SourceID
				}

				if visited[targetID] {
					continue
				}

				// Get target memory for temporal constraints
				var targetMem *domain.Memory
				if constraints.RespectTemporalOrder || constraints.MaxAge > 0 {
					targetMem, _ = s.memoryStore.GetByID(ctx, targetID, uuid.Nil) // tenantID not needed for timestamp check
				}

				// Temporal constraint: for causal edges, only traverse forward in time
				if constraints.RespectTemporalOrder && edge.RelationType == domain.RelationCausal {
					if targetMem != nil && item.createdAt != nil && targetMem.CreatedAt.Before(*item.createdAt) {
						continue // Skip backward traversal for causal edges
					}
				}

				// Age constraint
				if constraints.MaxAge > 0 && targetMem != nil {
					if time.Since(targetMem.CreatedAt) > constraints.MaxAge {
						continue
					}
				}

				// Apply type-specific decay multiplier
				typeDecay := float32(hopDecay)
				if multiplier, ok := domain.RelationDecayMultipliers[edge.RelationType]; ok {
					typeDecay = float32(multiplier)
				}

				// Decay activation as we traverse
				newActivation := item.activation * edge.Strength * typeDecay

				if newActivation > minActivation {
					newPath := make([]uuid.UUID, len(item.path)+1)
					copy(newPath, item.path)
					newPath[len(item.path)] = targetID

					var targetCreatedAt *time.Time
					if targetMem != nil {
						targetCreatedAt = &targetMem.CreatedAt
					}

					nextQueue = append(nextQueue, queueItem{
						memoryID:   targetID,
						activation: newActivation,
						path:       newPath,
						createdAt:  targetCreatedAt,
					})

					results = append(results, domain.GraphTraversalResult{
						MemoryID:       targetID,
						GraphRelevance: newActivation,
						PathLength:     hop + 1,
						RelationType:   edge.RelationType,
						Path:           newPath,
					})

					// Record traversal with strength boost
					_ = s.graphStore.RecordTraversal(ctx, edge.ID, traversalStrengthBoost)
				}
			}
		}

		queue = nextQueue
	}

	return results
}

// RecallWithEntities performs hybrid recall starting from entity names
func (s *HybridRecallService) RecallWithEntities(ctx context.Context, req domain.HybridRecallRequest, entityNames []string) ([]domain.ScoredMemory, error) {
	if len(entityNames) == 0 {
		return s.Recall(ctx, req)
	}

	// Find memories linked to these entities
	entityMemories := make(map[uuid.UUID]float32)
	for _, name := range entityNames {
		entity, err := s.entityStore.FindByNameOrAlias(ctx, req.AgentID, name)
		if err != nil {
			continue
		}

		memories, err := s.entityStore.GetMemoriesForEntity(ctx, entity.ID, req.TopK)
		if err != nil {
			continue
		}

		for _, m := range memories {
			if existing, ok := entityMemories[m.ID]; !ok || existing < m.Confidence {
				entityMemories[m.ID] = m.Confidence
			}
		}
	}

	// Do regular hybrid recall
	results, err := s.Recall(ctx, req)
	if err != nil {
		return nil, err
	}

	// Boost scores for entity-linked memories
	for i := range results {
		if conf, ok := entityMemories[results[i].ID]; ok {
			results[i].FinalScore += conf * 0.1 // Small boost for entity match
		}
	}

	// Re-sort
	sort.Slice(results, func(i, j int) bool {
		return results[i].FinalScore > results[j].FinalScore
	})

	return results, nil
}
