package service

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/Harshitk-cp/engram/internal/domain"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

var enumerationLineRE = regexp.MustCompile(`(?m)^\s*\d+[.)]\s`)

func isEnumeration(content string) bool {
	return len(enumerationLineRE.FindAllString(content, -1)) >= 4
}

type ConversationService struct {
	memorySvc *MemoryService
	llm       domain.LLMClient
	logger    *zap.Logger
}

func NewConversationService(memorySvc *MemoryService, llm domain.LLMClient, logger *zap.Logger) *ConversationService {
	return &ConversationService{memorySvc: memorySvc, llm: llm, logger: logger}
}

func (s *ConversationService) Ingest(ctx context.Context, req *domain.ConversationIngestRequest) (*domain.IngestResult, error) {
	if len(req.Messages) == 0 {
		return &domain.IngestResult{}, nil
	}
	start := time.Now()

	// Always store user turns first — fast, works even with no LLM.
	userMem, err := s.storeUserTurnsFallback(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("store user turns: %w", err)
	}
	result := &domain.IngestResult{Duration: time.Since(start).Milliseconds()}
	if userMem != nil {
		result.Stored = append(result.Stored, userMem)
	}

	result.Stored = append(result.Stored, s.storeEnumerationTurns(ctx, req)...)

	if s.llm == nil || userMem == nil {
		return result, nil
	}

	if req.Sync {
		// Synchronous path: block until LLM extraction is complete.
		stored, err := s.runExtraction(ctx, req, userMem.ID)
		if err != nil {
			s.logger.Warn("sync extraction failed, user-turns memory retained",
				zap.String("agent_id", req.AgentID.String()),
				zap.Error(err),
			)
		} else {
			result.Stored = result.Stored[:0] // replace with extracted facts
			result.Stored = append(result.Stored, stored...)
		}
	} else {
		// Async path: extraction runs in background, returns immediately.
		agentID := req.AgentID
		tenantID := req.TenantID
		messages := append([]domain.Message(nil), req.Messages...)
		eventDate := req.EventDate
		metadata := make(map[string]any, len(req.Metadata))
		for k, v := range req.Metadata {
			metadata[k] = v
		}
		userMemID := userMem.ID
		anchorID := req.AnchorID
		sessionID := req.SessionID

		go func() {
			bgCtx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
			defer cancel()
			asyncReq := &domain.ConversationIngestRequest{
				AgentID:   agentID,
				TenantID:  tenantID,
				Messages:  messages,
				EventDate: eventDate,
				Metadata:  metadata,
				AnchorID:  anchorID,
				SessionID: sessionID,
			}
			if _, err := s.runExtraction(bgCtx, asyncReq, userMemID); err != nil {
				s.logger.Warn("async extraction failed",
					zap.String("agent_id", agentID.String()),
					zap.Error(err),
				)
			}
		}()
	}

	result.Duration = time.Since(start).Milliseconds()
	return result, nil
}

func (s *ConversationService) runExtraction(ctx context.Context, req *domain.ConversationIngestRequest, userMemID uuid.UUID) ([]*domain.Memory, error) {
	facts, err := s.llm.IngestConversation(ctx, req.Messages)
	if err != nil {
		return nil, fmt.Errorf("LLM extraction: %w", err)
	}

	var stored []*domain.Memory
	for _, fact := range facts {
		if strings.TrimSpace(fact.Content) == "" {
			continue
		}
		m, err := s.storeExtractedFact(ctx, req, fact)
		if err != nil {
			s.logger.Warn("failed to store extracted fact",
				zap.String("content", fact.Content[:min(50, len(fact.Content))]),
				zap.Error(err),
			)
			continue
		}
		stored = append(stored, m)
	}

	if len(stored) > 0 {
		if err := s.memorySvc.Delete(ctx, userMemID, req.TenantID); err != nil {
			s.logger.Debug("could not archive user-turns memory",
				zap.String("memory_id", userMemID.String()),
				zap.Error(err),
			)
		}
	}

	s.logger.Debug("extraction complete",
		zap.String("agent_id", req.AgentID.String()),
		zap.Int("facts", len(stored)),
	)
	return stored, nil
}

