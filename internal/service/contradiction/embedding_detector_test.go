package contradiction

import (
	"context"
	"testing"

	"github.com/Harshitk-cp/engram/internal/domain"
)

// mockEmb returns a synthetic embedding biased toward the given dimension.
// Two embeddings with different bias dimensions will have low cosine similarity;
// same bias = high similarity. Used to place pairs in or out of the contradiction zone.
func mockEmb(dim, size int) []float32 {
	v := make([]float32, size)
	for i := range v {
		v[i] = 0.01
	}
	if dim < size {
		v[dim] = 1.0
	}
	return v
}

// contradictionZoneEmb returns two normalised unit vectors with cosine similarity ~0.71.
// They share a strong component on dim-0 but diverge on dims 1 and 2.
func contradictionZoneEmb() ([]float32, []float32) {
	// a = (1, 0, 0, ...) — unit vector along dim 0
	a := make([]float32, 16)
	a[0] = 1.0

	// b = (0.71, 0.71, 0, ...) — unit vector rotated 45° toward dim 1
	// cosine(a, b) = a·b / (|a||b|) = 0.71 / (1 × 1) ≈ 0.71 → in [0.60, 0.85]
	b := make([]float32, 16)
	b[0] = 0.7071
	b[1] = 0.7071
	return a, b
}

func TestEmbeddingDetector_Unrelated(t *testing.T) {
	d := NewEmbeddingDetector()
	// Very different embeddings → similarity well below 0.60 → no tension
	a := mockEmb(0, 16)
	b := mockEmb(15, 16)
	result, err := d.CheckTension(context.Background(), "User likes coffee", "User drives a Tesla", a, b)
	if err != nil {
		t.Fatal(err)
	}
	if result.Type != domain.ContradictionNone {
		t.Errorf("expected None, got %s", result.Type)
	}
}

func TestEmbeddingDetector_NearDuplicate(t *testing.T) {
	d := NewEmbeddingDetector()
	// Near-identical embeddings → similarity ≥ 0.85 → reinforce, not contradict
	a := mockEmb(0, 16)
	b := mockEmb(0, 16)
	result, err := d.CheckTension(context.Background(), "User prefers dark mode", "User always uses dark mode", a, b)
	if err != nil {
		t.Fatal(err)
	}
	if result.Type != domain.ContradictionNone {
		t.Errorf("expected None for near-duplicate, got %s", result.Type)
	}
}

func TestEmbeddingDetector_TemporalUpdate(t *testing.T) {
	d := NewEmbeddingDetector()
	a, b := contradictionZoneEmb()
	result, err := d.CheckTension(
		context.Background(),
		"User works at Google",
		"User recently moved to Anthropic",
		a, b,
	)
	if err != nil {
		t.Fatal(err)
	}
	if result.Type != domain.ContradictionTemporal {
		t.Errorf("expected Temporal, got %s (score=%.2f)", result.Type, result.TensionScore)
	}
}

func TestEmbeddingDetector_HardContradiction(t *testing.T) {
	d := NewEmbeddingDetector()
	a, b := contradictionZoneEmb()
	result, err := d.CheckTension(
		context.Background(),
		"User is a vegetarian",
		"User isn't vegetarian and eats meat regularly",
		a, b,
	)
	if err != nil {
		t.Fatal(err)
	}
	if result.Type != domain.ContradictionHard {
		t.Errorf("expected Hard, got %s (score=%.2f)", result.Type, result.TensionScore)
	}
}

func TestClassifyHeuristic_Preference(t *testing.T) {
	cases := []string{
		"I prefer dark mode",
		"User loves concise responses",
		"I enjoy working in the mornings",
		"User dislikes long meetings",
	}
	for _, c := range cases {
		got := ClassifyHeuristic(c)
		if got != domain.MemoryTypePreference {
			t.Errorf("ClassifyHeuristic(%q) = %s, want Preference", c, got)
		}
	}
}

func TestClassifyHeuristic_Constraint(t *testing.T) {
	cases := []string{
		"User must never receive spoilers",
		"Cannot share personal data with third parties",
	}
	for _, c := range cases {
		got := ClassifyHeuristic(c)
		if got != domain.MemoryTypeConstraint {
			t.Errorf("ClassifyHeuristic(%q) = %s, want Constraint", c, got)
		}
	}
}

func TestClassifyHeuristic_FallbackToFact(t *testing.T) {
	got := ClassifyHeuristic("User is a backend engineer at a fintech startup")
	if got != domain.MemoryTypeFact {
		t.Errorf("ClassifyHeuristic fallback = %s, want Fact", got)
	}
}
