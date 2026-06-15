package domain

import "github.com/google/uuid"

// CalibrationSample is one labeled prediction: the confidence a memory carried
// at the moment evidence arrived, and whether that evidence confirmed it.
//
// The label is derived from the sign of a confidence mutation: when the agent
// held belief at confidence p and feedback/outcome/contradiction evidence
// arrived, did the evidence confirm (new >= old → correct) or refute it
// (new < old → incorrect)? Predicted = old confidence, observed = the label.
type CalibrationSample struct {
	Confidence float64 // predicted confidence before the evidence (0..1)
	Correct    bool    // did the evidence confirm the belief?
}

// CalibrationBin is one bucket of a reliability diagram.
type CalibrationBin struct {
	Lower         float64 `json:"lower"`          // bin lower edge
	Upper         float64 `json:"upper"`          // bin upper edge
	Count         int     `json:"count"`          // samples in bin
	MeanPredicted float64 `json:"mean_predicted"` // avg predicted confidence
	Observed      float64 `json:"observed"`       // fraction actually correct
}

// CalibrationReport is the measured calibration of an agent's (or tenant's)
// confidence scores: how well predicted confidence matches observed correctness.
type CalibrationReport struct {
	AgentID *uuid.UUID `json:"agent_id,omitempty"`
	Samples int        `json:"samples"`
	// ECE: expected calibration error — sample-weighted mean gap between
	// predicted confidence and observed accuracy across bins. Lower is better;
	// 0 means perfectly calibrated.
	ECE float64 `json:"ece"`
	// MCE: maximum calibration error — the worst single-bin gap.
	MCE float64 `json:"mce"`
	// Brier: mean squared error of the probabilistic predictions. Lower is better.
	Brier float64 `json:"brier"`
	// Bins is the reliability diagram (empty bins omitted).
	Bins []CalibrationBin `json:"bins"`
	// Insufficient is true when there are too few samples to report a
	// trustworthy number; callers should not surface ECE/Brier as headline.
	Insufficient bool `json:"insufficient"`
}

// MinCalibrationSamples is the floor below which a calibration report is not
// statistically meaningful and is flagged Insufficient.
const MinCalibrationSamples = 30
