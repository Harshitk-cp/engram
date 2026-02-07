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
	openAIChatURL = "https://api.openai.com/v1/chat/completions"
	chatModel     = "gpt-4o-mini"
)

type OpenAIClient struct {
	apiKey     string
	httpClient *http.Client
}

func NewOpenAIClient(apiKey string) *OpenAIClient {
	return &OpenAIClient{
		apiKey:     apiKey,
		httpClient: &http.Client{},
	}
}

// chat types for OpenAI API
type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatRequest struct {
	Model       string        `json:"model"`
	Messages    []chatMessage `json:"messages"`
	Temperature float32       `json:"temperature"`
}

type chatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

func (c *OpenAIClient) complete(ctx context.Context, messages []chatMessage, temp float32) (string, error) {
	body, err := json.Marshal(chatRequest{
		Model:       chatModel,
		Messages:    messages,
		Temperature: temp,
	})
	if err != nil {
		return "", fmt.Errorf("marshal chat request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, openAIChatURL, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("create chat request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("chat request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read chat response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("chat API returned status %d: %s", resp.StatusCode, string(respBody))
	}

	var result chatResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", fmt.Errorf("unmarshal chat response: %w", err)
	}

	if result.Error != nil {
		return "", fmt.Errorf("chat API error: %s", result.Error.Message)
	}

	if len(result.Choices) == 0 {
		return "", fmt.Errorf("chat API returned no choices")
	}

	return strings.TrimSpace(result.Choices[0].Message.Content), nil
}

func (c *OpenAIClient) Classify(ctx context.Context, content string) (domain.MemoryType, error) {
	// Classify by running a single-message extraction and taking the first result type
	extracted, err := c.Extract(ctx, []domain.Message{{Role: "user", Content: content}})
	if err != nil {
		return domain.MemoryTypeFact, nil
	}
	if len(extracted) > 0 && domain.ValidMemoryType(string(extracted[0].Type)) {
		return extracted[0].Type, nil
	}
	return domain.MemoryTypeFact, nil
}

func (c *OpenAIClient) Extract(ctx context.Context, conversation []domain.Message) ([]domain.ExtractedMemory, error) {
	var sb strings.Builder
	for _, msg := range conversation {
		sb.WriteString(msg.Role)
		sb.WriteString(": ")
		sb.WriteString(msg.Content)
		sb.WriteString("\n")
	}

	messages := []chatMessage{
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

	// Compute confidence from evidence_type
	for i := range extracted {
		if extracted[i].EvidenceType != "" {
			extracted[i].Confidence = extracted[i].EvidenceType.InitialConfidence()
		} else if extracted[i].Confidence == 0 {
			extracted[i].Confidence = domain.EvidenceImplicit.InitialConfidence()
		}
	}

	return extracted, nil
}

func (c *OpenAIClient) Summarize(ctx context.Context, memories []domain.Memory) (string, error) {
	var sb strings.Builder
	for i, m := range memories {
		sb.WriteString(fmt.Sprintf("%d. [%s][%s] %s\n", i+1, provenanceTag(m.Provenance), m.Type, m.Content))
	}

	messages := []chatMessage{
		{Role: "user", Content: fmt.Sprintf(summarizePrompt, sb.String())},
	}

	result, err := c.complete(ctx, messages, 0.3)
	if err != nil {
		return "", fmt.Errorf("summarize: %w", err)
	}

	return result, nil
}

func (c *OpenAIClient) CheckContradiction(ctx context.Context, stmtA, stmtB string) (bool, error) {
	messages := []chatMessage{
		{Role: "user", Content: fmt.Sprintf(contradictionPrompt, stmtA, stmtB)},
	}

	result, err := c.complete(ctx, messages, 0)
	if err != nil {
		return false, fmt.Errorf("check contradiction: %w", err)
	}

	return strings.ToLower(strings.TrimSpace(result)) == "true", nil
}

func (c *OpenAIClient) CheckTension(ctx context.Context, stmtA, stmtB string) (*domain.TensionResult, error) {
	messages := []chatMessage{
		{Role: "user", Content: fmt.Sprintf(tensionPrompt, stmtA, stmtB)},
	}

	result, err := c.complete(ctx, messages, 0.2)
	if err != nil {
		return nil, fmt.Errorf("check tension: %w", err)
	}

	result = strings.TrimPrefix(result, "```json")
	result = strings.TrimPrefix(result, "```")
	result = strings.TrimSuffix(result, "```")
	result = strings.TrimSpace(result)

	var tension domain.TensionResult
	if err := json.Unmarshal([]byte(result), &tension); err != nil {
		return nil, fmt.Errorf("parse tension result: %w (raw: %s)", err, result)
	}

	if !domain.ValidContradictionType(string(tension.Type)) {
		tension.Type = domain.ContradictionNone
	}

	return &tension, nil
}

func (c *OpenAIClient) ExtractEpisodeStructure(ctx context.Context, content string) (*domain.EpisodeExtraction, error) {
	messages := []chatMessage{
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

func (c *OpenAIClient) ExtractProcedure(ctx context.Context, content string) (*domain.ProcedureExtraction, error) {
	messages := []chatMessage{
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

func (c *OpenAIClient) DetectSchemaPattern(ctx context.Context, memories []domain.Memory) (*domain.SchemaExtraction, error) {
	if len(memories) == 0 {
		return nil, nil
	}

	var sb strings.Builder
	for i, m := range memories {
		sb.WriteString(fmt.Sprintf("%d. [%s][%s] %s\n", i+1, provenanceTag(m.Provenance), m.Type, m.Content))
	}

	messages := []chatMessage{
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

type implicitFeedbackResponse struct {
	MemoryID   string  `json:"memory_id"`
	SignalType string  `json:"signal_type"`
	Confidence float32 `json:"confidence"`
	Evidence   string  `json:"evidence"`
}

type extractedEntityResponse struct {
	Name       string `json:"name"`
	EntityType string `json:"entity_type"`
	Role       string `json:"role"`
}

type detectedRelationshipResponse struct {
	TargetID     string  `json:"target_id"`
	RelationType string  `json:"relation_type"`
	Strength     float32 `json:"strength"`
	Reason       string  `json:"reason"`
}

func (c *OpenAIClient) DetectImplicitFeedback(ctx context.Context, memories []domain.Memory, conversation []domain.Message) ([]domain.ImplicitFeedback, error) {
	if len(memories) == 0 || len(conversation) == 0 {
		return nil, nil
	}

	var memSb strings.Builder
	for _, m := range memories {
		memSb.WriteString(fmt.Sprintf("- ID: %s\n  Content: %s\n", m.ID.String(), m.Content))
	}

	var convSb strings.Builder
	for _, msg := range conversation {
		convSb.WriteString(msg.Role)
		convSb.WriteString(": ")
		convSb.WriteString(msg.Content)
		convSb.WriteString("\n")
	}

	messages := []chatMessage{
		{Role: "user", Content: fmt.Sprintf(implicitFeedbackPrompt, memSb.String(), convSb.String())},
	}

	result, err := c.complete(ctx, messages, 0.3)
	if err != nil {
		return nil, fmt.Errorf("detect implicit feedback: %w", err)
	}

	// Strip markdown fences if present
	result = strings.TrimPrefix(result, "```json")
	result = strings.TrimPrefix(result, "```")
	result = strings.TrimSuffix(result, "```")
	result = strings.TrimSpace(result)

	var responses []implicitFeedbackResponse
	if err := json.Unmarshal([]byte(result), &responses); err != nil {
		return nil, fmt.Errorf("parse implicit feedback result: %w (raw: %s)", err, result)
	}

	var feedbacks []domain.ImplicitFeedback
	for _, r := range responses {
		memID, err := parseUUID(r.MemoryID)
		if err != nil {
			continue // Skip invalid UUIDs
		}
		signalType := domain.FeedbackType(r.SignalType)
		if !domain.ValidFeedbackType(string(signalType)) {
			continue // Skip invalid signal types
		}
		feedbacks = append(feedbacks, domain.ImplicitFeedback{
			MemoryID:   memID,
			SignalType: signalType,
			Confidence: r.Confidence,
			Evidence:   r.Evidence,
		})
	}

	return feedbacks, nil
}

func (c *OpenAIClient) ExtractEntities(ctx context.Context, content string) ([]domain.ExtractedEntity, error) {
	messages := []chatMessage{
		{Role: "user", Content: fmt.Sprintf(entityExtractionPrompt, content)},
	}

	result, err := c.complete(ctx, messages, 0.2)
	if err != nil {
		return nil, fmt.Errorf("extract entities: %w", err)
	}

	// Strip markdown fences if present
	result = strings.TrimPrefix(result, "```json")
	result = strings.TrimPrefix(result, "```")
	result = strings.TrimSuffix(result, "```")
	result = strings.TrimSpace(result)

	var responses []extractedEntityResponse
	if err := json.Unmarshal([]byte(result), &responses); err != nil {
		return nil, fmt.Errorf("parse entity extraction result: %w (raw: %s)", err, result)
	}

	var entities []domain.ExtractedEntity
	for _, r := range responses {
		entityType := domain.EntityType(r.EntityType)
		if !domain.ValidEntityType(r.EntityType) {
			entityType = domain.EntityOther
		}
		role := domain.MentionType(r.Role)
		if r.Role != string(domain.MentionSubject) && r.Role != string(domain.MentionObject) && r.Role != string(domain.MentionContext) {
			role = domain.MentionContext
		}
		entities = append(entities, domain.ExtractedEntity{
			Name:       r.Name,
			EntityType: entityType,
			Role:       role,
		})
	}

	return entities, nil
}

func (c *OpenAIClient) DetectRelationships(ctx context.Context, memory *domain.Memory, similarMemories []domain.MemoryWithScore) ([]domain.DetectedRelationship, error) {
	if memory == nil || len(similarMemories) == 0 {
		return nil, nil
	}

	var similarSb strings.Builder
	for _, m := range similarMemories {
		similarSb.WriteString(fmt.Sprintf("- ID: %s\n  Content: %s\n  Similarity: %.2f\n", m.ID.String(), m.Content, m.Score))
	}

	messages := []chatMessage{
		{Role: "user", Content: fmt.Sprintf(relationshipDetectionPrompt, memory.Content, memory.ID.String(), similarSb.String())},
	}

	result, err := c.complete(ctx, messages, 0.3)
	if err != nil {
		return nil, fmt.Errorf("detect relationships: %w", err)
	}

	// Strip markdown fences if present
	result = strings.TrimPrefix(result, "```json")
	result = strings.TrimPrefix(result, "```")
	result = strings.TrimSuffix(result, "```")
	result = strings.TrimSpace(result)

	var responses []detectedRelationshipResponse
	if err := json.Unmarshal([]byte(result), &responses); err != nil {
		return nil, fmt.Errorf("parse relationship detection result: %w (raw: %s)", err, result)
	}

	var relationships []domain.DetectedRelationship
	for _, r := range responses {
		targetID, err := parseUUID(r.TargetID)
		if err != nil {
			continue // Skip invalid UUIDs
		}
		relType := domain.RelationType(r.RelationType)
		if !domain.ValidRelationType(r.RelationType) {
			continue // Skip invalid relation types
		}
		relationships = append(relationships, domain.DetectedRelationship{
			SourceID:     memory.ID,
			TargetID:     targetID,
			RelationType: relType,
			Strength:     r.Strength,
			Reason:       r.Reason,
		})
	}

	return relationships, nil
}
