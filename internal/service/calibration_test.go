package service

import (
	"math"
	"testing"

	"github.com/Harshitk-cp/engram/internal/domain"
)

func sample(conf float64, correct bool) domain.CalibrationSample {
	return domain.CalibrationSample{Confidence: conf, Correct: correct}
}

func TestComputeCalibration_PerfectlyCalibrated(t *testing.T) {
	// In each decile, the observed accuracy exactly equals the predicted
	// confidence → ECE and MCE should be ~0.
	var samples []domain.CalibrationSample
	for d := 0; d < 10; d++ {
		conf := float64(d)/10 + 0.05 // bin midpoint: 0.05, 0.15, … 0.95
		// 100 samples per bin; `correct` count ≈ conf*100
		correct := int(math.Round(conf * 100))
		for i := 0; i < 100; i++ {
			samples = append(samples, sample(conf, i < correct))
		}
	}

	r := ComputeCalibration(samples, nil, 10)
	if r.Insufficient {
		t.Fatalf("expected sufficient samples, got insufficient (n=%d)", r.Samples)
	}
	if r.ECE > 0.01 {
		t.Errorf("perfectly calibrated input should have ECE ~0, got %.4f", r.ECE)
	}
	if r.MCE > 0.02 {
		t.Errorf("perfectly calibrated input should have MCE ~0, got %.4f", r.MCE)
	}
}

func TestComputeCalibration_Overconfident(t *testing.T) {
	// Always predicts 0.9 but is only right half the time → big calibration gap.
	var samples []domain.CalibrationSample
	for i := 0; i < 200; i++ {
		samples = append(samples, sample(0.9, i%2 == 0))
	}
	r := ComputeCalibration(samples, nil, 10)
	// gap = |0.5 observed - 0.9 predicted| = 0.4
	if math.Abs(r.ECE-0.4) > 0.02 {
		t.Errorf("expected ECE ~0.4 for overconfident input, got %.4f", r.ECE)
	}
	if math.Abs(r.MCE-0.4) > 0.02 {
		t.Errorf("expected MCE ~0.4, got %.4f", r.MCE)
	}
	// Brier for p=0.9, half right: mean(0.5*(0.1^2) + 0.5*(0.9^2)) = 0.41
	if math.Abs(r.Brier-0.41) > 0.01 {
		t.Errorf("expected Brier ~0.41, got %.4f", r.Brier)
	}
}

func TestComputeCalibration_Insufficient(t *testing.T) {
	r := ComputeCalibration([]domain.CalibrationSample{sample(0.8, true)}, nil, 10)
	if !r.Insufficient {
		t.Error("expected Insufficient=true for a single sample")
	}
	if r.Samples != 1 {
		t.Errorf("expected Samples=1, got %d", r.Samples)
	}
}

func TestComputeCalibration_Empty(t *testing.T) {
	r := ComputeCalibration(nil, nil, 10)
	if !r.Insufficient || r.Samples != 0 || len(r.Bins) != 0 {
		t.Errorf("empty input should be insufficient with no bins, got %+v", r)
	}
}

func TestComputeCalibration_ClampsOutOfRange(t *testing.T) {
	// Confidence values outside [0,1] must not panic or escape binning.
	samples := []domain.CalibrationSample{}
	for i := 0; i < 40; i++ {
		samples = append(samples, sample(1.4, true), sample(-0.3, false))
	}
	r := ComputeCalibration(samples, nil, 10)
	if r.Samples != 80 {
		t.Fatalf("expected 80 samples, got %d", r.Samples)
	}
	for _, b := range r.Bins {
		if b.MeanPredicted < 0 || b.MeanPredicted > 1 {
			t.Errorf("bin mean predicted out of range: %.2f", b.MeanPredicted)
		}
	}
}
