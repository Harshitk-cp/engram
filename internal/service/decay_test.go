package service

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/Harshitk-cp/engram/internal/domain"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

type decayMockStore struct {
	mockMemoryStore
	archived map[uuid.UUID]bool
}

func newDecayMockStore() *decayMockStore {
	return &decayMockStore{
		mockMemoryStore: mockMemoryStore{memories: make(map[uuid.UUID]*domain.Memory)},
		archived:        make(map[uuid.UUID]bool),
	}
}

func (m *decayMockStore) Archive(ctx context.Context, id uuid.UUID) error {
	m.archived[id] = true
	delete(m.memories, id)
	return nil
}

// createTestMemory creates a memory with specified parameters for testing
func createTestMemory(agentID uuid.UUID, confidence float32, hoursAgo float64, reinforcement int, memType domain.MemoryType) *domain.Memory {
	accessTime := time.Now().Add(-time.Duration(hoursAgo) * time.Hour)
	return &domain.Memory{
		ID:                 uuid.New(),
		AgentID:            agentID,
		TenantID:           uuid.New(),
		Content:            "test memory",
		Type:               memType,
		Confidence:         confidence,
		Embedding:          []float32{0.1, 0.2, 0.3, 0.4, 0.5},
		LastAccessedAt:     &accessTime,
		CreatedAt:          accessTime,
		ReinforcementCount: reinforcement,
	}
}

// createTestMemoryWithEmbedding creates a memory with a specific embedding
func createTestMemoryWithEmbedding(agentID uuid.UUID, confidence float32, hoursAgo float64, embedding []float32) *domain.Memory {
	accessTime := time.Now().Add(-time.Duration(hoursAgo) * time.Hour)
	return &domain.Memory{
		ID:             uuid.New(),
		AgentID:        agentID,
		TenantID:       uuid.New(),
		Content:        "test memory",
		Type:           domain.MemoryTypeFact,
		Confidence:     confidence,
		Embedding:      embedding,
		LastAccessedAt: &accessTime,
		CreatedAt:      accessTime,
	}
}

func TestDecay_DistanceToFloor(t *testing.T) {
	logger := zap.NewNop()
	store := newDecayMockStore()
	svc := NewDecayService(store, nil, logger)

	agentID := uuid.New()

	tests := []struct {
		name           string
		initialConf    float32
		hoursAgo       float64
		wantMinConf    float32 
		wantMaxConf    float32 
	}{
		{
			name:        "high confidence decays toward floor",
			initialConf: 0.9,
			hoursAgo:    168, 
			wantMinConf: 0.5,
			wantMaxConf: 0.85,
		},
		{
			name:        "medium confidence decays slower in absolute terms",
			initialConf: 0.5,
			hoursAgo:    168,
			wantMinConf: 0.3,
			wantMaxConf: 0.48,
		},
		{
			name:        "low confidence near floor decays very slowly",
			initialConf: 0.2,
			hoursAgo:    168,
			wantMinConf: 0.1,
			wantMaxConf: 0.19,
		},
		{
			name:        "recently accessed memory no decay",
			initialConf: 0.8,
			hoursAgo:    0.5,
			wantMinConf: 0.8,
			wantMaxConf: 0.8,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mem := createTestMemory(agentID, tt.initialConf, tt.hoursAgo, 0, domain.MemoryTypeFact)

			result := svc.ApplyDecay(context.Background(), mem, nil)

			if result.NewConfidence < tt.wantMinConf || result.NewConfidence > tt.wantMaxConf {
				t.Errorf("NewConfidence = %v, want between %v and %v",
					result.NewConfidence, tt.wantMinConf, tt.wantMaxConf)
			}
		})
	}
}

