package service

import (
	"context"
	"testing"
	"time"

	"github.com/Harshitk-cp/engram/internal/domain"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// Mock stores for consolidation tests

type mockMemoryStoreForConsolidation struct {
	memories []domain.Memory
	agentIDs []uuid.UUID
	archived []uuid.UUID
	updated  map[uuid.UUID]float32
}

func newMockMemoryStoreForConsolidation() *mockMemoryStoreForConsolidation {
	return &mockMemoryStoreForConsolidation{
		updated: make(map[uuid.UUID]float32),
	}
}

func (m *mockMemoryStoreForConsolidation) Create(ctx context.Context, mem *domain.Memory) error {
	mem.ID = uuid.New()
	now := time.Now()
	mem.CreatedAt = now
	mem.UpdatedAt = now
	mem.LastAccessedAt = &now
	m.memories = append(m.memories, *mem)
	return nil
}

func (m *mockMemoryStoreForConsolidation) GetByID(ctx context.Context, id uuid.UUID, tenantID uuid.UUID) (*domain.Memory, error) {
	for _, mem := range m.memories {
		if mem.ID == id {
			return &mem, nil
		}
	}
	return nil, nil
}

func (m *mockMemoryStoreForConsolidation) Delete(ctx context.Context, id uuid.UUID, tenantID uuid.UUID) error {
	return nil
}

func (m *mockMemoryStoreForConsolidation) Recall(ctx context.Context, embedding []float32, agentID uuid.UUID, tenantID uuid.UUID, opts domain.RecallOpts) ([]domain.MemoryWithScore, error) {
	return nil, nil
}

func (m *mockMemoryStoreForConsolidation) CountByAgentAndType(ctx context.Context, agentID uuid.UUID, memType domain.MemoryType) (int, error) {
	return 0, nil
}

func (m *mockMemoryStoreForConsolidation) ListOldestByAgentAndType(ctx context.Context, agentID uuid.UUID, memType domain.MemoryType, limit int) ([]domain.Memory, error) {
	return nil, nil
}

func (m *mockMemoryStoreForConsolidation) DeleteExpired(ctx context.Context) (int64, error) {
	return 0, nil
}

func (m *mockMemoryStoreForConsolidation) DeleteByRetention(ctx context.Context, agentID uuid.UUID, memType domain.MemoryType, retentionDays int) (int64, error) {
	return 0, nil
}

func (m *mockMemoryStoreForConsolidation) FindSimilar(ctx context.Context, agentID uuid.UUID, tenantID uuid.UUID, embedding []float32, threshold float32) ([]domain.MemoryWithScore, error) {
	return nil, nil
}

func (m *mockMemoryStoreForConsolidation) UpdateReinforcement(ctx context.Context, id uuid.UUID, confidence float32, reinforcementCount int) error {
	m.updated[id] = confidence
	return nil
}

func (m *mockMemoryStoreForConsolidation) UpdateConfidence(ctx context.Context, id uuid.UUID, confidence float32) error {
	m.updated[id] = confidence
	return nil
}

func (m *mockMemoryStoreForConsolidation) ListDistinctAgentIDs(ctx context.Context) ([]uuid.UUID, error) {
	return m.agentIDs, nil
}

func (m *mockMemoryStoreForConsolidation) GetByAgentForDecay(ctx context.Context, agentID uuid.UUID) ([]domain.Memory, error) {
	var result []domain.Memory
	for _, mem := range m.memories {
		if mem.AgentID == agentID {
			result = append(result, mem)
		}
	}
	return result, nil
}

func (m *mockMemoryStoreForConsolidation) Archive(ctx context.Context, id uuid.UUID) error {
	m.archived = append(m.archived, id)
	return nil
}

func (m *mockMemoryStoreForConsolidation) IncrementAccessAndBoost(ctx context.Context, id uuid.UUID, boost float32) error {
	return nil
}

type mockEpisodeStoreForConsolidation struct {
	episodes     []domain.Episode
	archived     []uuid.UUID
	statusUpdate map[uuid.UUID]domain.ConsolidationStatus
}

func newMockEpisodeStoreForConsolidation() *mockEpisodeStoreForConsolidation {
	return &mockEpisodeStoreForConsolidation{
		statusUpdate: make(map[uuid.UUID]domain.ConsolidationStatus),
	}
}

func (m *mockEpisodeStoreForConsolidation) Create(ctx context.Context, e *domain.Episode) error {
	e.ID = uuid.New()
	e.CreatedAt = time.Now()
	m.episodes = append(m.episodes, *e)
	return nil
}

func (m *mockEpisodeStoreForConsolidation) GetByID(ctx context.Context, id uuid.UUID, tenantID uuid.UUID) (*domain.Episode, error) {
	for _, ep := range m.episodes {
		if ep.ID == id {
			return &ep, nil
		}
	}
	return nil, nil
}

func (m *mockEpisodeStoreForConsolidation) GetByConversationID(ctx context.Context, conversationID uuid.UUID, tenantID uuid.UUID) ([]domain.Episode, error) {
	return nil, nil
}

func (m *mockEpisodeStoreForConsolidation) GetByTimeRange(ctx context.Context, agentID uuid.UUID, tenantID uuid.UUID, start, end time.Time) ([]domain.Episode, error) {
	var result []domain.Episode
	for _, ep := range m.episodes {
		if ep.AgentID == agentID && ep.CreatedAt.After(start) && ep.CreatedAt.Before(end) {
			result = append(result, ep)
		}
	}
	return result, nil
}

func (m *mockEpisodeStoreForConsolidation) GetByImportance(ctx context.Context, agentID uuid.UUID, tenantID uuid.UUID, minImportance float32, limit int) ([]domain.Episode, error) {
	return nil, nil
}

func (m *mockEpisodeStoreForConsolidation) FindSimilar(ctx context.Context, agentID uuid.UUID, tenantID uuid.UUID, embedding []float32, threshold float32, limit int) ([]domain.EpisodeWithScore, error) {
	return nil, nil
}

func (m *mockEpisodeStoreForConsolidation) GetUnconsolidated(ctx context.Context, agentID uuid.UUID, limit int) ([]domain.Episode, error) {
	var result []domain.Episode
	for _, ep := range m.episodes {
		if ep.AgentID == agentID && ep.ConsolidationStatus == domain.ConsolidationRaw {
			result = append(result, ep)
			if len(result) >= limit {
				break
			}
		}
	}
	return result, nil
}

func (m *mockEpisodeStoreForConsolidation) GetByConsolidationStatus(ctx context.Context, agentID uuid.UUID, tenantID uuid.UUID, status domain.ConsolidationStatus, limit int) ([]domain.Episode, error) {
	var result []domain.Episode
	for _, ep := range m.episodes {
		if ep.AgentID == agentID && ep.ConsolidationStatus == status {
			result = append(result, ep)
			if limit > 0 && len(result) >= limit {
				break
			}
		}
	}
	return result, nil
}

func (m *mockEpisodeStoreForConsolidation) UpdateConsolidationStatus(ctx context.Context, id uuid.UUID, status domain.ConsolidationStatus) error {
	m.statusUpdate[id] = status
	for i := range m.episodes {
		if m.episodes[i].ID == id {
			m.episodes[i].ConsolidationStatus = status
			break
		}
	}
	return nil
}

func (m *mockEpisodeStoreForConsolidation) LinkDerivedMemory(ctx context.Context, episodeID uuid.UUID, memoryID uuid.UUID, memoryType string) error {
	return nil
}

func (m *mockEpisodeStoreForConsolidation) ApplyDecay(ctx context.Context, agentID uuid.UUID) (int64, error) {
	return 0, nil
}

func (m *mockEpisodeStoreForConsolidation) GetWeakMemories(ctx context.Context, agentID uuid.UUID, threshold float32) ([]domain.Episode, error) {
	return nil, nil
}

func (m *mockEpisodeStoreForConsolidation) Archive(ctx context.Context, id uuid.UUID) error {
	m.archived = append(m.archived, id)
	return nil
}

func (m *mockEpisodeStoreForConsolidation) UpdateStrength(ctx context.Context, id uuid.UUID, strength float32) error {
	return nil
}

func (m *mockEpisodeStoreForConsolidation) RecordAccess(ctx context.Context, id uuid.UUID) error {
	return nil
}

func (m *mockEpisodeStoreForConsolidation) UpdateOutcome(ctx context.Context, id uuid.UUID, outcome domain.OutcomeType, description string) error {
	for i := range m.episodes {
		if m.episodes[i].ID == id {
			m.episodes[i].Outcome = outcome
			break
		}
	}
	return nil
}

func (m *mockEpisodeStoreForConsolidation) CreateAssociation(ctx context.Context, a *domain.EpisodeAssociation) error {
	return nil
}

func (m *mockEpisodeStoreForConsolidation) GetAssociations(ctx context.Context, episodeID uuid.UUID) ([]domain.EpisodeAssociation, error) {
	return nil, nil
}

func (m *mockEpisodeStoreForConsolidation) GetByAgentForDecay(ctx context.Context, agentID uuid.UUID) ([]domain.Episode, error) {
	return nil, nil
}

type mockProcedureStoreForConsolidation struct {
	procedures []domain.Procedure
	archived   []uuid.UUID
	updated    map[uuid.UUID]float32
}

func newMockProcedureStoreForConsolidation() *mockProcedureStoreForConsolidation {
	return &mockProcedureStoreForConsolidation{
		updated: make(map[uuid.UUID]float32),
	}
}

func (m *mockProcedureStoreForConsolidation) Create(ctx context.Context, p *domain.Procedure) error {
	p.ID = uuid.New()
	p.CreatedAt = time.Now()
	m.procedures = append(m.procedures, *p)
	return nil
}

func (m *mockProcedureStoreForConsolidation) GetByID(ctx context.Context, id uuid.UUID, tenantID uuid.UUID) (*domain.Procedure, error) {
	for _, p := range m.procedures {
		if p.ID == id {
			return &p, nil
		}
	}
	return nil, nil
}

func (m *mockProcedureStoreForConsolidation) GetByAgent(ctx context.Context, agentID uuid.UUID, tenantID uuid.UUID) ([]domain.Procedure, error) {
	var result []domain.Procedure
	for _, p := range m.procedures {
		if p.AgentID == agentID {
			result = append(result, p)
		}
	}
	return result, nil
}

func (m *mockProcedureStoreForConsolidation) FindByTriggerSimilarity(ctx context.Context, agentID uuid.UUID, tenantID uuid.UUID, embedding []float32, threshold float32, limit int) ([]domain.ProcedureWithScore, error) {
	return nil, nil
}

func (m *mockProcedureStoreForConsolidation) Reinforce(ctx context.Context, id uuid.UUID, episodeID uuid.UUID, boost float32) error {
	return nil
}

func (m *mockProcedureStoreForConsolidation) RecordOutcome(ctx context.Context, id uuid.UUID, success bool) error {
	return nil
}

func (m *mockProcedureStoreForConsolidation) GetByAgentForDecay(ctx context.Context, agentID uuid.UUID) ([]domain.Procedure, error) {
	var result []domain.Procedure
	for _, p := range m.procedures {
		if p.AgentID == agentID {
			result = append(result, p)
		}
	}
	return result, nil
}

func (m *mockProcedureStoreForConsolidation) Archive(ctx context.Context, id uuid.UUID) error {
	m.archived = append(m.archived, id)
	return nil
}

func (m *mockProcedureStoreForConsolidation) UpdateConfidence(ctx context.Context, id uuid.UUID, confidence float32) error {
	m.updated[id] = confidence
	return nil
}

func (m *mockProcedureStoreForConsolidation) CreateNewVersion(ctx context.Context, p *domain.Procedure) error {
	return m.Create(ctx, p)
}

func (m *mockProcedureStoreForConsolidation) RecordUse(ctx context.Context, id uuid.UUID, success bool) error {
	return nil
}

type mockSchemaStoreForConsolidation struct {
	schemas []domain.Schema
}

func newMockSchemaStoreForConsolidation() *mockSchemaStoreForConsolidation {
	return &mockSchemaStoreForConsolidation{}
}

func (m *mockSchemaStoreForConsolidation) Create(ctx context.Context, s *domain.Schema) error {
	s.ID = uuid.New()
	s.CreatedAt = time.Now()
	m.schemas = append(m.schemas, *s)
	return nil
}

func (m *mockSchemaStoreForConsolidation) GetByID(ctx context.Context, id uuid.UUID, tenantID uuid.UUID) (*domain.Schema, error) {
	for _, s := range m.schemas {
		if s.ID == id {
			return &s, nil
		}
	}
	return nil, nil
}

func (m *mockSchemaStoreForConsolidation) GetByAgent(ctx context.Context, agentID uuid.UUID, tenantID uuid.UUID) ([]domain.Schema, error) {
	var result []domain.Schema
	for _, s := range m.schemas {
		if s.AgentID == agentID {
			result = append(result, s)
		}
	}
	return result, nil
}

func (m *mockSchemaStoreForConsolidation) GetByName(ctx context.Context, agentID uuid.UUID, tenantID uuid.UUID, schemaType domain.SchemaType, name string) (*domain.Schema, error) {
	for _, s := range m.schemas {
		if s.AgentID == agentID && s.SchemaType == schemaType && s.Name == name {
			return &s, nil
		}
	}
	return nil, nil
}

func (m *mockSchemaStoreForConsolidation) FindSimilar(ctx context.Context, agentID uuid.UUID, tenantID uuid.UUID, embedding []float32, threshold float32, limit int) ([]domain.SchemaWithScore, error) {
	return nil, nil
}

func (m *mockSchemaStoreForConsolidation) AddEvidence(ctx context.Context, id uuid.UUID, memoryID *uuid.UUID, episodeID *uuid.UUID) error {
	return nil
}

func (m *mockSchemaStoreForConsolidation) UpdateConfidence(ctx context.Context, id uuid.UUID, confidence float32) error {
	return nil
}

func (m *mockSchemaStoreForConsolidation) RecordContradiction(ctx context.Context, id uuid.UUID, memoryID *uuid.UUID, episodeID *uuid.UUID, description string) error {
	return nil
}

func (m *mockSchemaStoreForConsolidation) RecordValidation(ctx context.Context, id uuid.UUID, success bool) error {
	return nil
}

func (m *mockSchemaStoreForConsolidation) Delete(ctx context.Context, id uuid.UUID, tenantID uuid.UUID) error {
	return nil
}

func (m *mockSchemaStoreForConsolidation) IncrementContradiction(ctx context.Context, id uuid.UUID) error {
	return nil
}

func (m *mockSchemaStoreForConsolidation) UpdateValidation(ctx context.Context, id uuid.UUID) error {
	return nil
}

func (m *mockSchemaStoreForConsolidation) RemoveEvidence(ctx context.Context, id uuid.UUID, memoryID *uuid.UUID, episodeID *uuid.UUID) error {
	return nil
}

func (m *mockSchemaStoreForConsolidation) Update(ctx context.Context, s *domain.Schema) error {
	return nil
}

type mockAssocStoreForConsolidation struct {
	associations []domain.MemoryAssociation
}

func (m *mockAssocStoreForConsolidation) Create(ctx context.Context, a *domain.MemoryAssociation) error {
	a.ID = uuid.New()
	a.CreatedAt = time.Now()
	m.associations = append(m.associations, *a)
	return nil
}

func (m *mockAssocStoreForConsolidation) GetBySource(ctx context.Context, sourceType domain.ActivatedMemoryType, sourceID uuid.UUID) ([]domain.MemoryAssociation, error) {
	return nil, nil
}

func (m *mockAssocStoreForConsolidation) GetByTarget(ctx context.Context, targetType domain.ActivatedMemoryType, targetID uuid.UUID) ([]domain.MemoryAssociation, error) {
	return nil, nil
}

func (m *mockAssocStoreForConsolidation) UpdateStrength(ctx context.Context, id uuid.UUID, strength float32) error {
	return nil
}

func (m *mockAssocStoreForConsolidation) Delete(ctx context.Context, id uuid.UUID) error {
	return nil
}

// Tests

func TestConsolidationService_Consolidate_NoEpisodes(t *testing.T) {
	logger := zap.NewNop()
	agentID := uuid.New()
	tenantID := uuid.New()

	memStore := newMockMemoryStoreForConsolidation()
	memStore.agentIDs = []uuid.UUID{agentID}

	episodeStore := newMockEpisodeStoreForConsolidation()
	procedureStore := newMockProcedureStoreForConsolidation()
	schemaStore := newMockSchemaStoreForConsolidation()
	assocStore := &mockAssocStoreForConsolidation{}

	svc := NewConsolidationService(
		memStore,
		episodeStore,
		procedureStore,
		schemaStore,
		assocStore,
		nil, // contradiction store
		nil, // embedding client
		nil, // llm client
		logger,
	)

	result, err := svc.Consolidate(context.Background(), agentID, tenantID, ConsolidationScopeRecent)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.EpisodesProcessed != 0 {
		t.Errorf("expected 0 episodes processed, got %d", result.EpisodesProcessed)
	}
}

func TestConsolidationService_ProcessEpisodes(t *testing.T) {
	logger := zap.NewNop()
	agentID := uuid.New()
	tenantID := uuid.New()

	memStore := newMockMemoryStoreForConsolidation()
	memStore.agentIDs = []uuid.UUID{agentID}

	episodeStore := newMockEpisodeStoreForConsolidation()

	// Add some raw episodes
	ep1ID := uuid.New()
	ep2ID := uuid.New()
	episodeStore.episodes = []domain.Episode{
		{
			ID:                  ep1ID,
			AgentID:             agentID,
			TenantID:            tenantID,
			RawContent:          "User asked about dark mode",
			ConsolidationStatus: domain.ConsolidationRaw,
			CreatedAt:           time.Now(),
		},
		{
			ID:                  ep2ID,
			AgentID:             agentID,
			TenantID:            tenantID,
			RawContent:          "User prefers night theme",
			ConsolidationStatus: domain.ConsolidationRaw,
			CreatedAt:           time.Now(),
		},
	}

	procedureStore := newMockProcedureStoreForConsolidation()
	schemaStore := newMockSchemaStoreForConsolidation()
	assocStore := &mockAssocStoreForConsolidation{}

	svc := NewConsolidationService(
		memStore,
		episodeStore,
		procedureStore,
		schemaStore,
		assocStore,
		nil,
		nil,
		nil,
		logger,
	)

	result, err := svc.Consolidate(context.Background(), agentID, tenantID, ConsolidationScopeRecent)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.EpisodesProcessed != 2 {
		t.Errorf("expected 2 episodes processed, got %d", result.EpisodesProcessed)
	}

	// Check status was updated
	if status, ok := episodeStore.statusUpdate[ep1ID]; !ok || status != domain.ConsolidationProcessed {
		t.Errorf("episode 1 status not updated to processed")
	}
	if status, ok := episodeStore.statusUpdate[ep2ID]; !ok || status != domain.ConsolidationProcessed {
		t.Errorf("episode 2 status not updated to processed")
	}
}

func TestConsolidationService_ApplyForgetting(t *testing.T) {
	logger := zap.NewNop()
	agentID := uuid.New()
	tenantID := uuid.New()

	oldTime := time.Now().Add(-60 * 24 * time.Hour)

	memStore := newMockMemoryStoreForConsolidation()
	memStore.agentIDs = []uuid.UUID{agentID}

	// Add a memory that should be archived (low confidence + old)
	memID := uuid.New()
	memStore.memories = []domain.Memory{
		{
			ID:             memID,
			AgentID:        agentID,
			TenantID:       tenantID,
			Content:        "old memory",
			Confidence:     0.15,
			DecayRate:      0.1,
			LastAccessedAt: &oldTime,
		},
	}

	episodeStore := newMockEpisodeStoreForConsolidation()
	procedureStore := newMockProcedureStoreForConsolidation()
	schemaStore := newMockSchemaStoreForConsolidation()
	assocStore := &mockAssocStoreForConsolidation{}

	svc := NewConsolidationService(
		memStore,
		episodeStore,
		procedureStore,
		schemaStore,
		assocStore,
		nil,
		nil,
		nil,
		logger,
	)

	result, err := svc.Consolidate(context.Background(), agentID, tenantID, ConsolidationScopeFull)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.MemoriesArchived != 1 {
		t.Errorf("expected 1 memory archived, got %d", result.MemoriesArchived)
	}

	// Verify memory was archived
	found := false
	for _, id := range memStore.archived {
		if id == memID {
			found = true
			break
		}
	}
	if !found {
		t.Error("memory should have been archived")
	}
}

func TestConsolidationService_GetMemoryHealth(t *testing.T) {
	logger := zap.NewNop()
	agentID := uuid.New()
	tenantID := uuid.New()

	now := time.Now()
	recentTime := now.Add(-1 * time.Hour)
	oldTime := now.Add(-48 * time.Hour)

	memStore := newMockMemoryStoreForConsolidation()
	memStore.agentIDs = []uuid.UUID{agentID}
	memStore.memories = []domain.Memory{
		{
			ID:             uuid.New(),
			AgentID:        agentID,
			TenantID:       tenantID,
			Content:        "high confidence memory",
			Confidence:     0.9,
			LastAccessedAt: &recentTime,
		},
		{
			ID:             uuid.New(),
			AgentID:        agentID,
			TenantID:       tenantID,
			Content:        "low confidence memory",
			Confidence:     0.2,
			LastAccessedAt: &oldTime,
		},
	}

	episodeStore := newMockEpisodeStoreForConsolidation()
	episodeStore.episodes = []domain.Episode{
		{
			ID:                  uuid.New(),
			AgentID:             agentID,
			TenantID:            tenantID,
			RawContent:          "unprocessed episode",
			ConsolidationStatus: domain.ConsolidationRaw,
			CreatedAt:           now.Add(-2 * time.Hour),
		},
	}

	procedureStore := newMockProcedureStoreForConsolidation()
	schemaStore := newMockSchemaStoreForConsolidation()

	svc := NewConsolidationService(
		memStore,
		episodeStore,
		procedureStore,
		schemaStore,
		nil,
		nil,
		nil,
		nil,
		logger,
	)

	stats, err := svc.GetMemoryHealth(context.Background(), agentID, tenantID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if stats.SemanticCount != 2 {
		t.Errorf("expected semantic count 2, got %d", stats.SemanticCount)
	}

	if stats.MemoriesAtRisk != 1 {
		t.Errorf("expected 1 memory at risk, got %d", stats.MemoriesAtRisk)
	}

	if stats.RecentlyReinforced != 1 {
		t.Errorf("expected 1 recently reinforced, got %d", stats.RecentlyReinforced)
	}

	if stats.EpisodicCount != 1 {
		t.Errorf("expected episodic count 1, got %d", stats.EpisodicCount)
	}

	expectedAvg := float32(0.55) // (0.9 + 0.2) / 2
	if stats.AverageConfidence < expectedAvg-0.01 || stats.AverageConfidence > expectedAvg+0.01 {
		t.Errorf("expected average confidence ~0.55, got %f", stats.AverageConfidence)
	}
}

func TestConsolidationService_ClusterMemories(t *testing.T) {
	svc := &ConsolidationService{}

	// Create memories with similar embeddings
	agentID := uuid.New()
	tenantID := uuid.New()

	// Two memories in one cluster (similar embeddings)
	mem1 := domain.Memory{
		ID:        uuid.New(),
		AgentID:   agentID,
		TenantID:  tenantID,
		Content:   "memory 1",
		Embedding: []float32{1.0, 0.0, 0.0},
	}
	mem2 := domain.Memory{
		ID:        uuid.New(),
		AgentID:   agentID,
		TenantID:  tenantID,
		Content:   "memory 2",
		Embedding: []float32{0.95, 0.1, 0.0}, // Very similar to mem1
	}
	// One memory in another cluster
	mem3 := domain.Memory{
		ID:        uuid.New(),
		AgentID:   agentID,
		TenantID:  tenantID,
		Content:   "memory 3",
		Embedding: []float32{0.0, 1.0, 0.0}, // Different direction
	}

	memories := []domain.Memory{mem1, mem2, mem3}
	clusters := svc.clusterMemories(memories)

	if len(clusters) != 2 {
		t.Errorf("expected 2 clusters, got %d", len(clusters))
	}

	// First cluster should have mem1 and mem2
	found1 := false
	found2 := false
	for _, cluster := range clusters {
		if len(cluster.Memories) == 2 {
			for _, m := range cluster.Memories {
				if m.ID == mem1.ID {
					found1 = true
				}
				if m.ID == mem2.ID {
					found2 = true
				}
			}
		}
	}

	if !found1 || !found2 {
		t.Error("mem1 and mem2 should be in the same cluster")
	}
}

func TestConsolidationService_StartStop(t *testing.T) {
	logger := zap.NewNop()

	memStore := newMockMemoryStoreForConsolidation()
	episodeStore := newMockEpisodeStoreForConsolidation()
	procedureStore := newMockProcedureStoreForConsolidation()
	schemaStore := newMockSchemaStoreForConsolidation()

	svc := NewConsolidationService(
		memStore,
		episodeStore,
		procedureStore,
		schemaStore,
		nil,
		nil,
		nil,
		nil,
		logger,
	)

	// Set a very short interval for testing
	svc.SetInterval(10 * time.Millisecond)

	// Start and immediately stop - should not panic
	svc.Start()
	time.Sleep(20 * time.Millisecond) // Let it tick once
	svc.Stop()

	// Should be able to stop without hanging
}
