package service

import (
	"context"
	"testing"

	"github.com/Harshitk-cp/engram/internal/domain"
	"github.com/google/uuid"
)

type mockGraphStore struct {
	edges map[uuid.UUID][]domain.GraphEdge
}

func newMockGraphStore() *mockGraphStore {
	return &mockGraphStore{
		edges: make(map[uuid.UUID][]domain.GraphEdge),
	}
}

func (m *mockGraphStore) CreateEdge(ctx context.Context, edge *domain.GraphEdge) error {
	edge.ID = uuid.New()
	m.edges[edge.SourceID] = append(m.edges[edge.SourceID], *edge)
	return nil
}

func (m *mockGraphStore) GetEdge(ctx context.Context, sourceID, targetID uuid.UUID, relationType domain.RelationType) (*domain.GraphEdge, error) {
	for _, edge := range m.edges[sourceID] {
		if edge.TargetID == targetID && edge.RelationType == relationType {
			return &edge, nil
		}
	}
	return nil, nil
}

func (m *mockGraphStore) DeleteEdge(ctx context.Context, id uuid.UUID) error {
	return nil
}

func (m *mockGraphStore) GetNeighbors(ctx context.Context, memoryID uuid.UUID, direction string, relationTypes []domain.RelationType) ([]domain.GraphEdge, error) {
	var result []domain.GraphEdge
	// Outgoing edges
	for _, edge := range m.edges[memoryID] {
		result = append(result, edge)
	}
	// Incoming edges
	for _, edges := range m.edges {
		for _, edge := range edges {
			if edge.TargetID == memoryID {
				result = append(result, edge)
			}
		}
	}
	return result, nil
}

func (m *mockGraphStore) UpdateTraversalStats(ctx context.Context, id uuid.UUID) error {
	return nil
}

func (m *mockGraphStore) GetEdgesBySource(ctx context.Context, sourceID uuid.UUID) ([]domain.GraphEdge, error) {
	return m.edges[sourceID], nil
}

func (m *mockGraphStore) GetEdgesByTarget(ctx context.Context, targetID uuid.UUID) ([]domain.GraphEdge, error) {
	var result []domain.GraphEdge
	for _, edges := range m.edges {
		for _, edge := range edges {
			if edge.TargetID == targetID {
				result = append(result, edge)
			}
		}
	}
	return result, nil
}

func (m *mockGraphStore) DeleteEdgesByMemory(ctx context.Context, memoryID uuid.UUID) error {
	delete(m.edges, memoryID)
	return nil
}

func (m *mockGraphStore) RecordTraversal(ctx context.Context, edgeID uuid.UUID, strengthBoost float32) error {
	return nil
}

func (m *mockGraphStore) ApplyEdgeDecay(ctx context.Context, agentID uuid.UUID, decayRate float64) (*domain.EdgeDecayResult, error) {
	return &domain.EdgeDecayResult{}, nil
}

func (m *mockGraphStore) PruneGraph(ctx context.Context, agentID uuid.UUID, rules domain.PruningRules) (*domain.EdgeDecayResult, error) {
	return &domain.EdgeDecayResult{}, nil
}

type mockEntityStore struct {
	entities map[uuid.UUID]*domain.Entity
	mentions map[uuid.UUID][]domain.EntityMention
}

func newMockEntityStore() *mockEntityStore {
	return &mockEntityStore{
		entities: make(map[uuid.UUID]*domain.Entity),
		mentions: make(map[uuid.UUID][]domain.EntityMention),
	}
}

func (m *mockEntityStore) Create(ctx context.Context, e *domain.Entity) error {
	e.ID = uuid.New()
	m.entities[e.ID] = e
	return nil
}

func (m *mockEntityStore) GetByID(ctx context.Context, id uuid.UUID) (*domain.Entity, error) {
	if e, ok := m.entities[id]; ok {
		return e, nil
	}
	return nil, nil
}

func (m *mockEntityStore) Delete(ctx context.Context, id uuid.UUID) error {
	delete(m.entities, id)
	return nil
}

func (m *mockEntityStore) FindByName(ctx context.Context, agentID uuid.UUID, name string) (*domain.Entity, error) {
	for _, e := range m.entities {
		if e.AgentID == agentID && e.Name == name {
			return e, nil
		}
	}
	return nil, nil
}

