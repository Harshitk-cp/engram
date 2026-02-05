package service

import (
	"context"
	"errors"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/Harshitk-cp/engram/internal/domain"
	"github.com/Harshitk-cp/engram/internal/store"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// Schema service constants
const (
	MinClusterSize            = 5    // Minimum memories to form a schema
	MinEvidenceConfidence     = 0.6  // Minimum confidence for memories to count as evidence
	MinEvidenceAge            = 24 * time.Hour // Minimum stability period
	SchemaSimilarityThreshold = 0.7  // For finding similar schemas
	SchemaConfidenceBoost     = 0.05 // Confidence boost per evidence
	MaxSchemaConfidence       = 0.95 // Maximum schema confidence
	MinSchemaMatchScore       = 0.3  // Minimum score to consider a schema match
	ContextMatchWeight        = 0.3  // Weight for context matching
	TimeMatchWeight           = 0.2  // Weight for time-based matching
	EmbeddingSimilarityWeight = 0.5  // Weight for embedding similarity
	ClusteringThreshold       = 0.65 // Cosine similarity threshold for clustering
)

var (
	ErrSchemaNotFound       = errors.New("schema not found")
	ErrInvalidSchemaType    = errors.New("invalid schema type")
	ErrInsufficientEvidence = errors.New("insufficient evidence to form schema")
)

// SchemaService handles schema detection, matching, and management.
type SchemaService struct {
	schemaStore     domain.SchemaStore
	memoryStore     domain.MemoryStore
	agentStore      domain.AgentStore
	embeddingClient domain.EmbeddingClient
	llmClient       domain.LLMClient
	logger          *zap.Logger
}

// NewSchemaService creates a new schema service.
func NewSchemaService(
	schemaStore domain.SchemaStore,
	memoryStore domain.MemoryStore,
	agentStore domain.AgentStore,
	embeddingClient domain.EmbeddingClient,
	llmClient domain.LLMClient,
	logger *zap.Logger,
) *SchemaService {
	return &SchemaService{
		schemaStore:     schemaStore,
		memoryStore:     memoryStore,
		agentStore:      agentStore,
		embeddingClient: embeddingClient,
		llmClient:       llmClient,
		logger:          logger,
	}
}

// SchemaMatchInput contains input for schema matching.
type SchemaMatchInput struct {
	AgentID       uuid.UUID
	TenantID      uuid.UUID
	Query         string   // Current query/situation to match against
	Contexts      []string // Current context tags (e.g., "debugging", "late_night")
	TimeOfDay     string   // "morning", "afternoon", "evening", "night"
	MinMatchScore float32  // Minimum match score (defaults to MinSchemaMatchScore)
	Limit         int      // Maximum results (defaults to 5)
}

// DetectSchemas identifies patterns across semantic memories and creates schemas.
func (s *SchemaService) DetectSchemas(ctx context.Context, agentID uuid.UUID, tenantID uuid.UUID) ([]domain.Schema, error) {
	// Get all memories for the agent
	allMemories, err := s.memoryStore.GetByAgentForDecay(ctx, agentID)
	if err != nil {
		return nil, err
	}

	// Filter to memories that meet evidence quality thresholds
	now := time.Now()
	var memories []domain.Memory
	for _, m := range allMemories {
		if m.Confidence < MinEvidenceConfidence {
			continue
		}
		if now.Sub(m.CreatedAt) < MinEvidenceAge {
			continue
		}
		memories = append(memories, m)
	}

	if len(memories) < MinClusterSize {
		s.logger.Debug("not enough qualified memories for schema detection",
			zap.String("agent_id", agentID.String()),
			zap.Int("qualified_count", len(memories)),
			zap.Int("total_count", len(allMemories)))
		return nil, nil
	}

	// Cluster memories by similarity
	clusters := s.clusterMemories(memories)

	var detectedSchemas []domain.Schema
	for _, cluster := range clusters {
		if len(cluster.Memories) < MinClusterSize {
			continue // Need minimum evidence
		}

		// Use LLM to detect schema pattern
		if s.llmClient == nil {
			continue
		}

		schemaExtraction, err := s.llmClient.DetectSchemaPattern(ctx, cluster.Memories)
		if err != nil {
			s.logger.Debug("failed to detect schema pattern", zap.Error(err))
			continue
		}
		if schemaExtraction == nil {
			continue
		}

		// Check if schema already exists
		existing, err := s.schemaStore.GetByName(ctx, agentID, tenantID, schemaExtraction.SchemaType, schemaExtraction.Name)
		if err == nil && existing != nil {
			// Update existing schema with new evidence
			if err := s.updateSchemaEvidence(ctx, existing, cluster); err != nil {
				s.logger.Debug("failed to update schema evidence", zap.Error(err))
			}
			detectedSchemas = append(detectedSchemas, *existing)
			continue
		}

		// Create new schema
		schema := &domain.Schema{
			AgentID:            agentID,
			TenantID:           tenantID,
			SchemaType:         schemaExtraction.SchemaType,
			Name:               schemaExtraction.Name,
			Description:        schemaExtraction.Description,
			Attributes:         schemaExtraction.Attributes,
			ApplicableContexts: schemaExtraction.ApplicableContexts,
			EvidenceMemories:   cluster.MemoryIDs,
			EvidenceCount:      len(cluster.Memories),
			Confidence:         s.calculateInitialConfidence(len(cluster.Memories)),
		}

		now := time.Now()
		schema.LastValidatedAt = &now

		// Generate embedding for the schema
		if s.embeddingClient != nil {
			schemaText := schema.Name + ": " + schema.Description
			embedding, err := s.embeddingClient.Embed(ctx, schemaText)
			if err != nil {
				s.logger.Debug("failed to generate schema embedding", zap.Error(err))
			} else {
				schema.Embedding = embedding
			}
		}

		if err := s.schemaStore.Create(ctx, schema); err != nil {
			s.logger.Debug("failed to create schema", zap.Error(err))
			continue
		}

		s.logger.Info("detected new schema",
			zap.String("schema_id", schema.ID.String()),
			zap.String("name", schema.Name),
			zap.String("type", string(schema.SchemaType)),
			zap.Int("evidence_count", schema.EvidenceCount))

		detectedSchemas = append(detectedSchemas, *schema)
	}

	return detectedSchemas, nil
}

// MatchSchemas finds schemas that apply to the current situation.
func (s *SchemaService) MatchSchemas(ctx context.Context, input SchemaMatchInput) ([]domain.SchemaMatch, error) {
	// Set defaults
	if input.MinMatchScore == 0 {
		input.MinMatchScore = MinSchemaMatchScore
	}
	if input.Limit == 0 {
		input.Limit = 5
	}

	// Get all schemas for the agent
	schemas, err := s.schemaStore.GetByAgent(ctx, input.AgentID, input.TenantID)
	if err != nil {
		return nil, err
	}

	if len(schemas) == 0 {
		return nil, nil
	}

	// Generate embedding for the query
	var queryEmbedding []float32
	if s.embeddingClient != nil && input.Query != "" {
		embedding, err := s.embeddingClient.Embed(ctx, input.Query)
		if err != nil {
			s.logger.Debug("failed to generate query embedding", zap.Error(err))
		} else {
			queryEmbedding = embedding
		}
	}

	// Score each schema against current context
	var matches []domain.SchemaMatch
	for _, schema := range schemas {
		score, reason := s.scoreSchemaMatch(schema, input, queryEmbedding)
		if score >= input.MinMatchScore {
			matches = append(matches, domain.SchemaMatch{
				Schema:      schema,
				MatchScore:  score,
				MatchReason: reason,
			})
		}
	}

	// Sort by score descending
	sort.Slice(matches, func(i, j int) bool {
		return matches[i].MatchScore > matches[j].MatchScore
	})

	// Limit results
	if len(matches) > input.Limit {
		matches = matches[:input.Limit]
	}

	return matches, nil
}

// GetByID retrieves a schema by ID.
func (s *SchemaService) GetByID(ctx context.Context, id uuid.UUID, tenantID uuid.UUID) (*domain.Schema, error) {
	schema, err := s.schemaStore.GetByID(ctx, id, tenantID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, ErrSchemaNotFound
		}
		return nil, err
	}
	return schema, nil
}

