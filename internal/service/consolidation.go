package service

import (
	"context"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/Harshitk-cp/engram/internal/domain"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// Consolidation constants
const (
	// Episode processing
	EpisodeBatchSize = 50 // Process episodes in batches

	// Semantic extraction
	SemanticExtractionConfidenceDiscount = 0.8 // Applied to auto-extracted beliefs
	SemanticSimilarityThreshold          = 0.85

	// Schema formation
	SchemaMinEvidenceCount = 5 // Minimum memories to form a schema

	// Pruning
	RedundancyThreshold         = 0.92 // Merge memories above this similarity
	ProcedureMergeThreshold     = 0.9  // Merge procedures above this similarity
	ProceduralDecayRate         = 0.01 // Very slow decay for procedures
	SchemaDecayRate             = 0.005 // Almost no decay for schemas
	MinProcedureSuccessRate     = 0.2   // Archive procedures below this
)

// ConsolidationResult contains the results of a consolidation run.
type ConsolidationResult struct {
	EpisodesProcessed    int `json:"episodes_processed"`
	SemanticExtracted    int `json:"semantic_extracted"`
	SemanticReinforced   int `json:"semantic_reinforced"`
	ProceduresLearned    int `json:"procedures_learned"`
	ProceduresReinforced int `json:"procedures_reinforced"`
	SchemasDetected      int `json:"schemas_detected"`
	SchemasUpdated       int `json:"schemas_updated"`
	MemoriesDecayed      int `json:"memories_decayed"`
	MemoriesArchived     int `json:"memories_archived"`
	MemoriesMerged       int `json:"memories_merged"`
	AssociationsCreated  int `json:"associations_created"`
}

// MemoryHealthStats contains statistics about memory system health.
type MemoryHealthStats struct {
	EpisodicCount        int      `json:"episodic_count"`
	SemanticCount        int      `json:"semantic_count"`
	ProceduralCount      int      `json:"procedural_count"`
	SchemaCount          int      `json:"schema_count"`
	MemoriesAtRisk       int      `json:"memories_at_risk"` // confidence/strength < 0.3
	RecentlyReinforced   int      `json:"recently_reinforced"`
	ContradictionCount   int      `json:"contradiction_count"`
	UncertaintyAreas     []string `json:"uncertainty_areas"`
	AverageConfidence    float32  `json:"average_confidence"`
	OldestUnprocessed    *time.Time `json:"oldest_unprocessed,omitempty"`
}

const (
	defaultConsolidationInterval = 6 * time.Hour // Consolidate every 6 hours
)

// ConsolidationService orchestrates the 5-stage memory consolidation pipeline.
type ConsolidationService struct {
	memoryStore        domain.MemoryStore
	episodeStore       domain.EpisodeStore
	procedureStore     domain.ProcedureStore
	schemaStore        domain.SchemaStore
	assocStore         domain.MemoryAssociationStore
	contradictionStore domain.ContradictionStore
	embeddingClient    domain.EmbeddingClient
	llmClient          domain.LLMClient
	logger             *zap.Logger

	// Background worker fields
	interval time.Duration
	stopCh   chan struct{}
	wg       sync.WaitGroup
}

// NewConsolidationService creates a new consolidation service.
func NewConsolidationService(
	memoryStore domain.MemoryStore,
	episodeStore domain.EpisodeStore,
	procedureStore domain.ProcedureStore,
	schemaStore domain.SchemaStore,
	assocStore domain.MemoryAssociationStore,
	contradictionStore domain.ContradictionStore,
	embeddingClient domain.EmbeddingClient,
	llmClient domain.LLMClient,
	logger *zap.Logger,
) *ConsolidationService {
	return &ConsolidationService{
		memoryStore:        memoryStore,
		episodeStore:       episodeStore,
		procedureStore:     procedureStore,
		schemaStore:        schemaStore,
		assocStore:         assocStore,
		contradictionStore: contradictionStore,
		embeddingClient:    embeddingClient,
		llmClient:          llmClient,
		logger:             logger,
		interval:           defaultConsolidationInterval,
		stopCh:             make(chan struct{}),
	}
}

// SetInterval sets the consolidation interval.
func (s *ConsolidationService) SetInterval(d time.Duration) {
	s.interval = d
}

// Start begins the background consolidation worker.
func (s *ConsolidationService) Start() {
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		ticker := time.NewTicker(s.interval)
		defer ticker.Stop()

		s.logger.Info("consolidation worker started", zap.Duration("interval", s.interval))

		for {
			select {
			case <-ticker.C:
				ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
				s.runConsolidation(ctx)
				cancel()
			case <-s.stopCh:
				s.logger.Info("consolidation worker stopped")
				return
			}
		}
	}()
}

