package domain

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestComputeMemoryBinding(t *testing.T) {
	anchor := uuid.New()
	session := uuid.New()
	tests := []struct {
		name    string
		anchor  *uuid.UUID
		session *uuid.UUID
		want    MemoryBinding
	}{
		{"no ids -> private", nil, nil, BindingPrivate},
		{"anchor -> anchored", &anchor, nil, BindingAnchored},
		{"session -> session", nil, &session, BindingSession},
		{"anchor+session -> session wins", &anchor, &session, BindingSession},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ComputeMemoryBinding(tt.anchor, tt.session); got != tt.want {
				t.Fatalf("ComputeMemoryBinding(%v,%v) = %q, want %q", tt.anchor, tt.session, got, tt.want)
			}
		})
	}
}

func TestDefaultDecayRate(t *testing.T) {
	if DefaultDecayRate(BindingSession) <= DefaultDecayRate(BindingAnchored) {
		t.Error("session should decay faster than anchored")
	}
	if DefaultDecayRate(BindingCanon) >= DefaultDecayRate(BindingPrivate) {
		t.Error("canon should be stickier than private")
	}
}

// An agent-only memory (no anchor) must not serialize anchor_id/binding noise
func TestMemoryJSON_AnchorOmitted(t *testing.T) {
	m := Memory{ID: uuid.New(), AgentID: uuid.New()}
	b, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "anchor_id") {
		t.Errorf("expected anchor_id omitted for unanchored memory, got: %s", b)
	}
}

func TestMemoryJSON_AnchorPresent(t *testing.T) {
	a := uuid.New()
	m := Memory{ID: uuid.New(), AgentID: uuid.New(), AnchorID: &a, Binding: BindingAnchored}
	b, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "anchor_id") || !strings.Contains(string(b), `"binding":"anchored"`) {
		t.Errorf("expected anchor_id and binding in JSON, got: %s", b)
	}
}
