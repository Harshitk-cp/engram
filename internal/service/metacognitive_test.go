package service

import (
	"context"
	"testing"
	"time"

	"github.com/Harshitk-cp/engram/internal/domain"
	"github.com/google/uuid"
)

// mockContradictionStoreForMetacog implements domain.ContradictionStore for metacognitive testing.
type mockContradictionStoreForMetacog struct {
	contradictions map[uuid.UUID][]domain.BeliefContradiction
}

func newMockContradictionStoreForMetacog() *mockContradictionStoreForMetacog {
	return &mockContradictionStoreForMetacog{
		contradictions: make(map[uuid.UUID][]domain.BeliefContradiction),
	}
}

func (m *mockContradictionStoreForMetacog) Create(ctx context.Context, beliefID, contradictedByID uuid.UUID) error {
	contradiction := domain.BeliefContradiction{
		ID:               uuid.New(),
		BeliefID:         beliefID,
		ContradictedByID: contradictedByID,
		DetectedAt:       time.Now(),
	}
	m.contradictions[beliefID] = append(m.contradictions[beliefID], contradiction)
	return nil
}

func (m *mockContradictionStoreForMetacog) GetByBeliefID(ctx context.Context, beliefID uuid.UUID) ([]domain.BeliefContradiction, error) {
	return m.contradictions[beliefID], nil
}

func (m *mockContradictionStoreForMetacog) GetByContradictedByID(ctx context.Context, contradictedByID uuid.UUID) ([]domain.BeliefContradiction, error) {
	var result []domain.BeliefContradiction
	for _, contradictions := range m.contradictions {
		for _, c := range contradictions {
			if c.ContradictedByID == contradictedByID {
				result = append(result, c)
			}
		}
	}
	return result, nil
}

func setupMetacognitiveTest() (*MetacognitiveService, *mockMemoryStore, *mockContradictionStoreForMetacog, *mockProcedureStore, *mockEpisodeStore, uuid.UUID, uuid.UUID) {
	memStore := newMockMemoryStore()
	episodeStore := newMockEpisodeStore()
	procedureStore := newMockProcedureStore()
	schemaStore := newMockSchemaStore()
	contradictionStore := newMockContradictionStoreForMetacog()
	embeddingClient := &mockEmbeddingClient{}

	svc := NewMetacognitiveService(memStore, episodeStore, procedureStore, schemaStore, contradictionStore, embeddingClient, testLogger())

	tenantID := uuid.New()
	agentID := uuid.New()

	return svc, memStore, contradictionStore, procedureStore, episodeStore, tenantID, agentID
}

func TestMetacognitiveService_AssessConfidence(t *testing.T) {
	svc, _, _, _, _, _, _ := setupMetacognitiveTest()
	ctx := context.Background()

	// Test with a recent, high-confidence memory
	now := time.Now()
	mem := domain.Memory{
		ID:                 uuid.New(),
		Content:            "User prefers dark mode",
		Type:               domain.MemoryTypePreference,
		Confidence:         0.9,
		LastVerifiedAt:     &now,
		ReinforcementCount: 3,
		Source:             string(domain.SourceUserStatement),
	}

	assessment, err := svc.AssessConfidence(ctx, mem)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if assessment.MemoryID != mem.ID {
		t.Fatalf("expected memory ID %s, got %s", mem.ID, assessment.MemoryID)
	}

	if assessment.BaseConfidence != 0.9 {
		t.Fatalf("expected base confidence 0.9, got %f", assessment.BaseConfidence)
	}

	// With a recent verification and reinforcement, adjusted should be high
	if assessment.AdjustedConfidence < 0.8 {
		t.Fatalf("expected adjusted confidence >= 0.8 for recent reinforced memory, got %f", assessment.AdjustedConfidence)
	}

	if assessment.Explanation == "" {
		t.Fatal("expected non-empty explanation")
	}

	// Verify factors are populated
	if _, ok := assessment.Factors["recency"]; !ok {
		t.Fatal("expected recency factor")
	}
	if _, ok := assessment.Factors["reinforcement"]; !ok {
		t.Fatal("expected reinforcement factor")
	}
	if _, ok := assessment.Factors["source"]; !ok {
		t.Fatal("expected source factor")
	}
}

