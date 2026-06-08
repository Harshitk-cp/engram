package service

import (
	"testing"

	"github.com/Harshitk-cp/engram/internal/domain"
	"github.com/google/uuid"
)

func TestSameAnchorCandidates(t *testing.T) {
	a := uuid.New()
	b := uuid.New()
	mk := func(anchor *uuid.UUID) domain.MemoryWithScore {
		return domain.MemoryWithScore{Memory: domain.Memory{ID: uuid.New(), AnchorID: anchor}}
	}
	candidates := []domain.MemoryWithScore{mk(&a), mk(&b), mk(nil), mk(&a)}

	gotA := sameAnchorCandidates(append([]domain.MemoryWithScore(nil), candidates...), &a)
	if len(gotA) != 2 {
		t.Fatalf("anchor A: want 2 candidates, got %d", len(gotA))
	}

	gotNil := sameAnchorCandidates(append([]domain.MemoryWithScore(nil), candidates...), nil)
	if len(gotNil) != 1 {
		t.Fatalf("unanchored: want 1 candidate, got %d", len(gotNil))
	}
}

func TestSameAnchor(t *testing.T) {
	a := uuid.New()
	b := uuid.New()
	if !sameAnchor(nil, nil) {
		t.Error("nil/nil should match")
	}
	if sameAnchor(&a, nil) || sameAnchor(nil, &a) {
		t.Error("anchored vs unanchored should not match")
	}
	if !sameAnchor(&a, &a) {
		t.Error("same anchor should match")
	}
	if sameAnchor(&a, &b) {
		t.Error("different anchors should not match")
	}
}
