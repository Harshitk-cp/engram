package llm

import (
	"context"

	"github.com/Harshitk-cp/engram/internal/domain"
)

// MockClient is a configurable LLM client for testing.
// Set the response fields to control what each method returns.
type MockClient struct {
	ClassifyResponse                domain.MemoryType
	ClassifyError                   error
	ExtractResponse                 []domain.ExtractedMemory
	ExtractError                    error
	SummarizeResponse               string
	SummarizeError                  error
	CheckContradictionResponse      bool
	CheckContradictionError         error
	ExtractEpisodeStructureResponse *domain.EpisodeExtraction
	ExtractEpisodeStructureError    error
	ExtractProcedureResponse        *domain.ProcedureExtraction
	ExtractProcedureError           error
	DetectSchemaPatternResponse     *domain.SchemaExtraction
	DetectSchemaPatternError        error

	// Call tracking for assertions
	ClassifyCalls                []string
	ExtractCalls                 [][]domain.Message
	SummarizeCalls               [][]domain.Memory
	CheckContradictionCalls      []struct{ A, B string }
	ExtractEpisodeStructureCalls []string
	ExtractProcedureCalls        []string
	DetectSchemaPatternCalls     [][]domain.Memory
}

func NewMockClient() *MockClient {
	return &MockClient{
		ClassifyResponse:  domain.MemoryTypeFact,
		SummarizeResponse: "Mock summary",
		ExtractResponse:   []domain.ExtractedMemory{},
		ExtractEpisodeStructureResponse: &domain.EpisodeExtraction{
			Entities:        []string{},
			Topics:          []string{},
			CausalLinks:     []domain.CausalLink{},
			ImportanceScore: 0.5,
		},
		ExtractProcedureResponse: &domain.ProcedureExtraction{
			TriggerPattern:  "When user asks about X",
			TriggerKeywords: []string{"X"},
			ActionTemplate:  "Respond with Y",
			ActionType:      domain.ActionTypeResponseStyle,
		},
		DetectSchemaPatternResponse: &domain.SchemaExtraction{
			SchemaType:         domain.SchemaTypeUserArchetype,
			Name:               "Mock Schema",
			Description:        "A mock schema for testing",
			Attributes:         map[string]any{"mock": true},
			ApplicableContexts: []string{"testing"},
			Confidence:         0.8,
		},
	}
}

func (c *MockClient) Classify(ctx context.Context, content string) (domain.MemoryType, error) {
	c.ClassifyCalls = append(c.ClassifyCalls, content)
	if c.ClassifyError != nil {
		return "", c.ClassifyError
	}
	return c.ClassifyResponse, nil
}

func (c *MockClient) Extract(ctx context.Context, conversation []domain.Message) ([]domain.ExtractedMemory, error) {
	c.ExtractCalls = append(c.ExtractCalls, conversation)
	if c.ExtractError != nil {
		return nil, c.ExtractError
	}
	return c.ExtractResponse, nil
}

func (c *MockClient) Summarize(ctx context.Context, memories []domain.Memory) (string, error) {
	c.SummarizeCalls = append(c.SummarizeCalls, memories)
	if c.SummarizeError != nil {
		return "", c.SummarizeError
	}
	return c.SummarizeResponse, nil
}

func (c *MockClient) CheckContradiction(ctx context.Context, stmtA, stmtB string) (bool, error) {
	c.CheckContradictionCalls = append(c.CheckContradictionCalls, struct{ A, B string }{stmtA, stmtB})
	if c.CheckContradictionError != nil {
		return false, c.CheckContradictionError
	}
	return c.CheckContradictionResponse, nil
}

func (c *MockClient) ExtractEpisodeStructure(ctx context.Context, content string) (*domain.EpisodeExtraction, error) {
	c.ExtractEpisodeStructureCalls = append(c.ExtractEpisodeStructureCalls, content)
	if c.ExtractEpisodeStructureError != nil {
		return nil, c.ExtractEpisodeStructureError
	}
	return c.ExtractEpisodeStructureResponse, nil
}

func (c *MockClient) ExtractProcedure(ctx context.Context, content string) (*domain.ProcedureExtraction, error) {
	c.ExtractProcedureCalls = append(c.ExtractProcedureCalls, content)
	if c.ExtractProcedureError != nil {
		return nil, c.ExtractProcedureError
	}
	return c.ExtractProcedureResponse, nil
}

func (c *MockClient) DetectSchemaPattern(ctx context.Context, memories []domain.Memory) (*domain.SchemaExtraction, error) {
	c.DetectSchemaPatternCalls = append(c.DetectSchemaPatternCalls, memories)
	if c.DetectSchemaPatternError != nil {
		return nil, c.DetectSchemaPatternError
	}
	return c.DetectSchemaPatternResponse, nil
}

// Reset clears all recorded calls and resets responses to defaults.
func (c *MockClient) Reset() {
	c.ClassifyResponse = domain.MemoryTypeFact
	c.ClassifyError = nil
	c.ExtractResponse = []domain.ExtractedMemory{}
	c.ExtractError = nil
	c.SummarizeResponse = "Mock summary"
	c.SummarizeError = nil
	c.CheckContradictionResponse = false
	c.CheckContradictionError = nil
	c.ExtractEpisodeStructureResponse = &domain.EpisodeExtraction{
		Entities:        []string{},
		Topics:          []string{},
		CausalLinks:     []domain.CausalLink{},
		ImportanceScore: 0.5,
	}
	c.ExtractEpisodeStructureError = nil
	c.ExtractProcedureResponse = &domain.ProcedureExtraction{
		TriggerPattern:  "When user asks about X",
		TriggerKeywords: []string{"X"},
		ActionTemplate:  "Respond with Y",
		ActionType:      domain.ActionTypeResponseStyle,
	}
	c.ExtractProcedureError = nil
	c.DetectSchemaPatternResponse = &domain.SchemaExtraction{
		SchemaType:         domain.SchemaTypeUserArchetype,
		Name:               "Mock Schema",
		Description:        "A mock schema for testing",
		Attributes:         map[string]any{"mock": true},
		ApplicableContexts: []string{"testing"},
		Confidence:         0.8,
	}
	c.DetectSchemaPatternError = nil
	c.ClassifyCalls = nil
	c.ExtractCalls = nil
	c.SummarizeCalls = nil
	c.CheckContradictionCalls = nil
	c.ExtractEpisodeStructureCalls = nil
	c.ExtractProcedureCalls = nil
	c.DetectSchemaPatternCalls = nil
}
