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

const (
	defaultDecayInterval = 1 * time.Hour

	SemanticDecayRate        = 0.02
	EpisodicDecayRate        = 0.10
	ImportanceDecayReduction = 0.5
	ArchiveThreshold         = 0.2
	DecayMinConfidence       = 0.1
)

type DecayResult struct {
	MemoriesDecayed  int `json:"memories_decayed"`
	MemoriesArchived int `json:"memories_archived"`
	EpisodesDecayed  int `json:"episodes_decayed"`
	EpisodesArchived int `json:"episodes_archived"`
}

type DecayService struct {
	memoryStore  domain.MemoryStore
	episodeStore domain.EpisodeStore
	logger       *zap.Logger

	interval time.Duration
	stopCh   chan struct{}
	wg       sync.WaitGroup
}

func NewDecayService(ms domain.MemoryStore, es domain.EpisodeStore, logger *zap.Logger) *DecayService {
	return &DecayService{
		memoryStore:  ms,
		episodeStore: es,
		logger:       logger,
		interval:     defaultDecayInterval,
		stopCh:       make(chan struct{}),
	}
}

func (s *DecayService) SetInterval(d time.Duration) {
	s.interval = d
}

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
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
				s.RunDecay(ctx)
				cancel()
			case <-s.stopCh:
				s.logger.Info("decay worker stopped")
				return
			}
		}
	}()
}

func (s *DecayService) Stop() {
	close(s.stopCh)
	s.wg.Wait()
}

func (s *DecayService) RunDecay(ctx context.Context) *DecayResult {
	totalResult := &DecayResult{}

	agentIDs, err := s.memoryStore.ListDistinctAgentIDs(ctx)
	if err != nil {
		s.logger.Error("failed to list agents for decay", zap.Error(err))
		return totalResult
	}

	for _, agentID := range agentIDs {
		result, err := s.applyDecayForAgent(ctx, agentID)
		if err != nil {
			s.logger.Error("decay failed for agent",
				zap.String("agent_id", agentID.String()),
				zap.Error(err))
			continue
		}

		totalResult.MemoriesDecayed += result.MemoriesDecayed
		totalResult.MemoriesArchived += result.MemoriesArchived
		totalResult.EpisodesDecayed += result.EpisodesDecayed
		totalResult.EpisodesArchived += result.EpisodesArchived

		if result.MemoriesDecayed > 0 || result.MemoriesArchived > 0 || result.EpisodesDecayed > 0 || result.EpisodesArchived > 0 {
			s.logger.Info("decay complete for agent",
				zap.String("agent_id", agentID.String()),
				zap.Int("memories_decayed", result.MemoriesDecayed),
				zap.Int("memories_archived", result.MemoriesArchived),
				zap.Int("episodes_decayed", result.EpisodesDecayed),
				zap.Int("episodes_archived", result.EpisodesArchived))
		}
	}

	return totalResult
}

func (s *DecayService) RunDecayForAgent(ctx context.Context, agentID uuid.UUID) (*DecayResult, error) {
	return s.applyDecayForAgent(ctx, agentID)
}

func (s *DecayService) applyDecayForAgent(ctx context.Context, agentID uuid.UUID) (*DecayResult, error) {
	result := &DecayResult{}
	now := time.Now()

	memories, err := s.memoryStore.GetByAgentForDecay(ctx, agentID)
	if err != nil {
		return nil, err
	}

	for _, mem := range memories {
		if mem.LastAccessedAt == nil {
			continue
		}

		hoursSinceAccess := now.Sub(*mem.LastAccessedAt).Hours()
		days := hoursSinceAccess / 24

		decayRate := float64(mem.DecayRate)
		if decayRate == 0 {
			decayRate = SemanticDecayRate
		}
		decayFactor := math.Exp(-decayRate * days)

		if mem.ReinforcementCount > 1 {
			decayFactor = math.Pow(decayFactor, 1.0/math.Log(float64(mem.ReinforcementCount+1)))
		}

		newConfidence := mem.Confidence * float32(decayFactor)

		if newConfidence < DecayMinConfidence {
			newConfidence = DecayMinConfidence
		}

		if newConfidence < ArchiveThreshold {
			if err := s.memoryStore.Archive(ctx, mem.ID); err != nil {
				s.logger.Warn("failed to archive memory", zap.Error(err))
			} else {
				result.MemoriesArchived++
			}
		} else if math.Abs(float64(newConfidence-mem.Confidence)) > 0.001 {
			if err := s.memoryStore.UpdateConfidence(ctx, mem.ID, newConfidence); err != nil {
				s.logger.Warn("failed to update memory confidence", zap.Error(err))
			} else {
				result.MemoriesDecayed++
			}
		}
	}

	if s.episodeStore != nil {
		decayed, err := s.episodeStore.ApplyDecay(ctx, agentID)
		if err != nil {
			s.logger.Warn("failed to apply episode decay", zap.Error(err))
		} else {
			result.EpisodesDecayed = int(decayed)
		}

		weakEpisodes, err := s.episodeStore.GetWeakMemories(ctx, agentID, ArchiveThreshold)
		if err != nil {
			s.logger.Warn("failed to get weak episodes", zap.Error(err))
		} else {
			for _, ep := range weakEpisodes {
				if err := s.episodeStore.Archive(ctx, ep.ID); err != nil {
					s.logger.Warn("failed to archive episode", zap.Error(err))
				} else {
					result.EpisodesArchived++
				}
			}
		}
	}

	return result, nil
}