// Stop halts the background consolidation worker.
func (s *ConsolidationService) Stop() {
	close(s.stopCh)
	s.wg.Wait()
}

// runConsolidation runs consolidation for all agents needing it.
func (s *ConsolidationService) runConsolidation(ctx context.Context) {
	agents, err := s.GetAgentsNeedingConsolidation(ctx)
	if err != nil {
		s.logger.Error("failed to get agents needing consolidation", zap.Error(err))
		return
	}

	for _, agentID := range agents {
		// Get tenant ID for this agent - we need it from the memory store
		tenantID, err := s.getTenantForAgent(ctx, agentID)
		if err != nil {
			s.logger.Warn("failed to get tenant for agent", zap.String("agent_id", agentID.String()), zap.Error(err))
			continue
		}

		result, err := s.Consolidate(ctx, agentID, tenantID, ConsolidationScopeRecent)
		if err != nil {
			s.logger.Error("consolidation failed",
				zap.String("agent_id", agentID.String()),
				zap.Error(err))
			continue
		}

		if result.EpisodesProcessed > 0 || result.SemanticExtracted > 0 || result.ProceduresLearned > 0 || result.MemoriesArchived > 0 {
			s.logger.Info("consolidation complete",
				zap.String("agent_id", agentID.String()),
				zap.Int("episodes_processed", result.EpisodesProcessed),
				zap.Int("semantic_extracted", result.SemanticExtracted),
				zap.Int("procedures_learned", result.ProceduresLearned),
				zap.Int("memories_archived", result.MemoriesArchived))
		}
	}
}

// getTenantForAgent retrieves the tenant ID for an agent.
func (s *ConsolidationService) getTenantForAgent(ctx context.Context, agentID uuid.UUID) (uuid.UUID, error) {
	// Get a memory for this agent to extract tenant ID
	memories, err := s.memoryStore.GetByAgentForDecay(ctx, agentID)
	if err != nil {
		return uuid.Nil, err
	}
	if len(memories) == 0 {
		return uuid.Nil, fmt.Errorf("no memories found for agent")
	}
	return memories[0].TenantID, nil
}

// ConsolidationScope defines the scope of consolidation.
type ConsolidationScope string

const (
	ConsolidationScopeRecent ConsolidationScope = "recent" // Only recent unprocessed
	ConsolidationScopeFull   ConsolidationScope = "full"   // Full consolidation
)

// Consolidate runs the full consolidation pipeline for an agent.
func (s *ConsolidationService) Consolidate(ctx context.Context, agentID uuid.UUID, tenantID uuid.UUID, scope ConsolidationScope) (*ConsolidationResult, error) {
	result := &ConsolidationResult{}

	s.logger.Info("starting consolidation",
		zap.String("agent_id", agentID.String()),
		zap.String("scope", string(scope)))

	// Stage 1: Process raw episodes
	stage1Result := s.processEpisodes(ctx, agentID, tenantID, scope)
	result.EpisodesProcessed = stage1Result.processed
	result.AssociationsCreated = stage1Result.associations

	// Stage 2: Extract semantic beliefs from processed episodes
	stage2Result := s.extractSemanticBeliefs(ctx, agentID, tenantID)
	result.SemanticExtracted = stage2Result.extracted
	result.SemanticReinforced = stage2Result.reinforced

	// Stage 3: Learn procedures from successful episodes
	stage3Result := s.learnProcedures(ctx, agentID, tenantID)
	result.ProceduresLearned = stage3Result.learned
	result.ProceduresReinforced = stage3Result.reinforced

	// Stage 4: Form/update schemas from semantic clusters
	stage4Result := s.formSchemas(ctx, agentID, tenantID)
	result.SchemasDetected = stage4Result.detected
	result.SchemasUpdated = stage4Result.updated

	// Stage 5: Apply forgetting and pruning
	stage5Result := s.applyForgetting(ctx, agentID, tenantID, scope == ConsolidationScopeFull)
	result.MemoriesDecayed = stage5Result.decayed
	result.MemoriesArchived = stage5Result.archived
	result.MemoriesMerged = stage5Result.merged

	s.logger.Info("consolidation complete",
		zap.String("agent_id", agentID.String()),
		zap.Int("episodes_processed", result.EpisodesProcessed),
		zap.Int("semantic_extracted", result.SemanticExtracted),
		zap.Int("procedures_learned", result.ProceduresLearned),
		zap.Int("schemas_detected", result.SchemasDetected),
		zap.Int("memories_archived", result.MemoriesArchived))

	return result, nil
}

