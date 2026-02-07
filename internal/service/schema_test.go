package service

import (
	"context"
	"testing"
	"time"

	"github.com/Harshitk-cp/engram/internal/domain"
	"github.com/Harshitk-cp/engram/internal/store"
	"github.com/google/uuid"
)

// mockSchemaStore implements domain.SchemaStore for testing.
type mockSchemaStore struct {
	schemas map[uuid.UUID]*domain.Schema
}

func newMockSchemaStore() *mockSchemaStore {
	return &mockSchemaStore{schemas: make(map[uuid.UUID]*domain.Schema)}
}

func (m *mockSchemaStore) Create(ctx context.Context, s *domain.Schema) error {
	s.ID = uuid.New()
	now := time.Now()
	s.CreatedAt = now
	s.UpdatedAt = now
	m.schemas[s.ID] = s
	return nil
}

func (m *mockSchemaStore) GetByID(ctx context.Context, id uuid.UUID, tenantID uuid.UUID) (*domain.Schema, error) {
	s, ok := m.schemas[id]
	if !ok || s.TenantID != tenantID {
		return nil, store.ErrNotFound
	}
	return s, nil
}

func (m *mockSchemaStore) GetByAgent(ctx context.Context, agentID uuid.UUID, tenantID uuid.UUID) ([]domain.Schema, error) {
	var results []domain.Schema
	for _, s := range m.schemas {
		if s.AgentID == agentID && s.TenantID == tenantID {
			results = append(results, *s)
		}
	}
	return results, nil
}

func (m *mockSchemaStore) GetByName(ctx context.Context, agentID uuid.UUID, tenantID uuid.UUID, schemaType domain.SchemaType, name string) (*domain.Schema, error) {
	for _, s := range m.schemas {
		if s.AgentID == agentID && s.TenantID == tenantID && s.SchemaType == schemaType && s.Name == name {
			return s, nil
		}
	}
	return nil, store.ErrNotFound
}

func (m *mockSchemaStore) Delete(ctx context.Context, id uuid.UUID, tenantID uuid.UUID) error {
	s, ok := m.schemas[id]
	if !ok || s.TenantID != tenantID {
		return store.ErrNotFound
	}
	delete(m.schemas, id)
	return nil
}

func (m *mockSchemaStore) FindSimilar(ctx context.Context, agentID uuid.UUID, tenantID uuid.UUID, embedding []float32, threshold float32, limit int) ([]domain.SchemaWithScore, error) {
	return []domain.SchemaWithScore{}, nil
}

func (m *mockSchemaStore) AddEvidence(ctx context.Context, id uuid.UUID, memoryID *uuid.UUID, episodeID *uuid.UUID) error {
	s, ok := m.schemas[id]
	if !ok {
		return store.ErrNotFound
	}
	if memoryID != nil {
		s.EvidenceMemories = append(s.EvidenceMemories, *memoryID)
		s.EvidenceCount++
	}
	if episodeID != nil {
		s.EvidenceEpisodes = append(s.EvidenceEpisodes, *episodeID)
		s.EvidenceCount++
	}
	return nil
}

func (m *mockSchemaStore) RemoveEvidence(ctx context.Context, id uuid.UUID, memoryID *uuid.UUID, episodeID *uuid.UUID) error {
	s, ok := m.schemas[id]
	if !ok {
		return store.ErrNotFound
	}
	s.EvidenceCount--
	if s.EvidenceCount < 0 {
		s.EvidenceCount = 0
	}
	return nil
}

func (m *mockSchemaStore) UpdateConfidence(ctx context.Context, id uuid.UUID, confidence float32) error {
	s, ok := m.schemas[id]
	if !ok {
		return store.ErrNotFound
	}
	s.Confidence = confidence
	return nil
}

func (m *mockSchemaStore) IncrementContradiction(ctx context.Context, id uuid.UUID) error {
	s, ok := m.schemas[id]
	if !ok {
		return store.ErrNotFound
	}
	s.ContradictionCount++
	return nil
}

