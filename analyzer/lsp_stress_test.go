//go:build integration

package analyzer

import (
	"context"
	"testing"
	"time"

	"github.com/dpopsuev/oculus/v3"
	"github.com/dpopsuev/oculus/v3/lsp/testcontainer"
	"github.com/dpopsuev/oculus/v3/testkit"
)

// TestLSPIntegration_Stress runs the full DeepFallback pipeline (LSP priority 100)
// on generated Go projects at increasing scale via container pool.
// This reproduces the Origami hang — if LSP is the bottleneck, it will show here.
//
// Run: go test -tags integration -run TestLSPIntegration_Stress -v -timeout 300s ./analyzer/...
func TestLSPIntegration_Stress(t *testing.T) {
	if err := testcontainer.Available(""); err != nil {
		t.Skipf("skipping: %v", err)
	}

	pool := testcontainer.NewPool("")
	defer pool.Shutdown(context.Background())

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

			// DeepFallback with container pool — LSP at priority 100
			da := NewDeepFallback(dir, pool)

			ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
			defer cancel()

			start := time.Now()
			cg, err := da.CallGraph(ctx, dir, oculus.CallGraphOpts{Depth: 5})
			elapsed := time.Since(start)

			if err != nil {
				t.Errorf("[%s] LSP pipeline FAILED after %v: %v", tier.Name, elapsed, err)
				return
			}

			typed := countTyped(cg.Edges)
			t.Logf("[%s] layer=%s components=%d nodes=%d edges=%d typed=%d duration=%v",
				tier.Name, cg.Layer, tier.Components,
				len(cg.Nodes), len(cg.Edges), typed, elapsed)

			if elapsed > 60*time.Second {
				t.Errorf("[%s] took %v — exceeds 60s budget", tier.Name, elapsed)
			}
		})
	}
}