// Stage 1: Episode Processing
type stage1Result struct {
	processed    int
	associations int
}

func (s *ConsolidationService) processEpisodes(ctx context.Context, agentID uuid.UUID, tenantID uuid.UUID, _ ConsolidationScope) stage1Result {
	result := stage1Result{}

	// Get unprocessed episodes
	episodes, err := s.episodeStore.GetUnconsolidated(ctx, agentID, EpisodeBatchSize)
	if err != nil {
		s.logger.Warn("failed to get unprocessed episodes", zap.Error(err))
		return result
	}

	if len(episodes) == 0 {
		return result
	}

	for _, ep := range episodes {
		// Skip if already processed (raw → processed)
		if ep.ConsolidationStatus != domain.ConsolidationRaw {
			continue
		}

		// Only run expensive LLM extraction for important or outcome-bearing episodes
		worthExtracting := ep.ImportanceScore >= 0.6 || ep.Outcome == domain.OutcomeSuccess || ep.Outcome == domain.OutcomeFailure

		if (len(ep.Entities) == 0 || len(ep.Topics) == 0) && s.llmClient != nil && worthExtracting {
			extraction, err := s.llmClient.ExtractEpisodeStructure(ctx, ep.RawContent)
			if err == nil && extraction != nil {
				ep.Entities = extraction.Entities
				ep.Topics = extraction.Topics
				ep.CausalLinks = extraction.CausalLinks
				if extraction.EmotionalValence != nil {
					ep.EmotionalValence = extraction.EmotionalValence
				}
				if extraction.EmotionalIntensity != nil {
					ep.EmotionalIntensity = extraction.EmotionalIntensity
				}
				if extraction.ImportanceScore > 0 {
					ep.ImportanceScore = extraction.ImportanceScore
				}
			}
		}

		// Create cross-memory associations based on entities/topics
		assocs := s.createEpisodeAssociations(ctx, &ep, tenantID)
		result.associations += assocs

		// Mark as processed
		if err := s.episodeStore.UpdateConsolidationStatus(ctx, ep.ID, domain.ConsolidationProcessed); err != nil {
			s.logger.Warn("failed to update episode status", zap.Error(err))
			continue
		}

		result.processed++
	}

	return result
}

// createEpisodeAssociations creates cross-memory associations for an episode.
func (s *ConsolidationService) createEpisodeAssociations(ctx context.Context, ep *domain.Episode, tenantID uuid.UUID) int {
	if s.assocStore == nil || len(ep.Embedding) == 0 {
		return 0
	}

	count := 0

	// Find similar semantic memories and create associations
	if s.memoryStore != nil {
		similar, err := s.memoryStore.FindSimilar(ctx, ep.AgentID, tenantID, ep.Embedding, 0.7)
		if err == nil {
			for _, mem := range similar {
				assoc := &domain.MemoryAssociation{
					SourceMemoryType:    domain.ActivatedMemoryTypeEpisodic,
					SourceMemoryID:      ep.ID,
					TargetMemoryType:    domain.ActivatedMemoryTypeSemantic,
					TargetMemoryID:      mem.ID,
					AssociationType:     domain.AssociationTypeThematic,
					AssociationStrength: mem.Score,
				}
				if err := s.assocStore.Create(ctx, assoc); err == nil {
					count++
				}
			}
		}
	}

	return count
}

