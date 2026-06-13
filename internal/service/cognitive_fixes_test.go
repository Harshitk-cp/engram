package service

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/Harshitk-cp/engram/internal/domain"
	"github.com/google/uuid"
)

// TestIncrementalMean_EqualWeighting proves the consolidation centroid fix:
// folding members in one at a time must produce the true equal-weight mean,
// not a pairwise average that over-weights the last-added member.
func TestIncrementalMean_EqualWeighting(t *testing.T) {
	vecs := [][]float32{
		{0, 0, 0},
		{3, 6, 9},
		{6, 6, 6},
	}
	// build centroid incrementally the way clusterMemories does
	centroid := cloneVector(vecs[0])
	for n := 1; n < len(vecs); n++ {
		centroid = incrementalMean(centroid, vecs[n], n+1)
	}
	// true mean = ([0,0,0]+[3,6,9]+[6,6,6])/3 = [3,4,5]
	want := []float32{3, 4, 5}
	for i := range want {
		if math.Abs(float64(centroid[i]-want[i])) > 1e-5 {
			t.Fatalf("centroid[%d]=%f, want %f (equal weighting violated)", i, centroid[i], want[i])
		}
	}
}

// TestIncrementalMean_DoesNotMutateSeed guards the aliasing fix: the seed
// embedding must not change when it is used to start a cluster centroid.
func TestIncrementalMean_DoesNotMutateSeed(t *testing.T) {
	seed := []float32{1, 2, 3}
	centroid := cloneVector(seed)
	_ = incrementalMean(centroid, []float32{9, 9, 9}, 2)
	for i, v := range []float32{1, 2, 3} {
		if seed[i] != v {
			t.Fatalf("seed mutated at %d: got %f want %f", i, seed[i], v)
		}
	}
}

// TestAssessConfidence_NoSaturation proves the metacognition fix: two
// high-confidence memories that differ in base must produce DISTINCT adjusted
// confidences. The old multiplicative formula pegged both at the 1.0 ceiling.
func TestAssessConfidence_NoSaturation(t *testing.T) {
	svc, _, _, _, _, _, _ := setupMetacognitiveTest()
	ctx := context.Background()
	now := time.Now()

	mk := func(base float32) domain.Memory {
		return domain.Memory{
			ID:                 uuid.New(),
			Content:            "x",
			Type:               domain.MemoryTypePreference,
			Confidence:         base,
			LastVerifiedAt:     &now,
			ReinforcementCount: 5, // strong reinforcement — the old formula saturated here
			Source:             string(domain.SourceUserStatement),
		}
	}

	a, err := svc.AssessConfidence(ctx, mk(0.80))
	if err != nil {
		t.Fatal(err)
	}
	b, err := svc.AssessConfidence(ctx, mk(0.95))
	if err != nil {
		t.Fatal(err)
	}

	if a.AdjustedConfidence >= 1.0 || b.AdjustedConfidence >= 1.0 {
		t.Fatalf("confidence saturated at ceiling: a=%f b=%f", a.AdjustedConfidence, b.AdjustedConfidence)
	}
	if b.AdjustedConfidence <= a.AdjustedConfidence {
		t.Fatalf("expected higher base → higher adjusted (resolution preserved); a(0.80)=%f b(0.95)=%f",
			a.AdjustedConfidence, b.AdjustedConfidence)
	}
}
