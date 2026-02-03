package service

import (
	"context"
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/Harshitk-cp/engram/internal/domain"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// Metacognition constants
const (
	// Confidence assessment
	RecencyDecayDays          = 30.0 // Days for recency factor to decay
	MaxReinforcementBonus     = 0.5  // Maximum bonus from reinforcement
	ContradictionPenaltyPer   = 0.1  // Penalty per contradiction
	MinAdjustedConfidence     = 0.05 // Floor for adjusted confidence
	MaxAdjustedConfidence     = 1.0  // Ceiling for adjusted confidence

	// Uncertainty thresholds
	LowConfidenceThreshold    = 0.6  // Below this is considered low confidence
	StaleMemoryDays           = 30   // Days since verification to be considered stale
	HighUncertaintyLevel      = 0.7  // Overall uncertainty level considered high

	// Strategy reflection
	MinProcedureUsesForEval   = 5    // Minimum uses before evaluating effectiveness
	LowSuccessRateThreshold   = 0.5  // Below this is underperforming
	HighSuccessRateThreshold  = 0.8  // Above this is highly effective
	RecentFailureLookbackDays = 30   // Days to look back for failure pattern analysis

	// Source reliability
	SourceReliabilityUserStatement  = 1.0
	SourceReliabilityExtraction     = 0.8
	SourceReliabilityAgentInference = 0.7
	SourceReliabilityToolOutput     = 0.9
	SourceReliabilityDefault        = 0.75
)

// ConfidenceAssessment contains the detailed assessment of a memory's confidence.
type ConfidenceAssessment struct {
	MemoryID           uuid.UUID          `json:"memory_id"`
	Content            string             `json:"content"`
	MemoryType         domain.MemoryType  `json:"memory_type"`
	BaseConfidence     float32            `json:"base_confidence"`
	AdjustedConfidence float32            `json:"adjusted_confidence"`
	Factors            map[string]float32 `json:"factors"`
	Explanation        string             `json:"explanation"`
}

// UncertaintyReport contains areas of uncertainty for an agent.
type UncertaintyReport struct {
	Topic               string          `json:"topic,omitempty"`
	UncertaintyLevel    float32         `json:"uncertainty_level"` // 0-1, higher = more uncertain
	ContradictedBeliefs []domain.Memory `json:"contradicted_beliefs,omitempty"`
	LowConfidenceBeliefs []domain.Memory `json:"low_confidence_beliefs,omitempty"`
	StaleBeliefs        []domain.Memory `json:"stale_beliefs,omitempty"`
	Recommendation      string          `json:"recommendation"`
}

// ProcedureAssessment contains the assessment of a procedure's effectiveness.
type ProcedureAssessment struct {
	Procedure      domain.Procedure `json:"procedure"`
	SuccessRate    float32          `json:"success_rate"`
	Recommendation string           `json:"recommendation"`
}

// FailurePattern represents a common pattern found in failed episodes.
type FailurePattern struct {
	Pattern     string   `json:"pattern"`
	Frequency   int      `json:"frequency"`
	Topics      []string `json:"topics,omitempty"`
	Suggestion  string   `json:"suggestion"`
}

// StrategyReflection contains the reflection on agent strategies.
type StrategyReflection struct {
	EffectiveStrategies       []ProcedureAssessment `json:"effective_strategies,omitempty"`
	UnderperformingStrategies []ProcedureAssessment `json:"underperforming_strategies,omitempty"`
	FailurePatterns           []FailurePattern      `json:"failure_patterns,omitempty"`
	Suggestions               []string              `json:"suggestions,omitempty"`
}

// ReflectionResult contains the full metacognitive reflection.
type ReflectionResult struct {
	ConfidenceAssessments []ConfidenceAssessment `json:"confidence_assessments,omitempty"`
	UncertaintyReport     *UncertaintyReport     `json:"uncertainty_report,omitempty"`
	StrategyReflection    *StrategyReflection    `json:"strategy_reflection,omitempty"`
	OverallHealthScore    float32                `json:"overall_health_score"`
	ActionItems           []string               `json:"action_items"`
}

// MetacognitiveService provides self-awareness about memory quality and strategy effectiveness.
type MetacognitiveService struct {
	memoryStore        domain.MemoryStore
	episodeStore       domain.EpisodeStore
	procedureStore     domain.ProcedureStore
	schemaStore        domain.SchemaStore
	contradictionStore domain.ContradictionStore
	embeddingClient    domain.EmbeddingClient
	logger             *zap.Logger
}

// NewMetacognitiveService creates a new metacognitive service.
func NewMetacognitiveService(
	memoryStore domain.MemoryStore,
	episodeStore domain.EpisodeStore,
	procedureStore domain.ProcedureStore,
	schemaStore domain.SchemaStore,
	contradictionStore domain.ContradictionStore,
	embeddingClient domain.EmbeddingClient,
	logger *zap.Logger,
) *MetacognitiveService {
	return &MetacognitiveService{
		memoryStore:        memoryStore,
		episodeStore:       episodeStore,
		procedureStore:     procedureStore,
		schemaStore:        schemaStore,
		contradictionStore: contradictionStore,
		embeddingClient:    embeddingClient,
		logger:             logger,
	}
}

// AssessConfidence evaluates how confident we should be in a memory.
func (s *MetacognitiveService) AssessConfidence(ctx context.Context, memory domain.Memory) (*ConfidenceAssessment, error) {
	assessment := &ConfidenceAssessment{
		MemoryID:       memory.ID,
		Content:        memory.Content,
		MemoryType:     memory.Type,
		BaseConfidence: memory.Confidence,
		Factors:        make(map[string]float32),
	}

	// Factor 1: Recency - how fresh is this memory?
	recencyFactor := s.calculateRecencyFactor(memory)
	assessment.Factors["recency"] = recencyFactor

	// Factor 2: Reinforcement - has this been confirmed multiple times?
	reinforcementFactor := s.calculateReinforcementFactor(memory)
	assessment.Factors["reinforcement"] = reinforcementFactor

	// Factor 3: Contradiction check
	contradictionPenalty := float32(0)
	if s.contradictionStore != nil {
		contradictions, err := s.contradictionStore.GetByBeliefID(ctx, memory.ID)
		if err != nil {
			s.logger.Debug("failed to get contradictions", zap.Error(err))
		} else {
			contradictionPenalty = float32(len(contradictions)) * ContradictionPenaltyPer
		}
	}
	assessment.Factors["contradictions"] = -contradictionPenalty

	// Factor 4: Source reliability
	sourceFactor := s.assessSourceReliability(memory.Source)
	assessment.Factors["source"] = sourceFactor

	// Combined assessment
	// Formula: base * recency * (1 + reinforcement) * source - contradiction_penalty
	adjusted := memory.Confidence * recencyFactor * (1 + reinforcementFactor) * sourceFactor - contradictionPenalty

	// Clamp to valid range
	if adjusted < MinAdjustedConfidence {
		adjusted = MinAdjustedConfidence
	}
	if adjusted > MaxAdjustedConfidence {
		adjusted = MaxAdjustedConfidence
	}
	assessment.AdjustedConfidence = adjusted

	// Generate explanation
	assessment.Explanation = s.generateConfidenceExplanation(assessment)

	return assessment, nil
}

// calculateRecencyFactor computes the recency decay factor.
func (s *MetacognitiveService) calculateRecencyFactor(memory domain.Memory) float32 {
	var lastVerified time.Time
	if memory.LastVerifiedAt != nil {
		lastVerified = *memory.LastVerifiedAt
	} else if memory.LastAccessedAt != nil {
		lastVerified = *memory.LastAccessedAt
	} else {
		lastVerified = memory.UpdatedAt
	}

	daysSince := time.Since(lastVerified).Hours() / 24
	// Exponential decay over RecencyDecayDays
	factor := math.Exp(-daysSince / RecencyDecayDays)
	return float32(factor)
}

// calculateReinforcementFactor computes bonus from reinforcement.
func (s *MetacognitiveService) calculateReinforcementFactor(memory domain.Memory) float32 {
	// Logarithmic scaling to prevent runaway confidence
	if memory.ReinforcementCount <= 0 {
		return 0
	}
	factor := math.Log(float64(memory.ReinforcementCount+1)) * 0.1
	if factor > MaxReinforcementBonus {
		factor = MaxReinforcementBonus
	}
	return float32(factor)
}

// assessSourceReliability returns a reliability factor based on memory source.
func (s *MetacognitiveService) assessSourceReliability(source string) float32 {
	switch domain.BeliefSource(source) {
	case domain.SourceUserStatement:
		return SourceReliabilityUserStatement
	case domain.SourceExtraction:
		return SourceReliabilityExtraction
	case domain.SourceAgentInference:
		return SourceReliabilityAgentInference
	case domain.SourceToolOutput:
		return SourceReliabilityToolOutput
	default:
		return SourceReliabilityDefault
	}
}

// generateConfidenceExplanation creates a human-readable explanation.
func (s *MetacognitiveService) generateConfidenceExplanation(assessment *ConfidenceAssessment) string {
	var explanation string

	// Overall confidence level
	if assessment.AdjustedConfidence >= 0.8 {
		explanation = "High confidence"
	} else if assessment.AdjustedConfidence >= 0.6 {
		explanation = "Moderate confidence"
	} else if assessment.AdjustedConfidence >= 0.4 {
		explanation = "Low confidence"
	} else {
		explanation = "Very low confidence"
	}

	// Add contributing factors
	if assessment.Factors["recency"] < 0.5 {
		explanation += " - memory is stale (not recently verified)"
	}
	if assessment.Factors["reinforcement"] > 0.2 {
		explanation += " - reinforced by repeated observations"
	}
	if assessment.Factors["contradictions"] < 0 {
		explanation += " - has contradicting information"
	}
	if assessment.Factors["source"] < 0.8 {
		explanation += " - from less reliable source"
	}

	return explanation
}

// DetectUncertainty identifies areas where the agent should be uncertain.
func (s *MetacognitiveService) DetectUncertainty(ctx context.Context, agentID, tenantID uuid.UUID, topic string) (*UncertaintyReport, error) {
	report := &UncertaintyReport{
		Topic:               topic,
		ContradictedBeliefs: []domain.Memory{},
		LowConfidenceBeliefs: []domain.Memory{},
		StaleBeliefs:        []domain.Memory{},
	}

	// Get relevant memories
	var memories []domain.Memory
	var err error

	if topic != "" && s.embeddingClient != nil {
		// Search by topic using embedding
		embedding, err := s.embeddingClient.Embed(ctx, topic)
		if err != nil {
			s.logger.Debug("failed to embed topic", zap.Error(err))
		} else {
			similar, err := s.memoryStore.FindSimilar(ctx, agentID, tenantID, embedding, 0.5)
			if err != nil {
				s.logger.Debug("failed to find similar memories", zap.Error(err))
			} else {
				for _, ms := range similar {
					memories = append(memories, ms.Memory)
				}
			}
		}
	}

	// If no topic-specific memories or no topic, get all agent memories
	if len(memories) == 0 {
		memories, err = s.memoryStore.GetByAgentForDecay(ctx, agentID)
		if err != nil {
			return nil, fmt.Errorf("failed to get memories: %w", err)
		}
	}

	// Analyze memories for uncertainty signals
	now := time.Now()
	staleThreshold := now.Add(-StaleMemoryDays * 24 * time.Hour)

	for _, mem := range memories {
		// Check for contradictions
		if s.contradictionStore != nil {
			contradictions, err := s.contradictionStore.GetByBeliefID(ctx, mem.ID)
			if err == nil && len(contradictions) > 0 {
				report.ContradictedBeliefs = append(report.ContradictedBeliefs, mem)
			}
		}

		// Check for low confidence
		if mem.Confidence < LowConfidenceThreshold {
			report.LowConfidenceBeliefs = append(report.LowConfidenceBeliefs, mem)
		}

		// Check for stale memories
		var lastVerified time.Time
		if mem.LastVerifiedAt != nil {
			lastVerified = *mem.LastVerifiedAt
		} else if mem.LastAccessedAt != nil {
			lastVerified = *mem.LastAccessedAt
		} else {
			lastVerified = mem.UpdatedAt
		}

		if lastVerified.Before(staleThreshold) {
			report.StaleBeliefs = append(report.StaleBeliefs, mem)
		}
	}

	// Calculate overall uncertainty level
	report.UncertaintyLevel = s.calculateUncertaintyLevel(report, len(memories))
	report.Recommendation = s.generateUncertaintyRecommendation(report)

	return report, nil
}

// calculateUncertaintyLevel computes the overall uncertainty 0-1.
func (s *MetacognitiveService) calculateUncertaintyLevel(report *UncertaintyReport, totalMemories int) float32 {
	if totalMemories == 0 {
		return 0
	}

	// Weight different uncertainty factors
	contradictionWeight := 0.4
	lowConfidenceWeight := 0.35
	stalenessWeight := 0.25

	contradictionRatio := float32(len(report.ContradictedBeliefs)) / float32(totalMemories)
	lowConfidenceRatio := float32(len(report.LowConfidenceBeliefs)) / float32(totalMemories)
	stalenessRatio := float32(len(report.StaleBeliefs)) / float32(totalMemories)

	uncertainty := float32(contradictionWeight)*contradictionRatio +
		float32(lowConfidenceWeight)*lowConfidenceRatio +
		float32(stalenessWeight)*stalenessRatio

	if uncertainty > 1.0 {
		uncertainty = 1.0
	}

	return uncertainty
}

// generateUncertaintyRecommendation creates actionable recommendations.
func (s *MetacognitiveService) generateUncertaintyRecommendation(report *UncertaintyReport) string {
	if report.UncertaintyLevel < 0.2 {
		return "Knowledge base is relatively certain. Continue normal operations."
	}

	var recommendations []string

	if len(report.ContradictedBeliefs) > 0 {
		recommendations = append(recommendations,
			fmt.Sprintf("Resolve %d contradicted beliefs by verifying with the user", len(report.ContradictedBeliefs)))
	}

	if len(report.LowConfidenceBeliefs) > 0 {
		recommendations = append(recommendations,
			fmt.Sprintf("Confirm %d low-confidence memories through conversation", len(report.LowConfidenceBeliefs)))
	}

	if len(report.StaleBeliefs) > 0 {
		recommendations = append(recommendations,
			fmt.Sprintf("Update %d stale memories that haven't been verified recently", len(report.StaleBeliefs)))
	}

	if len(recommendations) == 0 {
		return "Review memory health and verify uncertain beliefs with the user."
	}

	return recommendations[0]
}

// ReflectOnStrategy evaluates how well the agent's strategies are working.
func (s *MetacognitiveService) ReflectOnStrategy(ctx context.Context, agentID, tenantID uuid.UUID) (*StrategyReflection, error) {
	reflection := &StrategyReflection{
		EffectiveStrategies:       []ProcedureAssessment{},
		UnderperformingStrategies: []ProcedureAssessment{},
		FailurePatterns:           []FailurePattern{},
		Suggestions:               []string{},
	}

	// Get procedures for this agent
	if s.procedureStore == nil {
		return reflection, nil
	}

	procedures, err := s.procedureStore.GetByAgent(ctx, agentID, tenantID)
	if err != nil {
		s.logger.Debug("failed to get procedures", zap.Error(err))
		return reflection, nil
	}

	// Evaluate each procedure
	for _, proc := range procedures {
		if proc.UseCount < MinProcedureUsesForEval {
			continue // Not enough data to evaluate
		}

		assessment := ProcedureAssessment{
			Procedure:   proc,
			SuccessRate: proc.SuccessRate,
		}

		if proc.SuccessRate < LowSuccessRateThreshold {
			assessment.Recommendation = "Consider retiring or modifying this approach - success rate is below 50%"
			reflection.UnderperformingStrategies = append(reflection.UnderperformingStrategies, assessment)
		} else if proc.SuccessRate >= HighSuccessRateThreshold {
			assessment.Recommendation = "This approach is working well - consider applying to similar situations"
			reflection.EffectiveStrategies = append(reflection.EffectiveStrategies, assessment)
		}
	}

	// Analyze failure patterns from episodes
	if s.episodeStore != nil {
		failurePatterns := s.analyzeFailurePatterns(ctx, agentID, tenantID)
		reflection.FailurePatterns = failurePatterns
	}

	// Generate improvement suggestions
	reflection.Suggestions = s.generateImprovementSuggestions(reflection)

	return reflection, nil
}

// analyzeFailurePatterns identifies common patterns in failed episodes.
func (s *MetacognitiveService) analyzeFailurePatterns(ctx context.Context, agentID, tenantID uuid.UUID) []FailurePattern {
	patterns := []FailurePattern{}

	if s.episodeStore == nil {
		return patterns
	}

	// Get recent episodes with failures
	endTime := time.Now()
	startTime := endTime.Add(-RecentFailureLookbackDays * 24 * time.Hour)

	episodes, err := s.episodeStore.GetByTimeRange(ctx, agentID, tenantID, startTime, endTime)
	if err != nil {
		s.logger.Debug("failed to get episodes for failure analysis", zap.Error(err))
		return patterns
	}

	// Collect topics from failed episodes
	topicFrequency := make(map[string]int)
	failureCount := 0

	for _, ep := range episodes {
		if ep.Outcome != domain.OutcomeFailure {
			continue
		}
		failureCount++

		for _, topic := range ep.Topics {
			topicFrequency[topic]++
		}
	}

	if failureCount == 0 {
		return patterns
	}

	// Find topics that appear frequently in failures
	type topicCount struct {
		topic string
		count int
	}
	var sortedTopics []topicCount
	for topic, count := range topicFrequency {
		if count >= 2 { // At least 2 occurrences
			sortedTopics = append(sortedTopics, topicCount{topic, count})
		}
	}

	sort.Slice(sortedTopics, func(i, j int) bool {
		return sortedTopics[i].count > sortedTopics[j].count
	})

	// Convert to failure patterns (top 3)
	for i, tc := range sortedTopics {
		if i >= 3 {
			break
		}
		pattern := FailurePattern{
			Pattern:    fmt.Sprintf("Recurring failures involving '%s'", tc.topic),
			Frequency:  tc.count,
			Topics:     []string{tc.topic},
			Suggestion: fmt.Sprintf("Review and improve handling of '%s' related situations", tc.topic),
		}
		patterns = append(patterns, pattern)
	}

	return patterns
}

// generateImprovementSuggestions creates actionable suggestions.
func (s *MetacognitiveService) generateImprovementSuggestions(reflection *StrategyReflection) []string {
	suggestions := []string{}

	if len(reflection.UnderperformingStrategies) > 0 {
		suggestions = append(suggestions,
			fmt.Sprintf("Review %d underperforming strategies for potential improvements or retirement",
				len(reflection.UnderperformingStrategies)))
	}

	if len(reflection.FailurePatterns) > 0 {
		suggestions = append(suggestions,
			"Analyze recurring failure patterns to identify root causes")
	}

	if len(reflection.EffectiveStrategies) > 0 && len(reflection.UnderperformingStrategies) > 0 {
		suggestions = append(suggestions,
			"Consider applying patterns from effective strategies to improve underperforming ones")
	}

	if len(suggestions) == 0 {
		suggestions = append(suggestions, "Continue monitoring strategy effectiveness as more data is collected")
	}

	return suggestions
}

// Reflect performs a full metacognitive reflection for an agent.
func (s *MetacognitiveService) Reflect(ctx context.Context, agentID, tenantID uuid.UUID, focus string) (*ReflectionResult, error) {
	result := &ReflectionResult{
		ConfidenceAssessments: []ConfidenceAssessment{},
		ActionItems:           []string{},
	}

	// Confidence assessments
	if focus == "" || focus == "all" || focus == "confidence" {
		memories, err := s.memoryStore.GetByAgentForDecay(ctx, agentID)
		if err != nil {
			s.logger.Debug("failed to get memories for confidence assessment", zap.Error(err))
		} else {
			// Assess top memories (limit to avoid overwhelming response)
			limit := min(20, len(memories))

			for i := range limit {
				assessment, err := s.AssessConfidence(ctx, memories[i])
				if err != nil {
					s.logger.Debug("failed to assess confidence", zap.Error(err))
					continue
				}
				result.ConfidenceAssessments = append(result.ConfidenceAssessments, *assessment)
			}
		}
	}

	// Uncertainty report
	if focus == "" || focus == "all" || focus == "uncertainty" {
		uncertainty, err := s.DetectUncertainty(ctx, agentID, tenantID, "")
		if err != nil {
			s.logger.Debug("failed to detect uncertainty", zap.Error(err))
		} else {
			result.UncertaintyReport = uncertainty
		}
	}

	// Strategy reflection
	if focus == "" || focus == "all" || focus == "strategy" {
		strategy, err := s.ReflectOnStrategy(ctx, agentID, tenantID)
		if err != nil {
			s.logger.Debug("failed to reflect on strategy", zap.Error(err))
		} else {
			result.StrategyReflection = strategy
		}
	}

	// Calculate overall health score
	result.OverallHealthScore = s.calculateOverallHealthScore(result)

	// Generate action items
	result.ActionItems = s.generateActionItems(result)

	return result, nil
}

// calculateOverallHealthScore computes a 0-1 health score.
func (s *MetacognitiveService) calculateOverallHealthScore(result *ReflectionResult) float32 {
	var scores []float32
	var weights []float32

	// Confidence component
	if len(result.ConfidenceAssessments) > 0 {
		var avgConfidence float32
		for _, assessment := range result.ConfidenceAssessments {
			avgConfidence += assessment.AdjustedConfidence
		}
		avgConfidence /= float32(len(result.ConfidenceAssessments))
		scores = append(scores, avgConfidence)
		weights = append(weights, 0.4)
	}

	// Uncertainty component (inverted - low uncertainty is good)
	if result.UncertaintyReport != nil {
		certaintyScore := 1.0 - result.UncertaintyReport.UncertaintyLevel
		scores = append(scores, certaintyScore)
		weights = append(weights, 0.3)
	}

	// Strategy component
	if result.StrategyReflection != nil {
		totalStrategies := len(result.StrategyReflection.EffectiveStrategies) +
			len(result.StrategyReflection.UnderperformingStrategies)
		if totalStrategies > 0 {
			strategyScore := float32(len(result.StrategyReflection.EffectiveStrategies)) / float32(totalStrategies)
			scores = append(scores, strategyScore)
			weights = append(weights, 0.3)
		}
	}

	if len(scores) == 0 {
		return 0.5 // Default neutral score
	}

	// Weighted average
	var totalWeight float32
	var weightedSum float32
	for i, score := range scores {
		weightedSum += score * weights[i]
		totalWeight += weights[i]
	}

	return weightedSum / totalWeight
}

// generateActionItems creates prioritized action items.
func (s *MetacognitiveService) generateActionItems(result *ReflectionResult) []string {
	items := []string{}

	// High priority: contradictions
	if result.UncertaintyReport != nil && len(result.UncertaintyReport.ContradictedBeliefs) > 0 {
		items = append(items, fmt.Sprintf("PRIORITY: Resolve %d contradicted beliefs",
			len(result.UncertaintyReport.ContradictedBeliefs)))
	}

	// Medium priority: underperforming strategies
	if result.StrategyReflection != nil && len(result.StrategyReflection.UnderperformingStrategies) > 0 {
		items = append(items, fmt.Sprintf("Review %d underperforming strategies",
			len(result.StrategyReflection.UnderperformingStrategies)))
	}

	// Medium priority: low confidence memories
	if result.UncertaintyReport != nil && len(result.UncertaintyReport.LowConfidenceBeliefs) > 3 {
		items = append(items, fmt.Sprintf("Verify %d low-confidence memories",
			len(result.UncertaintyReport.LowConfidenceBeliefs)))
	}

	// Low priority: stale memories
	if result.UncertaintyReport != nil && len(result.UncertaintyReport.StaleBeliefs) > 5 {
		items = append(items, fmt.Sprintf("Update %d stale memories",
			len(result.UncertaintyReport.StaleBeliefs)))
	}

	// Failure patterns
	if result.StrategyReflection != nil && len(result.StrategyReflection.FailurePatterns) > 0 {
		items = append(items, "Investigate recurring failure patterns")
	}

	if len(items) == 0 {
		items = append(items, "Memory system is healthy - continue monitoring")
	}

	return items
}

// GetConfidenceExplanationForMemory returns a confidence explanation for a specific memory.
// This can be used to enhance recall responses with confidence context.
func (s *MetacognitiveService) GetConfidenceExplanationForMemory(ctx context.Context, memoryID, tenantID uuid.UUID) (string, float32, error) {
	memory, err := s.memoryStore.GetByID(ctx, memoryID, tenantID)
	if err != nil {
		return "", 0, fmt.Errorf("memory not found: %w", err)
	}

	assessment, err := s.AssessConfidence(ctx, *memory)
	if err != nil {
		return "", memory.Confidence, err
	}

	return assessment.Explanation, assessment.AdjustedConfidence, nil
}