// Stage 2: Semantic Extraction
type stage2Result struct {
	extracted  int
	reinforced int
}

func (s *ConsolidationService) extractSemanticBeliefs(ctx context.Context, agentID uuid.UUID, tenantID uuid.UUID) stage2Result {
	result := stage2Result{}

	if s.llmClient == nil || s.memoryStore == nil {
		return result
	}

	// Get recently processed episodes that haven't had beliefs extracted
	episodes, err := s.episodeStore.GetUnconsolidated(ctx, agentID, EpisodeBatchSize)
	if err != nil {
		return result
	}

	// Also check for episodes in "processed" state that need semantic extraction
	processedEps, err := s.getProcessedEpisodes(ctx, agentID, tenantID, EpisodeBatchSize)
	if err == nil {
		episodes = append(episodes, processedEps...)
	}

	for _, ep := range episodes {
		if ep.ConsolidationStatus != domain.ConsolidationProcessed {
			continue
		}

		if len(ep.DerivedSemanticIDs) > 0 {
			continue
		}

		// Skip low-importance episodes with neutral outcomes
		if ep.ImportanceScore < 0.6 && ep.Outcome != domain.OutcomeSuccess && ep.Outcome != domain.OutcomeFailure {
			_ = s.episodeStore.UpdateConsolidationStatus(ctx, ep.ID, domain.ConsolidationAbstracted)
			continue
		}

		// Extract beliefs using LLM
		extracted, err := s.llmClient.Extract(ctx, []domain.Message{
			{Role: "user", Content: ep.RawContent},
		})
		if err != nil {
			s.logger.Debug("failed to extract beliefs", zap.Error(err))
			continue
		}

		for _, belief := range extracted {
			// Generate embedding
			var embedding []float32
			if s.embeddingClient != nil {
				embedding, _ = s.embeddingClient.Embed(ctx, belief.Content)
			}

			// Check for similar existing beliefs
			if len(embedding) > 0 {
				similar, err := s.memoryStore.FindSimilar(ctx, agentID, tenantID, embedding, SemanticSimilarityThreshold)
				if err == nil && len(similar) > 0 {
					// Reinforce existing belief
					existingMem := similar[0]
					newConfidence := existingMem.Confidence + 0.05
					if newConfidence > 0.99 {
						newConfidence = 0.99
					}
					_ = s.memoryStore.UpdateReinforcement(ctx, existingMem.ID, newConfidence, existingMem.ReinforcementCount+1)
					result.reinforced++

					// Link episode to existing memory
					_ = s.episodeStore.LinkDerivedMemory(ctx, ep.ID, existingMem.ID, "semantic")
					continue
				}
			}

			// Create new belief with confidence from EvidenceType if available
			confidence := belief.Confidence * SemanticExtractionConfidenceDiscount
			if belief.EvidenceType != "" {
				confidence = belief.EvidenceType.InitialConfidence() * SemanticExtractionConfidenceDiscount
			}

			mem := &domain.Memory{
				AgentID:    agentID,
				TenantID:   tenantID,
				Content:    belief.Content,
				Type:       belief.Type,
				Confidence: confidence,
				Source:     fmt.Sprintf("episode:%s", ep.ID),
				Embedding:  embedding,
			}

			if err := s.memoryStore.Create(ctx, mem); err != nil {
				s.logger.Debug("failed to create belief", zap.Error(err))
				continue
			}

			// Link to episode
			_ = s.episodeStore.LinkDerivedMemory(ctx, ep.ID, mem.ID, "semantic")

			// Create association
			if s.assocStore != nil {
				assoc := &domain.MemoryAssociation{
					SourceMemoryType:    domain.ActivatedMemoryTypeEpisodic,
					SourceMemoryID:      ep.ID,
					TargetMemoryType:    domain.ActivatedMemoryTypeSemantic,
					TargetMemoryID:      mem.ID,
					AssociationType:     domain.AssociationTypeDerived,
					AssociationStrength: 0.9,
				}
				_ = s.assocStore.Create(ctx, assoc)
			}

			result.extracted++
		}

		// Mark episode as abstracted
		_ = s.episodeStore.UpdateConsolidationStatus(ctx, ep.ID, domain.ConsolidationAbstracted)
	}

	return result
}

