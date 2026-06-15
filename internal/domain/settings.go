package domain

import (
	"context"

	"github.com/google/uuid"
)

// EngineSettings holds the per-tenant tunable parameters of the cognitive
// engine. A tenant with no stored row uses DefaultEngineSettings(). Values are
// always Sanitize()-clamped so a bad write can't destabilize the engine.
type EngineSettings struct {
	// DecayBaseRate is λ (per hour) in the competition-aware decay formula:
	// conf → floor + (conf-floor)·e^(-λ·hours). Higher = faster forgetting.
	DecayBaseRate float64 `json:"decay_base_rate"`
	// DecayFloor is the lowest confidence decay alone can drive a memory to.
	DecayFloor float64 `json:"decay_floor"`
	// ArchiveThreshold: memories that decay below this are archived.
	ArchiveThreshold float64 `json:"archive_threshold"`
	// CompetitionWeight scales how strongly similar higher-confidence memories
	// suppress a competing one (interference). 0 disables competition.
	CompetitionWeight float64 `json:"competition_weight"`
	// ReinforcementLogOdds is the +Δ applied (in log-odds) when a memory is
	// reinforced (recall hit / helpful feedback).
	ReinforcementLogOdds float64 `json:"reinforcement_log_odds"`
	// ContradictionLogOdds is the −Δ applied when a memory is contradicted.
	ContradictionLogOdds float64 `json:"contradiction_log_odds"`

	// ── Provenance Firewall ──────────────────────────────────────────────────
	// FirewallEnabled turns on write-time quarantine of untrusted memories.
	// Off by default so existing tenants are unaffected.
	FirewallEnabled bool `json:"firewall_enabled"`
	// QuarantineProvenances lists provenances auto-quarantined when the firewall
	// is on (e.g. ["inferred","agent"] to hold model-generated content for review
	// while trusting "user"/"tool"). An explicit per-write quarantine flag is
	// always honored regardless of this list.
	QuarantineProvenances []string `json:"quarantine_provenances,omitempty"`
}

// ShouldQuarantine decides whether an incoming write must be held by the
// firewall. An explicit caller flag always quarantines (e.g. content from an
// untrusted channel); otherwise the tenant policy quarantines configured
// provenances when the firewall is enabled. Returns the reason for the audit log.
func (s EngineSettings) ShouldQuarantine(p Provenance, explicit bool) (bool, string) {
	if explicit {
		return true, "caller marked write untrusted"
	}
	if !s.FirewallEnabled {
		return false, ""
	}
	for _, qp := range s.QuarantineProvenances {
		if Provenance(qp) == p {
			return true, "firewall policy: provenance '" + string(p) + "' quarantined"
		}
	}
	return false, ""
}

// DefaultEngineSettings mirrors the engine's built-in constants. Keep in sync
// with service/decay.go and service/confidence.go defaults.
func DefaultEngineSettings() EngineSettings {
	return EngineSettings{
		DecayBaseRate:        0.001,
		DecayFloor:           0.1,
		ArchiveThreshold:     0.15,
		CompetitionWeight:    0.5,
		ReinforcementLogOdds: 0.3,
		ContradictionLogOdds: 0.5,
	}
}

func clampF(v, lo, hi, fallback float64) float64 {
	if v < lo || v > hi {
		if v < lo && v >= 0 {
			return lo
		}
		if v > hi {
			return hi
		}
		return fallback
	}
	return v
}

// Sanitize clamps each field to a safe range, falling back to the default when a
// value is nonsensical (e.g. negative). This guards the engine against a bad
// settings write.
func (s EngineSettings) Sanitize() EngineSettings {
	d := DefaultEngineSettings()
	out := EngineSettings{
		DecayBaseRate:        clampF(s.DecayBaseRate, 0, 1, d.DecayBaseRate),
		DecayFloor:           clampF(s.DecayFloor, 0, 0.9, d.DecayFloor),
		ArchiveThreshold:     clampF(s.ArchiveThreshold, 0, 0.9, d.ArchiveThreshold),
		CompetitionWeight:    clampF(s.CompetitionWeight, 0, 5, d.CompetitionWeight),
		ReinforcementLogOdds: clampF(s.ReinforcementLogOdds, 0, 5, d.ReinforcementLogOdds),
		ContradictionLogOdds: clampF(s.ContradictionLogOdds, 0, 5, d.ContradictionLogOdds),
		FirewallEnabled:      s.FirewallEnabled,
	}
	// Archive threshold below the decay floor would never trigger; keep it sane.
	if out.ArchiveThreshold > out.DecayFloor {
		// allow it — archiving above the floor is valid (decay stops at floor,
		// but other paths can push below archive). No correction needed.
	}
	// Keep only valid, non-quarantine provenances in the firewall list.
	for _, qp := range s.QuarantineProvenances {
		if ValidProvenance(qp) {
			out.QuarantineProvenances = append(out.QuarantineProvenances, qp)
		}
	}
	return out
}

// TenantSettingsStore persists per-tenant engine settings.
type TenantSettingsStore interface {
	// Get returns the tenant's settings, or DefaultEngineSettings() if none.
	Get(ctx context.Context, tenantID uuid.UUID) (EngineSettings, error)
	Upsert(ctx context.Context, tenantID uuid.UUID, s EngineSettings) error
}
