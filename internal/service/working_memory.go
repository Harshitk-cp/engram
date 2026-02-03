package service

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/Harshitk-cp/engram/internal/domain"
	"github.com/Harshitk-cp/engram/internal/store"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// Working memory constants
const (
	DefaultMaxSlots        = 7     // Miller's Law: 7 +/- 2
	SpreadingDecay         = 0.5   // Activation decays 50% per hop
	MaxSpreadingDepth      = 2     // Maximum hops for spreading activation
	DirectActivationBoost  = 1.0   // Full activation for direct matches
	GoalActivationBoost    = 1.2   // 20% boost for goal-relevant memories
	SchemaActivationBoost  = 1.1   // 10% boost for schema-activated memories
	TemporalActivationBase = 0.8   // Base activation for recent memories
	RecencyDecay           = 0.1   // Decay per hour for recency activation
	MinActivationLevel     = 0.1   // Minimum activation to be considered
	// MinSchemaMatchScore is defined in schema.go
)

var (
	ErrSessionNotFound = errors.New("working memory session not found")
)

// WorkingMemoryService manages working memory sessions and memory activation.
type WorkingMemoryService struct {
	wmStore         domain.WorkingMemoryStore
	assocStore      domain.MemoryAssociationStore
	memoryStore     domain.MemoryStore
	episodeStore    domain.EpisodeStore
	procedureStore  domain.ProcedureStore
	schemaStore     domain.SchemaStore
	embeddingClient domain.EmbeddingClient
	logger          *zap.Logger
}

// NewWorkingMemoryService creates a new working memory service.
func NewWorkingMemoryService(
	wmStore domain.WorkingMemoryStore,
	assocStore domain.MemoryAssociationStore,
	memoryStore domain.MemoryStore,
	episodeStore domain.EpisodeStore,
	procedureStore domain.ProcedureStore,
	schemaStore domain.SchemaStore,
	embeddingClient domain.EmbeddingClient,
	logger *zap.Logger,
) *WorkingMemoryService {
	return &WorkingMemoryService{
		wmStore:         wmStore,
		assocStore:      assocStore,
		memoryStore:     memoryStore,
		episodeStore:    episodeStore,
		procedureStore:  procedureStore,
		schemaStore:     schemaStore,
		embeddingClient: embeddingClient,
		logger:          logger,
	}
}

// ActivationResult holds the result of memory activation.
type ActivationResult struct {
	Session          *domain.WorkingMemorySession
	Activations      []activatedItem
	ActiveSchemas    []domain.SchemaMatch
	SlotUsage        int
	MaxSlots         int
	AssembledContext string
}

// activatedItem represents an activated memory with its details.
type activatedItem struct {
	Type            domain.ActivatedMemoryType
	ID              uuid.UUID
	Content         string
	Confidence      float32
	ActivationLevel float32
	Source          domain.ActivationSource
	Cue             string
}

