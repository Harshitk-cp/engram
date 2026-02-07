package service

import (
	"context"
	"math"

	"github.com/Harshitk-cp/engram/internal/domain"
	"github.com/google/uuid"
)

const (
	entityEmbeddingSimilarityThreshold = 0.85
)

type GraphBuilderService struct {
	memoryStore     domain.MemoryStore
	graphStore      domain.GraphStore
	entityStore     domain.EntityStore
	embeddingClient domain.EmbeddingClient
	llmClient       domain.LLMClient
}

func NewGraphBuilderService(
	memoryStore domain.MemoryStore,
	graphStore domain.GraphStore,
	entityStore domain.EntityStore,
	embeddingClient domain.EmbeddingClient,
	llmClient domain.LLMClient,
) *GraphBuilderService {
	return &GraphBuilderService{
		memoryStore:     memoryStore,
		graphStore:      graphStore,
		entityStore:     entityStore,
		embeddingClient: embeddingClient,
		llmClient:       llmClient,
	}
}

const (
	thematicSimilarityThreshold = 0.8
	similarMemoryLimit          = 5
)

func (s *GraphBuilderService) OnMemoryCreated(ctx context.Context, memory *domain.Memory) error {
	if memory == nil {
		return nil
	}

	// 1. Extract entities
	if err := s.extractAndLinkEntities(ctx, memory); err != nil {
		// Log but don't fail
	}

	// 2. Find similar memories and create thematic links
	if err := s.createThematicLinks(ctx, memory); err != nil {
		// Log but don't fail
	}

	// 3. Detect and create relationship edges
	if err := s.detectAndCreateRelationships(ctx, memory); err != nil {
		// Log but don't fail
	}

	return nil
}

func (s *GraphBuilderService) extractAndLinkEntities(ctx context.Context, memory *domain.Memory) error {
	entities, err := s.llmClient.ExtractEntities(ctx, memory.Content)
	if err != nil {
		return err
	}

	for _, extracted := range entities {
		entity, err := s.findOrCreateEntity(ctx, memory.AgentID, extracted)
		if err != nil || entity == nil {
			continue
		}

		// Link entity to memory
		mention := &domain.EntityMention{
			EntityID:    entity.ID,
			MemoryID:    memory.ID,
			MentionType: extracted.Role,
		}
		if err := s.entityStore.CreateMention(ctx, mention); err != nil {
			continue
		}

		// Create entity_link edges between memories that share this entity
		if err := s.linkMemoriesByEntity(ctx, memory, entity, extracted.Role); err != nil {
			continue
		}
	}

	return nil
}

// findOrCreateEntity uses exact match, then alias match, then embedding similarity
func (s *GraphBuilderService) findOrCreateEntity(ctx context.Context, agentID uuid.UUID, extracted domain.ExtractedEntity) (*domain.Entity, error) {
	// 1. Exact match on name or alias
	entity, err := s.entityStore.FindByNameOrAlias(ctx, agentID, extracted.Name)
	if err == nil && entity != nil {
		return entity, nil
	}

	// 2. Fuzzy match via embedding similarity
	if s.embeddingClient != nil {
		nameEmb, embErr := s.embeddingClient.Embed(ctx, extracted.Name)
		if embErr == nil && len(nameEmb) > 0 {
			candidates, findErr := s.entityStore.FindByEmbeddingSimilarity(
				ctx, agentID, extracted.EntityType, nameEmb, entityEmbeddingSimilarityThreshold, 1,
			)
			if findErr == nil && len(candidates) > 0 {
				// Add as alias to existing entity
				_ = s.entityStore.AddAlias(ctx, candidates[0].ID, extracted.Name)
				return &candidates[0], nil
			}

			// 3. Create new entity with embedding
			entity = &domain.Entity{
				AgentID:    agentID,
				Name:       extracted.Name,
				EntityType: extracted.EntityType,
				Aliases:    []string{},
				Embedding:  nameEmb,
			}
			if err := s.entityStore.Create(ctx, entity); err != nil {
				return nil, err
			}
			return entity, nil
		}
	}

	// 3. Create new entity without embedding
	entity = &domain.Entity{
		AgentID:    agentID,
		Name:       extracted.Name,
		EntityType: extracted.EntityType,
		Aliases:    []string{},
	}
	if err := s.entityStore.Create(ctx, entity); err != nil {
		return nil, err
	}
	return entity, nil
}

