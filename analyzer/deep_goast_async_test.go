package analyzer

import (
	"context"
	"testing"

	oculus "github.com/dpopsuev/oculus/v3"
)

// TestGoAST_AsyncEdgeKinds verifies that the Go AST walker emits coloured
// edges for goroutine spawns, channel sends, and channel receives.
func TestGoAST_AsyncEdgeKinds(t *testing.T) {
	dir := "testdata/go_async"
	a := NewGoASTDeep(dir)
	if a == nil {
		t.Skip("not a Go project")
	}

	cg, err := a.CallGraph(context.Background(), dir, oculus.CallGraphOpts{Depth: 5})
	if err != nil {
		t.Fatalf("CallGraph: %v", err)
	}

	kinds := make(map[string][]string) // kind → []callee
	for _, e := range cg.Edges {
		if e.Kind != "" && e.Kind != oculus.CallEdgeSync {
			kinds[e.Kind] = append(kinds[e.Kind], e.Callee)
		}
	}

	if len(kinds[oculus.CallEdgeGoroutine]) == 0 {
		t.Errorf("expected at least one %q edge, got none\nall edges: %v", oculus.CallEdgeGoroutine, cg.Edges)
	}
	if len(kinds[oculus.CallEdgeChanSend]) == 0 {
		t.Errorf("expected at least one %q edge, got none", oculus.CallEdgeChanSend)
	}
	if len(kinds[oculus.CallEdgeChanRecv]) == 0 {
		t.Errorf("expected at least one %q edge, got none", oculus.CallEdgeChanRecv)
	}

	t.Logf("async edges: goroutine=%v send=%v recv=%v",
		kinds[oculus.CallEdgeGoroutine],
		kinds[oculus.CallEdgeChanSend],
		kinds[oculus.CallEdgeChanRecv])
}