func TestMetacognitiveService_AssessConfidence_StaleMemory(t *testing.T) {
	svc, _, _, _, _, _, _ := setupMetacognitiveTest()
	ctx := context.Background()

	// Test with a stale memory (60 days old)
	staleTime := time.Now().Add(-60 * 24 * time.Hour)
	mem := domain.Memory{
		ID:                 uuid.New(),
		Content:            "User prefers dark mode",
		Type:               domain.MemoryTypePreference,
		Confidence:         0.9,
		LastVerifiedAt:     &staleTime,
		ReinforcementCount: 0,
		Source:             string(domain.SourceAgentInference),
	}

	assessment, err := svc.AssessConfidence(ctx, mem)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// With stale verification and no reinforcement, adjusted should be lower
	if assessment.AdjustedConfidence >= assessment.BaseConfidence {
		t.Fatalf("expected adjusted confidence < base for stale memory, got adjusted=%f, base=%f",
			assessment.AdjustedConfidence, assessment.BaseConfidence)
	}

	// Recency factor should be low
	if assessment.Factors["recency"] > 0.3 {
		t.Fatalf("expected low recency factor for stale memory, got %f", assessment.Factors["recency"])
	}
}

func TestMetacognitiveService_AssessConfidence_WithContradiction(t *testing.T) {
	svc, memStore, contradictionStore, _, _, tenantID, agentID := setupMetacognitiveTest()
	ctx := context.Background()

	// Create a memory with contradiction
	now := time.Now()
	mem := &domain.Memory{
		AgentID:            agentID,
		TenantID:           tenantID,
		Content:            "User prefers dark mode",
		Type:               domain.MemoryTypePreference,
		Confidence:         0.9,
		LastVerifiedAt:     &now,
		ReinforcementCount: 0,
		Source:             string(domain.SourceUserStatement),
	}
	_ = memStore.Create(ctx, mem)

	// Add a contradiction
	contradictingMemID := uuid.New()
	_ = contradictionStore.Create(ctx, mem.ID, contradictingMemID)

	assessment, err := svc.AssessConfidence(ctx, *mem)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// Contradiction should penalize confidence
	if assessment.Factors["contradictions"] >= 0 {
		t.Fatalf("expected negative contradiction factor, got %f", assessment.Factors["contradictions"])
	}
}

func TestMetacognitiveService_DetectUncertainty(t *testing.T) {
	svc, memStore, contradictionStore, _, _, tenantID, agentID := setupMetacognitiveTest()
	ctx := context.Background()

	// Create some memories with different states
	now := time.Now()
	staleTime := now.Add(-60 * 24 * time.Hour)

	// High confidence, recent memory
	goodMem := &domain.Memory{
		AgentID:        agentID,
		TenantID:       tenantID,
		Content:        "User prefers dark mode",
		Type:           domain.MemoryTypePreference,
		Confidence:     0.95,
		LastVerifiedAt: &now,
	}
	_ = memStore.Create(ctx, goodMem)

	// Low confidence memory
	lowConfMem := &domain.Memory{
		AgentID:        agentID,
		TenantID:       tenantID,
		Content:        "User might like Python",
		Type:           domain.MemoryTypeFact,
		Confidence:     0.4,
		LastVerifiedAt: &now,
	}
	_ = memStore.Create(ctx, lowConfMem)

	// Stale memory
	staleMem := &domain.Memory{
		AgentID:        agentID,
		TenantID:       tenantID,
		Content:        "User worked at Acme",
		Type:           domain.MemoryTypeFact,
		Confidence:     0.8,
		LastVerifiedAt: &staleTime,
	}
	_ = memStore.Create(ctx, staleMem)

	// Contradicted memory
	contradictedMem := &domain.Memory{
		AgentID:        agentID,
		TenantID:       tenantID,
		Content:        "User prefers light mode",
		Type:           domain.MemoryTypePreference,
		Confidence:     0.7,
		LastVerifiedAt: &now,
	}
	_ = memStore.Create(ctx, contradictedMem)
	_ = contradictionStore.Create(ctx, contradictedMem.ID, goodMem.ID)

	report, err := svc.DetectUncertainty(ctx, agentID, tenantID, "")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// Should detect low confidence beliefs
	if len(report.LowConfidenceBeliefs) != 1 {
		t.Fatalf("expected 1 low confidence belief, got %d", len(report.LowConfidenceBeliefs))
	}

	// Should detect stale beliefs
	if len(report.StaleBeliefs) != 1 {
		t.Fatalf("expected 1 stale belief, got %d", len(report.StaleBeliefs))
	}

	// Should detect contradicted beliefs
	if len(report.ContradictedBeliefs) != 1 {
		t.Fatalf("expected 1 contradicted belief, got %d", len(report.ContradictedBeliefs))
	}

	// Uncertainty level should be non-zero
	if report.UncertaintyLevel == 0 {
		t.Fatal("expected non-zero uncertainty level")
	}

	// Should have a recommendation
	if report.Recommendation == "" {
		t.Fatal("expected non-empty recommendation")
	}
}

