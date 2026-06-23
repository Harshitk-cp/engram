package domain

import (
	"time"

	"github.com/google/uuid"
)

type MemoryTier string

const (
	TierHot     MemoryTier = "hot"
	TierWarm    MemoryTier = "warm"
	TierCold    MemoryTier = "cold"
	TierArchive MemoryTier = "archive"
)

func ComputeTier(confidence float64) MemoryTier {
	switch {
	case confidence > 0.85:
		return TierHot
	case confidence > 0.70:
		return TierWarm
	case confidence > 0.40:
		return TierCold
	default:
		return TierArchive
	}
}

// AnnotateTiers fills each memory's derived Tier from its Confidence, making the
// API the single source of truth for tiering. Clients should display this rather
// than re-deriving from confidence: the stored confidence is float4, and
// re-bucketing the JSON-rounded value in another language disagrees with the
// server at the band boundaries (e.g. a 0.85 stored as ~0.8500000238 is "hot"
// here but "warm" if JS compares the rounded 0.85 against the same threshold).
func AnnotateTiers(memories []Memory) {
	for i := range memories {
		memories[i].Tier = ComputeTier(float64(memories[i].Confidence))
	}
}

type TierBehavior struct {
	Tier               MemoryTier
	AutoInject         bool
	RetrievalThreshold float64
	SummarizeOnAccess  bool
	DecayMultiplier    float64
}

var TierBehaviors = map[MemoryTier]TierBehavior{
	TierHot: {
		Tier:               TierHot,
		AutoInject:         true,
		RetrievalThreshold: 0.0,
		SummarizeOnAccess:  false,
		DecayMultiplier:    0.5,
	},
	TierWarm: {
		Tier:               TierWarm,
		AutoInject:         false,
		RetrievalThreshold: 0.5,
		SummarizeOnAccess:  false,
		DecayMultiplier:    1.0,
	},
	TierCold: {
		Tier:               TierCold,
		AutoInject:         false,
		RetrievalThreshold: 0.75,
		SummarizeOnAccess:  true,
		DecayMultiplier:    1.5,
	},
	TierArchive: {
		Tier:               TierArchive,
		AutoInject:         false,
		RetrievalThreshold: 1.0,
		SummarizeOnAccess:  true,
		DecayMultiplier:    2.0,
	},
}

func GetTierBehavior(tier MemoryTier) TierBehavior {
	if b, ok := TierBehaviors[tier]; ok {
		return b
	}
	return TierBehaviors[TierArchive]
}

var TierConfidenceThresholds = map[MemoryTier]struct{ Min, Max float64 }{
	TierHot:     {Min: 0.85, Max: 1.0},
	TierWarm:    {Min: 0.70, Max: 0.85},
	TierCold:    {Min: 0.40, Max: 0.70},
	TierArchive: {Min: 0.0, Max: 0.40},
}

func TierReason(confidence float64) string {
	tier := ComputeTier(confidence)
	switch tier {
	case TierHot:
		return "confidence > 0.85"
	case TierWarm:
		return "0.70 < confidence <= 0.85"
	case TierCold:
		return "0.40 < confidence <= 0.70"
	default:
		return "confidence <= 0.40"
	}
}

func AllTiers() []MemoryTier {
	return []MemoryTier{TierHot, TierWarm, TierCold, TierArchive}
}

func ValidTier(t string) bool {
	switch MemoryTier(t) {
	case TierHot, TierWarm, TierCold, TierArchive:
		return true
	}
	return false
}

func DefaultIncludeTiers() []MemoryTier {
	return []MemoryTier{TierHot, TierWarm}
}

// TierTransition records when a memory moves between tiers
type TierTransition struct {
	MemoryID   uuid.UUID  `json:"memory_id"`
	FromTier   MemoryTier `json:"from_tier"`
	ToTier     MemoryTier `json:"to_tier"`
	Reason     string     `json:"reason"`
	OccurredAt time.Time  `json:"occurred_at"`
}
