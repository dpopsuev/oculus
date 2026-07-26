package engine

import (
	"context"
	"testing"
	"time"

	"github.com/dpopsuev/oculus/v3/arch"
	"github.com/dpopsuev/oculus/v3/internal/testfixture"
)

func TestGetSymbolGraph_SyntheticRepositoryCompletesWithinBudget(t *testing.T) {
	root := testfixture.Repository(t, "go")
	report, err := arch.ScanAndBuild(context.Background(), root, arch.ScanOpts{})
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	engine := New(newMockStore(report), nil)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	started := time.Now()
	symbolGraph, err := engine.GetSymbolGraph(ctx, root)
	if err != nil {
		t.Fatalf("GetSymbolGraph after %v: %v", time.Since(started), err)
	}
	if len(symbolGraph.Nodes) == 0 || len(symbolGraph.Edges) == 0 {
		t.Fatalf("symbol graph nodes=%d edges=%d", len(symbolGraph.Nodes), len(symbolGraph.Edges))
	}
}
