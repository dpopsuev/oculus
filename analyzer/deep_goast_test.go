package analyzer

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/dpopsuev/oculus/v3"
	"github.com/dpopsuev/oculus/v3/internal/testfixture"
)

func TestGoASTCallGraph_SyntheticRepository(t *testing.T) {
	root := testfixture.Repository(t, "go")
	analyzer := NewGoASTDeep(root)
	if analyzer == nil {
		t.Fatal("expected Go AST analyzer")
	}
	callGraph, err := analyzer.CallGraph(context.Background(), root, oculus.CallGraphOpts{Entry: "main", Depth: 3})
	if err != nil {
		t.Fatalf("CallGraph: %v", err)
	}
	if len(callGraph.Nodes) == 0 || len(callGraph.Edges) == 0 {
		t.Fatalf("call graph nodes=%d edges=%d", len(callGraph.Nodes), len(callGraph.Edges))
	}
	if callGraph.Layer != oculus.LayerGoAST {
		t.Errorf("layer = %q, want goast", callGraph.Layer)
	}
}

func TestGoASTCallGraph_WithEntry(t *testing.T) {
	root := testfixture.Repository(t, "go")
	analyzer := NewGoASTDeep(root)
	callGraph, err := analyzer.CallGraph(context.Background(), root, oculus.CallGraphOpts{Entry: "main", Depth: 3})
	if err != nil {
		t.Fatalf("CallGraph: %v", err)
	}
	found := false
	for _, node := range callGraph.Nodes {
		if node.Name == "main" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected main node")
	}
	if len(callGraph.Edges) == 0 {
		t.Error("expected edges from main")
	}
}

func TestGoASTDataFlowTrace(t *testing.T) {
	root := testfixture.Repository(t, "go")
	analyzer := NewGoASTDeep(root)
	dataFlow, err := analyzer.DataFlowTrace(context.Background(), root, "main", 3)
	if err != nil {
		t.Fatalf("DataFlowTrace: %v", err)
	}
	if len(dataFlow.Nodes) == 0 {
		t.Error("expected data-flow nodes")
	}
	if dataFlow.Layer != oculus.LayerGoAST {
		t.Errorf("layer = %q, want goast", dataFlow.Layer)
	}
}

func TestGoASTDeep_NonGoRepo(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "pyproject.toml"), []byte("[project]\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if analyzer := NewGoASTDeep(dir); analyzer != nil {
		t.Error("expected nil for non-Go repository")
	}
}

func TestGoASTFallbackIntegration(t *testing.T) {
	root := testfixture.Repository(t, "go")
	fallback := NewDeepFallback(root, nil)
	callGraph, err := fallback.CallGraph(context.Background(), root, oculus.CallGraphOpts{Entry: "main", Depth: 3})
	if err != nil {
		t.Fatalf("CallGraph: %v", err)
	}
	if len(callGraph.Edges) == 0 {
		t.Error("expected fallback edges")
	}
}