func TestMetacognitiveService_DetectUncertainty_NoIssues(t *testing.T) {
	svc, memStore, _, _, _, tenantID, agentID := setupMetacognitiveTest()
	ctx := context.Background()

	// Create only good memories
	now := time.Now()
	for i := 0; i < 3; i++ {
		mem := &domain.Memory{
			AgentID:        agentID,
			TenantID:       tenantID,
			Content:        "Good memory",
			Type:           domain.MemoryTypeFact,
			Confidence:     0.9,
			LastVerifiedAt: &now,
		}
		_ = memStore.Create(ctx, mem)
	}

	report, err := svc.DetectUncertainty(ctx, agentID, tenantID, "")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// Should have low uncertainty
	if report.UncertaintyLevel > 0.2 {
		t.Fatalf("expected low uncertainty level for healthy memories, got %f", report.UncertaintyLevel)
	}
}

func TestMetacognitiveService_ReflectOnStrategy(t *testing.T) {
	svc, _, _, procedureStore, episodeStore, tenantID, agentID := setupMetacognitiveTest()
	ctx := context.Background()

	// Create an effective procedure (high success rate)
	effectiveProc := &domain.Procedure{
		AgentID:        agentID,
		TenantID:       tenantID,
		TriggerPattern: "When user asks about debugging",
		ActionTemplate: "Provide step-by-step troubleshooting",
		ActionType:     domain.ActionTypeProblemSolving,
		UseCount:       10,
		SuccessCount:   9,
		FailureCount:   1,
		SuccessRate:    0.9,
		Confidence:     0.8,
	}
	_ = procedureStore.Create(ctx, effectiveProc)

	// Create an underperforming procedure (low success rate)
	badProc := &domain.Procedure{
		AgentID:        agentID,
		TenantID:       tenantID,
		TriggerPattern: "When user seems frustrated",
		ActionTemplate: "Apologize profusely",
		ActionType:     domain.ActionTypeCommunication,
		UseCount:       10,
		SuccessCount:   3,
		FailureCount:   7,
		SuccessRate:    0.3,
		Confidence:     0.5,
	}
	_ = procedureStore.Create(ctx, badProc)

	// Create some failure episodes
	now := time.Now()
	for i := 0; i < 3; i++ {
		ep := &domain.Episode{
			AgentID:    agentID,
			TenantID:   tenantID,
			RawContent: "Failed interaction about authentication",
			Topics:     []string{"authentication", "error"},
			Outcome:    domain.OutcomeFailure,
			OccurredAt: now.Add(-time.Duration(i) * 24 * time.Hour),
		}
		_ = episodeStore.Create(ctx, ep)
	}

	reflection, err := svc.ReflectOnStrategy(ctx, agentID, tenantID)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// Should identify effective strategies
	if len(reflection.EffectiveStrategies) != 1 {
		t.Fatalf("expected 1 effective strategy, got %d", len(reflection.EffectiveStrategies))
	}

	// Should identify underperforming strategies
	if len(reflection.UnderperformingStrategies) != 1 {
		t.Fatalf("expected 1 underperforming strategy, got %d", len(reflection.UnderperformingStrategies))
	}

	// Should have failure patterns (authentication appears 3 times)
	if len(reflection.FailurePatterns) == 0 {
		t.Fatal("expected failure patterns to be detected")
	}

	// Should have suggestions
	if len(reflection.Suggestions) == 0 {
		t.Fatal("expected suggestions to be generated")
	}
}

