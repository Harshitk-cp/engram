package service

import (
	"context"
	"testing"
	"time"

	"github.com/Harshitk-cp/engram/internal/domain"
	"github.com/google/uuid"
)

func TestFeedbackEffects(t *testing.T) {
	tests := []struct {
		name               string
		feedbackType       domain.FeedbackType
		expectedLogOdds    float64
		expectedReinfDelta int
		expectedReview     bool
		expectedSummarize  bool
	}{
		{
			name:               "helpful increases confidence",
			feedbackType:       domain.FeedbackTypeHelpful,
			expectedLogOdds:    0.3,
			expectedReinfDelta: 1,
		},
		{
			name:               "unhelpful decreases confidence",
			feedbackType:       domain.FeedbackTypeUnhelpful,
			expectedLogOdds:    -0.5,
			expectedReinfDelta: -1,
		},
		{
			name:               "used slightly increases confidence",
			feedbackType:       domain.FeedbackTypeUsed,
			expectedLogOdds:    0.1,
			expectedReinfDelta: 0,
		},
		{
			name:               "ignored slightly decreases confidence",
			feedbackType:       domain.FeedbackTypeIgnored,
			expectedLogOdds:    -0.1,
			expectedReinfDelta: 0,
		},
		{
			name:               "contradicted triggers review",
			feedbackType:       domain.FeedbackTypeContradicted,
			expectedLogOdds:    -1.0,
			expectedReinfDelta: -2,
			expectedReview:     true,
		},
		{
			name:              "outdated triggers summarize",
			feedbackType:      domain.FeedbackTypeOutdated,
			expectedLogOdds:   -0.8,
			expectedReinfDelta: -1,
			expectedSummarize: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			effect, ok := domain.FeedbackEffects[tt.feedbackType]
			if !ok {
				t.Fatalf("no effect defined for feedback type %s", tt.feedbackType)
			}

			if effect.LogOddsDelta != tt.expectedLogOdds {
				t.Errorf("LogOddsDelta = %f, want %f", effect.LogOddsDelta, tt.expectedLogOdds)
			}
			if effect.ReinforcementDelta != tt.expectedReinfDelta {
				t.Errorf("ReinforcementDelta = %d, want %d", effect.ReinforcementDelta, tt.expectedReinfDelta)
			}
			if effect.TriggerReview != tt.expectedReview {
				t.Errorf("TriggerReview = %v, want %v", effect.TriggerReview, tt.expectedReview)
			}
			if effect.TriggerSummarize != tt.expectedSummarize {
				t.Errorf("TriggerSummarize = %v, want %v", effect.TriggerSummarize, tt.expectedSummarize)
			}
		})
	}
}

func TestLogOddsProportionality(t *testing.T) {
	delta := -1.0

	highConf := ApplyLogOddsDelta(0.95, delta)
	lowConf := ApplyLogOddsDelta(0.30, delta)

	highDrop := 0.95 - highConf
	lowDrop := 0.30 - lowConf

	if highDrop >= lowDrop {
		t.Errorf("high confidence should drop less: high=%.3f, low=%.3f", highDrop, lowDrop)
	}

	highPct := highDrop / 0.95 * 100
	lowPct := lowDrop / 0.30 * 100

	if highPct >= lowPct {
		t.Errorf("high confidence should have smaller %% change: high=%.1f%%, low=%.1f%%", highPct, lowPct)
	}
}

type mockMutationLogStore struct {
	logs []domain.MutationLog
}

func (m *mockMutationLogStore) Create(ctx context.Context, log *domain.MutationLog) error {
	log.ID = uuid.New()
	log.CreatedAt = time.Now()
	m.logs = append(m.logs, *log)
	return nil
}

func (m *mockMutationLogStore) GetByMemoryID(ctx context.Context, memoryID uuid.UUID, limit int) ([]domain.MutationLog, error) {
	var result []domain.MutationLog
	for _, log := range m.logs {
		if log.MemoryID == memoryID {
			result = append(result, log)
			if len(result) >= limit {
				break
			}
		}
	}
	return result, nil
}

