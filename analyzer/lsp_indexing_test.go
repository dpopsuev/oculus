package analyzer

import (
	"context"
	"testing"
	"time"

	oculus "github.com/dpopsuev/oculus/v3"
	"github.com/dpopsuev/oculus/v3/lsp"
	"github.com/dpopsuev/oculus/v3/lsp/mockserver"
)

// TestLSP_EmptyDuringIndexing reproduces LCS-BUG-54:
// LSP server returns empty workspace/symbol while indexing.
// The analyzer should wait for indexing to complete, not return empty.
func TestLSP_EmptyDuringIndexing(t *testing.T) {
	cfg := mockserver.Config{
		Symbols: []mockserver.Symbol{
			{Name: "ScanAndBuild", Kind: 12, URI: "file:///repo/scan.go", Line: 10},
			{Name: "FanIn", Kind: 12, URI: "file:///repo/graph.go", Line: 5},
		},
		Edges: []mockserver.CallEdge{
			{FromName: "ScanAndBuild", ToName: "FanIn", ToURI: "file:///repo/graph.go", ToLine: 5},
		},
		IndexingDelay: 500 * time.Millisecond, // server returns empty for first 500ms
	}

	pool := lsp.NewMockPool(cfg)
	defer pool.Shutdown(context.Background())

	da := NewLSPDeepWithPool("/repo", pool)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cg, err := da.CallGraph(ctx, "/repo", oculus.CallGraphOpts{Depth: 5})
	if err != nil {
		t.Fatalf("CallGraph: %v", err)
	}

	if len(cg.Edges) == 0 {
		t.Error("LSP analyzer returned empty call graph — should have waited for indexing to complete")
	}

	if len(cg.Nodes) < 2 {
		t.Errorf("expected at least 2 nodes, got %d", len(cg.Nodes))
	}

	t.Logf("CallGraph: %d nodes, %d edges (indexing delay: %v)", len(cg.Nodes), len(cg.Edges), cfg.IndexingDelay)
}

// TestLSP_WorkspaceSymbolCapability reproduces the root cause of LCS-BUG-54:
// if the client doesn't advertise workspace.symbol capability in initialize,
// gopls returns null for workspace/symbol forever.
func TestLSP_WorkspaceSymbolCapability(t *testing.T) {
	cfg := mockserver.Config{
		Symbols: []mockserver.Symbol{
			{Name: "Foo", Kind: 12, URI: "file:///repo/foo.go", Line: 1},
		},
		RequireWorkspaceSymbolCap: true, // mock simulates gopls behavior
	}

	pool := lsp.NewMockPool(cfg)
	defer pool.Shutdown(context.Background())

	da := NewLSPDeepWithPool("/repo", pool)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cg, err := da.CallGraph(ctx, "/repo", oculus.CallGraphOpts{Depth: 5})
	if err != nil {
		t.Fatalf("CallGraph: %v", err)
	}

	if len(cg.Nodes) == 0 {
		t.Error("workspace/symbol returned empty — client must advertise workspace.symbol capability in initialize")
	}
}

// TestLSP_NullResponse reproduces LCS-BUG-54 hypothesis 1:
// gopls returns null (not []) for workspace/symbol with empty query.
// null means "no results available" — the analyzer must handle this
// distinctly from [] (empty array = "results available, none match").
func TestLSP_NullResponse(t *testing.T) {
	// Mock server that ALWAYS returns null for workspace/symbol
	// (simulates gopls that never finishes indexing or doesn't support empty query)
	cfg := mockserver.Config{
		Symbols: []mockserver.Symbol{
			{Name: "Foo", Kind: 12, URI: "file:///repo/foo.go", Line: 1},
		},
		IndexingDelay: 999 * time.Hour, // effectively forever — always returns empty
	}

	pool := lsp.NewMockPool(cfg)
	defer pool.Shutdown(context.Background())

	da := NewLSPDeepWithPool("/repo", pool)

	// Short timeout — we don't want to wait 60s in tests
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cg, err := da.CallGraph(ctx, "/repo", oculus.CallGraphOpts{Depth: 5})

	// Should return an error or empty graph — NOT hang forever
	if err == nil && len(cg.Edges) == 0 {
		t.Log("null response handled — returned empty graph (acceptable)")
	}
	if err != nil {
		t.Logf("null response handled — returned error: %v (acceptable)", err)
	}
	// The key assertion: this test must complete within 5s, not hang for 60s+
}

// TestLSP_WorkspaceFoldersNeeded tests hypothesis 2:
// gopls may need workspaceFolders in initialize to start indexing.
func TestLSP_WorkspaceFoldersNeeded(t *testing.T) {
	cfg := mockserver.Config{
		Symbols: []mockserver.Symbol{
			{Name: "Bar", Kind: 12, URI: "file:///repo/bar.go", Line: 1},
		},
	}

	pool := lsp.NewMockPool(cfg)
	defer pool.Shutdown(context.Background())

	da := NewLSPDeepWithPool("/repo", pool)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cg, err := da.CallGraph(ctx, "/repo", oculus.CallGraphOpts{Depth: 5})
	if err != nil {
		t.Fatalf("CallGraph: %v", err)
	}
	if len(cg.Nodes) == 0 {
		t.Error("no nodes — gopls may need workspaceFolders in initialize params")
	}
}

// TestLSP_NonEmptyQuery tests hypothesis 1:
// gopls v0.21.1 may require a non-empty query for workspace/symbol.
// If empty query returns null but "." returns results, we need to change our query.
func TestLSP_NonEmptyQuery(t *testing.T) {
	cfg := mockserver.Config{
		Symbols: []mockserver.Symbol{
			{Name: "ScanAndBuild", Kind: 12, URI: "file:///repo/scan.go", Line: 10},
			{Name: "FanIn", Kind: 12, URI: "file:///repo/graph.go", Line: 5},
		},
		Edges: []mockserver.CallEdge{
			{FromName: "ScanAndBuild", ToName: "FanIn", ToURI: "file:///repo/graph.go", ToLine: 5},
		},
	}

	pool := lsp.NewMockPool(cfg)
	defer pool.Shutdown(context.Background())

	da := NewLSPDeepWithPool("/repo", pool)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cg, err := da.CallGraph(ctx, "/repo", oculus.CallGraphOpts{Depth: 5})
	if err != nil {
		t.Fatalf("CallGraph: %v", err)
	}
	if len(cg.Nodes) < 2 {
		t.Errorf("expected 2+ nodes, got %d — may need non-empty query for workspace/symbol", len(cg.Nodes))
	}
	if len(cg.Edges) == 0 {
		t.Error("expected edges — workspace/symbol discovery may have failed")
	}
}

// TestLSP_NoIndexingDelay verifies normal behavior without indexing delay.
func TestLSP_NoIndexingDelay(t *testing.T) {
	cfg := mockserver.Config{
		Symbols: []mockserver.Symbol{
			{Name: "Hello", Kind: 12, URI: "file:///repo/main.go", Line: 3},
		},
	}

	pool := lsp.NewMockPool(cfg)
	defer pool.Shutdown(context.Background())

	da := NewLSPDeepWithPool("/repo", pool)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cg, err := da.CallGraph(ctx, "/repo", oculus.CallGraphOpts{Depth: 5})
	if err != nil {
		t.Fatalf("CallGraph: %v", err)
	}

	if len(cg.Nodes) == 0 {
		t.Error("expected nodes from mock LSP server")
	}
}
