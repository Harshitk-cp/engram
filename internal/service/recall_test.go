package service

import (
	"math"
	"testing"
	"time"

	"github.com/Harshitk-cp/engram/internal/domain"
	"github.com/google/uuid"
)

func floatEq(a, b float64) bool {
	return math.Abs(a-b) < 0.0001
}

func TestRecallScorer_Score(t *testing.T) {
	scorer := NewRecallScorer()
	now := time.Now()

	mem := domain.MemoryWithScore{
		Memory: domain.Memory{
			ID:         uuid.New(),
			Content:    "test",
			Type:       domain.MemoryTypeFact,
			Confidence: 0.8,
			UpdatedAt:  now,
		},
		Score: 0.9,
	}

	scored := scorer.Score(mem, now)

	if scored.Breakdown == nil {
		t.Fatal("expected breakdown to be set")
	}
	if !floatEq(scored.Breakdown.Similarity, 0.9) {
		t.Errorf("expected similarity 0.9, got %f", scored.Breakdown.Similarity)
	}
	if !floatEq(scored.Breakdown.Confidence, 0.8) {
		t.Errorf("expected confidence 0.8, got %f", scored.Breakdown.Confidence)
	}
	if scored.Breakdown.Freshness != 1.0 {
		t.Errorf("expected freshness 1.0 for current time, got %f", scored.Breakdown.Freshness)
	}
	if scored.Breakdown.TypeWeight != 1.0 {
		t.Errorf("expected default type weight 1.0, got %f", scored.Breakdown.TypeWeight)
	}
}

func TestRecallScorer_FreshnessDecay(t *testing.T) {
	scorer := NewRecallScorer()
	now := time.Now()

	mem := domain.MemoryWithScore{
		Memory: domain.Memory{
			ID:         uuid.New(),
			Content:    "test",
			Type:       domain.MemoryTypeFact,
			Confidence: 1.0,
			UpdatedAt:  now.Add(-24 * time.Hour),
		},
		Score: 1.0,
	}

	scored := scorer.Score(mem, now)

	if scored.Breakdown.Freshness >= 1.0 {
		t.Errorf("expected freshness < 1.0 for day-old memory, got %f", scored.Breakdown.Freshness)
	}
	if scored.Breakdown.Freshness < 0.99 {
		t.Errorf("freshness decayed too much for 24h, got %f", scored.Breakdown.Freshness)
	}
}

func TestRecallScorer_TypeWeights(t *testing.T) {
	scorer := NewRecallScorer()
	scorer.TypeWeights = map[domain.MemoryType]float64{
		domain.MemoryTypeConstraint: 2.0,
		domain.MemoryTypePreference: 0.5,
	}
	now := time.Now()

	constraint := domain.MemoryWithScore{
		Memory: domain.Memory{
			ID:         uuid.New(),
			Content:    "constraint",
			Type:       domain.MemoryTypeConstraint,
			Confidence: 1.0,
			UpdatedAt:  now,
		},
		Score: 1.0,
	}

	preference := domain.MemoryWithScore{
		Memory: domain.Memory{
			ID:         uuid.New(),
			Content:    "preference",
			Type:       domain.MemoryTypePreference,
			Confidence: 1.0,
			UpdatedAt:  now,
		},
		Score: 1.0,
	}

	scoredC := scorer.Score(constraint, now)
	scoredP := scorer.Score(preference, now)

	if scoredC.Breakdown.TypeWeight != 2.0 {
		t.Errorf("expected constraint type weight 2.0, got %f", scoredC.Breakdown.TypeWeight)
	}
	if scoredP.Breakdown.TypeWeight != 0.5 {
		t.Errorf("expected preference type weight 0.5, got %f", scoredP.Breakdown.TypeWeight)
	}
	if scoredC.Breakdown.FinalScore <= scoredP.Breakdown.FinalScore {
		t.Error("expected constraint to score higher than preference with type weights")
	}
}

func TestRecallScorer_Rank(t *testing.T) {
	scorer := NewRecallScorer()
	now := time.Now()

	memories := []domain.MemoryWithScore{
		{Memory: domain.Memory{ID: uuid.New(), Content: "low", Confidence: 0.5, UpdatedAt: now}, Score: 0.5},
		{Memory: domain.Memory{ID: uuid.New(), Content: "high", Confidence: 0.9, UpdatedAt: now}, Score: 0.9},
		{Memory: domain.Memory{ID: uuid.New(), Content: "mid", Confidence: 0.7, UpdatedAt: now}, Score: 0.7},
	}

	scored := scorer.ScoreAndRank(memories, now)

	if len(scored) != 3 {
		t.Fatalf("expected 3 results, got %d", len(scored))
	}
	if scored[0].Content != "high" {
		t.Errorf("expected highest scored first, got %s", scored[0].Content)
	}
	if scored[2].Content != "low" {
		t.Errorf("expected lowest scored last, got %s", scored[2].Content)
	}
	for i := 0; i < len(scored)-1; i++ {
		if scored[i].Breakdown.FinalScore < scored[i+1].Breakdown.FinalScore {
			t.Error("results not in descending score order")
		}
	}
}

func TestRecallScorer_CompositeScore(t *testing.T) {
	scorer := NewRecallScorer()
	scorer.TypeWeights = map[domain.MemoryType]float64{
		domain.MemoryTypeFact: 1.5,
	}
	now := time.Now()

	mem := domain.MemoryWithScore{
		Memory: domain.Memory{
			ID:         uuid.New(),
			Type:       domain.MemoryTypeFact,
			Confidence: 0.8,
			UpdatedAt:  now,
		},
		Score: 0.9,
	}

	scored := scorer.Score(mem, now)

	expected := 0.9 * 0.8 * 1.0 * 1.5
	if !floatEq(scored.Breakdown.FinalScore, expected) {
		t.Errorf("expected final score %f, got %f", expected, scored.Breakdown.FinalScore)
	}
}

func TestRecallScorer_BreakdownFieldsSet(t *testing.T) {
	scorer := NewRecallScorer()
	now := time.Now()

	mem := domain.MemoryWithScore{
		Memory: domain.Memory{
			ID:         uuid.New(),
			Type:       domain.MemoryTypeFact,
			Confidence: 0.75,
			UpdatedAt:  now.Add(-time.Hour),
		},
		Score: 0.85,
	}

	scored := scorer.Score(mem, now)

	if scored.Breakdown.Similarity == 0 {
		t.Error("similarity should be set")
	}
	if scored.Breakdown.Confidence == 0 {
		t.Error("confidence should be set")
	}
	if scored.Breakdown.Freshness == 0 {
		t.Error("freshness should be set")
	}
	if scored.Breakdown.FinalScore == 0 {
		t.Error("final score should be set")
	}
}