func TestMetacognitiveService_Reflect(t *testing.T) {
	svc, memStore, _, procedureStore, _, tenantID, agentID := setupMetacognitiveTest()
	ctx := context.Background()

	// Setup test data
	now := time.Now()

	// Add a memory
	mem := &domain.Memory{
		AgentID:        agentID,
		TenantID:       tenantID,
		Content:        "Test memory",
		Type:           domain.MemoryTypeFact,
		Confidence:     0.8,
		LastVerifiedAt: &now,
	}
	_ = memStore.Create(ctx, mem)

	// Add a procedure
	proc := &domain.Procedure{
		AgentID:     agentID,
		TenantID:    tenantID,
		UseCount:    10,
		SuccessRate: 0.9,
		Confidence:  0.8,
	}
	_ = procedureStore.Create(ctx, proc)

	// Test full reflection (all focus areas)
	result, err := svc.Reflect(ctx, agentID, tenantID, "all")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// Should have confidence assessments
	if len(result.ConfidenceAssessments) == 0 {
		t.Fatal("expected confidence assessments")
	}

	// Should have uncertainty report
	if result.UncertaintyReport == nil {
		t.Fatal("expected uncertainty report")
	}

	// Should have strategy reflection
	if result.StrategyReflection == nil {
		t.Fatal("expected strategy reflection")
	}

	// Should have overall health score
	if result.OverallHealthScore <= 0 || result.OverallHealthScore > 1 {
		t.Fatalf("expected health score between 0 and 1, got %f", result.OverallHealthScore)
	}

	// Should have action items
	if len(result.ActionItems) == 0 {
		t.Fatal("expected action items")
	}
}

func TestMetacognitiveService_Reflect_FocusConfidence(t *testing.T) {
	svc, memStore, _, _, _, tenantID, agentID := setupMetacognitiveTest()
	ctx := context.Background()

	now := time.Now()
	mem := &domain.Memory{
		AgentID:        agentID,
		TenantID:       tenantID,
		Content:        "Test memory",
		Type:           domain.MemoryTypeFact,
		Confidence:     0.8,
		LastVerifiedAt: &now,
	}
	_ = memStore.Create(ctx, mem)

	result, err := svc.Reflect(ctx, agentID, tenantID, "confidence")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// Should have confidence assessments
	if len(result.ConfidenceAssessments) == 0 {
		t.Fatal("expected confidence assessments with focus=confidence")
	}

	// Should NOT have uncertainty report
	if result.UncertaintyReport != nil {
		t.Fatal("expected no uncertainty report with focus=confidence")
	}

	// Should NOT have strategy reflection
	if result.StrategyReflection != nil {
		t.Fatal("expected no strategy reflection with focus=confidence")
	}
}

func TestMetacognitiveService_GetConfidenceExplanationForMemory(t *testing.T) {
	svc, memStore, _, _, _, tenantID, agentID := setupMetacognitiveTest()
	ctx := context.Background()

	now := time.Now()
	mem := &domain.Memory{
		AgentID:        agentID,
		TenantID:       tenantID,
		Content:        "User prefers dark mode",
		Type:           domain.MemoryTypePreference,
		Confidence:     0.9,
		LastVerifiedAt: &now,
	}
	_ = memStore.Create(ctx, mem)

	explanation, adjustedConfidence, err := svc.GetConfidenceExplanationForMemory(ctx, mem.ID, tenantID)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if explanation == "" {
		t.Fatal("expected non-empty explanation")
	}

	if adjustedConfidence <= 0 || adjustedConfidence > 1 {
		t.Fatalf("expected adjusted confidence between 0 and 1, got %f", adjustedConfidence)
	}
}

func TestMetacognitiveService_SourceReliability(t *testing.T) {
	svc, _, _, _, _, _, _ := setupMetacognitiveTest()
	ctx := context.Background()

	now := time.Now()

	testCases := []struct {
		source           string
		expectedMinScore float32
	}{
		{string(domain.SourceUserStatement), 0.95},
		{string(domain.SourceToolOutput), 0.85},
		{string(domain.SourceExtraction), 0.75},
		{string(domain.SourceAgentInference), 0.65},
		{"unknown_source", 0.7},
	}

	for _, tc := range testCases {
		mem := domain.Memory{
			ID:             uuid.New(),
			Content:        "Test memory",
			Confidence:     1.0,
			LastVerifiedAt: &now,
			Source:         tc.source,
		}

		assessment, err := svc.AssessConfidence(ctx, mem)
		if err != nil {
			t.Fatalf("expected no error for source %s, got %v", tc.source, err)
		}

		if assessment.Factors["source"] < tc.expectedMinScore {
			t.Fatalf("expected source factor >= %f for %s, got %f",
				tc.expectedMinScore, tc.source, assessment.Factors["source"])
		}
	}
}