// GetByAgent retrieves all schemas for an agent.
func (s *SchemaService) GetByAgent(ctx context.Context, agentID uuid.UUID, tenantID uuid.UUID) ([]domain.Schema, error) {
	return s.schemaStore.GetByAgent(ctx, agentID, tenantID)
}

// Delete removes a schema.
func (s *SchemaService) Delete(ctx context.Context, id uuid.UUID, tenantID uuid.UUID) error {
	err := s.schemaStore.Delete(ctx, id, tenantID)
	if errors.Is(err, store.ErrNotFound) {
		return ErrSchemaNotFound
	}
	return err
}

// RecordContradiction records a contradiction for a schema.
func (s *SchemaService) RecordContradiction(ctx context.Context, id uuid.UUID, tenantID uuid.UUID) error {
	schema, err := s.schemaStore.GetByID(ctx, id, tenantID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return ErrSchemaNotFound
		}
		return err
	}

	// Increment contradiction count
	if err := s.schemaStore.IncrementContradiction(ctx, id); err != nil {
		return err
	}

	// Decrease confidence based on contradictions
	newConfidence := schema.Confidence - 0.1
	if newConfidence < 0.1 {
		newConfidence = 0.1
	}

	return s.schemaStore.UpdateConfidence(ctx, id, newConfidence)
}