func (m *mockSchemaStore) UpdateValidation(ctx context.Context, id uuid.UUID) error {
	s, ok := m.schemas[id]
	if !ok {
		return store.ErrNotFound
	}
	now := time.Now()
	s.LastValidatedAt = &now
	return nil
}

func (m *mockSchemaStore) Update(ctx context.Context, s *domain.Schema) error {
	_, ok := m.schemas[s.ID]
	if !ok {
		return store.ErrNotFound
	}
	m.schemas[s.ID] = s
	return nil
}

// mockMemoryStoreForSchema implements domain.MemoryStore for schema testing.
type mockMemoryStoreForSchema struct {
	memories map[uuid.UUID]*domain.Memory
}

func newMockMemoryStoreForSchema() *mockMemoryStoreForSchema {
	return &mockMemoryStoreForSchema{memories: make(map[uuid.UUID]*domain.Memory)}
}

func (m *mockMemoryStoreForSchema) Create(ctx context.Context, mem *domain.Memory) error {
	mem.ID = uuid.New()
	m.memories[mem.ID] = mem
	return nil
}

func (m *mockMemoryStoreForSchema) GetByID(ctx context.Context, id uuid.UUID, tenantID uuid.UUID) (*domain.Memory, error) {
	mem, ok := m.memories[id]
	if !ok || mem.TenantID != tenantID {
		return nil, store.ErrNotFound
	}
	return mem, nil
}

func (m *mockMemoryStoreForSchema) Delete(ctx context.Context, id uuid.UUID, tenantID uuid.UUID) error {
	delete(m.memories, id)
	return nil
}

func (m *mockMemoryStoreForSchema) Recall(ctx context.Context, embedding []float32, agentID uuid.UUID, tenantID uuid.UUID, opts domain.RecallOpts) ([]domain.MemoryWithScore, error) {
	return []domain.MemoryWithScore{}, nil
}

func (m *mockMemoryStoreForSchema) CountByAgentAndType(ctx context.Context, agentID uuid.UUID, memType domain.MemoryType) (int, error) {
	return 0, nil
}

func (m *mockMemoryStoreForSchema) ListOldestByAgentAndType(ctx context.Context, agentID uuid.UUID, memType domain.MemoryType, limit int) ([]domain.Memory, error) {
	return nil, nil
}

func (m *mockMemoryStoreForSchema) DeleteExpired(ctx context.Context) (int64, error) {
	return 0, nil
}

func (m *mockMemoryStoreForSchema) DeleteByRetention(ctx context.Context, agentID uuid.UUID, memType domain.MemoryType, retentionDays int) (int64, error) {
	return 0, nil
}

func (m *mockMemoryStoreForSchema) FindSimilar(ctx context.Context, agentID uuid.UUID, tenantID uuid.UUID, embedding []float32, threshold float32) ([]domain.MemoryWithScore, error) {
	return []domain.MemoryWithScore{}, nil
}

func (m *mockMemoryStoreForSchema) UpdateReinforcement(ctx context.Context, id uuid.UUID, confidence float32, reinforcementCount int) error {
	return nil
}

func (m *mockMemoryStoreForSchema) UpdateConfidence(ctx context.Context, id uuid.UUID, confidence float32) error {
	return nil
}

func (m *mockMemoryStoreForSchema) ListDistinctAgentIDs(ctx context.Context) ([]uuid.UUID, error) {
	return nil, nil
}

func (m *mockMemoryStoreForSchema) GetByAgentForDecay(ctx context.Context, agentID uuid.UUID) ([]domain.Memory, error) {
	var results []domain.Memory
	for _, mem := range m.memories {
		if mem.AgentID == agentID {
			results = append(results, *mem)
		}
	}
	return results, nil
}

