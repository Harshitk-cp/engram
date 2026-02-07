package service

import (
	"context"
	"math"
	"sync"
	"time"

	"github.com/Harshitk-cp/engram/internal/domain"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// Decay constants
const (
	// BaseDecayRate is λ_base for the decay formula (per hour)
	BaseDecayRate = 0.001

	// ConfidenceFloor is the minimum confidence a memory can decay to
	ConfidenceFloor = 0.1

	// CompetitorSimilarityThreshold - only memories with similarity above this compete
	CompetitorSimilarityThreshold = 0.7

	// CompetitionWeight scales how much competition affects decay
	CompetitionWeight = 0.5

	// MaxCompetitors limits how many competitors to consider
	MaxCompetitors = 10

	// MinHoursForDecay - don't decay memories accessed within this window
	MinHoursForDecay = 1.0

	// ArchiveThreshold - archive memories below this confidence
	ArchiveThreshold = 0.15
)

// DecayResult contains detailed information about a memory's decay
type DecayResult struct {
	MemoryID          uuid.UUID `json:"memory_id"`
	OldConfidence     float32   `json:"old_confidence"`
	NewConfidence     float32   `json:"new_confidence"`
	CompetitorCount   int       `json:"competitor_count"`
	CompetitionFactor float64   `json:"competition_factor"`
	EffectiveDecay    float64   `json:"effective_decay_rate"`
	HoursSinceAccess  float64   `json:"hours_since_access"`
	WasArchived       bool      `json:"was_archived"`
}

// BatchDecayResult contains results from a batch decay operation
type BatchDecayResult struct {
	Processed        int                     `json:"processed"`
	Decayed          int                     `json:"decayed"`
	Archived         int                     `json:"archived"`
	Errors           int                     `json:"errors"`
	EpisodesDecayed  int                     `json:"episodes_decayed"`
	EpisodesArchived int                     `json:"episodes_archived"`
	TierTransitions  []domain.TierTransition `json:"tier_transitions,omitempty"`
	Details          []DecayResult  `json:"details,omitempty"`
}

// DecayService implements competition-aware memory decay
type DecayService struct {
	memoryStore  domain.MemoryStore
	episodeStore domain.EpisodeStore
	logger       *zap.Logger

	// Configurable parameters
	BaseDecayRate     float64
	Floor             float64
	SimilarityRadius  float64
	CompetitionWeight float64

	// Background worker
	interval time.Duration
	stopCh   chan struct{}
	wg       sync.WaitGroup
}

// NewDecayService creates a new decay service
func NewDecayService(ms domain.MemoryStore, es domain.EpisodeStore, logger *zap.Logger) *DecayService {
	return &DecayService{
		memoryStore:       ms,
		episodeStore:      es,
		logger:            logger,
		BaseDecayRate:     BaseDecayRate,
		Floor:             ConfidenceFloor,
		SimilarityRadius:  CompetitorSimilarityThreshold,
		CompetitionWeight: CompetitionWeight,
		interval:          time.Hour,
		stopCh:            make(chan struct{}),
	}
}

// SetInterval sets the decay worker interval
func (s *DecayService) SetInterval(d time.Duration) {
	s.interval = d
}

// Start begins the background decay worker
func (s *DecayService) Start() {
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		ticker := time.NewTicker(s.interval)
		defer ticker.Stop()

		s.logger.Info("decay worker started", zap.Duration("interval", s.interval))

		for {
			select {
			case <-ticker.C:
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
				s.runDecayAllAgents(ctx)
				cancel()
			case <-s.stopCh:
				s.logger.Info("decay worker stopped")
				return
			}
		}
	}()
}

// Stop halts the background decay worker
func (s *DecayService) Stop() {
	close(s.stopCh)
	s.wg.Wait()
}

// runDecayAllAgents runs decay for all agents
func (s *DecayService) runDecayAllAgents(ctx context.Context) {
	agentIDs, err := s.memoryStore.ListDistinctAgentIDs(ctx)
	if err != nil {
		s.logger.Error("failed to list agents for decay", zap.Error(err))
		return
	}

	for _, agentID := range agentIDs {
		result, err := s.BatchDecay(ctx, agentID)
		if err != nil {
			s.logger.Error("decay failed for agent",
				zap.String("agent_id", agentID.String()),
				zap.Error(err))
			continue
		}

		if result.Decayed > 0 || result.Archived > 0 {
			s.logger.Info("decay complete",
				zap.String("agent_id", agentID.String()),
				zap.Int("processed", result.Processed),
				zap.Int("decayed", result.Decayed),
				zap.Int("archived", result.Archived),
				zap.Int("tier_transitions", len(result.TierTransitions)))
		}
	}
}