// getProcessedEpisodes gets episodes in "processed" state ready for semantic extraction.
func (s *ConsolidationService) getProcessedEpisodes(ctx context.Context, agentID uuid.UUID, tenantID uuid.UUID, limit int) ([]domain.Episode, error) {
	return s.episodeStore.GetByConsolidationStatus(ctx, agentID, tenantID, domain.ConsolidationProcessed, limit)
}

// Stage 3: Procedural Learning
type stage3Result struct {
	learned    int
	reinforced int
}

func (s *ConsolidationService) learnProcedures(ctx context.Context, agentID uuid.UUID, tenantID uuid.UUID) stage3Result {
	result := stage3Result{}

	if s.llmClient == nil || s.procedureStore == nil || s.episodeStore == nil {
		return result
	}

	// Get episodes with successful outcomes that haven't been processed for procedures
	// For now, get recent episodes and check outcomes
	episodes, err := s.episodeStore.GetByTimeRange(ctx, agentID, tenantID,
		time.Now().Add(-24*time.Hour*7), time.Now()) // Last 7 days
	if err != nil {
		return result
	}

	for _, ep := range episodes {
		if ep.Outcome != domain.OutcomeSuccess {
			continue
		}

		// Require minimum importance for procedure extraction
		if ep.ImportanceScore < 0.6 {
			continue
		}

		if len(ep.DerivedProceduralIDs) > 0 {
			continue
		}

		// Extract procedure pattern
		pattern, err := s.llmClient.ExtractProcedure(ctx, ep.RawContent)
		if err != nil || pattern == nil || pattern.TriggerPattern == "" {
			continue
		}

		// Generate embedding
		var embedding []float32
		if s.embeddingClient != nil {
			embedding, _ = s.embeddingClient.Embed(ctx, pattern.TriggerPattern)
		}

		// Check for similar existing procedure
		if len(embedding) > 0 {
			similar, err := s.procedureStore.FindByTriggerSimilarity(ctx, agentID, tenantID, embedding, ProcedureMergeThreshold, 1)
			if err == nil && len(similar) > 0 {
				// Reinforce existing procedure
				_ = s.procedureStore.Reinforce(ctx, similar[0].ID, ep.ID, 0.05)
				_ = s.episodeStore.LinkDerivedMemory(ctx, ep.ID, similar[0].ID, "procedural")
				result.reinforced++
				continue
			}
		}

		// Create new procedure
		proc := &domain.Procedure{
			AgentID:             agentID,
			TenantID:            tenantID,
			TriggerPattern:      pattern.TriggerPattern,
			TriggerKeywords:     pattern.TriggerKeywords,
			TriggerEmbedding:    embedding,
			ActionTemplate:      pattern.ActionTemplate,
			ActionType:          pattern.ActionType,
			DerivedFromEpisodes: []uuid.UUID{ep.ID},
			Confidence:          0.5,
			MemoryStrength:      1.0,
		}

		now := time.Now()
		proc.LastVerifiedAt = &now

		if err := s.procedureStore.Create(ctx, proc); err != nil {
			s.logger.Debug("failed to create procedure", zap.Error(err))
			continue
		}

		_ = s.episodeStore.LinkDerivedMemory(ctx, ep.ID, proc.ID, "procedural")

		// Create association
		if s.assocStore != nil {
			assoc := &domain.MemoryAssociation{
				SourceMemoryType:    domain.ActivatedMemoryTypeEpisodic,
				SourceMemoryID:      ep.ID,
				TargetMemoryType:    domain.ActivatedMemoryTypeProcedural,
				TargetMemoryID:      proc.ID,
				AssociationType:     domain.AssociationTypeDerived,
				AssociationStrength: 0.9,
			}
			_ = s.assocStore.Create(ctx, assoc)
		}

		result.learned++
	}

	return result
}

// Stage 4: Schema Formation
type stage4Result struct {
	detected int
	updated  int
}