// Activate performs intelligent memory activation using spreading activation.
func (s *WorkingMemoryService) Activate(ctx context.Context, input domain.ActivationInput) (*domain.WorkingMemoryResult, error) {
	// 1. Get or create session
	session, err := s.getOrCreateSession(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("get or create session: %w", err)
	}

	// Update goal if provided
	if input.Goal != "" {
		session.CurrentGoal = input.Goal
	}

	// Update context if provided
	if len(input.Context) > 0 {
		session.ActiveContext = input.Context
	}

	// 2. Direct activation from cues
	activations := s.activateFromCues(ctx, input.AgentID, input.TenantID, input.Cues)
	s.logger.Debug("direct activations", zap.Int("count", len(activations)))

	// 3. Goal-directed activation bias
	if session.CurrentGoal != "" {
		goalActivations := s.activateFromGoal(ctx, input.AgentID, input.TenantID, session.CurrentGoal)
		activations = s.mergeActivations(activations, goalActivations, GoalActivationBoost)
		s.logger.Debug("after goal activation", zap.Int("count", len(activations)))
	}

	// 4. Schema-directed activation
	activeSchemas := s.getActiveSchemas(ctx, input.AgentID, input.TenantID, input.Cues, input.Context)
	for _, schemaMatch := range activeSchemas {
		schemaActivations := s.activateFromSchema(ctx, input.AgentID, input.TenantID, schemaMatch.Schema)
		activations = s.mergeActivations(activations, schemaActivations, SchemaActivationBoost)
	}
	s.logger.Debug("after schema activation", zap.Int("count", len(activations)), zap.Int("active_schemas", len(activeSchemas)))

	// 5. Temporal activation (recent episodes)
	recentActivations := s.activateRecent(ctx, input.AgentID, input.TenantID, 24*time.Hour)
	activations = s.mergeActivations(activations, recentActivations, TemporalActivationBase)

	// 6. Spreading activation through associations
	spreadActivations := s.spread(ctx, activations, MaxSpreadingDepth)
	activations = s.mergeActivations(activations, spreadActivations, SpreadingDecay)
	s.logger.Debug("after spreading", zap.Int("count", len(activations)))

	// 7. Competition for limited slots (weighted by confidence)
	winners := s.compete(activations, session.MaxSlots)

	// 8. Save activations to session
	if err := s.saveActivations(ctx, session, winners, activeSchemas); err != nil {
		s.logger.Error("failed to save activations", zap.Error(err))
	}

	// 9. Update session
	if err := s.wmStore.UpdateSession(ctx, session); err != nil {
		s.logger.Error("failed to update session", zap.Error(err))
	}

	// 10. Build result
	result := &domain.WorkingMemoryResult{
		Session:   session,
		SlotUsage: len(winners),
		MaxSlots:  session.MaxSlots,
	}

	// Convert to ActivatedContent
	for _, item := range winners {
		result.Activations = append(result.Activations, domain.ActivatedContent{
			Type:       item.Type,
			ID:         item.ID,
			Content:    item.Content,
			Confidence: item.Confidence,
			Score:      item.ActivationLevel * item.Confidence,
		})
	}

	// Add active schemas
	result.ActiveSchemas = append(result.ActiveSchemas, activeSchemas...)

	// Assemble context for LLM
	result.AssembledContext = s.assembleContext(winners, activeSchemas)

	return result, nil
}

// getOrCreateSession retrieves or creates a working memory session.
func (s *WorkingMemoryService) getOrCreateSession(ctx context.Context, input domain.ActivationInput) (*domain.WorkingMemorySession, error) {
	session, err := s.wmStore.GetSession(ctx, input.AgentID, input.TenantID)
	if err == nil {
		return session, nil
	}

	if !errors.Is(err, store.ErrNotFound) {
		return nil, err
	}

	// Create new session
	session = &domain.WorkingMemorySession{
		AgentID:        input.AgentID,
		TenantID:       input.TenantID,
		CurrentGoal:    input.Goal,
		ActiveContext:  input.Context,
		ReasoningState: make(map[string]any),
		MaxSlots:       DefaultMaxSlots,
	}

	if err := s.wmStore.CreateSession(ctx, session); err != nil {
		return nil, fmt.Errorf("create session: %w", err)
	}

	return session, nil
}

// activateFromCues activates memories based on semantic similarity to cues.
func (s *WorkingMemoryService) activateFromCues(ctx context.Context, agentID, tenantID uuid.UUID, cues []string) []activatedItem {
	if len(cues) == 0 || s.embeddingClient == nil {
		return nil
	}

	// Combine cues for embedding
	combinedCue := strings.Join(cues, " ")
	embedding, err := s.embeddingClient.Embed(ctx, combinedCue)
	if err != nil {
		s.logger.Debug("failed to embed cues", zap.Error(err))
		return nil
	}

	var activations []activatedItem

	// Search semantic memories
	if s.memoryStore != nil {
		memories, err := s.memoryStore.Recall(ctx, embedding, agentID, tenantID, domain.RecallOpts{
			TopK:          10,
			MinConfidence: MinActivationLevel,
		})
		if err == nil {
			for _, m := range memories {
				activations = append(activations, activatedItem{
					Type:            domain.ActivatedMemoryTypeSemantic,
					ID:              m.ID,
					Content:         m.Content,
					Confidence:      m.Confidence,
					ActivationLevel: m.Score * DirectActivationBoost,
					Source:          domain.ActivationSourceDirect,
					Cue:             combinedCue,
				})
			}
		}
	}

	// Search episodic memories
	if s.episodeStore != nil {
		episodes, err := s.episodeStore.FindSimilar(ctx, agentID, tenantID, embedding, 0.5, 5)
		if err == nil {
			for _, e := range episodes {
				activations = append(activations, activatedItem{
					Type:            domain.ActivatedMemoryTypeEpisodic,
					ID:              e.ID,
					Content:         e.RawContent,
					Confidence:      e.MemoryStrength,
					ActivationLevel: e.Score * DirectActivationBoost,
					Source:          domain.ActivationSourceDirect,
					Cue:             combinedCue,
				})
			}
		}
	}

	// Search procedural memories
	if s.procedureStore != nil {
		procedures, err := s.procedureStore.FindByTriggerSimilarity(ctx, agentID, tenantID, embedding, 0.6, 5)
		if err == nil {
			for _, p := range procedures {
				activations = append(activations, activatedItem{
					Type:            domain.ActivatedMemoryTypeProcedural,
					ID:              p.ID,
					Content:         fmt.Sprintf("When: %s\nDo: %s", p.TriggerPattern, p.ActionTemplate),
					Confidence:      p.Confidence * p.SuccessRate,
					ActivationLevel: p.Score * DirectActivationBoost,
					Source:          domain.ActivationSourceDirect,
					Cue:             combinedCue,
				})
			}
		}
	}

	return activations
}