// ValidateSchema validates a schema and updates its validation timestamp.
func (s *SchemaService) ValidateSchema(ctx context.Context, id uuid.UUID, tenantID uuid.UUID) error {
	_, err := s.schemaStore.GetByID(ctx, id, tenantID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return ErrSchemaNotFound
		}
		return err
	}

	return s.schemaStore.UpdateValidation(ctx, id)
}

// clusterMemories groups memories by semantic similarity.
func (s *SchemaService) clusterMemories(memories []domain.Memory) []domain.MemoryCluster {
	if len(memories) == 0 {
		return nil
	}

	// Simple single-linkage clustering based on embeddings
	// Memories with embeddings get clustered; others are ignored for now
	var memoriesWithEmbeddings []domain.Memory
	for _, m := range memories {
		if len(m.Embedding) > 0 {
			memoriesWithEmbeddings = append(memoriesWithEmbeddings, m)
		}
	}

	if len(memoriesWithEmbeddings) < MinClusterSize {
		return nil
	}

	// Track which memories are assigned to clusters
	assigned := make(map[uuid.UUID]bool)
	var clusters []domain.MemoryCluster

	for i, seed := range memoriesWithEmbeddings {
		if assigned[seed.ID] {
			continue
		}

		// Start a new cluster with this memory
		cluster := domain.MemoryCluster{
			Memories:  []domain.Memory{seed},
			MemoryIDs: []uuid.UUID{seed.ID},
			Centroid:  seed.Embedding,
		}
		assigned[seed.ID] = true

		// Find similar memories
		for j := i + 1; j < len(memoriesWithEmbeddings); j++ {
			candidate := memoriesWithEmbeddings[j]
			if assigned[candidate.ID] {
				continue
			}

			similarity := cosineSimilarity(cluster.Centroid, candidate.Embedding)
			if similarity >= ClusteringThreshold {
				cluster.Memories = append(cluster.Memories, candidate)
				cluster.MemoryIDs = append(cluster.MemoryIDs, candidate.ID)
				assigned[candidate.ID] = true

				// Update centroid (simple average)
				cluster.Centroid = averageVectors(cluster.Centroid, candidate.Embedding)
			}
		}

		// Extract theme from cluster
		cluster.Theme = s.extractClusterTheme(cluster.Memories)

		clusters = append(clusters, cluster)
	}

	return clusters
}

// extractClusterTheme extracts a theme from cluster memories.
func (s *SchemaService) extractClusterTheme(memories []domain.Memory) string {
	if len(memories) == 0 {
		return ""
	}

	// Simple approach: find common words/themes
	// For MVP, just use the type distribution
	typeCounts := make(map[domain.MemoryType]int)
	for _, m := range memories {
		typeCounts[m.Type]++
	}

	maxType := domain.MemoryTypeFact
	maxCount := 0
	for t, count := range typeCounts {
		if count > maxCount {
			maxType = t
			maxCount = count
		}
	}

	return string(maxType) + "-cluster"
}