func (s *ConsolidationService) formSchemas(ctx context.Context, agentID uuid.UUID, tenantID uuid.UUID) stage4Result {
	result := stage4Result{}

	if s.llmClient == nil || s.schemaStore == nil || s.memoryStore == nil {
		return result
	}

	// Get all semantic memories for clustering
	allMemories, err := s.memoryStore.GetByAgentForDecay(ctx, agentID)
	if err != nil || len(allMemories) < SchemaMinEvidenceCount {
		return result
	}

	// Filter to memories with embeddings, sufficient confidence, and stability
	now := time.Now()
	var memoriesWithEmbeddings []domain.Memory
	for _, m := range allMemories {
		if len(m.Embedding) == 0 {
			continue
		}
		if m.Confidence < 0.6 {
			continue
		}
		if now.Sub(m.CreatedAt) < 24*time.Hour {
			continue
		}
		memoriesWithEmbeddings = append(memoriesWithEmbeddings, m)
	}

	if len(memoriesWithEmbeddings) < SchemaMinEvidenceCount {
		return result
	}

	// Cluster memories by similarity
	clusters := s.clusterMemories(memoriesWithEmbeddings)

	for _, cluster := range clusters {
		if len(cluster.Memories) < SchemaMinEvidenceCount {
			continue
		}

		// Use LLM to detect schema pattern
		extraction, err := s.llmClient.DetectSchemaPattern(ctx, cluster.Memories)
		if err != nil || extraction == nil {
			continue
		}

		// Check if schema already exists
		existing, err := s.schemaStore.GetByName(ctx, agentID, tenantID, extraction.SchemaType, extraction.Name)
		if err == nil && existing != nil {
			// Update existing schema with new evidence
			newCount := 0
			existingIDs := make(map[uuid.UUID]bool)
			for _, id := range existing.EvidenceMemories {
				existingIDs[id] = true
			}
			for _, id := range cluster.MemoryIDs {
				if !existingIDs[id] {
					_ = s.schemaStore.AddEvidence(ctx, existing.ID, &id, nil)
					newCount++
				}
			}
			if newCount > 0 {
				// Boost confidence
				newConfidence := existing.Confidence + float32(newCount)*0.02
				if newConfidence > 0.95 {
					newConfidence = 0.95
				}
				_ = s.schemaStore.UpdateConfidence(ctx, existing.ID, newConfidence)
				result.updated++
			}
			continue
		}

		// Create new schema
		schema := &domain.Schema{
			AgentID:            agentID,
			TenantID:           tenantID,
			SchemaType:         extraction.SchemaType,
			Name:               extraction.Name,
			Description:        extraction.Description,
			Attributes:         extraction.Attributes,
			ApplicableContexts: extraction.ApplicableContexts,
			EvidenceMemories:   cluster.MemoryIDs,
			EvidenceCount:      len(cluster.Memories),
			Confidence:         0.5 + float32(len(cluster.Memories))*0.05,
		}

		if schema.Confidence > 0.8 {
			schema.Confidence = 0.8
		}

		now := time.Now()
		schema.LastValidatedAt = &now

		// Generate embedding for schema
		if s.embeddingClient != nil {
			schemaText := schema.Name + ": " + schema.Description
			embedding, _ := s.embeddingClient.Embed(ctx, schemaText)
			schema.Embedding = embedding
		}

		if err := s.schemaStore.Create(ctx, schema); err != nil {
			s.logger.Debug("failed to create schema", zap.Error(err))
			continue
		}

		// Create associations from memories to schema
		if s.assocStore != nil {
			for _, memID := range cluster.MemoryIDs {
				assoc := &domain.MemoryAssociation{
					SourceMemoryType:    domain.ActivatedMemoryTypeSemantic,
					SourceMemoryID:      memID,
					TargetMemoryType:    domain.ActivatedMemoryTypeSchema,
					TargetMemoryID:      schema.ID,
					AssociationType:     domain.AssociationTypeDerived,
					AssociationStrength: 0.8,
				}
				_ = s.assocStore.Create(ctx, assoc)
			}
		}

		result.detected++
	}

	return result
}

