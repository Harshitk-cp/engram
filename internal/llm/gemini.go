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
	geminiBaseURL = "https://generativelanguage.googleapis.com/v1beta/models/gemini-2.0-flash:generateContent"
)

type GeminiClient struct {
	apiKey     string
	httpClient *http.Client
}

func NewGeminiClient(apiKey string) *GeminiClient {
	return &GeminiClient{
		apiKey:     apiKey,
		httpClient: &http.Client{},
	}
}

type geminiPart struct {
	Text string `json:"text"`
}

type geminiContent struct {
	Parts []geminiPart `json:"parts"`
	Role  string       `json:"role,omitempty"`
}

type geminiRequest struct {
	Contents []geminiContent `json:"contents"`
}

type geminiResponse struct {
	Candidates []struct {
		Content struct {
			Parts []struct {
				Text string `json:"text"`
			} `json:"parts"`
		} `json:"content"`
	} `json:"candidates"`
	Error *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Status  string `json:"status"`
	} `json:"error,omitempty"`
}

func (c *GeminiClient) complete(ctx context.Context, prompt string) (string, error) {
	body, err := json.Marshal(geminiRequest{
		Contents: []geminiContent{
			{
				Parts: []geminiPart{{Text: prompt}},
				Role:  "user",
			},
		},
	})
	if err != nil {
		return "", fmt.Errorf("marshal gemini request: %w", err)
	}

	url := fmt.Sprintf("%s?key=%s", geminiBaseURL, c.apiKey)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("create gemini request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("gemini request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read gemini response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("gemini API returned status %d: %s", resp.StatusCode, string(respBody))
	}

	var result geminiResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", fmt.Errorf("unmarshal gemini response: %w", err)
	}

	if result.Error != nil {
		return "", fmt.Errorf("gemini API error: %s", result.Error.Message)
	}

	if len(result.Candidates) == 0 || len(result.Candidates[0].Content.Parts) == 0 {
		return "", fmt.Errorf("gemini API returned no content")
	}

	return strings.TrimSpace(result.Candidates[0].Content.Parts[0].Text), nil
}

func (c *GeminiClient) Classify(ctx context.Context, content string) (domain.MemoryType, error) {
	extracted, err := c.Extract(ctx, []domain.Message{{Role: "user", Content: content}})
	if err != nil {
		return domain.MemoryTypeFact, nil
	}
	if len(extracted) > 0 && domain.ValidMemoryType(string(extracted[0].Type)) {
		return extracted[0].Type, nil
	}
	return domain.MemoryTypeFact, nil
}

func (c *GeminiClient) Extract(ctx context.Context, conversation []domain.Message) ([]domain.ExtractedMemory, error) {
	var sb strings.Builder
	for _, msg := range conversation {
		sb.WriteString(msg.Role)
		sb.WriteString(": ")
		sb.WriteString(msg.Content)
		sb.WriteString("\n")
	}

	prompt := fmt.Sprintf(extractPrompt, sb.String())

	result, err := c.complete(ctx, prompt)
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

	for i := range extracted {
		if extracted[i].EvidenceType != "" {
			extracted[i].Confidence = extracted[i].EvidenceType.InitialConfidence()
		} else if extracted[i].Confidence == 0 {
			extracted[i].Confidence = domain.EvidenceImplicit.InitialConfidence()
		}
	}

	return extracted, nil
}

func (c *GeminiClient) Summarize(ctx context.Context, memories []domain.Memory) (string, error) {
	var sb strings.Builder
	for i, m := range memories {
		sb.WriteString(fmt.Sprintf("%d. [%s][%s] %s\n", i+1, provenanceTag(m.Provenance), m.Type, m.Content))
	}

	prompt := fmt.Sprintf(summarizePrompt, sb.String())

	result, err := c.complete(ctx, prompt)
	if err != nil {
		return "", fmt.Errorf("summarize: %w", err)
	}

	return result, nil
}

func (c *GeminiClient) CheckContradiction(ctx context.Context, stmtA, stmtB string) (bool, error) {
	prompt := fmt.Sprintf(contradictionPrompt, stmtA, stmtB)

	result, err := c.complete(ctx, prompt)
	if err != nil {
		return false, fmt.Errorf("check contradiction: %w", err)
	}

	return strings.ToLower(strings.TrimSpace(result)) == "true", nil
}

