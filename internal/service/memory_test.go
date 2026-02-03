package service

import (
	"context"
	"testing"

	"github.com/Harshitk-cp/engram/internal/domain"
	"github.com/Harshitk-cp/engram/internal/store"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// mockMemoryStore implements domain.MemoryStore for testing.
type mockMemoryStore struct {
	memories map[uuid.UUID]*domain.Memory
}

func newMockMemoryStore() *mockMemoryStore {
	return &mockMemoryStore{memories: make(map[uuid.UUID]*domain.Memory)}
}

func (m *mockMemoryStore) Create(ctx context.Context, mem *domain.Memory) error {
	mem.ID = uuid.New()
	m.memories[mem.ID] = mem
	return nil
}

func (m *mockMemoryStore) GetByID(ctx context.Context, id uuid.UUID, tenantID uuid.UUID) (*domain.Memory, error) {
	mem, ok := m.memories[id]
	if !ok || mem.TenantID != tenantID {
		return nil, store.ErrNotFound
	}
	return mem, nil
}

func (m *mockMemoryStore) Delete(ctx context.Context, id uuid.UUID, tenantID uuid.UUID) error {
	mem, ok := m.memories[id]
	if !ok || mem.TenantID != tenantID {
		return store.ErrNotFound
	}
	delete(m.memories, id)
	return nil
}

func (m *mockMemoryStore) Recall(ctx context.Context, emb []float32, agentID uuid.UUID, tenantID uuid.UUID, opts domain.RecallOpts) ([]domain.MemoryWithScore, error) {
	var results []domain.MemoryWithScore
	for _, mem := range m.memories {
		if mem.AgentID != agentID || mem.TenantID != tenantID {
			continue
		}
		if opts.MemoryType != nil && mem.Type != *opts.MemoryType {
			continue
		}
		if mem.Confidence < opts.MinConfidence {
			continue
		}
		results = append(results, domain.MemoryWithScore{
			Memory: *mem,
			Score:  0.85,
		})
		if len(results) >= opts.TopK {
			break
		}
	}
	return results, nil
}

func (m *mockMemoryStore) CountByAgentAndType(ctx context.Context, agentID uuid.UUID, memType domain.MemoryType) (int, error) {
	count := 0
	for _, mem := range m.memories {
		if mem.AgentID == agentID && mem.Type == memType {
			count++
		}
	}
	return count, nil
}

func (m *mockMemoryStore) ListOldestByAgentAndType(ctx context.Context, agentID uuid.UUID, memType domain.MemoryType, limit int) ([]domain.Memory, error) {
	var results []domain.Memory
	for _, mem := range m.memories {
		if mem.AgentID == agentID && mem.Type == memType {
			results = append(results, *mem)
			if len(results) >= limit {
				break
			}
		}
	}
	return results, nil
}

func (m *mockMemoryStore) DeleteExpired(ctx context.Context) (int64, error) {
	return 0, nil
}

func (m *mockMemoryStore) DeleteByRetention(ctx context.Context, agentID uuid.UUID, memType domain.MemoryType, retentionDays int) (int64, error) {
	return 0, nil
}

func (m *mockMemoryStore) FindSimilar(ctx context.Context, agentID uuid.UUID, tenantID uuid.UUID, embedding []float32, threshold float32) ([]domain.MemoryWithScore, error) {
	// Return empty slice - no similar memories by default
	return []domain.MemoryWithScore{}, nil
}

func (m *mockMemoryStore) UpdateReinforcement(ctx context.Context, id uuid.UUID, confidence float32, reinforcementCount int) error {
	mem, ok := m.memories[id]
	if !ok {
		return store.ErrNotFound
	}
	mem.Confidence = confidence
	mem.ReinforcementCount = reinforcementCount
	return nil
}

func (m *mockMemoryStore) UpdateConfidence(ctx context.Context, id uuid.UUID, confidence float32) error {
	mem, ok := m.memories[id]
	if !ok {
		return store.ErrNotFound
	}
	mem.Confidence = confidence
	return nil
}

func (m *mockMemoryStore) ListDistinctAgentIDs(ctx context.Context) ([]uuid.UUID, error) {
	agentMap := make(map[uuid.UUID]bool)
	for _, mem := range m.memories {
		agentMap[mem.AgentID] = true
	}
	var agentIDs []uuid.UUID
	for id := range agentMap {
		agentIDs = append(agentIDs, id)
	}
	return agentIDs, nil
}

func (m *mockMemoryStore) GetByAgentForDecay(ctx context.Context, agentID uuid.UUID) ([]domain.Memory, error) {
	var results []domain.Memory
	for _, mem := range m.memories {
		if mem.AgentID == agentID {
			results = append(results, *mem)
		}
	}
	return results, nil
}

func (m *mockMemoryStore) Archive(ctx context.Context, id uuid.UUID) error {
	delete(m.memories, id)
	return nil
}

func (m *mockMemoryStore) IncrementAccessAndBoost(ctx context.Context, id uuid.UUID, boost float32) error {
	mem, ok := m.memories[id]
	if !ok {
		return store.ErrNotFound
	}
	mem.AccessCount++
	mem.Confidence += boost
	if mem.Confidence > 0.99 {
		mem.Confidence = 0.99
	}
	return nil
}

// mockEmbeddingClient implements domain.EmbeddingClient for testing.
type mockEmbeddingClient struct{}

func (m *mockEmbeddingClient) Embed(ctx context.Context, text string) ([]float32, error) {
	return make([]float32, 1536), nil
}

// mockLLMClient implements domain.LLMClient for testing.
type mockLLMClient struct {
	classifyResult           domain.MemoryType
	extractResult            []domain.ExtractedMemory
	summarizeResult          string
	checkContradictionResult bool
}

func newMockLLMClient() *mockLLMClient {
	return &mockLLMClient{
		classifyResult: domain.MemoryTypePreference,
		extractResult: []domain.ExtractedMemory{
			{Type: domain.MemoryTypePreference, Content: "User prefers bullet points", Confidence: 0.9},
			{Type: domain.MemoryTypeConstraint, Content: "User only uses open source", Confidence: 0.95},
		},
		summarizeResult:          "User prefers bullet points and only uses open source tools",
		checkContradictionResult: false,
	}
}

func (m *mockLLMClient) Classify(ctx context.Context, content string) (domain.MemoryType, error) {
	return m.classifyResult, nil
}

func (m *mockLLMClient) Extract(ctx context.Context, conversation []domain.Message) ([]domain.ExtractedMemory, error) {
	return m.extractResult, nil
}

func (m *mockLLMClient) Summarize(ctx context.Context, memories []domain.Memory) (string, error) {
	return m.summarizeResult, nil
}

func (m *mockLLMClient) CheckContradiction(ctx context.Context, stmtA, stmtB string) (bool, error) {
	return m.checkContradictionResult, nil
}

func (m *mockLLMClient) ExtractEpisodeStructure(ctx context.Context, content string) (*domain.EpisodeExtraction, error) {
	return &domain.EpisodeExtraction{
		Entities:        []string{},
		Topics:          []string{},
		CausalLinks:     []domain.CausalLink{},
		ImportanceScore: 0.5,
	}, nil
}

func (m *mockLLMClient) ExtractProcedure(ctx context.Context, content string) (*domain.ProcedureExtraction, error) {
	return &domain.ProcedureExtraction{
		TriggerPattern:  "When user asks about X",
		TriggerKeywords: []string{"X"},
		ActionTemplate:  "Respond with Y",
		ActionType:      domain.ActionTypeResponseStyle,
	}, nil
}

func (m *mockLLMClient) DetectSchemaPattern(ctx context.Context, memories []domain.Memory) (*domain.SchemaExtraction, error) {
	if len(memories) < 3 {
		return nil, nil
	}
	return &domain.SchemaExtraction{
		SchemaType:         domain.SchemaTypeUserArchetype,
		Name:               "Mock Schema",
		Description:        "A mock schema for testing",
		Attributes:         map[string]any{"mock": true},
		ApplicableContexts: []string{"testing"},
		Confidence:         0.8,
	}, nil
}

func testLogger() *zap.Logger {
	logger, _ := zap.NewDevelopment()
	return logger
}

func setupMemoryTest() (*MemoryService, *mockMemoryStore, uuid.UUID, uuid.UUID) {
	agentStore := newMockAgentStore()
	memStore := newMockMemoryStore()
	embClient := &mockEmbeddingClient{}
	llmClient := newMockLLMClient()
	svc := NewMemoryService(memStore, agentStore, embClient, llmClient, testLogger())

	tenantID := uuid.New()
	agent := &domain.Agent{
		TenantID:   tenantID,
		ExternalID: "bot-1",
		Name:       "Test Bot",
	}
	_ = agentStore.Create(context.Background(), agent)

	return svc, memStore, tenantID, agent.ID
}

func TestMemoryService_Create(t *testing.T) {
	svc, _, tenantID, agentID := setupMemoryTest()
	ctx := context.Background()

	mem := &domain.Memory{
		AgentID:  agentID,
		TenantID: tenantID,
		Content:  "User prefers dark mode",
		Type:     domain.MemoryTypePreference,
	}

	_, err := svc.Create(ctx, mem)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if mem.ID == uuid.Nil {
		t.Fatal("expected memory ID to be set")
	}
	if mem.Confidence != 1.0 {
		t.Fatalf("expected default confidence 1.0, got %f", mem.Confidence)
	}
	if len(mem.Embedding) != 1536 {
		t.Fatalf("expected embedding of length 1536, got %d", len(mem.Embedding))
	}
}

func TestMemoryService_Create_LLMClassification(t *testing.T) {
	svc, _, tenantID, agentID := setupMemoryTest()
	ctx := context.Background()

	mem := &domain.Memory{
		AgentID:  agentID,
		TenantID: tenantID,
		Content:  "User prefers dark mode",
		// Type intentionally left empty — LLM should classify
	}

	_, err := svc.Create(ctx, mem)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if mem.Type != domain.MemoryTypePreference {
		t.Fatalf("expected LLM-classified type 'preference', got %s", mem.Type)
	}
}

func TestMemoryService_Create_DefaultType_NoLLM(t *testing.T) {
	agentStore := newMockAgentStore()
	memStore := newMockMemoryStore()
	svc := NewMemoryService(memStore, agentStore, nil, nil, testLogger())

	tenantID := uuid.New()
	agent := &domain.Agent{TenantID: tenantID, ExternalID: "bot-1", Name: "Bot"}
	_ = agentStore.Create(context.Background(), agent)

	mem := &domain.Memory{
		AgentID:  agent.ID,
		TenantID: tenantID,
		Content:  "Some fact",
	}

	_, err := svc.Create(context.Background(), mem)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if mem.Type != domain.MemoryTypeFact {
		t.Fatalf("expected default type 'fact' without LLM, got %s", mem.Type)
	}
}

func TestMemoryService_Create_EmptyContent(t *testing.T) {
	svc, _, tenantID, agentID := setupMemoryTest()

	mem := &domain.Memory{AgentID: agentID, TenantID: tenantID}
	_, err := svc.Create(context.Background(), mem)
	if err != ErrMemoryContentEmpty {
		t.Fatalf("expected ErrMemoryContentEmpty, got %v", err)
	}
}

func TestMemoryService_Create_NoAgentID(t *testing.T) {
	svc, _, tenantID, _ := setupMemoryTest()

	mem := &domain.Memory{TenantID: tenantID, Content: "something"}
	_, err := svc.Create(context.Background(), mem)
	if err != ErrMemoryAgentIDMissing {
		t.Fatalf("expected ErrMemoryAgentIDMissing, got %v", err)
	}
}

func TestMemoryService_Create_InvalidType(t *testing.T) {
	svc, _, tenantID, agentID := setupMemoryTest()

	mem := &domain.Memory{AgentID: agentID, TenantID: tenantID, Content: "something", Type: "invalid"}
	_, err := svc.Create(context.Background(), mem)
	if err != ErrInvalidMemoryType {
		t.Fatalf("expected ErrInvalidMemoryType, got %v", err)
	}
}

func TestMemoryService_Create_AgentNotFound(t *testing.T) {
	svc, _, tenantID, _ := setupMemoryTest()

	mem := &domain.Memory{AgentID: uuid.New(), TenantID: tenantID, Content: "something"}
	_, err := svc.Create(context.Background(), mem)
	if err != ErrAgentNotFound {
		t.Fatalf("expected ErrAgentNotFound, got %v", err)
	}
}

func TestMemoryService_GetByID(t *testing.T) {
	svc, _, tenantID, agentID := setupMemoryTest()
	ctx := context.Background()

	mem := &domain.Memory{AgentID: agentID, TenantID: tenantID, Content: "Test memory", Type: domain.MemoryTypeFact}
	_, _ = svc.Create(ctx, mem)

	found, err := svc.GetByID(ctx, mem.ID, tenantID)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if found.Content != "Test memory" {
		t.Fatalf("expected content 'Test memory', got %s", found.Content)
	}
}

func TestMemoryService_GetByID_NotFound(t *testing.T) {
	svc, _, tenantID, _ := setupMemoryTest()

	_, err := svc.GetByID(context.Background(), uuid.New(), tenantID)
	if err != ErrMemoryNotFound {
		t.Fatalf("expected ErrMemoryNotFound, got %v", err)
	}
}

func TestMemoryService_Delete(t *testing.T) {
	svc, _, tenantID, agentID := setupMemoryTest()
	ctx := context.Background()

	mem := &domain.Memory{AgentID: agentID, TenantID: tenantID, Content: "To be deleted", Type: domain.MemoryTypeFact}
	_, _ = svc.Create(ctx, mem)

	if err := svc.Delete(ctx, mem.ID, tenantID); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	_, err := svc.GetByID(ctx, mem.ID, tenantID)
	if err != ErrMemoryNotFound {
		t.Fatalf("expected ErrMemoryNotFound after delete, got %v", err)
	}
}

func TestMemoryService_Delete_NotFound(t *testing.T) {
	svc, _, tenantID, _ := setupMemoryTest()

	err := svc.Delete(context.Background(), uuid.New(), tenantID)
	if err != ErrMemoryNotFound {
		t.Fatalf("expected ErrMemoryNotFound, got %v", err)
	}
}

func TestMemoryService_Recall(t *testing.T) {
	svc, _, tenantID, agentID := setupMemoryTest()
	ctx := context.Background()

	for _, content := range []string{"Likes dark mode", "Prefers Python", "Works at Acme"} {
		mem := &domain.Memory{AgentID: agentID, TenantID: tenantID, Content: content, Type: domain.MemoryTypePreference}
		_, _ = svc.Create(ctx, mem)
	}

	results, err := svc.Recall(ctx, "what does the user prefer?", agentID, tenantID, domain.RecallOpts{TopK: 10})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}
	if results[0].Score == 0 {
		t.Fatal("expected non-zero score")
	}
}

