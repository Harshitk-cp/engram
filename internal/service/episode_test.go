package service

import (
	"context"
	"testing"
	"time"

	"github.com/Harshitk-cp/engram/internal/domain"
	"github.com/Harshitk-cp/engram/internal/store"
	"github.com/google/uuid"
)

// mockEpisodeStore implements domain.EpisodeStore for testing.
type mockEpisodeStore struct {
	episodes     map[uuid.UUID]*domain.Episode
	associations []domain.EpisodeAssociation
}

func newMockEpisodeStore() *mockEpisodeStore {
	return &mockEpisodeStore{
		episodes:     make(map[uuid.UUID]*domain.Episode),
		associations: []domain.EpisodeAssociation{},
	}
}

func (m *mockEpisodeStore) Create(ctx context.Context, e *domain.Episode) error {
	e.ID = uuid.New()
	e.CreatedAt = time.Now()
	e.UpdatedAt = time.Now()
	e.LastAccessedAt = time.Now()
	m.episodes[e.ID] = e
	return nil
}

func (m *mockEpisodeStore) GetByID(ctx context.Context, id uuid.UUID, tenantID uuid.UUID) (*domain.Episode, error) {
	e, ok := m.episodes[id]
	if !ok || e.TenantID != tenantID {
		return nil, store.ErrNotFound
	}
	return e, nil
}

func (m *mockEpisodeStore) GetByConversationID(ctx context.Context, conversationID uuid.UUID, tenantID uuid.UUID) ([]domain.Episode, error) {
	var results []domain.Episode
	for _, e := range m.episodes {
		if e.ConversationID != nil && *e.ConversationID == conversationID && e.TenantID == tenantID {
			results = append(results, *e)
		}
	}
	return results, nil
}

func (m *mockEpisodeStore) GetByTimeRange(ctx context.Context, agentID uuid.UUID, tenantID uuid.UUID, start, end time.Time) ([]domain.Episode, error) {
	var results []domain.Episode
	for _, e := range m.episodes {
		if e.AgentID == agentID && e.TenantID == tenantID {
			if !e.OccurredAt.Before(start) && !e.OccurredAt.After(end) {
				results = append(results, *e)
			}
		}
	}
	return results, nil
}

func (m *mockEpisodeStore) GetByImportance(ctx context.Context, agentID uuid.UUID, tenantID uuid.UUID, minImportance float32, limit int) ([]domain.Episode, error) {
	var results []domain.Episode
	for _, e := range m.episodes {
		if e.AgentID == agentID && e.TenantID == tenantID && e.ImportanceScore >= minImportance {
			results = append(results, *e)
			if len(results) >= limit {
				break
			}
		}
	}
	return results, nil
}

func (m *mockEpisodeStore) FindSimilar(ctx context.Context, agentID uuid.UUID, tenantID uuid.UUID, embedding []float32, threshold float32, limit int) ([]domain.EpisodeWithScore, error) {
	return []domain.EpisodeWithScore{}, nil
}

func (m *mockEpisodeStore) GetUnconsolidated(ctx context.Context, agentID uuid.UUID, limit int) ([]domain.Episode, error) {
	var results []domain.Episode
	for _, e := range m.episodes {
		if e.AgentID == agentID && e.ConsolidationStatus == domain.ConsolidationRaw {
			results = append(results, *e)
			if len(results) >= limit {
				break
			}
		}
	}
	return results, nil
}

func (m *mockEpisodeStore) GetByConsolidationStatus(ctx context.Context, agentID uuid.UUID, tenantID uuid.UUID, status domain.ConsolidationStatus, limit int) ([]domain.Episode, error) {
	var results []domain.Episode
	for _, e := range m.episodes {
		if e.AgentID == agentID && e.TenantID == tenantID && e.ConsolidationStatus == status {
			results = append(results, *e)
			if limit > 0 && len(results) >= limit {
				break
			}
		}
	}
	return results, nil
}

func (m *mockEpisodeStore) UpdateConsolidationStatus(ctx context.Context, id uuid.UUID, status domain.ConsolidationStatus) error {
	e, ok := m.episodes[id]
	if !ok {
		return store.ErrNotFound
	}
	e.ConsolidationStatus = status
	return nil
}

func (m *mockEpisodeStore) LinkDerivedMemory(ctx context.Context, episodeID uuid.UUID, memoryID uuid.UUID, memoryType string) error {
	e, ok := m.episodes[episodeID]
	if !ok {
		return store.ErrNotFound
	}
	if memoryType == "semantic" {
		e.DerivedSemanticIDs = append(e.DerivedSemanticIDs, memoryID)
	} else {
		e.DerivedProceduralIDs = append(e.DerivedProceduralIDs, memoryID)
	}
	return nil
}