// ApplyDecay applies decay to a single memory
func (s *DecayService) ApplyDecay(ctx context.Context, memory *domain.Memory, allMemories []domain.Memory) *DecayResult {
	result := &DecayResult{
		MemoryID:      memory.ID,
		OldConfidence: memory.Confidence,
	}

	// Calculate time since last access
	var hoursSinceAccess float64
	if memory.LastAccessedAt != nil {
		hoursSinceAccess = time.Since(*memory.LastAccessedAt).Hours()
	} else {
		hoursSinceAccess = time.Since(memory.CreatedAt).Hours()
	}
	result.HoursSinceAccess = hoursSinceAccess

	// No decay for recently accessed memories
	if hoursSinceAccess < MinHoursForDecay {
		result.NewConfidence = memory.Confidence
		return result
	}

	competitors := s.findCompetitors(memory, allMemories)
	result.CompetitorCount = len(competitors)

	competitionFactor := s.calculateCompetition(memory, competitors)
	result.CompetitionFactor = competitionFactor

	effectiveDecay := s.BaseDecayRate * (1 + competitionFactor)
	result.EffectiveDecay = effectiveDecay

	// Distance-to-floor decay: conf_new = floor + (conf - floor) × exp(-λ_eff × t)
	distanceToFloor := float64(memory.Confidence) - s.Floor
	decayFactor := math.Exp(-effectiveDecay * hoursSinceAccess)
	newConfidence := s.Floor + distanceToFloor*decayFactor

	// Apply reinforcement bonus (well-reinforced memories resist decay)
	if memory.ReinforcementCount > 0 {

		reinforcementBonus := 1.0 + 0.15*math.Log(float64(memory.ReinforcementCount+1))

		resistanceFactor := 1.0 - 1.0/reinforcementBonus
		newConfidence = newConfidence + (float64(memory.Confidence)-newConfidence)*resistanceFactor
	}

	if newConfidence < s.Floor {
		newConfidence = s.Floor
	}
	if newConfidence > float64(memory.Confidence) {
		newConfidence = float64(memory.Confidence)
	}

	result.NewConfidence = float32(newConfidence)

	if result.NewConfidence < ArchiveThreshold {
		result.WasArchived = true
	}

	return result
}

// findCompetitors finds memories that compete for the same "slot"
func (s *DecayService) findCompetitors(memory *domain.Memory, allMemories []domain.Memory) []domain.Memory {
	if len(memory.Embedding) == 0 {
		return nil
	}

	var competitors []domain.Memory

	for i := range allMemories {
		candidate := &allMemories[i]

		if candidate.ID == memory.ID {
			continue
		}

		if len(candidate.Embedding) == 0 {
			continue
		}

		if candidate.Type != memory.Type {
			continue
		}

		similarity := cosineSimilarity(memory.Embedding, candidate.Embedding)
		if similarity >= float32(s.SimilarityRadius) {
			competitors = append(competitors, *candidate)
		}

		if len(competitors) >= MaxCompetitors {
			break
		}
	}

	return competitors
}

// calculateCompetition measures how much competing beliefs suppress this one
func (s *DecayService) calculateCompetition(memory *domain.Memory, competitors []domain.Memory) float64 {
	if len(competitors) == 0 {
		return 0
	}

	var totalCompetition float64

	for i := range competitors {
		comp := &competitors[i]

		if comp.Confidence <= memory.Confidence {
			continue
		}

		confidenceDelta := float64(comp.Confidence - memory.Confidence)
		similarity := float64(cosineSimilarity(memory.Embedding, comp.Embedding))

		totalCompetition += confidenceDelta * similarity
	}

	normalizedCompetition := totalCompetition / (1 + float64(memory.Confidence))

	return s.CompetitionWeight * normalizedCompetition
}