func TestMemoryService_Recall_WithTypeFilter(t *testing.T) {
	svc, _, tenantID, agentID := setupMemoryTest()
	ctx := context.Background()

	pref := &domain.Memory{AgentID: agentID, TenantID: tenantID, Content: "Likes dark mode", Type: domain.MemoryTypePreference}
	fact := &domain.Memory{AgentID: agentID, TenantID: tenantID, Content: "Works at Acme", Type: domain.MemoryTypeFact}
	_, _ = svc.Create(ctx, pref)
	_, _ = svc.Create(ctx, fact)

	mt := domain.MemoryTypePreference
	results, err := svc.Recall(ctx, "query", agentID, tenantID, domain.RecallOpts{TopK: 10, MemoryType: &mt})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 preference result, got %d", len(results))
	}
}

func TestMemoryService_Recall_EmptyQuery(t *testing.T) {
	svc, _, tenantID, agentID := setupMemoryTest()

	_, err := svc.Recall(context.Background(), "", agentID, tenantID, domain.RecallOpts{TopK: 10})
	if err != ErrRecallQueryEmpty {
		t.Fatalf("expected ErrRecallQueryEmpty, got %v", err)
	}
}

func TestMemoryService_Recall_TopK(t *testing.T) {
	svc, _, tenantID, agentID := setupMemoryTest()
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		mem := &domain.Memory{AgentID: agentID, TenantID: tenantID, Content: "memory", Type: domain.MemoryTypeFact}
		_, _ = svc.Create(ctx, mem)
	}

	results, err := svc.Recall(ctx, "query", agentID, tenantID, domain.RecallOpts{TopK: 2})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results with top_k=2, got %d", len(results))
	}
}