// activateFromGoal activates memories relevant to the current goal.
func (s *WorkingMemoryService) activateFromGoal(ctx context.Context, agentID, tenantID uuid.UUID, goal string) []activatedItem {
	if goal == "" || s.embeddingClient == nil {
		return nil
	}

	embedding, err := s.embeddingClient.Embed(ctx, goal)
	if err != nil {
		return nil
	}

	var activations []activatedItem

	// Search semantic memories
	if s.memoryStore != nil {
		memories, err := s.memoryStore.Recall(ctx, embedding, agentID, tenantID, domain.RecallOpts{
			TopK:          5,
			MinConfidence: MinActivationLevel,
		})
		if err == nil {
			for _, m := range memories {
				activations = append(activations, activatedItem{
					Type:            domain.ActivatedMemoryTypeSemantic,
					ID:              m.ID,
					Content:         m.Content,
					Confidence:      m.Confidence,
					ActivationLevel: m.Score,
					Source:          domain.ActivationSourceGoal,
					Cue:             "goal: " + goal,
				})
			}
		}
	}

	// Search procedures
	if s.procedureStore != nil {
		procedures, err := s.procedureStore.FindByTriggerSimilarity(ctx, agentID, tenantID, embedding, 0.5, 3)
		if err == nil {
			for _, p := range procedures {
				activations = append(activations, activatedItem{
					Type:            domain.ActivatedMemoryTypeProcedural,
					ID:              p.ID,
					Content:         fmt.Sprintf("When: %s\nDo: %s", p.TriggerPattern, p.ActionTemplate),
					Confidence:      p.Confidence * p.SuccessRate,
					ActivationLevel: p.Score,
					Source:          domain.ActivationSourceGoal,
					Cue:             "goal: " + goal,
				})
			}
		}
	}

	return activations
}

// getActiveSchemas finds schemas that match the current context.
func (s *WorkingMemoryService) getActiveSchemas(ctx context.Context, agentID, tenantID uuid.UUID, cues []string, context []domain.Message) []domain.SchemaMatch {
	if s.schemaStore == nil {
		return nil
	}

	schemas, err := s.schemaStore.GetByAgent(ctx, agentID, tenantID)
	if err != nil || len(schemas) == 0 {
		return nil
	}

	// Score each schema
	var matches []domain.SchemaMatch
	for _, schema := range schemas {
		score := s.scoreSchemaForContext(schema, cues, context)
		if score >= MinSchemaMatchScore {
			matches = append(matches, domain.SchemaMatch{
				Schema:     schema,
				MatchScore: score,
			})
		}
	}

	// Sort by score
	sort.Slice(matches, func(i, j int) bool {
		return matches[i].MatchScore > matches[j].MatchScore
	})

	// Return top 3
	if len(matches) > 3 {
		matches = matches[:3]
	}

	return matches
}