// BatchDecay applies decay to all memories for an agent
func (s *DecayService) BatchDecay(ctx context.Context, agentID uuid.UUID) (*BatchDecayResult, error) {
	result := &BatchDecayResult{
		TierTransitions: []domain.TierTransition{},
	}

	// Get all memories for this agent
	memories, err := s.memoryStore.GetByAgentForDecay(ctx, agentID)
	if err != nil {
		return nil, err
	}

	if len(memories) == 0 {
		return result, nil
	}

	result.Processed = len(memories)

	for i := range memories {
		mem := &memories[i]

		if mem.LastAccessedAt == nil && time.Since(mem.CreatedAt).Hours() < MinHoursForDecay {
			continue
		}

		decayResult := s.ApplyDecay(ctx, mem, memories)

		confidenceDelta := math.Abs(float64(decayResult.NewConfidence - decayResult.OldConfidence))
		if confidenceDelta < 0.001 {
			continue
		}

		oldTier := domain.ComputeTier(float64(decayResult.OldConfidence))
		newTier := domain.ComputeTier(float64(decayResult.NewConfidence))
		if oldTier != newTier {
			result.TierTransitions = append(result.TierTransitions, domain.TierTransition{
				MemoryID:   mem.ID,
				FromTier:   oldTier,
				ToTier:     newTier,
				Reason:     "decay",
				OccurredAt: time.Now(),
			})
		}

		if decayResult.WasArchived {
			if err := s.memoryStore.Archive(ctx, mem.ID); err != nil {
				s.logger.Debug("failed to archive memory",
					zap.String("memory_id", mem.ID.String()),
					zap.Error(err))
				result.Errors++
				continue
			}
			result.Archived++
		} else {
			if err := s.memoryStore.UpdateConfidence(ctx, mem.ID, decayResult.NewConfidence); err != nil {
				s.logger.Debug("failed to update memory confidence",
					zap.String("memory_id", mem.ID.String()),
					zap.Error(err))
				result.Errors++
				continue
			}
			result.Decayed++
		}
	}

	if s.episodeStore != nil {
		decayed, err := s.episodeStore.ApplyDecay(ctx, agentID)
		if err != nil {
			s.logger.Debug("failed to apply episode decay", zap.Error(err))
		} else {
			result.EpisodesDecayed = int(decayed)
		}

		weakEpisodes, err := s.episodeStore.GetWeakMemories(ctx, agentID, ArchiveThreshold)
		if err != nil {
			s.logger.Debug("failed to get weak episodes", zap.Error(err))
		} else {
			for _, ep := range weakEpisodes {
				if err := s.episodeStore.Archive(ctx, ep.ID); err != nil {
					s.logger.Debug("failed to archive episode", zap.Error(err))
				} else {
					result.EpisodesArchived++
				}
			}
		}
	}

	return result, nil
}

// BatchDecayWithDetails returns detailed results for each memory
func (s *DecayService) BatchDecayWithDetails(ctx context.Context, agentID uuid.UUID) (*BatchDecayResult, error) {
	result := &BatchDecayResult{
		TierTransitions: []domain.TierTransition{},
		Details:         []DecayResult{},
	}

	memories, err := s.memoryStore.GetByAgentForDecay(ctx, agentID)
	if err != nil {
		return nil, err
	}

	if len(memories) == 0 {
		return result, nil
	}

	result.Processed = len(memories)

	for i := range memories {
		mem := &memories[i]

		if mem.LastAccessedAt == nil && time.Since(mem.CreatedAt).Hours() < MinHoursForDecay {
			continue
		}

		decayResult := s.ApplyDecay(ctx, mem, memories)
		result.Details = append(result.Details, *decayResult)

		confidenceDelta := math.Abs(float64(decayResult.NewConfidence - decayResult.OldConfidence))
		if confidenceDelta < 0.001 {
			continue
		}

		oldTier := domain.ComputeTier(float64(decayResult.OldConfidence))
		newTier := domain.ComputeTier(float64(decayResult.NewConfidence))
		if oldTier != newTier {
			result.TierTransitions = append(result.TierTransitions, domain.TierTransition{
				MemoryID:   mem.ID,
				FromTier:   oldTier,
				ToTier:     newTier,
				Reason:     "decay",
				OccurredAt: time.Now(),
			})
		}

		if decayResult.WasArchived {
			if err := s.memoryStore.Archive(ctx, mem.ID); err != nil {
				result.Errors++
				continue
			}
			result.Archived++
		} else {
			if err := s.memoryStore.UpdateConfidence(ctx, mem.ID, decayResult.NewConfidence); err != nil {
				result.Errors++
				continue
			}
			result.Decayed++
		}
	}

	return result, nil
}

// RunDecayForAgent runs decay for a specific agent
func (s *DecayService) RunDecayForAgent(ctx context.Context, agentID uuid.UUID) (*BatchDecayResult, error) {
	return s.BatchDecay(ctx, agentID)
}