func (s *GraphBuilderService) linkMemoriesByEntity(ctx context.Context, memory *domain.Memory, entity *domain.Entity, currentMentionType domain.MentionType) error {
	mentions, err := s.entityStore.GetMentionsByEntity(ctx, entity.ID)
	if err != nil {
		return err
	}

	currentWeight := domain.MentionTypeWeights[currentMentionType]

	for _, mention := range mentions {
		if mention.MemoryID == memory.ID {
			continue
		}

		// Strength = geometric mean of both mention weights
		otherWeight := domain.MentionTypeWeights[mention.MentionType]
		strength := float32(math.Sqrt(currentWeight * otherWeight))

		// Create entity_link edge (symmetric edge created automatically by store)
		edge := &domain.GraphEdge{
			SourceID:     memory.ID,
			TargetID:     mention.MemoryID,
			RelationType: domain.RelationEntityLink,
			Strength:     strength,
		}
		_ = s.graphStore.CreateEdge(ctx, edge)
	}

	return nil
}

func (s *GraphBuilderService) createThematicLinks(ctx context.Context, memory *domain.Memory) error {
	if len(memory.Embedding) == 0 {
		// Generate embedding if not present
		embedding, err := s.embeddingClient.Embed(ctx, memory.Content)
		if err != nil {
			return err
		}
		memory.Embedding = embedding
	}

	// Find similar memories
	similar, err := s.memoryStore.FindSimilar(ctx, memory.AgentID, memory.TenantID, memory.Embedding, thematicSimilarityThreshold)
	if err != nil {
		return err
	}

	for _, sim := range similar {
		if sim.ID == memory.ID {
			continue
		}

		// Create thematic link with similarity as strength
		edge := &domain.GraphEdge{
			SourceID:     memory.ID,
			TargetID:     sim.ID,
			RelationType: domain.RelationThematic,
			Strength:     sim.Score,
		}
		_ = s.graphStore.CreateEdge(ctx, edge)
	}

	return nil
}

func (s *GraphBuilderService) detectAndCreateRelationships(ctx context.Context, memory *domain.Memory) error {
	if len(memory.Embedding) == 0 {
		embedding, err := s.embeddingClient.Embed(ctx, memory.Content)
		if err != nil {
			return err
		}
		memory.Embedding = embedding
	}

	// Get similar memories for relationship detection
	similar, err := s.memoryStore.FindSimilar(ctx, memory.AgentID, memory.TenantID, memory.Embedding, 0.5)
	if err != nil {
		return err
	}

	if len(similar) == 0 {
		return nil
	}

	// Limit to top N for LLM call
	if len(similar) > similarMemoryLimit {
		similar = similar[:similarMemoryLimit]
	}

	// Use LLM to detect relationships
	relationships, err := s.llmClient.DetectRelationships(ctx, memory, similar)
	if err != nil {
		return err
	}

	for _, rel := range relationships {
		edge := &domain.GraphEdge{
			SourceID:     rel.SourceID,
			TargetID:     rel.TargetID,
			RelationType: rel.RelationType,
			Strength:     rel.Strength,
		}
		_ = s.graphStore.CreateEdge(ctx, edge)
	}

	return nil
}

func (s *GraphBuilderService) OnMemoryDeleted(ctx context.Context, memoryID uuid.UUID) error {
	// Delete all graph edges involving this memory
	if err := s.graphStore.DeleteEdgesByMemory(ctx, memoryID); err != nil {
		return err
	}

	// Delete entity mentions for this memory
	if err := s.entityStore.DeleteMentionsByMemory(ctx, memoryID); err != nil {
		return err
	}

	return nil
}