// scoreSchemaForContext scores how well a schema matches the current context.
func (s *WorkingMemoryService) scoreSchemaForContext(schema domain.Schema, cues []string, context []domain.Message) float32 {
	var score float32

	// Check cue overlap with applicable contexts
	for _, ctx := range schema.ApplicableContexts {
		for _, cue := range cues {
			if strings.Contains(strings.ToLower(cue), strings.ToLower(ctx)) {
				score += 0.2
			}
		}
	}

	// Check context content against schema attributes
	for _, msg := range context {
		content := strings.ToLower(msg.Content)
		for key := range schema.Attributes {
			if strings.Contains(content, strings.ToLower(key)) {
				score += 0.1
			}
		}
	}

	// Weight by schema confidence
	score *= schema.Confidence

	// Cap at 1.0
	if score > 1.0 {
		score = 1.0
	}

	return score
}

// activateFromSchema activates memories related to an active schema.
func (s *WorkingMemoryService) activateFromSchema(ctx context.Context, _, tenantID uuid.UUID, schema domain.Schema) []activatedItem {
	var activations []activatedItem

	// Activate evidence memories
	for _, memID := range schema.EvidenceMemories {
		if s.memoryStore != nil {
			mem, err := s.memoryStore.GetByID(ctx, memID, tenantID)
			if err == nil {
				activations = append(activations, activatedItem{
					Type:            domain.ActivatedMemoryTypeSemantic,
					ID:              mem.ID,
					Content:         mem.Content,
					Confidence:      mem.Confidence,
					ActivationLevel: 0.6, // Moderate activation from schema
					Source:          domain.ActivationSourceSchema,
					Cue:             "schema: " + schema.Name,
				})
			}
		}
	}

	// Activate evidence episodes
	for _, epID := range schema.EvidenceEpisodes {
		if s.episodeStore != nil {
			ep, err := s.episodeStore.GetByID(ctx, epID, tenantID)
			if err == nil {
				activations = append(activations, activatedItem{
					Type:            domain.ActivatedMemoryTypeEpisodic,
					ID:              ep.ID,
					Content:         ep.RawContent,
					Confidence:      ep.MemoryStrength,
					ActivationLevel: 0.5,
					Source:          domain.ActivationSourceSchema,
					Cue:             "schema: " + schema.Name,
				})
			}
		}
	}

	return activations
}

// activateRecent activates recently accessed memories.
func (s *WorkingMemoryService) activateRecent(ctx context.Context, agentID, tenantID uuid.UUID, window time.Duration) []activatedItem {
	var activations []activatedItem
	cutoff := time.Now().Add(-window)

	// Get recent episodes
	if s.episodeStore != nil {
		episodes, err := s.episodeStore.GetByTimeRange(ctx, agentID, tenantID, cutoff, time.Now())
		if err == nil {
			for _, ep := range episodes {
				// Decay based on age
				hoursSinceOccurred := time.Since(ep.OccurredAt).Hours()
				recencyLevel := float32(1.0 - (hoursSinceOccurred * RecencyDecay))
				if recencyLevel < MinActivationLevel {
					continue
				}

				activations = append(activations, activatedItem{
					Type:            domain.ActivatedMemoryTypeEpisodic,
					ID:              ep.ID,
					Content:         ep.RawContent,
					Confidence:      ep.MemoryStrength,
					ActivationLevel: recencyLevel,
					Source:          domain.ActivationSourceTemporal,
					Cue:             "recent",
				})
			}
		}
	}

	return activations
}

// spread performs spreading activation through memory associations.
func (s *WorkingMemoryService) spread(ctx context.Context, seeds []activatedItem, maxDepth int) []activatedItem {
	if s.assocStore == nil || maxDepth == 0 {
		return nil
	}

	var spread []activatedItem
	visited := make(map[string]bool)

	// Mark seeds as visited
	for _, seed := range seeds {
		key := fmt.Sprintf("%s:%s", seed.Type, seed.ID)
		visited[key] = true
	}

	// BFS spreading
	current := seeds
	for depth := 0; depth < maxDepth && len(current) > 0; depth++ {
		var next []activatedItem
		decayFactor := float32(1.0)
		for d := 0; d <= depth; d++ {
			decayFactor *= SpreadingDecay
		}

		for _, item := range current {
			// Get associations from this memory
			assocs, err := s.assocStore.GetBySource(ctx, item.Type, item.ID)
			if err != nil {
				continue
			}

			for _, assoc := range assocs {
				key := fmt.Sprintf("%s:%s", assoc.TargetMemoryType, assoc.TargetMemoryID)
				if visited[key] {
					continue
				}
				visited[key] = true

				// Calculate spread activation
				spreadLevel := item.ActivationLevel * assoc.AssociationStrength * decayFactor
				if spreadLevel < MinActivationLevel {
					continue
				}

				// Get the target memory content
				content, confidence := s.getMemoryContent(ctx, assoc.TargetMemoryType, assoc.TargetMemoryID, item.ID)
				if content == "" {
					continue
				}

				activated := activatedItem{
					Type:            assoc.TargetMemoryType,
					ID:              assoc.TargetMemoryID,
					Content:         content,
					Confidence:      confidence,
					ActivationLevel: spreadLevel,
					Source:          domain.ActivationSourceSpread,
					Cue:             fmt.Sprintf("spread from %s", item.Type),
				}
				spread = append(spread, activated)
				next = append(next, activated)
			}
		}
		current = next
	}

	return spread
}

