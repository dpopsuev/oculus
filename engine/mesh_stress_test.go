package engine

import (
	"context"
	"testing"
	"time"

	"github.com/dpopsuev/oculus/v3/arch"
	"github.com/dpopsuev/oculus/v3/testkit"
)

// TestGetSymbolGraph_Stress runs the full GetSymbolGraph pipeline on
// generated Go projects at increasing scale. Identifies which tier
// causes the timeout and logs per-phase timing (via slog debug spans).
//
// Run with: go test ./engine/... -run TestGetSymbolGraph_Stress -v -timeout 300s
func TestGetSymbolGraph_Stress(t *testing.T) {
	if testing.Short() {
		t.Skip("stress test")
	}

	tiers := []testkit.ScaleTier{
		testkit.TierSmall,
		testkit.TierMedium,
		testkit.TierLarge,
	}

	for _, tier := range tiers {
		t.Run(tier.Name, func(t *testing.T) {
			dir := t.TempDir()
			if err := testkit.GenerateGoProject(dir, tier); err != nil {
				t.Fatalf("generate: %v", err)
			}
			testkit.InitGitRepo(dir)

			report, err := arch.ScanAndBuild(context.Background(), dir, arch.ScanOpts{})
			if err != nil {
				t.Fatalf("scan: %v", err)
			}
			store := newMockStore(report)
			eng := New(store, nil)

			ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
			defer cancel()

			start := time.Now()
			sg, err := eng.GetSymbolGraph(ctx, dir)
			elapsed := time.Since(start)

			if err != nil {
				t.Errorf("[%s] FAILED after %v: %v", tier.Name, elapsed, err)
				return
			}

			t.Logf("[%s] components=%d nodes=%d edges=%d duration=%v",
				tier.Name, tier.Components, len(sg.Nodes), len(sg.Edges), elapsed)
		})
	}
}
