package domain

import "testing"

func TestComputeTier(t *testing.T) {
	tests := []struct {
		name       string
		confidence float64
		want       MemoryTier
	}{
		{"hot - 0.99", 0.99, TierHot},
		{"hot - 0.86", 0.86, TierHot},
		{"hot boundary - 0.851", 0.851, TierHot},
		{"warm - 0.85", 0.85, TierWarm},
		{"warm - 0.75", 0.75, TierWarm},
		{"warm boundary - 0.701", 0.701, TierWarm},
		{"cold - 0.70", 0.70, TierCold},
		{"cold - 0.50", 0.50, TierCold},
		{"cold boundary - 0.401", 0.401, TierCold},
		{"archive - 0.40", 0.40, TierArchive},
		{"archive - 0.20", 0.20, TierArchive},
		{"archive - 0.0", 0.0, TierArchive},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ComputeTier(tt.confidence)
			if got != tt.want {
				t.Errorf("ComputeTier(%v) = %v, want %v", tt.confidence, got, tt.want)
			}
		})
	}
}

func TestTierBehaviors(t *testing.T) {
	t.Run("hot tier auto-injects", func(t *testing.T) {
		b := GetTierBehavior(TierHot)
		if !b.AutoInject {
			t.Error("hot tier should auto-inject")
		}
		if b.RetrievalThreshold != 0.0 {
			t.Errorf("hot tier retrieval threshold should be 0.0, got %v", b.RetrievalThreshold)
		}
	})

	t.Run("warm tier no auto-inject", func(t *testing.T) {
		b := GetTierBehavior(TierWarm)
		if b.AutoInject {
			t.Error("warm tier should not auto-inject")
		}
		if b.RetrievalThreshold != 0.5 {
			t.Errorf("warm tier retrieval threshold should be 0.5, got %v", b.RetrievalThreshold)
		}
	})

	t.Run("cold tier summarizes on access", func(t *testing.T) {
		b := GetTierBehavior(TierCold)
		if !b.SummarizeOnAccess {
			t.Error("cold tier should summarize on access")
		}
		if b.RetrievalThreshold != 0.75 {
			t.Errorf("cold tier retrieval threshold should be 0.75, got %v", b.RetrievalThreshold)
		}
	})

	t.Run("archive tier never directly retrieved", func(t *testing.T) {
		b := GetTierBehavior(TierArchive)
		if b.RetrievalThreshold != 1.0 {
			t.Errorf("archive tier retrieval threshold should be 1.0, got %v", b.RetrievalThreshold)
		}
		if b.DecayMultiplier != 2.0 {
			t.Errorf("archive tier decay multiplier should be 2.0, got %v", b.DecayMultiplier)
		}
	})
}

func TestTierReason(t *testing.T) {
	tests := []struct {
		confidence float64
		contains   string
	}{
		{0.90, "0.85"},
		{0.75, "0.70"},
		{0.50, "0.40"},
		{0.30, "0.40"},
	}

	for _, tt := range tests {
		reason := TierReason(tt.confidence)
		if reason == "" {
			t.Errorf("TierReason(%v) returned empty string", tt.confidence)
		}
	}
}

func TestValidTier(t *testing.T) {
	validTiers := []string{"hot", "warm", "cold", "archive"}
	for _, tier := range validTiers {
		if !ValidTier(tier) {
			t.Errorf("ValidTier(%q) = false, want true", tier)
		}
	}

	invalidTiers := []string{"", "unknown", "HOT", "Hot"}
	for _, tier := range invalidTiers {
		if ValidTier(tier) {
			t.Errorf("ValidTier(%q) = true, want false", tier)
		}
	}
}

func TestAllTiers(t *testing.T) {
	tiers := AllTiers()
	if len(tiers) != 4 {
		t.Errorf("AllTiers() returned %d tiers, want 4", len(tiers))
	}

	expected := map[MemoryTier]bool{
		TierHot:     true,
		TierWarm:    true,
		TierCold:    true,
		TierArchive: true,
	}
	for _, tier := range tiers {
		if !expected[tier] {
			t.Errorf("unexpected tier: %v", tier)
		}
	}
}

func TestDefaultIncludeTiers(t *testing.T) {
	tiers := DefaultIncludeTiers()
	if len(tiers) != 2 {
		t.Errorf("DefaultIncludeTiers() returned %d tiers, want 2", len(tiers))
	}

	tierSet := make(map[MemoryTier]bool)
	for _, tier := range tiers {
		tierSet[tier] = true
	}

	if !tierSet[TierHot] {
		t.Error("DefaultIncludeTiers should include hot")
	}
	if !tierSet[TierWarm] {
		t.Error("DefaultIncludeTiers should include warm")
	}
}

func TestGetTierBehavior_UnknownTier(t *testing.T) {
	b := GetTierBehavior(MemoryTier("unknown"))
	if b.Tier != TierArchive {
		t.Errorf("unknown tier should fall back to archive behavior, got %v", b.Tier)
	}
}

// Memories are stored in a float4 (REAL) column, so a "round" 0.85 reads back as
// float32(0.85) ≈ 0.8500000238 — which Postgres buckets as hot (> 0.85). When the
// server hands that stored value to ComputeTier it must agree, so the Beliefs
// browser (which now displays the server-computed tier) matches the dashboard's
// SQL tier counts instead of re-bucketing a JSON-rounded 0.85 to warm in JS.
func TestComputeTier_StoredFloat32Boundary(t *testing.T) {
	cases := []struct {
		stored float32
		want   MemoryTier
	}{
		{0.85, TierHot},  // float32(0.85) rounds up past 0.85 — hot, like the SQL
		{0.95, TierHot},
		{0.80, TierWarm},
		{0.75, TierWarm},
		{0.70, TierCold}, // float32(0.70) rounds down below 0.70 — cold, like the SQL
		{0.50, TierCold},
		{0.30, TierArchive},
	}
	for _, c := range cases {
		if got := ComputeTier(float64(c.stored)); got != c.want {
			t.Errorf("ComputeTier(float64(float32(%v))=%v) = %q, want %q",
				c.stored, float64(c.stored), got, c.want)
		}
	}
}

func TestAnnotateTiers(t *testing.T) {
	mems := []Memory{{Confidence: 0.85}, {Confidence: 0.80}, {Confidence: 0.70}, {Confidence: 0.30}}
	AnnotateTiers(mems)
	want := []MemoryTier{TierHot, TierWarm, TierCold, TierArchive}
	for i := range mems {
		if mems[i].Tier != want[i] {
			t.Errorf("mems[%d] (conf %v): Tier = %q, want %q", i, mems[i].Confidence, mems[i].Tier, want[i])
		}
	}
}