func (m *mockMemoryStoreForSchema) Archive(ctx context.Context, id uuid.UUID) error {
	return nil
}

func (m *mockMemoryStoreForSchema) IncrementAccessAndBoost(ctx context.Context, id uuid.UUID, boost float32) error {
	return nil
}

func (m *mockMemoryStoreForSchema) GetByTier(ctx context.Context, agentID uuid.UUID, tenantID uuid.UUID, tier domain.MemoryTier, limit int) ([]domain.Memory, error) {
	return nil, nil
}

func (m *mockMemoryStoreForSchema) GetTierCounts(ctx context.Context, agentID uuid.UUID, tenantID uuid.UUID) (map[domain.MemoryTier]int, error) {
	return nil, nil
}

func (m *mockMemoryStoreForSchema) SetNeedsReview(ctx context.Context, id uuid.UUID, needsReview bool) error {
	return nil
}

func (m *mockMemoryStoreForSchema) GetNeedsReview(ctx context.Context, agentID uuid.UUID, tenantID uuid.UUID, limit int) ([]domain.Memory, error) {
	return nil, nil
}

func setupSchemaTest() (*SchemaService, *mockSchemaStore, *mockMemoryStoreForSchema, uuid.UUID, uuid.UUID) {
	agentStore := newMockAgentStore()
	schemaStore := newMockSchemaStore()
	memoryStore := newMockMemoryStoreForSchema()
	embClient := &mockEmbeddingClient{}
	llmClient := newMockLLMClient()
	svc := NewSchemaService(schemaStore, memoryStore, agentStore, embClient, llmClient, testLogger())

	tenantID := uuid.New()
	agent := &domain.Agent{
		TenantID:   tenantID,
		ExternalID: "bot-1",
		Name:       "Test Bot",
	}
	_ = agentStore.Create(context.Background(), agent)

	return svc, schemaStore, memoryStore, tenantID, agent.ID
}

func TestSchemaService_GetByID(t *testing.T) {
	svc, schemaStore, _, tenantID, agentID := setupSchemaTest()
	ctx := context.Background()

	// Create a schema directly in the store
	schema := &domain.Schema{
		AgentID:            agentID,
		TenantID:           tenantID,
		SchemaType:         domain.SchemaTypeUserArchetype,
		Name:               "Technical Expert",
		Description:        "User with deep technical knowledge",
		Attributes:         map[string]any{"expertise_level": "expert"},
		Confidence:         0.8,
		ApplicableContexts: []string{"debugging", "code_review"},
	}
	_ = schemaStore.Create(ctx, schema)

	// Get the schema
	found, err := svc.GetByID(ctx, schema.ID, tenantID)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if found.ID != schema.ID {
		t.Fatalf("expected ID %s, got %s", schema.ID, found.ID)
	}
	if found.Name != schema.Name {
		t.Fatalf("expected name %q, got %q", schema.Name, found.Name)
	}
	if found.SchemaType != schema.SchemaType {
		t.Fatalf("expected schema_type %q, got %q", schema.SchemaType, found.SchemaType)
	}
}

func TestSchemaService_GetByID_NotFound(t *testing.T) {
	svc, _, _, tenantID, _ := setupSchemaTest()
	ctx := context.Background()

	_, err := svc.GetByID(ctx, uuid.New(), tenantID)
	if err != ErrSchemaNotFound {
		t.Fatalf("expected ErrSchemaNotFound, got %v", err)
	}
}

