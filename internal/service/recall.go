package service

import (
	"math"
	"sort"
	"time"

	"github.com/Harshitk-cp/engram/internal/domain"
)

const (
	DefaultFreshnessDecay  = 0.0001
	DefaultConfidenceFloor = 0.3
)

type RecallScorer struct {
	FreshnessDecay  float64
	ConfidenceFloor float64
	TypeWeights     map[domain.MemoryType]float64
}

type ScoreBreakdown struct {
	Similarity float64 `json:"similarity"`
	Confidence float64 `json:"confidence"`
	Freshness  float64 `json:"freshness"`
	TypeWeight float64 `json:"type_weight,omitempty"`
	FinalScore float64 `json:"final_score"`
}

type ScoredMemory struct {
	domain.MemoryWithScore
	Breakdown *ScoreBreakdown `json:"score_breakdown,omitempty"`
}

func NewRecallScorer() *RecallScorer {
	return &RecallScorer{
		FreshnessDecay:  DefaultFreshnessDecay,
		ConfidenceFloor: DefaultConfidenceFloor,
	}
}

func (s *RecallScorer) Score(mem domain.MemoryWithScore, now time.Time) ScoredMemory {
	similarity := float64(mem.Score)
	confidence := float64(mem.Confidence)

	ageHours := now.Sub(mem.UpdatedAt).Hours()
	if ageHours < 0 {
		ageHours = 0
	}
	freshness := math.Exp(-s.FreshnessDecay * ageHours)

	typeWeight := 1.0
	if s.TypeWeights != nil {
		if w, ok := s.TypeWeights[mem.Type]; ok {
			typeWeight = w
		}
	}

	finalScore := similarity * confidence * freshness * typeWeight

	return ScoredMemory{
		MemoryWithScore: domain.MemoryWithScore{
			Memory: mem.Memory,
			Score:  float32(finalScore),
		},
		Breakdown: &ScoreBreakdown{
			Similarity: similarity,
			Confidence: confidence,
			Freshness:  freshness,
			TypeWeight: typeWeight,
			FinalScore: finalScore,
		},
	}
}

func (s *RecallScorer) Rank(memories []ScoredMemory) []ScoredMemory {
	sort.Slice(memories, func(i, j int) bool {
		return memories[i].Breakdown.FinalScore > memories[j].Breakdown.FinalScore
	})
	return memories
}

func (s *RecallScorer) ScoreAndRank(memories []domain.MemoryWithScore, now time.Time) []ScoredMemory {
	scored := make([]ScoredMemory, 0, len(memories))
	for _, mem := range memories {
		scored = append(scored, s.Score(mem, now))
	}
	return s.Rank(scored)
}
