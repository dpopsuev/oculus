package engine

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/dpopsuev/oculus/arch"
)

const origamiPath = "/home/dpopsuev/Workspace/origami"

// TestOrigami_GetSymbolGraph runs the full GetSymbolGraph pipeline on
// the real Origami codebase (48 packages, 549 Go files). This is the
// exact scenario that hangs in production.
//
// Run: go test ./engine/... -run TestOrigami_GetSymbolGraph -v -timeout 120s
func TestOrigami_GetSymbolGraph(t *testing.T) {
	if testing.Short() {
		t.Skip("stress test on real repo")
	}
	if _, err := os.Stat(origamiPath); err != nil {
		t.Skipf("origami not found at %s", origamiPath)
	}

	report, err := arch.ScanAndBuild(origamiPath, arch.ScanOpts{})
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	t.Logf("origami scan: %d components, %d edges",
		len(report.Architecture.Services), len(report.Architecture.Edges))

	store := newMockStore(report)
	eng := New(store, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	start := time.Now()
	sg, err := eng.GetSymbolGraph(ctx, origamiPath)
	elapsed := time.Since(start)

	if err != nil {
		t.Errorf("GetSymbolGraph FAILED after %v: %v", elapsed, err)
		return
	}

	t.Logf("origami symbol graph: nodes=%d edges=%d duration=%v",
		len(sg.Nodes), len(sg.Edges), elapsed)

	if elapsed > 120*time.Second {
		t.Errorf("took %v — exceeds 120s budget", elapsed)
	}
}