func TestMemoryService_Extract(t *testing.T) {
	svc, _, tenantID, agentID := setupMemoryTest()
	ctx := context.Background()

	conversation := []domain.Message{
		{Role: "user", Content: "I always want bullet points"},
		{Role: "assistant", Content: "Got it."},
		{Role: "user", Content: "Never suggest paid tools."},
	}

	results, err := svc.Extract(ctx, agentID, tenantID, conversation, false)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 extracted, got %d", len(results))
	}
	if results[0].Stored {
		t.Fatal("expected stored=false when auto_store=false")
	}
}

func TestMemoryService_Extract_AutoStore(t *testing.T) {
	svc, memStore, tenantID, agentID := setupMemoryTest()
	ctx := context.Background()

	conversation := []domain.Message{
		{Role: "user", Content: "I prefer dark mode"},
	}

	results, err := svc.Extract(ctx, agentID, tenantID, conversation, true)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	for _, r := range results {
		if !r.Stored {
			t.Fatal("expected stored=true when auto_store=true")
		}
		if r.ID == uuid.Nil {
			t.Fatal("expected ID to be set for stored memory")
		}
	}
	if len(memStore.memories) != 2 {
		t.Fatalf("expected 2 memories in store, got %d", len(memStore.memories))
	}
}

func TestMemoryService_Summarize(t *testing.T) {
	svc, _, _, _ := setupMemoryTest()
	ctx := context.Background()

	memories := []domain.Memory{
		{Content: "User prefers dark mode", Type: domain.MemoryTypePreference},
		{Content: "User only uses open source", Type: domain.MemoryTypeConstraint},
	}

	summary, err := svc.Summarize(ctx, memories)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if summary == "" {
		t.Fatal("expected non-empty summary")
	}
}