func (m *mockEntityStore) FindByNameOrAlias(ctx context.Context, agentID uuid.UUID, name string) (*domain.Entity, error) {
	return m.FindByName(ctx, agentID, name)
}

func (m *mockEntityStore) GetByAgent(ctx context.Context, agentID uuid.UUID) ([]domain.Entity, error) {
	var result []domain.Entity
	for _, e := range m.entities {
		if e.AgentID == agentID {
			result = append(result, *e)
		}
	}
	return result, nil
}

func (m *mockEntityStore) AddAlias(ctx context.Context, id uuid.UUID, alias string) error {
	if e, ok := m.entities[id]; ok {
		e.Aliases = append(e.Aliases, alias)
	}
	return nil
}

func (m *mockEntityStore) CreateMention(ctx context.Context, mention *domain.EntityMention) error {
	m.mentions[mention.EntityID] = append(m.mentions[mention.EntityID], *mention)
	return nil
}

func (m *mockEntityStore) GetMentionsByEntity(ctx context.Context, entityID uuid.UUID) ([]domain.EntityMention, error) {
	return m.mentions[entityID], nil
}

func (m *mockEntityStore) GetMentionsByMemory(ctx context.Context, memoryID uuid.UUID) ([]domain.EntityMention, error) {
	var result []domain.EntityMention
	for _, mentions := range m.mentions {
		for _, mention := range mentions {
			if mention.MemoryID == memoryID {
				result = append(result, mention)
			}
		}
	}
	return result, nil
}

func (m *mockEntityStore) DeleteMentionsByMemory(ctx context.Context, memoryID uuid.UUID) error {
	return nil
}

func (m *mockEntityStore) GetMemoriesForEntity(ctx context.Context, entityID uuid.UUID, limit int) ([]domain.Memory, error) {
	return nil, nil
}

func (m *mockEntityStore) GetEntitiesForMemory(ctx context.Context, memoryID uuid.UUID) ([]domain.Entity, error) {
	return nil, nil
}

func (m *mockEntityStore) FindByEmbeddingSimilarity(ctx context.Context, agentID uuid.UUID, entityType domain.EntityType, embedding []float32, threshold float32, limit int) ([]domain.Entity, error) {
	return nil, nil
}

func (m *mockEntityStore) UpdateEmbedding(ctx context.Context, id uuid.UUID, embedding []float32) error {
	return nil
}

