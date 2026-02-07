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
