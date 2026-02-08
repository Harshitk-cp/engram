package service

import (
	"context"
	"math"
	"time"

	"github.com/Harshitk-cp/engram/internal/domain"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

const (
	DefaultReinforcementLogOdds = 0.3
	DefaultContradictionLogOdds = 0.5
	DefaultMaxConfidence        = 0.99
	DefaultMinConfidence        = 0.01
	DefaultDecayLambda          = 0.001
)

func Logit(p float64) float64 {
	p = clampConfidence(p)
	return math.Log(p / (1 - p))
}

func Sigmoid(x float64) float64 {
	return 1.0 / (1.0 + math.Exp(-x))
}

func ApplyLogOddsDelta(confidence float32, logOddsDelta float64) float32 {
	logOdds := Logit(float64(confidence))
	newLogOdds := logOdds + logOddsDelta
	newConf := Sigmoid(newLogOdds)
	return float32(clampConfidence(newConf))
}

func clampConfidence(p float64) float64 {
	if p < DefaultMinConfidence {
		return DefaultMinConfidence
	}
	if p > DefaultMaxConfidence {
		return DefaultMaxConfidence
	}
	return p
}

type ConfidenceService struct {
	store  domain.MemoryStore
	logger *zap.Logger

	ReinforcementLogOdds float64
	ContradictionLogOdds float64
	MaxConfidence        float64
	MinConfidence        float64
	DecayLambda          float64
}

func NewConfidenceService(store domain.MemoryStore, logger *zap.Logger) *ConfidenceService {
	return &ConfidenceService{
		store:                store,
		logger:               logger,
		ReinforcementLogOdds: DefaultReinforcementLogOdds,
		ContradictionLogOdds: DefaultContradictionLogOdds,
		MaxConfidence:        DefaultMaxConfidence,
		MinConfidence:        DefaultMinConfidence,
		DecayLambda:          DefaultDecayLambda,
	}
}

func (s *ConfidenceService) Reinforce(ctx context.Context, memoryID uuid.UUID, tenantID uuid.UUID) error {
	memory, err := s.store.GetByID(ctx, memoryID, tenantID)
	if err != nil {
		return err
	}

	newConfidence := ApplyLogOddsDelta(memory.Confidence, s.ReinforcementLogOdds)
	newCount := memory.ReinforcementCount + 1

	s.logger.Debug("reinforcing memory",
		zap.String("memory_id", memoryID.String()),
		zap.Float32("old_confidence", memory.Confidence),
		zap.Float32("new_confidence", newConfidence),
		zap.Int("reinforcement_count", newCount))

	return s.store.UpdateReinforcement(ctx, memoryID, newConfidence, newCount)
}

func (s *ConfidenceService) Penalize(ctx context.Context, memoryID uuid.UUID, tenantID uuid.UUID) error {
	memory, err := s.store.GetByID(ctx, memoryID, tenantID)
	if err != nil {
		return err
	}

	newConfidence := ApplyLogOddsDelta(memory.Confidence, -s.ContradictionLogOdds)
	newCount := memory.ReinforcementCount - 1
	if newCount < 0 {
		newCount = 0
	}

	s.logger.Debug("penalizing memory",
		zap.String("memory_id", memoryID.String()),
		zap.Float32("old_confidence", memory.Confidence),
		zap.Float32("new_confidence", newConfidence),
		zap.Int("reinforcement_count", newCount))

	return s.store.UpdateReinforcement(ctx, memoryID, newConfidence, newCount)
}

func (s *ConfidenceService) ApplyDecay(memory *domain.Memory) float64 {
	if memory.LastAccessedAt == nil {
		return float64(memory.Confidence)
	}

	elapsed := time.Since(*memory.LastAccessedAt)
	hours := elapsed.Hours()

	decayFactor := math.Exp(-s.DecayLambda * hours)
	decayed := float64(memory.Confidence) * decayFactor

	if decayed < s.MinConfidence {
		decayed = s.MinConfidence
	}

	return decayed
}

func (s *ConfidenceService) GetDecayedConfidence(ctx context.Context, memoryID uuid.UUID, tenantID uuid.UUID) (float64, error) {
	memory, err := s.store.GetByID(ctx, memoryID, tenantID)
	if err != nil {
		return 0, err
	}
	return s.ApplyDecay(memory), nil
}

type ConfidenceStats struct {
	MemoryID           uuid.UUID `json:"memory_id"`
	RawConfidence      float32   `json:"raw_confidence"`
	DecayedConfidence  float64   `json:"decayed_confidence"`
	ReinforcementCount int       `json:"reinforcement_count"`
	Provenance         string    `json:"provenance"`
	HoursSinceAccess   float64   `json:"hours_since_access"`
	DecayFactor        float64   `json:"decay_factor"`
}

func (s *ConfidenceService) GetStats(ctx context.Context, memoryID uuid.UUID, tenantID uuid.UUID) (*ConfidenceStats, error) {
	memory, err := s.store.GetByID(ctx, memoryID, tenantID)
	if err != nil {
		return nil, err
	}

	var hoursSinceAccess float64
	var decayFactor float64 = 1.0
	if memory.LastAccessedAt != nil {
		hoursSinceAccess = time.Since(*memory.LastAccessedAt).Hours()
		decayFactor = math.Exp(-s.DecayLambda * hoursSinceAccess)
	}

	return &ConfidenceStats{
		MemoryID:           memory.ID,
		RawConfidence:      memory.Confidence,
		DecayedConfidence:  s.ApplyDecay(memory),
		ReinforcementCount: memory.ReinforcementCount,
		Provenance:         string(memory.Provenance),
		HoursSinceAccess:   hoursSinceAccess,
		DecayFactor:        decayFactor,
	}, nil
}
