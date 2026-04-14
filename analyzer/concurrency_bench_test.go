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

// TestLSPConcurrency_Scaling measures call graph performance at different
// concurrency levels to find the optimal LSPConcurrency value.
//
// Run: go test -tags integration -run TestLSPConcurrency_Scaling -v -timeout 600s ./analyzer/...
func TestLSPConcurrency_Scaling(t *testing.T) {
	if err := testcontainer.Available(""); err != nil {
		t.Skipf("skipping: %v", err)
	}

	tier := testkit.TierMedium // 50 components, 150 edges
	dir := t.TempDir()
	if err := testkit.GenerateGoProject(dir, tier); err != nil {
		t.Fatalf("generate: %v", err)
	}
	testkit.InitGitRepo(dir)

	levels := []int{1, 2, 4, 8, 16, 32}

	t.Logf("%-12s %-10s %-10s %-10s", "concurrency", "edges", "typed", "duration")
	for _, n := range levels {
		pool := testcontainer.NewPool("")

		LSPConcurrency = n
		da := NewDeepFallback(dir, pool)

		ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
		start := time.Now()
		cg, err := da.CallGraph(ctx, dir, oculus.CallGraphOpts{Depth: 5})
		elapsed := time.Since(start)
		cancel()
		pool.Shutdown(context.Background())

		if err != nil {
			t.Logf("%-12d %-10s %-10s %v (error: %v)", n, "-", "-", elapsed, err)
			continue
		}

		typed := countTyped(cg.Edges)
		t.Logf("%-12d %-10d %-10d %v", n, len(cg.Edges), typed, elapsed)
	}

	// Reset to default
	LSPConcurrency = 8
}