func (m *mockMutationLogStore) GetByAgentID(ctx context.Context, agentID uuid.UUID, since time.Time, limit int) ([]domain.MutationLog, error) {
	var result []domain.MutationLog
	for _, log := range m.logs {
		if log.AgentID == agentID && log.CreatedAt.After(since) {
			result = append(result, log)
			if len(result) >= limit {
				break
			}
		}
	}
	return result, nil
}

func TestLearningService_RecordOutcome_Success(t *testing.T) {
	memStore := newMockMemoryStore()
	logger := testLogger()

	// Create a memory
	agentID := uuid.New()
	tenantID := uuid.New()
	mem := &domain.Memory{
		AgentID:            agentID,
		TenantID:           tenantID,
		Content:            "Test memory",
		Type:               domain.MemoryTypeFact,
		Confidence:         0.7,
		ReinforcementCount: 1,
	}
	_ = memStore.Create(context.Background(), mem)

	mutationStore := &mockMutationLogStore{}

	svc := NewLearningService(memStore, nil, logger)
	svc.SetMutationLogStore(mutationStore)

	record := domain.OutcomeRecord{
		EpisodeID:    uuid.New(),
		MemoriesUsed: []uuid.UUID{mem.ID},
		Outcome:      domain.OutcomeSuccess,
		OccurredAt:   time.Now(),
	}

	err := svc.RecordOutcome(context.Background(), record)
	if err != nil {
		t.Fatalf("RecordOutcome failed: %v", err)
	}

	// Check memory was updated
	updated, err := memStore.GetByID(context.Background(), mem.ID, tenantID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}

	helpfulEffect := domain.FeedbackEffects[domain.FeedbackTypeHelpful]
	expectedConf := ApplyLogOddsDelta(0.7, helpfulEffect.LogOddsDelta)
	if !approxEqual(updated.Confidence, expectedConf, 0.001) {
		t.Errorf("Confidence = %f, want ~%f", updated.Confidence, expectedConf)
	}

	if len(mutationStore.logs) != 1 {
		t.Errorf("Expected 1 mutation log, got %d", len(mutationStore.logs))
	}
	if len(mutationStore.logs) > 0 {
		log := mutationStore.logs[0]
		if log.MutationType != domain.MutationOutcome {
			t.Errorf("MutationType = %s, want %s", log.MutationType, domain.MutationOutcome)
		}
	}
}

func TestLearningService_RecordOutcome_Failure(t *testing.T) {
	memStore := newMockMemoryStore()
	logger := testLogger()

	// Create a memory
	agentID := uuid.New()
	tenantID := uuid.New()
	mem := &domain.Memory{
		AgentID:            agentID,
		TenantID:           tenantID,
		Content:            "Test memory",
		Type:               domain.MemoryTypeFact,
		Confidence:         0.7,
		ReinforcementCount: 3,
	}
	_ = memStore.Create(context.Background(), mem)

	svc := NewLearningService(memStore, nil, logger)

	record := domain.OutcomeRecord{
		EpisodeID:    uuid.New(),
		MemoriesUsed: []uuid.UUID{mem.ID},
		Outcome:      domain.OutcomeFailure,
		OccurredAt:   time.Now(),
	}

	err := svc.RecordOutcome(context.Background(), record)
	if err != nil {
		t.Fatalf("RecordOutcome failed: %v", err)
	}

	// Check memory was updated
	updated, err := memStore.GetByID(context.Background(), mem.ID, tenantID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}

	unhelpfulEffect := domain.FeedbackEffects[domain.FeedbackTypeUnhelpful]
	expectedConf := ApplyLogOddsDelta(0.7, unhelpfulEffect.LogOddsDelta)
	if !approxEqual(updated.Confidence, expectedConf, 0.001) {
		t.Errorf("Confidence = %f, want ~%f", updated.Confidence, expectedConf)
	}

	expectedReinf := 2
	if updated.ReinforcementCount != expectedReinf {
		t.Errorf("ReinforcementCount = %d, want %d", updated.ReinforcementCount, expectedReinf)
	}
}