// scoreSchemaMatch calculates how well a schema matches the current situation.
func (s *SchemaService) scoreSchemaMatch(schema domain.Schema, input SchemaMatchInput, queryEmbedding []float32) (float32, string) {
	var score float32 = 0
	var reasons []string

	// Context matching
	contextScore := s.scoreContextMatch(schema.ApplicableContexts, input.Contexts)
	if contextScore > 0 {
		score += contextScore * ContextMatchWeight
		reasons = append(reasons, "context match")
	}

	// Time matching
	if input.TimeOfDay != "" {
		timeScore := s.scoreTimeMatch(schema.Attributes, input.TimeOfDay)
		if timeScore > 0 {
			score += timeScore * TimeMatchWeight
			reasons = append(reasons, "time preference match")
		}
	}

	// Embedding similarity
	if len(queryEmbedding) > 0 && len(schema.Embedding) > 0 {
		similarity := cosineSimilarity(queryEmbedding, schema.Embedding)
		if similarity > 0.5 {
			score += similarity * EmbeddingSimilarityWeight
			reasons = append(reasons, "semantic similarity")
		}
	}

	// Weight by schema confidence
	score *= schema.Confidence

	reason := strings.Join(reasons, ", ")
	if reason == "" {
		reason = "low match"
	}

	return score, reason
}

// scoreContextMatch scores how well contexts match.
func (s *SchemaService) scoreContextMatch(schemaContexts []string, inputContexts []string) float32 {
	if len(schemaContexts) == 0 || len(inputContexts) == 0 {
		return 0
	}

	matches := 0
	for _, sc := range schemaContexts {
		for _, ic := range inputContexts {
			if strings.EqualFold(sc, ic) || strings.Contains(strings.ToLower(ic), strings.ToLower(sc)) {
				matches++
				break
			}
		}
	}

	if matches == 0 {
		return 0
	}

	return float32(matches) / float32(len(schemaContexts))
}

// scoreTimeMatch scores time preference matching.
func (s *SchemaService) scoreTimeMatch(attributes map[string]any, timeOfDay string) float32 {
	if timeOfDay == "" || attributes == nil {
		return 0
	}

	// Check for time_preference attribute
	if pref, ok := attributes["time_preference"]; ok {
		if prefStr, ok := pref.(string); ok {
			if strings.EqualFold(prefStr, timeOfDay) {
				return 1.0
			}
		}
	}

	// Check for work_hours attribute
	if hours, ok := attributes["work_hours"]; ok {
		if hoursStr, ok := hours.(string); ok {
			if strings.Contains(strings.ToLower(hoursStr), strings.ToLower(timeOfDay)) {
				return 0.8
			}
		}
	}

	return 0
}

// updateSchemaEvidence updates an existing schema with new evidence.
func (s *SchemaService) updateSchemaEvidence(ctx context.Context, schema *domain.Schema, cluster domain.MemoryCluster) error {
	// Add new memory IDs that aren't already in evidence
	existingIDs := make(map[uuid.UUID]bool)
	for _, id := range schema.EvidenceMemories {
		existingIDs[id] = true
	}

	newCount := 0
	for _, id := range cluster.MemoryIDs {
		if !existingIDs[id] {
			if err := s.schemaStore.AddEvidence(ctx, schema.ID, &id, nil); err != nil {
				s.logger.Debug("failed to add evidence", zap.Error(err))
				continue
			}
			newCount++
		}
	}

	if newCount > 0 {
		// Boost confidence
		newConfidence := schema.Confidence + float32(newCount)*SchemaConfidenceBoost
		if newConfidence > MaxSchemaConfidence {
			newConfidence = MaxSchemaConfidence
		}
		return s.schemaStore.UpdateConfidence(ctx, schema.ID, newConfidence)
	}

	return nil
}

// calculateInitialConfidence calculates initial confidence based on evidence count.
func (s *SchemaService) calculateInitialConfidence(evidenceCount int) float32 {
	// Start at 0.5 and increase with more evidence
	confidence := 0.5 + float32(evidenceCount)*0.05
	if confidence > 0.8 {
		confidence = 0.8 // Cap initial confidence
	}
	return confidence
}

// cosineSimilarity calculates cosine similarity between two vectors.
func cosineSimilarity(a, b []float32) float32 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}

	var dotProduct, normA, normB float64
	for i := 0; i < len(a); i++ {
		dotProduct += float64(a[i]) * float64(b[i])
		normA += float64(a[i]) * float64(a[i])
		normB += float64(b[i]) * float64(b[i])
	}

	if normA == 0 || normB == 0 {
		return 0
	}

	return float32(dotProduct / (math.Sqrt(normA) * math.Sqrt(normB)))
}

// averageVectors computes the element-wise average of two vectors.
func averageVectors(a, b []float32) []float32 {
	if len(a) != len(b) {
		return a
	}

	result := make([]float32, len(a))
	for i := 0; i < len(a); i++ {
		result[i] = (a[i] + b[i]) / 2
	}
	return result
}