func (m *mockEpisodeStore) ApplyDecay(ctx context.Context, agentID uuid.UUID) (int64, error) {
	return 0, nil
}

func (m *mockEpisodeStore) GetByAgentForDecay(ctx context.Context, agentID uuid.UUID) ([]domain.Episode, error) {
	var results []domain.Episode
	for _, e := range m.episodes {
		if e.AgentID == agentID && e.ConsolidationStatus != domain.ConsolidationArchived {
			results = append(results, *e)
		}
	}
	return results, nil
}

func (m *mockEpisodeStore) GetWeakMemories(ctx context.Context, agentID uuid.UUID, threshold float32) ([]domain.Episode, error) {
	var results []domain.Episode
	for _, e := range m.episodes {
		if e.AgentID == agentID && e.MemoryStrength < threshold {
			results = append(results, *e)
		}
	}
	return results, nil
}

func (m *mockEpisodeStore) Archive(ctx context.Context, id uuid.UUID) error {
	e, ok := m.episodes[id]
	if !ok {
		return store.ErrNotFound
	}
	e.ConsolidationStatus = domain.ConsolidationArchived
	return nil
}

func (m *mockEpisodeStore) UpdateStrength(ctx context.Context, id uuid.UUID, strength float32) error {
	e, ok := m.episodes[id]
	if !ok {
		return store.ErrNotFound
	}
	e.MemoryStrength = strength
	return nil
}

func (m *mockEpisodeStore) RecordAccess(ctx context.Context, id uuid.UUID) error {
	e, ok := m.episodes[id]
	if !ok {
		return store.ErrNotFound
	}
	e.AccessCount++
	e.LastAccessedAt = time.Now()
	return nil
}

func (m *mockEpisodeStore) UpdateOutcome(ctx context.Context, id uuid.UUID, outcome domain.OutcomeType, description string) error {
	e, ok := m.episodes[id]
	if !ok {
		return store.ErrNotFound
	}
	e.Outcome = outcome
	e.OutcomeDescription = description
	return nil
}

func (m *mockEpisodeStore) CreateAssociation(ctx context.Context, a *domain.EpisodeAssociation) error {
	a.ID = uuid.New()
	a.CreatedAt = time.Now()
	m.associations = append(m.associations, *a)
	return nil
}

func (m *mockEpisodeStore) GetAssociations(ctx context.Context, episodeID uuid.UUID) ([]domain.EpisodeAssociation, error) {
	var results []domain.EpisodeAssociation
	for _, a := range m.associations {
		if a.EpisodeAID == episodeID || a.EpisodeBID == episodeID {
			results = append(results, a)
		}
	}
	return results, nil
}

func setupEpisodeTest() (*EpisodeService, *mockEpisodeStore, uuid.UUID, uuid.UUID) {
	agentStore := newMockAgentStore()
	episodeStore := newMockEpisodeStore()
	embClient := &mockEmbeddingClient{}
	llmClient := newMockLLMClient()
	svc := NewEpisodeService(episodeStore, agentStore, embClient, llmClient, testLogger())

	tenantID := uuid.New()
	agent := &domain.Agent{
		TenantID:   tenantID,
		ExternalID: "bot-1",
		Name:       "Test Bot",
	}
	_ = agentStore.Create(context.Background(), agent)

	return svc, episodeStore, tenantID, agent.ID
}

func TestEpisodeService_Encode(t *testing.T) {
	svc, episodeStore, tenantID, agentID := setupEpisodeTest()
	ctx := context.Background()

	input := EncodeInput{
		AgentID:    agentID,
		TenantID:   tenantID,
		RawContent: "I hate light mode, it hurts my eyes",
	}

	episode, err := svc.Encode(ctx, input)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if episode.ID == uuid.Nil {
		t.Fatal("expected episode ID to be set")
	}
	if episode.RawContent != input.RawContent {
		t.Fatalf("expected raw_content %q, got %q", input.RawContent, episode.RawContent)
	}
	if episode.ConsolidationStatus != domain.ConsolidationRaw {
		t.Fatalf("expected consolidation_status 'raw', got %q", episode.ConsolidationStatus)
	}
	if episode.MemoryStrength != 1.0 {
		t.Fatalf("expected memory_strength 1.0, got %f", episode.MemoryStrength)
	}

	// Verify episode was stored
	if len(episodeStore.episodes) != 1 {
		t.Fatalf("expected 1 episode in store, got %d", len(episodeStore.episodes))
	}
}

