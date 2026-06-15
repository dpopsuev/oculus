package analyzer_test

import (
	"context"
	"errors"
	"testing"

	"github.com/dpopsuev/oculus/v3/analyzer"
)

// TestRacer_BelowThreshold_ReturnsDegraded verifies that when all analyzer
// attempts produce results but none meet the minimum quality threshold,
// Race() returns the best result with Degraded=true rather than an opaque error.
//
// Given a racer with min quality 50
// And one attempt with quality 10 that returns non-empty results
// When Race() runs
// Then it returns the best below-threshold result with Degraded=true
func TestRacer_BelowThreshold_ReturnsDegraded(t *testing.T) {
	r := analyzer.NewRacer(
		func(v []string) bool { return len(v) == 0 },
		analyzer.Attempt[[]string]{
			Name:    "low-quality",
			Quality: analyzer.QualityRegex,
			Fn: func(ctx context.Context) ([]string, error) {
				return []string{"ClassA", "ClassB"}, nil
			},
		},
	).WithMinQuality(analyzer.QualityTreeSitter)

	result, err := r.Race(context.Background())
	if err != nil {
		t.Fatalf("below-threshold result should be returned as degraded, not error: %v", err)
	}
	if len(result.Value) == 0 {
		t.Fatal("degraded result has zero values — should return what we have")
	}
	if !result.Degraded {
		t.Error("result should be marked Degraded=true when below quality threshold")
	}
}

// TestRacer_NoResults_StillErrors verifies that when all attempts genuinely
// return empty results (not just below-threshold), the error is still returned.
func TestRacer_NoResults_StillErrors(t *testing.T) {
	r := analyzer.NewRacer(
		func(v []string) bool { return len(v) == 0 },
		analyzer.Attempt[[]string]{
			Name:    "empty",
			Quality: analyzer.QualityRegex,
			Fn: func(ctx context.Context) ([]string, error) {
				return nil, nil
			},
		},
	).WithMinQuality(analyzer.QualityTreeSitter)

	_, err := r.Race(context.Background())
	if !errors.Is(err, analyzer.ErrNoQualifiedResult) {
		t.Errorf("genuinely empty result should still return ErrNoQualifiedResult, got %v", err)
	}
}

// TestRacer_AboveThreshold_NotDegraded verifies that a result meeting the
// threshold is returned with Degraded=false (no regression).
func TestRacer_AboveThreshold_NotDegraded(t *testing.T) {
	r := analyzer.NewRacer(
		func(v []string) bool { return len(v) == 0 },
		analyzer.Attempt[[]string]{
			Name:    "high-quality",
			Quality: analyzer.QualityTreeSitter,
			Fn: func(ctx context.Context) ([]string, error) {
				return []string{"ClassA"}, nil
			},
		},
	).WithMinQuality(analyzer.QualityTreeSitter)

	result, err := r.Race(context.Background())
	if err != nil {
		t.Fatalf("above-threshold result should not error: %v", err)
	}
	if result.Degraded {
		t.Error("above-threshold result should not be marked Degraded")
	}
}