func TestHybridRecallService_VectorOnlyRecall(t *testing.T) {
	// Setup
	memStore := newMockMemoryStore()
	graphStore := newMockGraphStore()
	entityStore := newMockEntityStore()
	embClient := &mockEmbeddingClient{}
	llmClient := newMockLLMClient()

	svc := NewHybridRecallService(memStore, graphStore, entityStore, embClient, llmClient)

	tenantID := uuid.New()
	agentID := uuid.New()

	// Create test memories
	for i := 0; i < 3; i++ {
		mem := &domain.Memory{
			AgentID:    agentID,
			TenantID:   tenantID,
			Type:       domain.MemoryTypeFact,
			Content:    "Test memory content",
			Confidence: 0.9,
			Embedding:  []float32{0.1, 0.2, 0.3},
		}
		_ = memStore.Create(context.Background(), mem)
	}

	// Test vector-only recall
	req := domain.HybridRecallRequest{
		Query:        "test query",
		AgentID:      agentID,
		TenantID:     tenantID,
		TopK:         10,
		VectorWeight: 1.0,
		GraphWeight:  0.0,
		UseGraph:     false,
	}

	results, err := svc.Recall(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(results) != 3 {
		t.Errorf("expected 3 results, got %d", len(results))
	}

	// All results should have vector score only
	for _, r := range results {
		if r.VectorScore == 0 {
			t.Error("expected non-zero vector score")
		}
		if r.GraphScore != 0 {
			t.Error("expected zero graph score when graph is disabled")
		}
	}
}

func TestHybridRecallService_WithGraphTraversal(t *testing.T) {
	// Setup
	memStore := newMockMemoryStore()
	graphStore := newMockGraphStore()
	entityStore := newMockEntityStore()
	embClient := &mockEmbeddingClient{}
	llmClient := newMockLLMClient()

	svc := NewHybridRecallService(memStore, graphStore, entityStore, embClient, llmClient)

	tenantID := uuid.New()
	agentID := uuid.New()

	// Create test memories
	var memIDs []uuid.UUID
	for i := 0; i < 3; i++ {
		mem := &domain.Memory{
			AgentID:    agentID,
			TenantID:   tenantID,
			Type:       domain.MemoryTypeFact,
			Content:    "Test memory content",
			Confidence: 0.9,
			Embedding:  []float32{0.1, 0.2, 0.3},
		}
		_ = memStore.Create(context.Background(), mem)
		memIDs = append(memIDs, mem.ID)
	}

	// Create graph edges
	edge := &domain.GraphEdge{
		SourceID:     memIDs[0],
		TargetID:     memIDs[1],
		RelationType: domain.RelationThematic,
		Strength:     0.8,
	}
	_ = graphStore.CreateEdge(context.Background(), edge)

	// Test hybrid recall with graph
	req := domain.HybridRecallRequest{
		Query:        "test query",
		AgentID:      agentID,
		TenantID:     tenantID,
		TopK:         10,
		VectorWeight: 0.6,
		GraphWeight:  0.4,
		MaxGraphHops: 2,
		UseGraph:     true,
	}

	results, err := svc.Recall(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(results) == 0 {
		t.Error("expected at least one result")
	}
}

func TestHybridRecallService_DefaultValues(t *testing.T) {
	// Setup
	memStore := newMockMemoryStore()
	graphStore := newMockGraphStore()
	entityStore := newMockEntityStore()
	embClient := &mockEmbeddingClient{}
	llmClient := newMockLLMClient()

	svc := NewHybridRecallService(memStore, graphStore, entityStore, embClient, llmClient)

	tenantID := uuid.New()
	agentID := uuid.New()

	// Test with zero values - should use defaults
	req := domain.HybridRecallRequest{
		Query:    "test query",
		AgentID:  agentID,
		TenantID: tenantID,
		// TopK, VectorWeight, GraphWeight, MaxGraphHops all 0
	}

	_, err := svc.Recall(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestGraphTraversalAlgorithm(t *testing.T) {
	// Setup
	memStore := newMockMemoryStore()
	graphStore := newMockGraphStore()
	entityStore := newMockEntityStore()
	embClient := &mockEmbeddingClient{}
	llmClient := newMockLLMClient()

	svc := NewHybridRecallService(memStore, graphStore, entityStore, embClient, llmClient)

	tenantID := uuid.New()
	agentID := uuid.New()

	// Create a chain of memories: A -> B -> C
	memA := &domain.Memory{AgentID: agentID, TenantID: tenantID, Type: domain.MemoryTypeFact, Content: "A", Confidence: 0.9, Embedding: []float32{0.1}}
	memB := &domain.Memory{AgentID: agentID, TenantID: tenantID, Type: domain.MemoryTypeFact, Content: "B", Confidence: 0.9, Embedding: []float32{0.1}}
	memC := &domain.Memory{AgentID: agentID, TenantID: tenantID, Type: domain.MemoryTypeFact, Content: "C", Confidence: 0.9, Embedding: []float32{0.1}}

	_ = memStore.Create(context.Background(), memA)
	_ = memStore.Create(context.Background(), memB)
	_ = memStore.Create(context.Background(), memC)

	// A -> B (strong)
	_ = graphStore.CreateEdge(context.Background(), &domain.GraphEdge{
		SourceID:     memA.ID,
		TargetID:     memB.ID,
		RelationType: domain.RelationCausal,
		Strength:     0.9,
	})

	// B -> C (weaker)
	_ = graphStore.CreateEdge(context.Background(), &domain.GraphEdge{
		SourceID:     memB.ID,
		TargetID:     memC.ID,
		RelationType: domain.RelationCausal,
		Strength:     0.6,
	})

	// Test traversal from A with 2 hops should reach C
	seeds := []domain.MemoryWithScore{
		{Memory: *memA, Score: 0.9},
	}

	results := svc.traverseGraph(context.Background(), seeds, 2)

	// Should find B (1 hop) and C (2 hops)
	if len(results) < 1 {
		t.Errorf("expected at least 1 graph result, got %d", len(results))
	}

	// Check activation decay
	for _, r := range results {
		if r.GraphRelevance <= 0 || r.GraphRelevance > 1 {
			t.Errorf("graph relevance should be between 0 and 1, got %f", r.GraphRelevance)
		}
	}
}
