package service

import (
	"context"

	"github.com/Harshitk-cp/engram/internal/domain"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

type ImplicitFeedbackDetector struct {
	llmClient        domain.LLMClient
	feedbackStore    domain.FeedbackStore
	memoryStore      domain.MemoryStore
	mutationLogStore domain.MutationLogStore
	logger           *zap.Logger
}

func NewImplicitFeedbackDetector(
	llmClient domain.LLMClient,
	feedbackStore domain.FeedbackStore,
	memoryStore domain.MemoryStore,
	logger *zap.Logger,
) *ImplicitFeedbackDetector {
	return &ImplicitFeedbackDetector{
		llmClient:     llmClient,
		feedbackStore: feedbackStore,
		memoryStore:   memoryStore,
		logger:        logger,
	}
}

func (d *ImplicitFeedbackDetector) SetMutationLogStore(mls domain.MutationLogStore) {
	d.mutationLogStore = mls
}

// DetectRequest holds the input for implicit feedback detection.
type DetectRequest struct {
	AgentID      uuid.UUID
	TenantID     uuid.UUID
	Memories     []domain.Memory
	Conversation []domain.Message
}

// DetectAndApply analyzes a conversation for implicit feedback signals and applies them.
// Returns the detected implicit feedbacks.
func (d *ImplicitFeedbackDetector) DetectAndApply(ctx context.Context, req DetectRequest) ([]domain.ImplicitFeedback, error) {
	if len(req.Memories) == 0 || len(req.Conversation) == 0 {
		return nil, nil
	}

	// Use LLM to detect implicit feedback
	feedbacks, err := d.llmClient.DetectImplicitFeedback(ctx, req.Memories, req.Conversation)
	if err != nil {
		return nil, err
	}

	if len(feedbacks) == 0 {
		return nil, nil
	}

	// Apply each detected feedback
	for _, fb := range feedbacks {
		// Only apply high-confidence detections
		if fb.Confidence < 0.6 {
			d.logger.Debug("skipping low-confidence implicit feedback",
				zap.String("memory_id", fb.MemoryID.String()),
				zap.String("signal_type", string(fb.SignalType)),
				zap.Float32("confidence", fb.Confidence),
			)
			continue
		}

		// Get current memory state
		memory, err := d.memoryStore.GetByID(ctx, fb.MemoryID, req.TenantID)
		if err != nil {
			d.logger.Warn("failed to get memory for implicit feedback",
				zap.String("memory_id", fb.MemoryID.String()),
				zap.Error(err),
			)
			continue
		}

		// Apply effect
		effect, hasEffect := domain.FeedbackEffects[fb.SignalType]
		if !hasEffect {
			continue
		}

		oldConfidence := memory.Confidence
		oldReinforcement := memory.ReinforcementCount

		newConfidence := ApplyLogOddsDelta(memory.Confidence, effect.LogOddsDelta)

		newReinforcement := memory.ReinforcementCount + effect.ReinforcementDelta
		if newReinforcement < 0 {
			newReinforcement = 0
		}

		// Update memory
		if err := d.memoryStore.UpdateReinforcement(ctx, memory.ID, newConfidence, newReinforcement); err != nil {
			d.logger.Warn("failed to update memory on implicit feedback", zap.Error(err))
			continue
		}

		// Set review flag if needed
		if effect.TriggerReview {
			if err := d.memoryStore.SetNeedsReview(ctx, memory.ID, true); err != nil {
				d.logger.Warn("failed to set needs_review flag", zap.Error(err))
			}
		}

		// Store the feedback record
		feedback := &domain.Feedback{
			MemoryID:   fb.MemoryID,
			AgentID:    req.AgentID,
			SignalType: fb.SignalType,
			Context: map[string]any{
				"implicit":   true,
				"confidence": fb.Confidence,
				"evidence":   fb.Evidence,
			},
		}
		if err := d.feedbackStore.Create(ctx, feedback); err != nil {
			d.logger.Warn("failed to store implicit feedback", zap.Error(err))
		}

		// Log mutation
		if d.mutationLogStore != nil {
			mutation := &domain.MutationLog{
				MemoryID:              memory.ID,
				AgentID:               req.AgentID,
				MutationType:          domain.MutationFeedback,
				SourceType:            domain.MutationSourceImplicit,
				SourceID:              &feedback.ID,
				OldConfidence:         &oldConfidence,
				NewConfidence:         &newConfidence,
				OldReinforcementCount: &oldReinforcement,
				NewReinforcementCount: &newReinforcement,
				Reason:                "implicit feedback: " + string(fb.SignalType) + " (" + fb.Evidence + ")",
			}
			if err := d.mutationLogStore.Create(ctx, mutation); err != nil {
				d.logger.Warn("failed to log mutation", zap.Error(err))
			}
		}

		d.logger.Debug("applied implicit feedback effect",
			zap.String("memory_id", memory.ID.String()),
			zap.String("signal_type", string(fb.SignalType)),
			zap.Float32("old_confidence", oldConfidence),
			zap.Float32("new_confidence", newConfidence),
			zap.String("evidence", fb.Evidence),
		)
	}

	return feedbacks, nil
}
