package service

import (
	"context"
	"sort"

	"github.com/Harshitk-cp/engram/internal/domain"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

type CalibrationService struct {
	mutations domain.MutationLogStore
	logger    *zap.Logger
}

func NewCalibrationService(mutations domain.MutationLogStore, logger *zap.Logger) *CalibrationService {
	return &CalibrationService{mutations: mutations, logger: logger}
}

// DefaultCalibrationBins is the number of equal-width buckets in the reliability
// diagram (10 → deciles, the standard for ECE).
const DefaultCalibrationBins = 10

// Report computes the calibration report for a tenant,
func (s *CalibrationService) Report(ctx context.Context, tenantID uuid.UUID, agentID *uuid.UUID) (*domain.CalibrationReport, error) {
	samples, err := s.mutations.CalibrationSamples(ctx, tenantID, agentID)
	if err != nil {
		return nil, err
	}
	return ComputeCalibration(samples, agentID, DefaultCalibrationBins), nil
}

// ComputeCalibration is the pure, testable core: bin samples by predicted
// confidence and compute calibration metrics.
func ComputeCalibration(samples []domain.CalibrationSample, agentID *uuid.UUID, nBins int) *domain.CalibrationReport {
	if nBins <= 0 {
		nBins = DefaultCalibrationBins
	}
	report := &domain.CalibrationReport{
		AgentID: agentID,
		Samples: len(samples),
		Bins:    []domain.CalibrationBin{},
	}
	if len(samples) < domain.MinCalibrationSamples {
		report.Insufficient = true
	}
	if len(samples) == 0 {
		return report
	}

	type acc struct {
		count   int
		sumConf float64
		correct int
	}
	bins := make([]acc, nBins)
	var brier float64

	for _, sm := range samples {
		c := sm.Confidence
		if c < 0 {
			c = 0
		} else if c > 1 {
			c = 1
		}
		// bin index; the top edge (c == 1) lands in the last bin
		idx := int(c * float64(nBins))
		if idx >= nBins {
			idx = nBins - 1
		}
		bins[idx].count++
		bins[idx].sumConf += c
		label := 0.0
		if sm.Correct {
			bins[idx].correct++
			label = 1.0
		}
		brier += (c - label) * (c - label)
	}

	n := float64(len(samples))
	report.Brier = brier / n

	binWidth := 1.0 / float64(nBins)
	for i := 0; i < nBins; i++ {
		b := bins[i]
		if b.count == 0 {
			continue
		}
		meanPred := b.sumConf / float64(b.count)
		observed := float64(b.correct) / float64(b.count)
		gap := observed - meanPred
		if gap < 0 {
			gap = -gap
		}
		weight := float64(b.count) / n
		report.ECE += weight * gap
		if gap > report.MCE {
			report.MCE = gap
		}
		report.Bins = append(report.Bins, domain.CalibrationBin{
			Lower:         float64(i) * binWidth,
			Upper:         float64(i+1) * binWidth,
			Count:         b.count,
			MeanPredicted: meanPred,
			Observed:      observed,
		})
	}

	sort.Slice(report.Bins, func(i, j int) bool {
		return report.Bins[i].Lower < report.Bins[j].Lower
	})
	return report
}