// getMemoryContent retrieves content for a memory by type.
func (s *WorkingMemoryService) getMemoryContent(ctx context.Context, memType domain.ActivatedMemoryType, memID, tenantID uuid.UUID) (string, float32) {
	switch memType {
	case domain.ActivatedMemoryTypeSemantic:
		if s.memoryStore != nil {
			mem, err := s.memoryStore.GetByID(ctx, memID, tenantID)
			if err == nil {
				return mem.Content, mem.Confidence
			}
		}
	case domain.ActivatedMemoryTypeEpisodic:
		if s.episodeStore != nil {
			ep, err := s.episodeStore.GetByID(ctx, memID, tenantID)
			if err == nil {
				return ep.RawContent, ep.MemoryStrength
			}
		}
	case domain.ActivatedMemoryTypeProcedural:
		if s.procedureStore != nil {
			proc, err := s.procedureStore.GetByID(ctx, memID, tenantID)
			if err == nil {
				return fmt.Sprintf("When: %s\nDo: %s", proc.TriggerPattern, proc.ActionTemplate), proc.Confidence
			}
		}
	case domain.ActivatedMemoryTypeSchema:
		if s.schemaStore != nil {
			schema, err := s.schemaStore.GetByID(ctx, memID, tenantID)
			if err == nil {
				return fmt.Sprintf("%s: %s", schema.Name, schema.Description), schema.Confidence
			}
		}
	}
	return "", 0
}

// mergeActivations merges two activation lists, combining duplicates.
func (s *WorkingMemoryService) mergeActivations(a, b []activatedItem, boost float32) []activatedItem {
	// Create map for deduplication
	byKey := make(map[string]activatedItem)

	for _, item := range a {
		key := fmt.Sprintf("%s:%s", item.Type, item.ID)
		byKey[key] = item
	}

	for _, item := range b {
		key := fmt.Sprintf("%s:%s", item.Type, item.ID)
		item.ActivationLevel *= boost

		if existing, ok := byKey[key]; ok {
			// Take higher activation, keep both sources
			if item.ActivationLevel > existing.ActivationLevel {
				existing.ActivationLevel = item.ActivationLevel
				existing.Source = item.Source
				existing.Cue = item.Cue
			}
			byKey[key] = existing
		} else {
			byKey[key] = item
		}
	}

	result := make([]activatedItem, 0, len(byKey))
	for _, item := range byKey {
		result = append(result, item)
	}

	return result
}

// compete selects the top memories for limited working memory slots.
func (s *WorkingMemoryService) compete(activations []activatedItem, maxSlots int) []activatedItem {
	if len(activations) == 0 {
		return nil
	}

	// Sort by effective score: activation_level * confidence
	sort.Slice(activations, func(i, j int) bool {
		scoreI := activations[i].ActivationLevel * activations[i].Confidence
		scoreJ := activations[j].ActivationLevel * activations[j].Confidence
		return scoreI > scoreJ
	})

	// Take top maxSlots
	if len(activations) > maxSlots {
		activations = activations[:maxSlots]
	}

	return activations
}