// clusterMemories groups memories by embedding similarity.
func (s *ConsolidationService) clusterMemories(memories []domain.Memory) []domain.MemoryCluster {
	const clusterThreshold = 0.65

	assigned := make(map[uuid.UUID]bool)
	var clusters []domain.MemoryCluster

	for i, seed := range memories {
		if assigned[seed.ID] {
			continue
		}

		cluster := domain.MemoryCluster{
			Memories:  []domain.Memory{seed},
			MemoryIDs: []uuid.UUID{seed.ID},
			Centroid:  seed.Embedding,
		}
		assigned[seed.ID] = true

		for j := i + 1; j < len(memories); j++ {
			candidate := memories[j]
			if assigned[candidate.ID] {
				continue
			}

			similarity := cosineSimilarity(cluster.Centroid, candidate.Embedding)
			if similarity >= clusterThreshold {
				cluster.Memories = append(cluster.Memories, candidate)
				cluster.MemoryIDs = append(cluster.MemoryIDs, candidate.ID)
				assigned[candidate.ID] = true

				// Update centroid (simple average)
				cluster.Centroid = averageVectors(cluster.Centroid, candidate.Embedding)
			}
		}

		clusters = append(clusters, cluster)
	}

	return clusters
}

// Stage 5: Forgetting and Pruning
type stage5Result struct {
	decayed  int
	archived int
	merged   int
}

func (s *ConsolidationService) applyForgetting(ctx context.Context, agentID uuid.UUID, tenantID uuid.UUID, fullPrune bool) stage5Result {
	result := stage5Result{}
	now := time.Now()

	// Apply decay to semantic memories
	if s.memoryStore != nil {
		memories, err := s.memoryStore.GetByAgentForDecay(ctx, agentID)
		if err == nil {
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

				// Reinforcement slows decay
				if mem.ReinforcementCount > 1 {
					decayFactor = math.Pow(decayFactor, 1.0/math.Log(float64(mem.ReinforcementCount+1)))
				}

				newConfidence := mem.Confidence * float32(decayFactor)
				if newConfidence < DecayMinConfidence {
					newConfidence = DecayMinConfidence
				}

				if newConfidence < ArchiveThreshold {
					if err := s.memoryStore.Archive(ctx, mem.ID); err == nil {
						result.archived++
					}
				} else if math.Abs(float64(newConfidence-mem.Confidence)) > 0.001 {
					if err := s.memoryStore.UpdateConfidence(ctx, mem.ID, newConfidence); err == nil {
						result.decayed++
					}
				}
			}

			// Merge redundant memories if full prune
			if fullPrune {
				merged := s.mergeRedundantMemories(ctx, agentID, tenantID, memories)
				result.merged = merged
			}
		}
	}

	// Apply decay to procedures
	if s.procedureStore != nil {
		procedures, err := s.procedureStore.GetByAgentForDecay(ctx, agentID)
		if err == nil {
			for _, proc := range procedures {
				// Archive low success rate procedures
				if proc.UseCount > 5 && proc.SuccessRate < MinProcedureSuccessRate {
					if err := s.procedureStore.Archive(ctx, proc.ID); err == nil {
						result.archived++
					}
					continue
				}

				// Very slow decay for procedures
				if proc.LastUsedAt != nil {
					daysSinceUse := now.Sub(*proc.LastUsedAt).Hours() / 24
					decayFactor := math.Exp(-ProceduralDecayRate * daysSinceUse)
					newConfidence := proc.Confidence * float32(decayFactor)

					if newConfidence < DecayMinConfidence {
						newConfidence = DecayMinConfidence
					}

					if math.Abs(float64(newConfidence-proc.Confidence)) > 0.001 {
						if err := s.procedureStore.UpdateConfidence(ctx, proc.ID, newConfidence); err == nil {
							result.decayed++
						}
					}
				}
			}
		}
	}

	// Episodes are handled by the DecayService

	return result
}

