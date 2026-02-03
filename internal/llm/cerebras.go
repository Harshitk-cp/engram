package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/Harshitk-cp/engram/internal/domain"
)

const (
	cerebrasAPIURL = "https://api.cerebras.ai/v1/chat/completions"
	cerebrasModel  = "llama-3.3-70b"
)

type CerebrasClient struct {
	apiKey     string
	httpClient *http.Client
}

func NewCerebrasClient(apiKey string) *CerebrasClient {
	return &CerebrasClient{
		apiKey:     apiKey,
		httpClient: &http.Client{},
	}
}

// Cerebras uses OpenAI-compatible request/response format
type cerebrasMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type cerebrasRequest struct {
	Model       string            `json:"model"`
	Messages    []cerebrasMessage `json:"messages"`
	Temperature float32           `json:"temperature"`
}

type cerebrasResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

func (c *CerebrasClient) complete(ctx context.Context, messages []cerebrasMessage, temp float32) (string, error) {
	body, err := json.Marshal(cerebrasRequest{
		Model:       cerebrasModel,
		Messages:    messages,
		Temperature: temp,
	})
	if err != nil {
		return "", fmt.Errorf("marshal cerebras request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, cerebrasAPIURL, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("create cerebras request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("cerebras request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read cerebras response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("cerebras API returned status %d: %s", resp.StatusCode, string(respBody))
	}

	var result cerebrasResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", fmt.Errorf("unmarshal cerebras response: %w", err)
	}

	if result.Error != nil {
		return "", fmt.Errorf("cerebras API error: %s", result.Error.Message)
	}

	if len(result.Choices) == 0 {
		return "", fmt.Errorf("cerebras API returned no choices")
	}

	return strings.TrimSpace(result.Choices[0].Message.Content), nil
}

func (c *CerebrasClient) Classify(ctx context.Context, content string) (domain.MemoryType, error) {
	messages := []cerebrasMessage{
		{Role: "user", Content: fmt.Sprintf(classifyPrompt, content)},
	}

	result, err := c.complete(ctx, messages, 0)
	if err != nil {
		return "", fmt.Errorf("classify: %w", err)
	}

	memType := domain.MemoryType(strings.ToLower(strings.TrimSpace(result)))
	if !domain.ValidMemoryType(string(memType)) {
		return domain.MemoryTypeFact, nil // fallback
	}
	return memType, nil
}

func (c *CerebrasClient) Extract(ctx context.Context, conversation []domain.Message) ([]domain.ExtractedMemory, error) {
	var sb strings.Builder
	for _, msg := range conversation {
		sb.WriteString(msg.Role)
		sb.WriteString(": ")
		sb.WriteString(msg.Content)
		sb.WriteString("\n")
	}

	messages := []cerebrasMessage{
		{Role: "user", Content: fmt.Sprintf(extractPrompt, sb.String())},
	}

	result, err := c.complete(ctx, messages, 0.2)
	if err != nil {
		return nil, fmt.Errorf("extract: %w", err)
	}

	// Strip markdown fences if present
	result = strings.TrimPrefix(result, "```json")
	result = strings.TrimPrefix(result, "```")
	result = strings.TrimSuffix(result, "```")
	result = strings.TrimSpace(result)

	var extracted []domain.ExtractedMemory
	if err := json.Unmarshal([]byte(result), &extracted); err != nil {
		return nil, fmt.Errorf("parse extraction result: %w (raw: %s)", err, result)
	}

	return extracted, nil
}

func (c *CerebrasClient) Summarize(ctx context.Context, memories []domain.Memory) (string, error) {
	var sb strings.Builder
	for i, m := range memories {
		sb.WriteString(fmt.Sprintf("%d. [%s] %s\n", i+1, m.Type, m.Content))
	}

	messages := []cerebrasMessage{
		{Role: "user", Content: fmt.Sprintf(summarizePrompt, sb.String())},
	}

	result, err := c.complete(ctx, messages, 0.3)
	if err != nil {
		return "", fmt.Errorf("summarize: %w", err)
	}

	return result, nil
}

func (c *CerebrasClient) CheckContradiction(ctx context.Context, stmtA, stmtB string) (bool, error) {
	messages := []cerebrasMessage{
		{Role: "user", Content: fmt.Sprintf(contradictionPrompt, stmtA, stmtB)},
	}

	result, err := c.complete(ctx, messages, 0)
	if err != nil {
		return false, fmt.Errorf("check contradiction: %w", err)
	}

	return strings.ToLower(strings.TrimSpace(result)) == "true", nil
}

func (c *CerebrasClient) ExtractEpisodeStructure(ctx context.Context, content string) (*domain.EpisodeExtraction, error) {
	messages := []cerebrasMessage{
		{Role: "user", Content: fmt.Sprintf(episodeExtractionPrompt, content)},
	}

	result, err := c.complete(ctx, messages, 0.2)
	if err != nil {
		return nil, fmt.Errorf("extract episode structure: %w", err)
	}

	// Strip markdown fences if present
	result = strings.TrimPrefix(result, "```json")
	result = strings.TrimPrefix(result, "```")
	result = strings.TrimSuffix(result, "```")
	result = strings.TrimSpace(result)

	var extraction domain.EpisodeExtraction
	if err := json.Unmarshal([]byte(result), &extraction); err != nil {
		return nil, fmt.Errorf("parse episode extraction result: %w (raw: %s)", err, result)
	}

	return &extraction, nil
}

func (c *CerebrasClient) ExtractProcedure(ctx context.Context, content string) (*domain.ProcedureExtraction, error) {
	messages := []cerebrasMessage{
		{Role: "user", Content: fmt.Sprintf(procedureExtractionPrompt, content)},
	}

	result, err := c.complete(ctx, messages, 0.2)
	if err != nil {
		return nil, fmt.Errorf("extract procedure: %w", err)
	}

	// Strip markdown fences if present
	result = strings.TrimPrefix(result, "```json")
	result = strings.TrimPrefix(result, "```")
	result = strings.TrimSuffix(result, "```")
	result = strings.TrimSpace(result)

	var extraction domain.ProcedureExtraction
	if err := json.Unmarshal([]byte(result), &extraction); err != nil {
		return nil, fmt.Errorf("parse procedure extraction result: %w (raw: %s)", err, result)
	}

	// Validate action type
	if !extraction.ActionType.IsValid() {
		extraction.ActionType = domain.ActionTypeResponseStyle // default
	}

	return &extraction, nil
}

func (c *CerebrasClient) DetectSchemaPattern(ctx context.Context, memories []domain.Memory) (*domain.SchemaExtraction, error) {
	if len(memories) == 0 {
		return nil, nil
	}

	var sb strings.Builder
	for i, m := range memories {
		sb.WriteString(fmt.Sprintf("%d. [%s] %s\n", i+1, m.Type, m.Content))
	}

	messages := []cerebrasMessage{
		{Role: "user", Content: fmt.Sprintf(schemaPatternPrompt, sb.String())},
	}

	result, err := c.complete(ctx, messages, 0.3)
	if err != nil {
		return nil, fmt.Errorf("detect schema pattern: %w", err)
	}

	// Strip markdown fences if present
	result = strings.TrimPrefix(result, "```json")
	result = strings.TrimPrefix(result, "```")
	result = strings.TrimSuffix(result, "```")
	result = strings.TrimSpace(result)

	// Check for null response
	if result == "null" || result == "" {
		return nil, nil
	}

	var extraction domain.SchemaExtraction
	if err := json.Unmarshal([]byte(result), &extraction); err != nil {
		return nil, fmt.Errorf("parse schema extraction result: %w (raw: %s)", err, result)
	}

	// Validate schema type
	if !extraction.SchemaType.IsValid() {
		return nil, nil
	}

	return &extraction, nil
}