func (s *ConversationService) storeExtractedFact(ctx context.Context, req *domain.ConversationIngestRequest, fact domain.ExtractedConversationMemory) (*domain.Memory, error) {
	provenance := domain.ProvenanceUser
	if fact.Source == "assistant" {
		provenance = domain.ProvenanceAgent
	}

	memType := fact.Type
	if !domain.ValidMemoryType(string(memType)) {
		memType = domain.MemoryTypeFact
	}

	evidenceType := fact.EvidenceType
	if !domain.ValidEvidenceType(string(evidenceType)) {
		evidenceType = domain.EvidenceExplicit
	}

	confidence := fact.Confidence
	if confidence == 0 {
		confidence = evidenceType.InitialConfidence()
	}

	metadata := map[string]any{
		"source":        fact.Source,
		"ingest_source": "conversation",
		"evidence_type": string(evidenceType),
	}
	for k, v := range req.Metadata {
		metadata[k] = v
	}

	m := &domain.Memory{
		ID:         uuid.New(),
		AgentID:    req.AgentID,
		TenantID:   req.TenantID,
		Content:    fact.Content,
		Type:       memType,
		Provenance: provenance,
		Confidence: confidence,
		EventDate:  req.EventDate,
		Metadata:   metadata,
		AnchorID:   req.AnchorID,
		SessionID:  req.SessionID,
	}

	if _, err := s.memorySvc.Create(ctx, m); err != nil {
		return nil, err
	}
	return m, nil
}

func (s *ConversationService) storeUserTurnsFallback(ctx context.Context, req *domain.ConversationIngestRequest) (*domain.Memory, error) {
	var sb strings.Builder
	for _, msg := range req.Messages {
		if msg.Role != "user" {
			continue
		}
		if sb.Len() > 0 {
			sb.WriteString("\n")
		}
		sb.WriteString("User: ")
		sb.WriteString(msg.Content)
	}
	if sb.Len() == 0 {
		return nil, nil
	}

	metadata := map[string]any{"ingest_source": "conversation_fallback"}
	for k, v := range req.Metadata {
		metadata[k] = v
	}

	m := &domain.Memory{
		ID:         uuid.New(),
		AgentID:    req.AgentID,
		TenantID:   req.TenantID,
		Content:    sb.String(),
		Type:       domain.MemoryTypeFact,
		Provenance: domain.ProvenanceUser,
		Confidence: domain.EvidenceExplicit.InitialConfidence(),
		EventDate:  req.EventDate,
		Metadata:   metadata,
		AnchorID:   req.AnchorID,
		SessionID:  req.SessionID,
	}

	if _, err := s.memorySvc.Create(ctx, m); err != nil {
		return nil, fmt.Errorf("fallback store: %w", err)
	}
	return m, nil
}

func (s *ConversationService) storeEnumerationTurns(ctx context.Context, req *domain.ConversationIngestRequest) []*domain.Memory {
	var stored []*domain.Memory
	for _, msg := range req.Messages {
		if !isEnumeration(msg.Content) {
			continue
		}
		provenance := domain.ProvenanceUser
		if msg.Role == "assistant" {
			provenance = domain.ProvenanceAgent
		}
		metadata := map[string]any{
			"ingest_source": "conversation_list",
			"source":        msg.Role,
		}
		for k, v := range req.Metadata {
			metadata[k] = v
		}
		m := &domain.Memory{
			ID:         uuid.New(),
			AgentID:    req.AgentID,
			TenantID:   req.TenantID,
			Content:    msg.Content,
			Type:       domain.MemoryTypeFact,
			Provenance: provenance,
			Confidence: domain.EvidenceExplicit.InitialConfidence(),
			EventDate:  req.EventDate,
			Metadata:   metadata,
			AnchorID:   req.AnchorID,
			SessionID:  req.SessionID,
		}
		if _, err := s.memorySvc.Create(ctx, m); err != nil {
			s.logger.Warn("failed to store enumeration turn",
				zap.String("agent_id", req.AgentID.String()), zap.Error(err))
			continue
		}
		stored = append(stored, m)
	}
	return stored
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
