package analyzer

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

func TestRacer_InteractiveCancelsLosers(t *testing.T) {
	var slowStarted atomic.Bool
	var slowSawCancel atomic.Bool

	r := NewRacer(func(s string) bool { return s == "" },
		Attempt[string]{
			Name: "fast", Quality: QualityTreeSitter,
			Fn: func(ctx context.Context) (string, error) {
				return "ast", nil
			},
		},
		Attempt[string]{
			Name: "slow", Quality: QualityLSP,
			Fn: func(ctx context.Context) (string, error) {
				slowStarted.Store(true)
				select {
				case <-ctx.Done():
					slowSawCancel.Store(true)
					return "", ctx.Err()
				case <-time.After(2 * time.Second):
					return "lsp", nil
				}
			},
		},
	).WithMinQuality(QualityTreeSitter).WithInteractive(true)

	start := time.Now()
	res, err := r.Race(context.Background())
	elapsed := time.Since(start)
	if err != nil {
		t.Fatal(err)
	}
	if res.Winner != "fast" || res.Value != "ast" {
		t.Fatalf("winner=%s value=%q", res.Winner, res.Value)
	}
	if elapsed > 500*time.Millisecond {
		t.Fatalf("interactive race took %v; losers should be cancelled", elapsed)
	}
	// Give the slow goroutine a moment to observe cancel.
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) && !slowSawCancel.Load() {
		time.Sleep(10 * time.Millisecond)
	}
	if slowStarted.Load() && !slowSawCancel.Load() {
		t.Fatal("slow attempt started but did not see cancel")
	}
}