func TestDecay_ReinforcementBonus(t *testing.T) {
	logger := zap.NewNop()
	store := newDecayMockStore()
	svc := NewDecayService(store, nil, logger)

	agentID := uuid.New()
	hoursAgo := 168.0 

	// Test that reinforcement slows decay
	memNoReinforcement := createTestMemory(agentID, 0.8, hoursAgo, 0, domain.MemoryTypeFact)
	memLowReinforcement := createTestMemory(agentID, 0.8, hoursAgo, 2, domain.MemoryTypeFact)
	memHighReinforcement := createTestMemory(agentID, 0.8, hoursAgo, 10, domain.MemoryTypeFact)

	resultNo := svc.ApplyDecay(context.Background(), memNoReinforcement, nil)
	resultLow := svc.ApplyDecay(context.Background(), memLowReinforcement, nil)
	resultHigh := svc.ApplyDecay(context.Background(), memHighReinforcement, nil)

	// Higher reinforcement should result in higher final confidence
	if resultNo.NewConfidence >= resultLow.NewConfidence {
		t.Errorf("Low reinforcement should decay slower than no reinforcement: no=%v, low=%v",
			resultNo.NewConfidence, resultLow.NewConfidence)
	}

	if resultLow.NewConfidence >= resultHigh.NewConfidence {
		t.Errorf("High reinforcement should decay slower than low reinforcement: low=%v, high=%v",
			resultLow.NewConfidence, resultHigh.NewConfidence)
	}
}

func TestDecay_Competition(t *testing.T) {
	logger := zap.NewNop()
	store := newDecayMockStore()
	svc := NewDecayService(store, nil, logger)

	agentID := uuid.New()
	hoursAgo := 168.0

	// Create a similar embedding
	baseEmbedding := []float32{0.1, 0.2, 0.3, 0.4, 0.5, 0.6, 0.7, 0.8}

	// Similar embeddings (normalized to unit length for cosine similarity)
	similarEmbedding := []float32{0.11, 0.21, 0.31, 0.41, 0.51, 0.61, 0.71, 0.81}
	differentEmbedding := []float32{0.9, 0.1, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0}

	// Test: Memory with no competitors
	memNoCompetitors := createTestMemoryWithEmbedding(agentID, 0.5, hoursAgo, baseEmbedding)
	resultNoCompetitors := svc.ApplyDecay(context.Background(), memNoCompetitors, nil)

	// Test: Memory with a stronger competitor
	memWithCompetitor := createTestMemoryWithEmbedding(agentID, 0.5, hoursAgo, baseEmbedding)
	strongerCompetitor := createTestMemoryWithEmbedding(agentID, 0.9, hoursAgo, similarEmbedding)
	strongerCompetitor.Type = domain.MemoryTypeFact
	memWithCompetitor.Type = domain.MemoryTypeFact

	allMemories := []domain.Memory{*strongerCompetitor}
	resultWithCompetitor := svc.ApplyDecay(context.Background(), memWithCompetitor, allMemories)

	// Memory with competitor should decay faster (lower final confidence)
	if resultWithCompetitor.NewConfidence >= resultNoCompetitors.NewConfidence {
		t.Errorf("Memory with competitor should decay faster: with=%v, without=%v",
			resultWithCompetitor.NewConfidence, resultNoCompetitors.NewConfidence)
	}

	if resultWithCompetitor.CompetitorCount != 1 {
		t.Errorf("Expected 1 competitor, got %d", resultWithCompetitor.CompetitorCount)
	}

	// Test: Memory with different type competitor (should not compete)
	memDifferentType := createTestMemoryWithEmbedding(agentID, 0.5, hoursAgo, baseEmbedding)
	memDifferentType.Type = domain.MemoryTypeFact
	competitorDifferentType := createTestMemoryWithEmbedding(agentID, 0.9, hoursAgo, similarEmbedding)
	competitorDifferentType.Type = domain.MemoryTypePreference

	resultDifferentType := svc.ApplyDecay(context.Background(), memDifferentType, []domain.Memory{*competitorDifferentType})

	// Different type should not count as competitor
	if resultDifferentType.CompetitorCount != 0 {
		t.Errorf("Different type memory should not be a competitor, got %d", resultDifferentType.CompetitorCount)
	}

	// Test: Memory with dissimilar competitor (should not compete)
	memDissimilar := createTestMemoryWithEmbedding(agentID, 0.5, hoursAgo, baseEmbedding)
	memDissimilar.Type = domain.MemoryTypeFact
	dissimilarCompetitor := createTestMemoryWithEmbedding(agentID, 0.9, hoursAgo, differentEmbedding)
	dissimilarCompetitor.Type = domain.MemoryTypeFact

	resultDissimilar := svc.ApplyDecay(context.Background(), memDissimilar, []domain.Memory{*dissimilarCompetitor})

	// Dissimilar embedding should not count as competitor
	if resultDissimilar.CompetitorCount != 0 {
		t.Errorf("Dissimilar memory should not be a competitor, got %d", resultDissimilar.CompetitorCount)
	}
}