// mergeRedundantMemories finds and merges highly similar memories.
func (s *ConsolidationService) mergeRedundantMemories(ctx context.Context, _, _ uuid.UUID, memories []domain.Memory) int {
	merged := 0
	toArchive := make(map[uuid.UUID]bool)

	for i := 0; i < len(memories); i++ {
		if toArchive[memories[i].ID] {
			continue
		}
		if len(memories[i].Embedding) == 0 {
			continue
		}

		for j := i + 1; j < len(memories); j++ {
			if toArchive[memories[j].ID] {
				continue
			}
			if len(memories[j].Embedding) == 0 {
				continue
			}

			similarity := cosineSimilarity(memories[i].Embedding, memories[j].Embedding)
			if similarity >= RedundancyThreshold {
				// Merge: keep the one with higher confidence/reinforcement
				keepIdx, archiveIdx := i, j
				if memories[j].Confidence > memories[i].Confidence ||
					memories[j].ReinforcementCount > memories[i].ReinforcementCount {
					keepIdx, archiveIdx = j, i
				}

				// Reinforce the kept memory
				newConfidence := memories[keepIdx].Confidence + 0.02
				if newConfidence > 0.99 {
					newConfidence = 0.99
				}
				_ = s.memoryStore.UpdateReinforcement(ctx, memories[keepIdx].ID, newConfidence,
					memories[keepIdx].ReinforcementCount+1)

				// Archive the redundant one
				_ = s.memoryStore.Archive(ctx, memories[archiveIdx].ID)
				toArchive[memories[archiveIdx].ID] = true
				merged++
			}
		}
	}

	return merged
}

// GetMemoryHealth returns statistics about the memory system health for an agent.
func (s *ConsolidationService) GetMemoryHealth(ctx context.Context, agentID uuid.UUID, tenantID uuid.UUID) (*MemoryHealthStats, error) {
	stats := &MemoryHealthStats{
		UncertaintyAreas: []string{},
	}

	// Count memories by type
	if s.memoryStore != nil {
		memories, err := s.memoryStore.GetByAgentForDecay(ctx, agentID)
		if err == nil {
			stats.SemanticCount = len(memories)

			var totalConfidence float32
			recentThreshold := time.Now().Add(-24 * time.Hour)

			for _, m := range memories {
				totalConfidence += m.Confidence

				if m.Confidence < 0.3 {
					stats.MemoriesAtRisk++
				}

				if m.LastAccessedAt != nil && m.LastAccessedAt.After(recentThreshold) {
					stats.RecentlyReinforced++
				}

				// Track low confidence areas
				if m.Confidence < 0.5 {
					// Group by type for uncertainty areas
					typeStr := string(m.Type)
					found := false
					for _, area := range stats.UncertaintyAreas {
						if area == typeStr {
							found = true
							break
						}
					}
					if !found {
						stats.UncertaintyAreas = append(stats.UncertaintyAreas, typeStr)
					}
				}
			}

			if len(memories) > 0 {
				stats.AverageConfidence = totalConfidence / float32(len(memories))
			}
		}
	}

	// Count episodes
	if s.episodeStore != nil {
		episodes, err := s.episodeStore.GetUnconsolidated(ctx, agentID, 1000)
		if err == nil {
			stats.EpisodicCount = len(episodes)
			if len(episodes) > 0 {
				// Find oldest unprocessed
				oldest := episodes[0].CreatedAt
				for _, ep := range episodes {
					if ep.CreatedAt.Before(oldest) {
						oldest = ep.CreatedAt
					}
				}
				stats.OldestUnprocessed = &oldest
			}
		}
	}

	// Count procedures
	if s.procedureStore != nil {
		procedures, err := s.procedureStore.GetByAgent(ctx, agentID, tenantID)
		if err == nil {
			stats.ProceduralCount = len(procedures)
		}
	}

	// Count schemas
	if s.schemaStore != nil {
		schemas, err := s.schemaStore.GetByAgent(ctx, agentID, tenantID)
		if err == nil {
			stats.SchemaCount = len(schemas)
		}
	}

	// Count contradictions
	if s.contradictionStore != nil {
		// Would need a count method in the store
		stats.ContradictionCount = 0
	}

	return stats, nil
}

// GetAgentsNeedingConsolidation returns agent IDs that have unprocessed episodes.
func (s *ConsolidationService) GetAgentsNeedingConsolidation(ctx context.Context) ([]uuid.UUID, error) {
	// This would need a new store method. For now, use memory store's list.
	return s.memoryStore.ListDistinctAgentIDs(ctx)
}