func TestSchemaService_GetByAgent(t *testing.T) {
	svc, schemaStore, _, tenantID, agentID := setupSchemaTest()
	ctx := context.Background()

	// Create multiple schemas
	schema1 := &domain.Schema{
		AgentID:    agentID,
		TenantID:   tenantID,
		SchemaType: domain.SchemaTypeUserArchetype,
		Name:       "Technical Expert",
		Confidence: 0.8,
	}
	schema2 := &domain.Schema{
		AgentID:    agentID,
		TenantID:   tenantID,
		SchemaType: domain.SchemaTypeSituationTemplate,
		Name:       "Debugging Session",
		Confidence: 0.7,
	}
	_ = schemaStore.Create(ctx, schema1)
	_ = schemaStore.Create(ctx, schema2)

	// Get all schemas
	schemas, err := svc.GetByAgent(ctx, agentID, tenantID)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(schemas) != 2 {
		t.Fatalf("expected 2 schemas, got %d", len(schemas))
	}
}

func TestSchemaService_Delete(t *testing.T) {
	svc, schemaStore, _, tenantID, agentID := setupSchemaTest()
	ctx := context.Background()

	// Create a schema
	schema := &domain.Schema{
		AgentID:    agentID,
		TenantID:   tenantID,
		SchemaType: domain.SchemaTypeUserArchetype,
		Name:       "To Be Deleted",
		Confidence: 0.5,
	}
	_ = schemaStore.Create(ctx, schema)

	// Delete the schema
	err := svc.Delete(ctx, schema.ID, tenantID)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// Verify it's deleted
	_, err = svc.GetByID(ctx, schema.ID, tenantID)
	if err != ErrSchemaNotFound {
		t.Fatalf("expected ErrSchemaNotFound after delete, got %v", err)
	}
}

func TestSchemaService_Delete_NotFound(t *testing.T) {
	svc, _, _, tenantID, _ := setupSchemaTest()
	ctx := context.Background()

	err := svc.Delete(ctx, uuid.New(), tenantID)
	if err != ErrSchemaNotFound {
		t.Fatalf("expected ErrSchemaNotFound, got %v", err)
	}
}

func TestSchemaService_RecordContradiction(t *testing.T) {
	svc, schemaStore, _, tenantID, agentID := setupSchemaTest()
	ctx := context.Background()

	// Create a schema with high confidence
	schema := &domain.Schema{
		AgentID:    agentID,
		TenantID:   tenantID,
		SchemaType: domain.SchemaTypeUserArchetype,
		Name:       "Test Schema",
		Confidence: 0.8,
	}
	_ = schemaStore.Create(ctx, schema)

	// Record contradiction
	err := svc.RecordContradiction(ctx, schema.ID, tenantID)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// Verify contradiction count increased
	updated := schemaStore.schemas[schema.ID]
	if updated.ContradictionCount != 1 {
		t.Fatalf("expected contradiction_count 1, got %d", updated.ContradictionCount)
	}

	// Verify confidence decreased
	if updated.Confidence >= 0.8 {
		t.Fatalf("expected confidence to decrease from 0.8, got %f", updated.Confidence)
	}
}

func TestSchemaService_RecordContradiction_NotFound(t *testing.T) {
	svc, _, _, tenantID, _ := setupSchemaTest()
	ctx := context.Background()

	err := svc.RecordContradiction(ctx, uuid.New(), tenantID)
	if err != ErrSchemaNotFound {
		t.Fatalf("expected ErrSchemaNotFound, got %v", err)
	}
}

func TestSchemaService_ValidateSchema(t *testing.T) {
	svc, schemaStore, _, tenantID, agentID := setupSchemaTest()
	ctx := context.Background()

	// Create a schema
	schema := &domain.Schema{
		AgentID:    agentID,
		TenantID:   tenantID,
		SchemaType: domain.SchemaTypeUserArchetype,
		Name:       "Test Schema",
		Confidence: 0.7,
	}
	_ = schemaStore.Create(ctx, schema)

	// Validate schema
	err := svc.ValidateSchema(ctx, schema.ID, tenantID)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// Verify validation timestamp was updated
	updated := schemaStore.schemas[schema.ID]
	if updated.LastValidatedAt == nil {
		t.Fatal("expected last_validated_at to be set")
	}
}