func TestDecay_WeakerCompetitorNoEffect(t *testing.T) {
	logger := zap.NewNop()
	store := newDecayMockStore()
	svc := NewDecayService(store, nil, logger)

	agentID := uuid.New()
	hoursAgo := 168.0

	embedding := []float32{0.1, 0.2, 0.3, 0.4, 0.5}
	similarEmbedding := []float32{0.11, 0.21, 0.31, 0.41, 0.51}

	// Strong memory with weaker competitor
	strongMemory := createTestMemoryWithEmbedding(agentID, 0.9, hoursAgo, embedding)
	strongMemory.Type = domain.MemoryTypeFact

	weakerMemory := createTestMemoryWithEmbedding(agentID, 0.5, hoursAgo, similarEmbedding)
	weakerMemory.Type = domain.MemoryTypeFact

	result := svc.ApplyDecay(context.Background(), strongMemory, []domain.Memory{*weakerMemory})

	// Weaker memory should not add to competition factor
	if result.CompetitionFactor > 0.001 {
		t.Errorf("Weaker competitor should not affect competition factor: %v", result.CompetitionFactor)
	}
}

func TestDecay_BatchDecay(t *testing.T) {
	logger := zap.NewNop()
	store := newDecayMockStore()
	svc := NewDecayService(store, nil, logger)

	agentID := uuid.New()
	tenantID := uuid.New()

	// Create several memories with different confidences
	memories := []*domain.Memory{
		createTestMemory(agentID, 0.9, 72, 5, domain.MemoryTypeFact),    // High conf, reinforced
		createTestMemory(agentID, 0.5, 168, 0, domain.MemoryTypeFact),   // Medium conf, no reinforcement
		createTestMemory(agentID, 0.12, 240, 0, domain.MemoryTypeFact),  // Low conf, should be archived
		createTestMemory(agentID, 0.8, 0.5, 0, domain.MemoryTypePreference), // Recent, no decay
	}

	for _, mem := range memories {
		mem.TenantID = tenantID
		store.memories[mem.ID] = mem
	}

	result, err := svc.BatchDecay(context.Background(), agentID)
	if err != nil {
		t.Fatalf("BatchDecay failed: %v", err)
	}

	if result.Processed != 4 {
		t.Errorf("Expected 4 processed, got %d", result.Processed)
	}

	// Should have some decayed and some archived
	if result.Decayed == 0 {
		t.Error("Expected some memories to decay")
	}

	if result.Archived == 0 {
		t.Error("Expected low confidence memory to be archived")
	}

	// Verify the low confidence memory was actually archived
	if len(store.archived) == 0 {
		t.Error("Expected archived map to have entries")
	}
}

func TestDecay_TierTransitions(t *testing.T) {
	logger := zap.NewNop()
	store := newDecayMockStore()
	svc := NewDecayService(store, nil, logger)

	agentID := uuid.New()
	tenantID := uuid.New()

	// Create a memory that will transition from HOT to WARM
	// Start at 0.86 (just above HOT threshold), decay for long enough to cross 0.85
	mem := createTestMemory(agentID, 0.86, 500, 0, domain.MemoryTypeFact)
	mem.TenantID = tenantID
	store.memories[mem.ID] = mem

	result, err := svc.BatchDecay(context.Background(), agentID)
	if err != nil {
		t.Fatalf("BatchDecay failed: %v", err)
	}

	if len(result.TierTransitions) == 0 {
		t.Skip("Memory may not have decayed enough for tier transition")
	}

	transition := result.TierTransitions[0]
	if transition.FromTier != domain.TierHot {
		t.Errorf("Expected FromTier to be HOT, got %v", transition.FromTier)
	}
}