func TestEpisodeService_Encode_WithConversationID(t *testing.T) {
	svc, _, tenantID, agentID := setupEpisodeTest()
	ctx := context.Background()

	convID := uuid.New()
	input := EncodeInput{
		AgentID:        agentID,
		TenantID:       tenantID,
		RawContent:     "Test content",
		ConversationID: &convID,
	}

	episode, err := svc.Encode(ctx, input)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if episode.ConversationID == nil || *episode.ConversationID != convID {
		t.Fatal("expected conversation_id to be set")
	}
}

func TestEpisodeService_Encode_ContentEmpty(t *testing.T) {
	svc, _, tenantID, agentID := setupEpisodeTest()
	ctx := context.Background()

	input := EncodeInput{
		AgentID:    agentID,
		TenantID:   tenantID,
		RawContent: "",
	}

	_, err := svc.Encode(ctx, input)
	if err != ErrEpisodeContentEmpty {
		t.Fatalf("expected ErrEpisodeContentEmpty, got %v", err)
	}
}

func TestEpisodeService_Encode_AgentNotFound(t *testing.T) {
	svc, _, tenantID, _ := setupEpisodeTest()
	ctx := context.Background()

	input := EncodeInput{
		AgentID:    uuid.New(), // Unknown agent
		TenantID:   tenantID,
		RawContent: "Test content",
	}

	_, err := svc.Encode(ctx, input)
	if err != ErrAgentNotFound {
		t.Fatalf("expected ErrAgentNotFound, got %v", err)
	}
}

func TestEpisodeService_GetByID(t *testing.T) {
	svc, episodeStore, tenantID, agentID := setupEpisodeTest()
	ctx := context.Background()

	// Create an episode first
	ep := &domain.Episode{
		AgentID:    agentID,
		TenantID:   tenantID,
		RawContent: "Test episode",
	}
	_ = episodeStore.Create(ctx, ep)

	// Get it
	episode, err := svc.GetByID(ctx, ep.ID, tenantID)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if episode.RawContent != ep.RawContent {
		t.Fatalf("expected raw_content %q, got %q", ep.RawContent, episode.RawContent)
	}
}

func TestEpisodeService_GetByID_NotFound(t *testing.T) {
	svc, _, tenantID, _ := setupEpisodeTest()
	ctx := context.Background()

	_, err := svc.GetByID(ctx, uuid.New(), tenantID)
	if err != ErrEpisodeNotFound {
		t.Fatalf("expected ErrEpisodeNotFound, got %v", err)
	}
}

func TestEpisodeService_RecordOutcome(t *testing.T) {
	svc, episodeStore, tenantID, agentID := setupEpisodeTest()
	ctx := context.Background()

	// Create an episode first
	ep := &domain.Episode{
		AgentID:    agentID,
		TenantID:   tenantID,
		RawContent: "Test episode",
	}
	_ = episodeStore.Create(ctx, ep)

	// Record outcome
	err := svc.RecordOutcome(ctx, ep.ID, tenantID, domain.OutcomeSuccess, "User was satisfied")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// Verify outcome was recorded
	updated := episodeStore.episodes[ep.ID]
	if updated.Outcome != domain.OutcomeSuccess {
		t.Fatalf("expected outcome 'success', got %q", updated.Outcome)
	}
	if updated.OutcomeDescription != "User was satisfied" {
		t.Fatalf("expected description 'User was satisfied', got %q", updated.OutcomeDescription)
	}
}

func TestEpisodeService_RecordOutcome_InvalidType(t *testing.T) {
	svc, episodeStore, tenantID, agentID := setupEpisodeTest()
	ctx := context.Background()

	// Create an episode first
	ep := &domain.Episode{
		AgentID:    agentID,
		TenantID:   tenantID,
		RawContent: "Test episode",
	}
	_ = episodeStore.Create(ctx, ep)

	// Record invalid outcome
	err := svc.RecordOutcome(ctx, ep.ID, tenantID, "invalid", "")
	if err != ErrInvalidOutcomeType {
		t.Fatalf("expected ErrInvalidOutcomeType, got %v", err)
	}
}

func TestExtractTimeOfDay(t *testing.T) {
	tests := []struct {
		hour     int
		expected string
	}{
		{5, "morning"},
		{11, "morning"},
		{12, "afternoon"},
		{16, "afternoon"},
		{17, "evening"},
		{20, "evening"},
		{21, "night"},
		{23, "night"},
		{0, "night"},
		{4, "night"},
	}

	for _, tc := range tests {
		t := time.Date(2024, 1, 15, tc.hour, 0, 0, 0, time.UTC)
		result := extractTimeOfDay(t)
		if result != tc.expected {
			println("hour", tc.hour, "expected", tc.expected, "got", result)
		}
	}
}
