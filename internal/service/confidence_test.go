package service

import (
	"context"
	"testing"
	"time"

	"github.com/Harshitk-cp/engram/internal/domain"
	"github.com/Harshitk-cp/engram/internal/store"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

type mockMemoryStoreForConfidence struct {
	memories     map[uuid.UUID]*domain.Memory
	reinforced   map[uuid.UUID]struct{ conf float32; count int }
}

func newMockMemoryStoreForConfidence() *mockMemoryStoreForConfidence {
	return &mockMemoryStoreForConfidence{
		memories:   make(map[uuid.UUID]*domain.Memory),
		reinforced: make(map[uuid.UUID]struct{ conf float32; count int }),
	}
}

func (m *mockMemoryStoreForConfidence) Create(ctx context.Context, mem *domain.Memory) error {
	mem.ID = uuid.New()
	now := time.Now()
	mem.CreatedAt = now
	mem.UpdatedAt = now
	if mem.LastAccessedAt == nil {
		mem.LastAccessedAt = &now
	}
	m.memories[mem.ID] = mem
	return nil
}

func (m *mockMemoryStoreForConfidence) GetByID(ctx context.Context, id uuid.UUID, tenantID uuid.UUID) (*domain.Memory, error) {
	mem, ok := m.memories[id]
	if !ok {
		return nil, store.ErrNotFound
	}
	return mem, nil
}

func (m *mockMemoryStoreForConfidence) Delete(ctx context.Context, id uuid.UUID, tenantID uuid.UUID) error {
	return nil
}

func (m *mockMemoryStoreForConfidence) Recall(ctx context.Context, embedding []float32, agentID uuid.UUID, tenantID uuid.UUID, opts domain.RecallOpts) ([]domain.MemoryWithScore, error) {
	return nil, nil
}

func (m *mockMemoryStoreForConfidence) CountByAgentAndType(ctx context.Context, agentID uuid.UUID, memType domain.MemoryType) (int, error) {
	return 0, nil
}

func (m *mockMemoryStoreForConfidence) ListOldestByAgentAndType(ctx context.Context, agentID uuid.UUID, memType domain.MemoryType, limit int) ([]domain.Memory, error) {
	return nil, nil
}

func (m *mockMemoryStoreForConfidence) DeleteExpired(ctx context.Context) (int64, error) {
	return 0, nil
}

func (m *mockMemoryStoreForConfidence) DeleteByRetention(ctx context.Context, agentID uuid.UUID, memType domain.MemoryType, retentionDays int) (int64, error) {
	return 0, nil
}

func (m *mockMemoryStoreForConfidence) FindSimilar(ctx context.Context, agentID uuid.UUID, tenantID uuid.UUID, embedding []float32, threshold float32) ([]domain.MemoryWithScore, error) {
	return nil, nil
}

func (m *mockMemoryStoreForConfidence) UpdateReinforcement(ctx context.Context, id uuid.UUID, confidence float32, reinforcementCount int) error {
	mem := m.memories[id]
	if mem != nil {
		mem.Confidence = confidence
		mem.ReinforcementCount = reinforcementCount
	}
	m.reinforced[id] = struct{ conf float32; count int }{confidence, reinforcementCount}
	return nil
}

func (m *mockMemoryStoreForConfidence) UpdateConfidence(ctx context.Context, id uuid.UUID, confidence float32) error {
	mem := m.memories[id]
	if mem != nil {
		mem.Confidence = confidence
	}
	return nil
}

func (m *mockMemoryStoreForConfidence) ListDistinctAgentIDs(ctx context.Context) ([]uuid.UUID, error) {
	return nil, nil
}

func (m *mockMemoryStoreForConfidence) GetByAgentForDecay(ctx context.Context, agentID uuid.UUID) ([]domain.Memory, error) {
	return nil, nil
}

func (m *mockMemoryStoreForConfidence) Archive(ctx context.Context, id uuid.UUID) error {
	return nil
}

func (m *mockMemoryStoreForConfidence) IncrementAccessAndBoost(ctx context.Context, id uuid.UUID, boost float32) error {
	return nil
}

func (m *mockMemoryStoreForConfidence) GetByTier(ctx context.Context, agentID uuid.UUID, tenantID uuid.UUID, tier domain.MemoryTier, limit int) ([]domain.Memory, error) {
	return nil, nil
}

func (m *mockMemoryStoreForConfidence) GetTierCounts(ctx context.Context, agentID uuid.UUID, tenantID uuid.UUID) (map[domain.MemoryTier]int, error) {
	return nil, nil
}

func (m *mockMemoryStoreForConfidence) SetNeedsReview(ctx context.Context, id uuid.UUID, needsReview bool) error {
	return nil
}

func (m *mockMemoryStoreForConfidence) GetNeedsReview(ctx context.Context, agentID uuid.UUID, tenantID uuid.UUID, limit int) ([]domain.Memory, error) {
	return nil, nil
}

func TestConfidenceService_Reinforce(t *testing.T) {
	logger := zap.NewNop()
	agentID := uuid.New()
	tenantID := uuid.New()

	memStore := newMockMemoryStoreForConfidence()

	mem := &domain.Memory{
		AgentID:            agentID,
		TenantID:           tenantID,
		Confidence:         0.6,
		ReinforcementCount: 1,
		Provenance:         domain.ProvenanceAgent,
	}
	_ = memStore.Create(context.Background(), mem)

	svc := NewConfidenceService(memStore, logger)

	err := svc.Reinforce(context.Background(), mem.ID, tenantID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	updated := memStore.reinforced[mem.ID]
	expectedConf := 0.6 + DefaultReinforcementBoost
	if float64(updated.conf) < expectedConf-0.001 || float64(updated.conf) > expectedConf+0.001 {
		t.Errorf("confidence = %f, want ~%f", updated.conf, expectedConf)
	}
	if updated.count != 2 {
		t.Errorf("reinforcement count = %d, want 2", updated.count)
	}
}

func TestConfidenceService_Reinforce_CapsAtMax(t *testing.T) {
	logger := zap.NewNop()
	agentID := uuid.New()
	tenantID := uuid.New()

	memStore := newMockMemoryStoreForConfidence()

	mem := &domain.Memory{
		AgentID:            agentID,
		TenantID:           tenantID,
		Confidence:         0.98,
		ReinforcementCount: 5,
		Provenance:         domain.ProvenanceUser,
	}
	_ = memStore.Create(context.Background(), mem)

	svc := NewConfidenceService(memStore, logger)

	err := svc.Reinforce(context.Background(), mem.ID, tenantID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	updated := memStore.reinforced[mem.ID]
	if updated.conf > float32(DefaultMaxConfidence) {
		t.Errorf("confidence = %f, should not exceed %f", updated.conf, DefaultMaxConfidence)
	}
}

func TestConfidenceService_Penalize(t *testing.T) {
	logger := zap.NewNop()
	agentID := uuid.New()
	tenantID := uuid.New()

	memStore := newMockMemoryStoreForConfidence()

	mem := &domain.Memory{
		AgentID:            agentID,
		TenantID:           tenantID,
		Confidence:         0.6,
		ReinforcementCount: 3,
		Provenance:         domain.ProvenanceAgent,
	}
	_ = memStore.Create(context.Background(), mem)

	svc := NewConfidenceService(memStore, logger)

	err := svc.Penalize(context.Background(), mem.ID, tenantID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	updated := memStore.reinforced[mem.ID]
	expectedConf := float32(0.6 - DefaultContradictionPenalty)
	if updated.conf != expectedConf {
		t.Errorf("confidence = %f, want %f", updated.conf, expectedConf)
	}
	if updated.count != 2 {
		t.Errorf("reinforcement count = %d, want 2", updated.count)
	}
}

func TestConfidenceService_Penalize_FloorsAtMin(t *testing.T) {
	logger := zap.NewNop()
	agentID := uuid.New()
	tenantID := uuid.New()

	memStore := newMockMemoryStoreForConfidence()

	mem := &domain.Memory{
		AgentID:            agentID,
		TenantID:           tenantID,
		Confidence:         0.05,
		ReinforcementCount: 1,
		Provenance:         domain.ProvenanceInferred,
	}
	_ = memStore.Create(context.Background(), mem)

	svc := NewConfidenceService(memStore, logger)

	err := svc.Penalize(context.Background(), mem.ID, tenantID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	updated := memStore.reinforced[mem.ID]
	if updated.conf < float32(DefaultMinConfidence) {
		t.Errorf("confidence = %f, should not go below %f", updated.conf, DefaultMinConfidence)
	}
}

func TestConfidenceService_ApplyDecay(t *testing.T) {
	logger := zap.NewNop()
	memStore := newMockMemoryStoreForConfidence()
	svc := NewConfidenceService(memStore, logger)

	tests := []struct {
		name           string
		hoursAgo       float64
		initialConf    float32
		wantDecayed    bool
	}{
		{
			name:        "recent memory no decay",
			hoursAgo:    1,
			initialConf: 0.8,
			wantDecayed: false,
		},
		{
			name:        "old memory decays",
			hoursAgo:    700,
			initialConf: 0.8,
			wantDecayed: true,
		},
		{
			name:        "very old memory significant decay",
			hoursAgo:    2000,
			initialConf: 0.9,
			wantDecayed: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			accessTime := time.Now().Add(-time.Duration(tt.hoursAgo) * time.Hour)
			mem := &domain.Memory{
				Confidence:     tt.initialConf,
				LastAccessedAt: &accessTime,
			}

			decayed := svc.ApplyDecay(mem)

			if tt.wantDecayed {
				if decayed >= float64(tt.initialConf) {
					t.Errorf("expected decay, got %f (initial: %f)", decayed, tt.initialConf)
				}
			} else {
				diff := float64(tt.initialConf) - decayed
				if diff > 0.01 {
					t.Errorf("unexpected significant decay: %f (initial: %f)", decayed, tt.initialConf)
				}
			}
		})
	}
}

func TestConfidenceService_ApplyDecay_NilLastAccessed(t *testing.T) {
	logger := zap.NewNop()
	memStore := newMockMemoryStoreForConfidence()
	svc := NewConfidenceService(memStore, logger)

	mem := &domain.Memory{
		Confidence:     0.8,
		LastAccessedAt: nil,
	}

	decayed := svc.ApplyDecay(mem)

	diff := decayed - 0.8
	if diff < -0.001 || diff > 0.001 {
		t.Errorf("expected no decay when LastAccessedAt is nil, got %f", decayed)
	}
}

func TestProvenance_InitialConfidence(t *testing.T) {
	tests := []struct {
		provenance domain.Provenance
		expected   float32
	}{
		{domain.ProvenanceUser, 0.9},
		{domain.ProvenanceTool, 0.8},
		{domain.ProvenanceAgent, 0.6},
		{domain.ProvenanceDerived, 0.5},
		{domain.ProvenanceInferred, 0.4},
		{domain.Provenance("unknown"), 0.5},
	}

	for _, tt := range tests {
		t.Run(string(tt.provenance), func(t *testing.T) {
			got := tt.provenance.InitialConfidence()
			if got != tt.expected {
				t.Errorf("InitialConfidence() = %f, want %f", got, tt.expected)
			}
		})
	}
}

func TestValidProvenance(t *testing.T) {
	validCases := []string{"user", "agent", "tool", "derived", "inferred"}
	for _, v := range validCases {
		if !domain.ValidProvenance(v) {
			t.Errorf("expected %q to be valid", v)
		}
	}

	invalidCases := []string{"", "unknown", "invalid", "USER"}
	for _, v := range invalidCases {
		if domain.ValidProvenance(v) {
			t.Errorf("expected %q to be invalid", v)
		}
	}
}

func TestConfidenceService_GetStats(t *testing.T) {
	logger := zap.NewNop()
	agentID := uuid.New()
	tenantID := uuid.New()

	memStore := newMockMemoryStoreForConfidence()
	svc := NewConfidenceService(memStore, logger)

	accessTime := time.Now().Add(-100 * time.Hour)
	memID := uuid.New()
	mem := &domain.Memory{
		ID:                 memID,
		AgentID:            agentID,
		TenantID:           tenantID,
		Confidence:         0.7,
		ReinforcementCount: 3,
		Provenance:         domain.ProvenanceTool,
		LastAccessedAt:     &accessTime,
	}
	memStore.memories[memID] = mem

	stats, err := svc.GetStats(context.Background(), memID, tenantID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if stats.RawConfidence != 0.7 {
		t.Errorf("RawConfidence = %f, want 0.7", stats.RawConfidence)
	}
	if stats.ReinforcementCount != 3 {
		t.Errorf("ReinforcementCount = %d, want 3", stats.ReinforcementCount)
	}
	if stats.Provenance != "tool" {
		t.Errorf("Provenance = %s, want tool", stats.Provenance)
	}
	if stats.DecayedConfidence >= float64(stats.RawConfidence) {
		t.Error("DecayedConfidence should be less than RawConfidence after 100 hours")
	}
	if stats.HoursSinceAccess < 99 || stats.HoursSinceAccess > 101 {
		t.Errorf("HoursSinceAccess = %f, expected ~100", stats.HoursSinceAccess)
	}
}