func (c *GeminiClient) CheckTension(ctx context.Context, stmtA, stmtB string) (*domain.TensionResult, error) {
	prompt := fmt.Sprintf(tensionPrompt, stmtA, stmtB)

	result, err := c.complete(ctx, prompt)
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

func (c *GeminiClient) ExtractEpisodeStructure(ctx context.Context, content string) (*domain.EpisodeExtraction, error) {
	prompt := fmt.Sprintf(episodeExtractionPrompt, content)

	result, err := c.complete(ctx, prompt)
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

func (c *GeminiClient) ExtractProcedure(ctx context.Context, content string) (*domain.ProcedureExtraction, error) {
	prompt := fmt.Sprintf(procedureExtractionPrompt, content)

	result, err := c.complete(ctx, prompt)
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

func (c *GeminiClient) DetectSchemaPattern(ctx context.Context, memories []domain.Memory) (*domain.SchemaExtraction, error) {
	if len(memories) == 0 {
		return nil, nil
	}

	var sb strings.Builder
	for i, m := range memories {
		sb.WriteString(fmt.Sprintf("%d. [%s][%s] %s\n", i+1, provenanceTag(m.Provenance), m.Type, m.Content))
	}

	prompt := fmt.Sprintf(schemaPatternPrompt, sb.String())

	result, err := c.complete(ctx, prompt)
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

func (c *GeminiClient) DetectImplicitFeedback(ctx context.Context, memories []domain.Memory, conversation []domain.Message) ([]domain.ImplicitFeedback, error) {
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

	prompt := fmt.Sprintf(implicitFeedbackPrompt, memSb.String(), convSb.String())

	result, err := c.complete(ctx, prompt)
	if err != nil {
		return nil, fmt.Errorf("detect implicit feedback: %w", err)
	}

	result = strings.TrimPrefix(result, "```json")
	result = strings.TrimPrefix(result, "```")
	result = strings.TrimSuffix(result, "```")
	result = strings.TrimSpace(result)

	var responses []struct {
		MemoryID   string  `json:"memory_id"`
		SignalType string  `json:"signal_type"`
		Confidence float32 `json:"confidence"`
		Evidence   string  `json:"evidence"`
	}
	if err := json.Unmarshal([]byte(result), &responses); err != nil {
		return nil, fmt.Errorf("parse implicit feedback result: %w (raw: %s)", err, result)
	}

	var feedbacks []domain.ImplicitFeedback
	for _, r := range responses {
		memID, err := parseUUID(r.MemoryID)
		if err != nil {
			continue
		}
		signalType := domain.FeedbackType(r.SignalType)
		if !domain.ValidFeedbackType(string(signalType)) {
			continue
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

func (c *GeminiClient) ExtractEntities(ctx context.Context, content string) ([]domain.ExtractedEntity, error) {
	prompt := fmt.Sprintf(entityExtractionPrompt, content)

	result, err := c.complete(ctx, prompt)
	if err != nil {
		return nil, fmt.Errorf("extract entities: %w", err)
	}

	result = strings.TrimPrefix(result, "```json")
	result = strings.TrimPrefix(result, "```")
	result = strings.TrimSuffix(result, "```")
	result = strings.TrimSpace(result)

	var responses []struct {
		Name       string `json:"name"`
		EntityType string `json:"entity_type"`
		Role       string `json:"role"`
	}
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

func (c *GeminiClient) DetectRelationships(ctx context.Context, memory *domain.Memory, similarMemories []domain.MemoryWithScore) ([]domain.DetectedRelationship, error) {
	if memory == nil || len(similarMemories) == 0 {
		return nil, nil
	}

	var similarSb strings.Builder
	for _, m := range similarMemories {
		similarSb.WriteString(fmt.Sprintf("- ID: %s\n  Content: %s\n  Similarity: %.2f\n", m.ID.String(), m.Content, m.Score))
	}

	prompt := fmt.Sprintf(relationshipDetectionPrompt, memory.Content, memory.ID.String(), similarSb.String())

	result, err := c.complete(ctx, prompt)
	if err != nil {
		return nil, fmt.Errorf("detect relationships: %w", err)
	}

	result = strings.TrimPrefix(result, "```json")
	result = strings.TrimPrefix(result, "```")
	result = strings.TrimSuffix(result, "```")
	result = strings.TrimSpace(result)

	var responses []struct {
		TargetID     string  `json:"target_id"`
		RelationType string  `json:"relation_type"`
		Strength     float32 `json:"strength"`
		Reason       string  `json:"reason"`
	}
	if err := json.Unmarshal([]byte(result), &responses); err != nil {
		return nil, fmt.Errorf("parse relationship detection result: %w (raw: %s)", err, result)
	}

	var relationships []domain.DetectedRelationship
	for _, r := range responses {
		targetID, err := parseUUID(r.TargetID)
		if err != nil {
			continue
		}
		relType := domain.RelationType(r.RelationType)
		if !domain.ValidRelationType(r.RelationType) {
			continue
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