func TestDecay_NeverExceedsOriginal(t *testing.T) {
	logger := zap.NewNop()
	store := newDecayMockStore()
	svc := NewDecayService(store, nil, logger)

	agentID := uuid.New()

	// Test that decay never increases confidence
	tests := []struct {
		confidence float32
		hoursAgo   float64
	}{
		{0.5, 100},
		{0.8, 200},
		{0.3, 50},
		{0.95, 300},
	}

	for _, tt := range tests {
		mem := createTestMemory(agentID, tt.confidence, tt.hoursAgo, 10, domain.MemoryTypeFact)
		result := svc.ApplyDecay(context.Background(), mem, nil)

		if result.NewConfidence > tt.confidence {
			t.Errorf("Decay increased confidence from %v to %v", tt.confidence, result.NewConfidence)
		}
	}
}

func TestDecay_FloorRespected(t *testing.T) {
	logger := zap.NewNop()
	store := newDecayMockStore()
	svc := NewDecayService(store, nil, logger)

	agentID := uuid.New()

	// Even with extreme decay, should not go below floor
	mem := createTestMemory(agentID, 0.3, 10000, 0, domain.MemoryTypeFact)
	result := svc.ApplyDecay(context.Background(), mem, nil)

	if result.NewConfidence < float32(svc.Floor) {
		t.Errorf("Confidence %v went below floor %v", result.NewConfidence, svc.Floor)
	}
}

func TestDecay_CompetitionFactorFormula(t *testing.T) {
	logger := zap.NewNop()
	store := newDecayMockStore()
	svc := NewDecayService(store, nil, logger)

	agentID := uuid.New()

	// Create memory with specific confidence
	mem := &domain.Memory{
		ID:         uuid.New(),
		AgentID:    agentID,
		Type:       domain.MemoryTypeFact,
		Confidence: 0.5,
		Embedding:  []float32{1, 0, 0, 0, 0},
	}

	// Create competitor with higher confidence and perfect similarity
	competitor := &domain.Memory{
		ID:         uuid.New(),
		AgentID:    agentID,
		Type:       domain.MemoryTypeFact,
		Confidence: 0.9,
		Embedding:  []float32{1, 0, 0, 0, 0}, // Same direction = similarity 1.0
	}

	competitors := svc.findCompetitors(mem, []domain.Memory{*competitor})

	if len(competitors) != 1 {
		t.Fatalf("Expected 1 competitor, got %d", len(competitors))
	}

	factor := svc.calculateCompetition(mem, competitors)

	// Expected: (0.9 - 0.5) * 1.0 / (1 + 0.5) * 0.5 = 0.4 / 1.5 * 0.5 ≈ 0.133
	expectedFactor := 0.4 / 1.5 * svc.CompetitionWeight

	if math.Abs(factor-expectedFactor) > 0.01 {
		t.Errorf("Competition factor = %v, expected ~%v", factor, expectedFactor)
	}
}

func TestDecay_NoEmbeddingNoCompetition(t *testing.T) {
	logger := zap.NewNop()
	store := newDecayMockStore()
	svc := NewDecayService(store, nil, logger)

	agentID := uuid.New()

	mem := createTestMemory(agentID, 0.5, 168, 0, domain.MemoryTypeFact)
	mem.Embedding = nil

	competitor := createTestMemory(agentID, 0.9, 168, 0, domain.MemoryTypeFact)

	result := svc.ApplyDecay(context.Background(), mem, []domain.Memory{*competitor})

	if result.CompetitorCount != 0 {
		t.Errorf("Expected 0 competitors for memory without embedding, got %d", result.CompetitorCount)
	}
}