func TestSchemaService_ValidateSchema_NotFound(t *testing.T) {
	svc, _, _, tenantID, _ := setupSchemaTest()
	ctx := context.Background()

	err := svc.ValidateSchema(ctx, uuid.New(), tenantID)
	if err != ErrSchemaNotFound {
		t.Fatalf("expected ErrSchemaNotFound, got %v", err)
	}
}

func TestSchemaService_MatchSchemas_Empty(t *testing.T) {
	svc, _, _, tenantID, agentID := setupSchemaTest()
	ctx := context.Background()

	input := SchemaMatchInput{
		AgentID:  agentID,
		TenantID: tenantID,
		Query:    "Help me debug this code",
		Contexts: []string{"debugging"},
	}

	matches, err := svc.MatchSchemas(ctx, input)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(matches) != 0 {
		t.Fatalf("expected 0 matches, got %d", len(matches))
	}
}

func TestSchemaService_MatchSchemas_WithContextMatch(t *testing.T) {
	svc, schemaStore, _, tenantID, agentID := setupSchemaTest()
	ctx := context.Background()

	// Create a schema with applicable contexts
	schema := &domain.Schema{
		AgentID:            agentID,
		TenantID:           tenantID,
		SchemaType:         domain.SchemaTypeUserArchetype,
		Name:               "Debug Expert",
		Confidence:         0.9,
		ApplicableContexts: []string{"debugging", "troubleshooting"},
	}
	_ = schemaStore.Create(ctx, schema)

	input := SchemaMatchInput{
		AgentID:       agentID,
		TenantID:      tenantID,
		Query:         "Help me debug",
		Contexts:      []string{"debugging"},
		MinMatchScore: 0.1, // Low threshold for testing
	}

	matches, err := svc.MatchSchemas(ctx, input)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("expected 1 match, got %d", len(matches))
	}
	if matches[0].Schema.ID != schema.ID {
		t.Fatalf("expected schema ID %s, got %s", schema.ID, matches[0].Schema.ID)
	}
}

func TestSchemaService_DetectSchemas_InsufficientMemories(t *testing.T) {
	svc, _, memoryStore, tenantID, agentID := setupSchemaTest()
	ctx := context.Background()

	// Create fewer memories than MinClusterSize
	_ = memoryStore.Create(ctx, &domain.Memory{
		AgentID:  agentID,
		TenantID: tenantID,
		Type:     domain.MemoryTypePreference,
		Content:  "User prefers dark mode",
	})

	schemas, err := svc.DetectSchemas(ctx, agentID, tenantID)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(schemas) != 0 {
		t.Fatalf("expected 0 schemas due to insufficient memories, got %d", len(schemas))
	}
}

func TestCosineSimilarity(t *testing.T) {
	// Test identical vectors
	a := []float32{1, 0, 0}
	b := []float32{1, 0, 0}
	similarity := cosineSimilarity(a, b)
	if similarity < 0.99 {
		t.Fatalf("expected similarity ~1.0 for identical vectors, got %f", similarity)
	}

	// Test orthogonal vectors
	a = []float32{1, 0, 0}
	b = []float32{0, 1, 0}
	similarity = cosineSimilarity(a, b)
	if similarity > 0.01 {
		t.Fatalf("expected similarity ~0.0 for orthogonal vectors, got %f", similarity)
	}

	// Test opposite vectors
	a = []float32{1, 0, 0}
	b = []float32{-1, 0, 0}
	similarity = cosineSimilarity(a, b)
	if similarity > -0.99 {
		t.Fatalf("expected similarity ~-1.0 for opposite vectors, got %f", similarity)
	}

	// Test empty vectors
	similarity = cosineSimilarity([]float32{}, []float32{})
	if similarity != 0 {
		t.Fatalf("expected similarity 0 for empty vectors, got %f", similarity)
	}

	// Test different length vectors
	a = []float32{1, 0}
	b = []float32{1, 0, 0}
	similarity = cosineSimilarity(a, b)
	if similarity != 0 {
		t.Fatalf("expected similarity 0 for different length vectors, got %f", similarity)
	}
}