func approxEqual(a, b, tolerance float32) bool {
	diff := a - b
	if diff < 0 {
		diff = -diff
	}
	return diff < tolerance
}

func TestLearningService_RecordOutcome_Neutral(t *testing.T) {
	memStore := newMockMemoryStore()
	logger := testLogger()

	agentID := uuid.New()
	tenantID := uuid.New()
	mem := &domain.Memory{
		AgentID:            agentID,
		TenantID:           tenantID,
		Content:            "Test memory",
		Type:               domain.MemoryTypeFact,
		Confidence:         0.7,
		ReinforcementCount: 1,
	}
	_ = memStore.Create(context.Background(), mem)

	svc := NewLearningService(memStore, nil, logger)

	record := domain.OutcomeRecord{
		EpisodeID:    uuid.New(),
		MemoriesUsed: []uuid.UUID{mem.ID},
		Outcome:      domain.OutcomeNeutral,
		OccurredAt:   time.Now(),
	}

	err := svc.RecordOutcome(context.Background(), record)
	if err != nil {
		t.Fatalf("RecordOutcome failed: %v", err)
	}

	updated, err := memStore.GetByID(context.Background(), mem.ID, tenantID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}

	if updated.Confidence != 0.7 {
		t.Errorf("Confidence = %f, want %f (unchanged)", updated.Confidence, 0.7)
	}
}

func TestConfidenceBounds(t *testing.T) {
	memStore := newMockMemoryStore()
	logger := testLogger()

	helpfulEffect := domain.FeedbackEffects[domain.FeedbackTypeHelpful]
	unhelpfulEffect := domain.FeedbackEffects[domain.FeedbackTypeUnhelpful]

	tests := []struct {
		name         string
		initialConf  float32
		outcome      domain.OutcomeType
		expectedConf float32
	}{
		{
			name:         "high confidence increases proportionally",
			initialConf:  0.97,
			outcome:      domain.OutcomeSuccess,
			expectedConf: ApplyLogOddsDelta(0.97, helpfulEffect.LogOddsDelta),
		},
		{
			name:         "low confidence decreases proportionally",
			initialConf:  0.15,
			outcome:      domain.OutcomeFailure,
			expectedConf: ApplyLogOddsDelta(0.15, unhelpfulEffect.LogOddsDelta),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			agentID := uuid.New()
			tenantID := uuid.New()
			mem := &domain.Memory{
				AgentID:            agentID,
				TenantID:           tenantID,
				Content:            "Test memory",
				Type:               domain.MemoryTypeFact,
				Confidence:         tt.initialConf,
				ReinforcementCount: 5,
			}
			_ = memStore.Create(context.Background(), mem)

			svc := NewLearningService(memStore, nil, logger)

			record := domain.OutcomeRecord{
				EpisodeID:    uuid.New(),
				MemoriesUsed: []uuid.UUID{mem.ID},
				Outcome:      tt.outcome,
				OccurredAt:   time.Now(),
			}

			_ = svc.RecordOutcome(context.Background(), record)

			updated, _ := memStore.GetByID(context.Background(), mem.ID, tenantID)
			if updated.Confidence < tt.expectedConf-0.001 || updated.Confidence > tt.expectedConf+0.001 {
				t.Errorf("Confidence = %f, want ~%f", updated.Confidence, tt.expectedConf)
			}
		})
	}
}

func TestConfidenceNeverExceedsBounds(t *testing.T) {
	for i := 0; i < 100; i++ {
		conf := ApplyLogOddsDelta(0.99, 10.0)
		if conf > DefaultMaxConfidence {
			t.Errorf("confidence %f exceeds max %f", conf, DefaultMaxConfidence)
		}
	}
	for i := 0; i < 100; i++ {
		conf := ApplyLogOddsDelta(0.01, -10.0)
		if conf < DefaultMinConfidence {
			t.Errorf("confidence %f below min %f", conf, DefaultMinConfidence)
		}
	}
}
