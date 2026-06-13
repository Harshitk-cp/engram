package service

import (
	"testing"

	"github.com/Harshitk-cp/engram/internal/domain"
	"github.com/google/uuid"
)

func TestSameScopeCandidates(t *testing.T) {
	a := uuid.New()
	b := uuid.New()
	mk := func(anchor, session *uuid.UUID) domain.MemoryWithScore {
		return domain.MemoryWithScore{Memory: domain.Memory{ID: uuid.New(), AnchorID: anchor, SessionID: session}}
	}
	candidates := []domain.MemoryWithScore{mk(&a, nil), mk(&b, nil), mk(nil, nil), mk(&a, nil)}

	gotA := sameScopeCandidates(append([]domain.MemoryWithScore(nil), candidates...), &domain.Memory{AnchorID: &a})
	if len(gotA) != 2 {
		t.Fatalf("anchor A: want 2 candidates, got %d", len(gotA))
	}

	gotNil := sameScopeCandidates(append([]domain.MemoryWithScore(nil), candidates...), &domain.Memory{})
	if len(gotNil) != 1 {
		t.Fatalf("unanchored: want 1 candidate, got %d", len(gotNil))
	}
}

// Two anonymous sessions (anchor NULL, distinct session IDs) are different
// subjects: their memories must never reinforce or supersede each other.
func TestSameScopeCandidates_AnonymousSessionsAreDistinct(t *testing.T) {
	sessA := uuid.New()
	sessB := uuid.New()
	mk := func(session *uuid.UUID) domain.MemoryWithScore {
		return domain.MemoryWithScore{Memory: domain.Memory{ID: uuid.New(), SessionID: session}}
	}
	candidates := []domain.MemoryWithScore{mk(&sessA), mk(&sessB), mk(nil)}

	got := sameScopeCandidates(append([]domain.MemoryWithScore(nil), candidates...), &domain.Memory{SessionID: &sessA})
	if len(got) != 1 || got[0].SessionID == nil || *got[0].SessionID != sessA {
		t.Fatalf("session A: want only its own candidate, got %d", len(got))
	}

	gotPrivate := sameScopeCandidates(append([]domain.MemoryWithScore(nil), candidates...), &domain.Memory{})
	if len(gotPrivate) != 1 || gotPrivate[0].SessionID != nil {
		t.Fatalf("private scope: want only the session-less candidate, got %d", len(gotPrivate))
	}
}

func TestSameUUIDPtr(t *testing.T) {
	a := uuid.New()
	b := uuid.New()
	if !sameUUIDPtr(nil, nil) {
		t.Error("nil/nil should match")
	}
	if sameUUIDPtr(&a, nil) || sameUUIDPtr(nil, &a) {
		t.Error("set vs nil should not match")
	}
	if !sameUUIDPtr(&a, &a) {
		t.Error("same value should match")
	}
	if sameUUIDPtr(&a, &b) {
		t.Error("different values should not match")
	}
}
