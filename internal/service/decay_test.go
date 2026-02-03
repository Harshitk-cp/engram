package service

import (
	"context"
	"testing"
	"time"

	"github.com/Harshitk-cp/engram/internal/domain"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

type mockMemoryStoreForDecay struct {
	memories []domain.Memory
	agentIDs []uuid.UUID
	archived []uuid.UUID
	updated  map[uuid.UUID]float32
	boosted  map[uuid.UUID]float32
}

func newMockMemoryStoreForDecay() *mockMemoryStoreForDecay {
	return &mockMemoryStoreForDecay{
		updated: make(map[uuid.UUID]float32),
		boosted: make(map[uuid.UUID]float32),
	}
}

func (m *mockMemoryStoreForDecay) Create(ctx context.Context, mem *domain.Memory) error {
	mem.ID = uuid.New()
	now := time.Now()
	mem.CreatedAt = now
	mem.UpdatedAt = now
	mem.LastAccessedAt = &now
	m.memories = append(m.memories, *mem)
	return nil
}

func (m *mockMemoryStoreForDecay) GetByID(ctx context.Context, id uuid.UUID, tenantID uuid.UUID) (*domain.Memory, error) {
	return nil, nil
}

func (m *mockMemoryStoreForDecay) Delete(ctx context.Context, id uuid.UUID, tenantID uuid.UUID) error {
	return nil
}

func (m *mockMemoryStoreForDecay) Recall(ctx context.Context, embedding []float32, agentID uuid.UUID, tenantID uuid.UUID, opts domain.RecallOpts) ([]domain.MemoryWithScore, error) {
	return nil, nil
}

func (m *mockMemoryStoreForDecay) CountByAgentAndType(ctx context.Context, agentID uuid.UUID, memType domain.MemoryType) (int, error) {
	return 0, nil
}

func (m *mockMemoryStoreForDecay) ListOldestByAgentAndType(ctx context.Context, agentID uuid.UUID, memType domain.MemoryType, limit int) ([]domain.Memory, error) {
	return nil, nil
}

func (m *mockMemoryStoreForDecay) DeleteExpired(ctx context.Context) (int64, error) {
	return 0, nil
}

func (m *mockMemoryStoreForDecay) DeleteByRetention(ctx context.Context, agentID uuid.UUID, memType domain.MemoryType, retentionDays int) (int64, error) {
	return 0, nil
}

func (m *mockMemoryStoreForDecay) FindSimilar(ctx context.Context, agentID uuid.UUID, tenantID uuid.UUID, embedding []float32, threshold float32) ([]domain.MemoryWithScore, error) {
	return nil, nil
}

func (m *mockMemoryStoreForDecay) UpdateReinforcement(ctx context.Context, id uuid.UUID, confidence float32, reinforcementCount int) error {
	return nil
}

func (m *mockMemoryStoreForDecay) UpdateConfidence(ctx context.Context, id uuid.UUID, confidence float32) error {
	m.updated[id] = confidence
	return nil
}

func (m *mockMemoryStoreForDecay) ListDistinctAgentIDs(ctx context.Context) ([]uuid.UUID, error) {
	return m.agentIDs, nil
}

func (m *mockMemoryStoreForDecay) GetByAgentForDecay(ctx context.Context, agentID uuid.UUID) ([]domain.Memory, error) {
	var result []domain.Memory
	for _, mem := range m.memories {
		if mem.AgentID == agentID {
			result = append(result, mem)
		}
	}
	return result, nil
}

func (m *mockMemoryStoreForDecay) Archive(ctx context.Context, id uuid.UUID) error {
	m.archived = append(m.archived, id)
	return nil
}

func (m *mockMemoryStoreForDecay) IncrementAccessAndBoost(ctx context.Context, id uuid.UUID, boost float32) error {
	m.boosted[id] = boost
	return nil
}

type mockEpisodeStoreForDecay struct {
	decayedCount int64
	weakEpisodes []domain.Episode
	archived     []uuid.UUID
}

func (m *mockEpisodeStoreForDecay) Create(ctx context.Context, e *domain.Episode) error {
	return nil
}

func (m *mockEpisodeStoreForDecay) GetByID(ctx context.Context, id uuid.UUID, tenantID uuid.UUID) (*domain.Episode, error) {
	return nil, nil
}

func (m *mockEpisodeStoreForDecay) GetByConversationID(ctx context.Context, conversationID uuid.UUID, tenantID uuid.UUID) ([]domain.Episode, error) {
	return nil, nil
}

func (m *mockEpisodeStoreForDecay) GetByTimeRange(ctx context.Context, agentID uuid.UUID, tenantID uuid.UUID, start, end time.Time) ([]domain.Episode, error) {
	return nil, nil
}

func (m *mockEpisodeStoreForDecay) GetByImportance(ctx context.Context, agentID uuid.UUID, tenantID uuid.UUID, minImportance float32, limit int) ([]domain.Episode, error) {
	return nil, nil
}

func (m *mockEpisodeStoreForDecay) FindSimilar(ctx context.Context, agentID uuid.UUID, tenantID uuid.UUID, embedding []float32, threshold float32, limit int) ([]domain.EpisodeWithScore, error) {
	return nil, nil
}

func (m *mockEpisodeStoreForDecay) GetUnconsolidated(ctx context.Context, agentID uuid.UUID, limit int) ([]domain.Episode, error) {
	return nil, nil
}

func (m *mockEpisodeStoreForDecay) GetByConsolidationStatus(ctx context.Context, agentID uuid.UUID, tenantID uuid.UUID, status domain.ConsolidationStatus, limit int) ([]domain.Episode, error) {
	return nil, nil
}

func (m *mockEpisodeStoreForDecay) UpdateConsolidationStatus(ctx context.Context, id uuid.UUID, status domain.ConsolidationStatus) error {
	return nil
}

func (m *mockEpisodeStoreForDecay) LinkDerivedMemory(ctx context.Context, episodeID uuid.UUID, memoryID uuid.UUID, memoryType string) error {
	return nil
}

func (m *mockEpisodeStoreForDecay) ApplyDecay(ctx context.Context, agentID uuid.UUID) (int64, error) {
	return m.decayedCount, nil
}

func (m *mockEpisodeStoreForDecay) GetWeakMemories(ctx context.Context, agentID uuid.UUID, threshold float32) ([]domain.Episode, error) {
	return m.weakEpisodes, nil
}

func (m *mockEpisodeStoreForDecay) Archive(ctx context.Context, id uuid.UUID) error {
	m.archived = append(m.archived, id)
	return nil
}

func (m *mockEpisodeStoreForDecay) UpdateStrength(ctx context.Context, id uuid.UUID, strength float32) error {
	return nil
}

func (m *mockEpisodeStoreForDecay) RecordAccess(ctx context.Context, id uuid.UUID) error {
	return nil
}

func (m *mockEpisodeStoreForDecay) UpdateOutcome(ctx context.Context, id uuid.UUID, outcome domain.OutcomeType, description string) error {
	return nil
}

func (m *mockEpisodeStoreForDecay) CreateAssociation(ctx context.Context, a *domain.EpisodeAssociation) error {
	return nil
}

func (m *mockEpisodeStoreForDecay) GetAssociations(ctx context.Context, episodeID uuid.UUID) ([]domain.EpisodeAssociation, error) {
	return nil, nil
}

func (m *mockEpisodeStoreForDecay) GetByAgentForDecay(ctx context.Context, agentID uuid.UUID) ([]domain.Episode, error) {
	return nil, nil
}

func TestDecayService_RunDecayForAgent(t *testing.T) {
	logger := zap.NewNop()
	agentID := uuid.New()
	tenantID := uuid.New()

	recentTime := time.Now().Add(-1 * time.Hour)
	fewDaysAgo := time.Now().Add(-5 * 24 * time.Hour)
	oldTime := time.Now().Add(-60 * 24 * time.Hour)

	tests := []struct {
		name             string
		memories         []domain.Memory
		wantDecayed      int
		wantArchived     int
	}{
		{
			name:         "no memories",
			memories:     nil,
			wantDecayed:  0,
			wantArchived: 0,
		},
		{
			name: "recent memory no decay",
			memories: []domain.Memory{
				{
					ID:             uuid.New(),
					AgentID:        agentID,
					TenantID:       tenantID,
					Confidence:     0.8,
					DecayRate:      0.02,
					LastAccessedAt: &recentTime,
				},
			},
			wantDecayed:  0,
			wantArchived: 0,
		},
		{
			name: "moderately old memory decays",
			memories: []domain.Memory{
				{
					ID:             uuid.New(),
					AgentID:        agentID,
					TenantID:       tenantID,
					Confidence:     0.9,
					DecayRate:      0.02,
					LastAccessedAt: &fewDaysAgo,
				},
			},
			wantDecayed:  1,
			wantArchived: 0,
		},
		{
			name: "very old low confidence memory archives",
			memories: []domain.Memory{
				{
					ID:             uuid.New(),
					AgentID:        agentID,
					TenantID:       tenantID,
					Confidence:     0.25,
					DecayRate:      0.1,
					LastAccessedAt: &oldTime,
				},
			},
			wantDecayed:  0,
			wantArchived: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			memStore := newMockMemoryStoreForDecay()
			memStore.memories = tt.memories
			memStore.agentIDs = []uuid.UUID{agentID}

			episodeStore := &mockEpisodeStoreForDecay{}

			svc := NewDecayService(memStore, episodeStore, logger)

			result, err := svc.RunDecayForAgent(context.Background(), agentID)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if result.MemoriesDecayed != tt.wantDecayed {
				t.Errorf("MemoriesDecayed = %d, want %d", result.MemoriesDecayed, tt.wantDecayed)
			}
			if result.MemoriesArchived != tt.wantArchived {
				t.Errorf("MemoriesArchived = %d, want %d", result.MemoriesArchived, tt.wantArchived)
			}
		})
	}
}