// saveActivations persists activations to the database.
func (s *WorkingMemoryService) saveActivations(ctx context.Context, session *domain.WorkingMemorySession, items []activatedItem, schemas []domain.SchemaMatch) error {
	// Clear existing activations
	if err := s.wmStore.ClearActivations(ctx, session.ID); err != nil {
		return err
	}
	if err := s.wmStore.ClearSchemaActivations(ctx, session.ID); err != nil {
		return err
	}

	// Save new activations
	for i, item := range items {
		pos := i + 1
		activation := &domain.WorkingMemoryActivation{
			SessionID:       session.ID,
			MemoryType:      item.Type,
			MemoryID:        item.ID,
			ActivationLevel: item.ActivationLevel,
			ActivationSource: item.Source,
			ActivationCue:   item.Cue,
			SlotPosition:    &pos,
		}
		if err := s.wmStore.CreateActivation(ctx, activation); err != nil {
			s.logger.Debug("failed to save activation", zap.Error(err))
		}
	}

	// Save schema activations
	for _, match := range schemas {
		schemaAct := &domain.SchemaActivation{
			SessionID:  session.ID,
			SchemaID:   match.Schema.ID,
			MatchScore: match.MatchScore,
		}
		if err := s.wmStore.CreateSchemaActivation(ctx, schemaAct); err != nil {
			s.logger.Debug("failed to save schema activation", zap.Error(err))
		}
	}

	return nil
}

// assembleContext creates a formatted context string for LLM injection.
func (s *WorkingMemoryService) assembleContext(items []activatedItem, schemas []domain.SchemaMatch) string {
	if len(items) == 0 && len(schemas) == 0 {
		return ""
	}

	var parts []string

	// Group by type
	var beliefs, episodes, procedures []string

	for _, item := range items {
		switch item.Type {
		case domain.ActivatedMemoryTypeSemantic:
			beliefs = append(beliefs, fmt.Sprintf("- %s (confidence: %.0f%%)", item.Content, item.Confidence*100))
		case domain.ActivatedMemoryTypeEpisodic:
			// Truncate long episodes
			content := item.Content
			if len(content) > 200 {
				content = content[:200] + "..."
			}
			episodes = append(episodes, fmt.Sprintf("- %s", content))
		case domain.ActivatedMemoryTypeProcedural:
			procedures = append(procedures, fmt.Sprintf("- %s", item.Content))
		}
	}

	if len(beliefs) > 0 {
		parts = append(parts, "**Known Facts/Preferences:**\n"+strings.Join(beliefs, "\n"))
	}

	if len(episodes) > 0 {
		parts = append(parts, "**Relevant Past Experiences:**\n"+strings.Join(episodes, "\n"))
	}

	if len(procedures) > 0 {
		parts = append(parts, "**Applicable Patterns:**\n"+strings.Join(procedures, "\n"))
	}

	if len(schemas) > 0 {
		var schemaStrs []string
		for _, sm := range schemas {
			schemaStrs = append(schemaStrs, fmt.Sprintf("- %s: %s", sm.Schema.Name, sm.Schema.Description))
		}
		parts = append(parts, "**Active Mental Models:**\n"+strings.Join(schemaStrs, "\n"))
	}

	return strings.Join(parts, "\n\n")
}

// GetSession retrieves the current working memory session for an agent.
func (s *WorkingMemoryService) GetSession(ctx context.Context, agentID, tenantID uuid.UUID) (*domain.WorkingMemorySession, error) {
	session, err := s.wmStore.GetSession(ctx, agentID, tenantID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, ErrSessionNotFound
		}
		return nil, err
	}

	// Load activations
	activations, err := s.wmStore.GetActivations(ctx, session.ID)
	if err == nil {
		session.Activations = activations
	}

	// Load schema activations
	schemaActs, err := s.wmStore.GetSchemaActivations(ctx, session.ID)
	if err == nil {
		session.ActiveSchemas = schemaActs
	}

	return session, nil
}

// ClearSession clears the working memory session for an agent.
func (s *WorkingMemoryService) ClearSession(ctx context.Context, agentID, tenantID uuid.UUID) error {
	return s.wmStore.DeleteSession(ctx, agentID, tenantID)
}

// UpdateGoal updates the current goal in working memory.
func (s *WorkingMemoryService) UpdateGoal(ctx context.Context, agentID, tenantID uuid.UUID, goal string) error {
	session, err := s.wmStore.GetSession(ctx, agentID, tenantID)
	if err != nil {
		return err
	}

	session.CurrentGoal = goal
	return s.wmStore.UpdateSession(ctx, session)
}

// CreateAssociation creates a memory association for spreading activation.
func (s *WorkingMemoryService) CreateAssociation(ctx context.Context, assoc *domain.MemoryAssociation) error {
	return s.assocStore.Create(ctx, assoc)
}
