package analyzer_test

import (
	"context"
	"errors"
	"testing"

	"github.com/dpopsuev/oculus/v3/analyzer"
)

// TestBug101_Racer_BelowThreshold_ReturnsDegraded reproduces LCS-BUG-101:
// When all analyzer attempts produce results but none meet the minimum quality
// threshold, Race() returns ErrNoQualifiedResult and throws away the results.
// The agent sees an opaque error with no actionable output.
//
// Fix: when below-threshold results exist, return the best one with
// Degraded=true rather than erroring. The caller decides whether to use it.
//
// Given a racer with min quality 50
// And one attempt with quality 10 that returns non-empty results
// When Race() runs
// Then it returns the best below-threshold result with Degraded=true
// And no error
func TestBug101_Racer_BelowThreshold_ReturnsDegraded(t *testing.T) {
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
		t.Fatalf("BUG-101: below-threshold result should be returned as degraded, not error: %v", err)
	}
	if len(result.Value) == 0 {
		t.Fatal("BUG-101: degraded result has zero values — should return what we have")
	}
	if !result.Degraded {
		t.Error("BUG-101: result should be marked Degraded=true when below quality threshold")
	}
}

// TestBug101_Racer_NoResults_StillErrors verifies that when all attempts
// genuinely return empty results (not just below-threshold), the error
// is still returned. Degraded only applies when there is content to return.
func TestBug101_Racer_NoResults_StillErrors(t *testing.T) {
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

// TestBug101_Racer_AboveThreshold_NotDegraded verifies that a result meeting
// the threshold is returned with Degraded=false (no regression).
func TestBug101_Racer_AboveThreshold_NotDegraded(t *testing.T) {
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