func TestAverageVectors(t *testing.T) {
	a := []float32{2, 4, 6}
	b := []float32{4, 2, 0}

	result := averageVectors(a, b)
	expected := []float32{3, 3, 3}

	if len(result) != len(expected) {
		t.Fatalf("expected length %d, got %d", len(expected), len(result))
	}

	for i := range result {
		if result[i] != expected[i] {
			t.Fatalf("expected result[%d] = %f, got %f", i, expected[i], result[i])
		}
	}

	// Test different length vectors (should return first)
	a = []float32{1, 2}
	b = []float32{1, 2, 3}
	result = averageVectors(a, b)
	if len(result) != len(a) {
		t.Fatalf("expected to return first vector for different lengths")
	}
}

func TestCalculateInitialConfidence(t *testing.T) {
	svc, _, _, _, _ := setupSchemaTest()

	// Test minimum cluster size
	confidence := svc.calculateInitialConfidence(3)
	expectedMin := float32(0.5 + 3*0.05) // 0.65
	if confidence != expectedMin {
		t.Fatalf("expected confidence %f for 3 items, got %f", expectedMin, confidence)
	}

	// Test large evidence count (should cap at 0.8)
	confidence = svc.calculateInitialConfidence(100)
	if confidence != 0.8 {
		t.Fatalf("expected confidence to cap at 0.8, got %f", confidence)
	}
}

func TestScoreContextMatch(t *testing.T) {
	svc, _, _, _, _ := setupSchemaTest()

	// Test exact match
	schemaContexts := []string{"debugging", "code_review"}
	inputContexts := []string{"debugging"}
	score := svc.scoreContextMatch(schemaContexts, inputContexts)
	if score != 0.5 { // 1 match out of 2 schema contexts
		t.Fatalf("expected score 0.5 for 1/2 match, got %f", score)
	}

	// Test full match
	inputContexts = []string{"debugging", "code_review"}
	score = svc.scoreContextMatch(schemaContexts, inputContexts)
	if score != 1.0 {
		t.Fatalf("expected score 1.0 for full match, got %f", score)
	}

	// Test no match
	inputContexts = []string{"other_context"}
	score = svc.scoreContextMatch(schemaContexts, inputContexts)
	if score != 0 {
		t.Fatalf("expected score 0 for no match, got %f", score)
	}

	// Test empty contexts
	score = svc.scoreContextMatch([]string{}, inputContexts)
	if score != 0 {
		t.Fatalf("expected score 0 for empty schema contexts, got %f", score)
	}
}

func TestScoreTimeMatch(t *testing.T) {
	svc, _, _, _, _ := setupSchemaTest()

	// Test matching time preference
	attributes := map[string]any{"time_preference": "night"}
	score := svc.scoreTimeMatch(attributes, "night")
	if score != 1.0 {
		t.Fatalf("expected score 1.0 for matching time preference, got %f", score)
	}

	// Test non-matching time
	score = svc.scoreTimeMatch(attributes, "morning")
	if score != 0 {
		t.Fatalf("expected score 0 for non-matching time, got %f", score)
	}

	// Test work_hours attribute
	attributes = map[string]any{"work_hours": "late night"}
	score = svc.scoreTimeMatch(attributes, "night")
	if score != 0.8 {
		t.Fatalf("expected score 0.8 for work_hours containing time, got %f", score)
	}

	// Test nil attributes
	score = svc.scoreTimeMatch(nil, "night")
	if score != 0 {
		t.Fatalf("expected score 0 for nil attributes, got %f", score)
	}

	// Test empty time of day
	score = svc.scoreTimeMatch(attributes, "")
	if score != 0 {
		t.Fatalf("expected score 0 for empty time_of_day, got %f", score)
	}
}
