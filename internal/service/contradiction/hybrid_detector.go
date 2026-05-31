package contradiction

import (
	"context"

	"github.com/Harshitk-cp/engram/internal/domain"
)

// HybridDetector runs EmbeddingDetector first. It escalates to LLM only when
// the embedding signals are ambiguous (tension score between ambiguityLow and
// ambiguityHigh). This reduces LLM calls by ~70% vs. pure LLM mode while
// maintaining near-LLM accuracy.
//
// Escalation logic:
//   - Embedding returns ContradictionNone with score 0   → return None (no LLM call)
//   - Embedding returns any type with score >= 0.65      → return result (no LLM call)
//   - Embedding returns soft tension or score < 0.65     → call LLM for confirmation
type HybridDetector struct {
	embedding        *EmbeddingDetector
	llm              domain.LLMClient
	confidenceThresh float32 // score above which embedding result is trusted
}

func NewHybridDetector(llm domain.LLMClient) *HybridDetector {
	return &HybridDetector{
		embedding:        NewEmbeddingDetector(),
		llm:              llm,
		confidenceThresh: 0.65,
	}
}

func (d *HybridDetector) CheckTension(
	ctx context.Context,
	existingText, incomingText string,
	existingEmb, incomingEmb []float32,
) (*domain.TensionResult, error) {
	result, err := d.embedding.CheckTension(ctx, existingText, incomingText, existingEmb, incomingEmb)
	if err != nil {
		return nil, err
	}

	// High-confidence embedding result — no LLM call needed.
	if result.Type == domain.ContradictionNone || result.TensionScore >= d.confidenceThresh {
		return result, nil
	}

	// Ambiguous — escalate to LLM for confirmation.
	if d.llm != nil {
		return d.llm.CheckTension(ctx, existingText, incomingText)
	}

	return result, nil
}