func TestDecayService_HighReinforcementSlowsDecay(t *testing.T) {
	logger := zap.NewNop()
	agentID := uuid.New()
	tenantID := uuid.New()
	fewDaysAgo := time.Now().Add(-10 * 24 * time.Hour)

	memStore := newMockMemoryStoreForDecay()
	memStore.agentIDs = []uuid.UUID{agentID}

	lowReinforcementMem := domain.Memory{
		ID:                 uuid.New(),
		AgentID:            agentID,
		TenantID:           tenantID,
		Confidence:         0.8,
		DecayRate:          0.02,
		ReinforcementCount: 1,
		LastAccessedAt:     &fewDaysAgo,
	}

	highReinforcementMem := domain.Memory{
		ID:                 uuid.New(),
		AgentID:            agentID,
		TenantID:           tenantID,
		Confidence:         0.8,
		DecayRate:          0.02,
		ReinforcementCount: 10,
		LastAccessedAt:     &fewDaysAgo,
	}

	memStore.memories = []domain.Memory{lowReinforcementMem, highReinforcementMem}

	episodeStore := &mockEpisodeStoreForDecay{}
	svc := NewDecayService(memStore, episodeStore, logger)

	_, err := svc.RunDecayForAgent(context.Background(), agentID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	lowConf, lowOk := memStore.updated[lowReinforcementMem.ID]
	highConf, highOk := memStore.updated[highReinforcementMem.ID]

	if !lowOk {
		t.Fatal("low reinforcement memory was not updated")
	}
	if !highOk {
		t.Fatal("high reinforcement memory was not updated")
	}

	if highConf <= lowConf {
		t.Errorf("high reinforcement memory should decay slower: high=%f, low=%f", highConf, lowConf)
	}
}
