package contradiction

import (
	"context"

	"github.com/Harshitk-cp/engram/internal/domain"
)

// LLMDetector delegates contradiction checking to an LLM client.
// This is the existing behavior, preserved unchanged behind the Detector interface.
type LLMDetector struct {
	client domain.LLMClient
}

func NewLLMDetector(client domain.LLMClient) *LLMDetector {
	return &LLMDetector{client: client}
}

func (d *LLMDetector) CheckTension(
	ctx context.Context,
	existingText, incomingText string,
	_, _ []float32,
) (*domain.TensionResult, error) {
	return d.client.CheckTension(ctx, existingText, incomingText)
}
