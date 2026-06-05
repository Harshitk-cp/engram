// Package contradiction provides ContradictionDetector implementations.
//
// LLMDetector   — delegates to an LLM API (highest accuracy, external calls).
// EmbeddingDetector — cosine similarity + text heuristics (zero external calls, <1ms).
// HybridDetector — embedding-first, LLM escalation only on ambiguous cases (~70% fewer LLM calls).
//
// Selection is controlled by LLM_PROVIDER config:
//   - "none"      → EmbeddingDetector
//   - any provider → LLMDetector wrapping that provider
//   - hybrid mode  → set via NewHybridDetector explicitly in router
package contradiction

import (
	"context"

	"github.com/Harshitk-cp/engram/internal/domain"
)

// Detector classifies the relationship between two belief statements.
type Detector interface {
	CheckTension(
		ctx context.Context,
		existingText, incomingText string,
		existingEmb, incomingEmb []float32,
	) (*domain.TensionResult, error)
}
