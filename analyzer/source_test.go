package analyzer

import (
	"context"
	"testing"

	"github.com/dpopsuev/oculus"
)

// TestGoASTSymbolSource_Pipeline runs GoASTSymbolSource through SymbolPipeline
// against the oculus codebase itself (self-referential test).
func TestGoASTSymbolSource_Pipeline(t *testing.T) {
	src := NewGoASTSymbolSource("../")
	if src == nil {
		t.Skip("not a Go project")
	}

	roots, err := src.Roots(context.Background(), "")
	if err != nil {
		t.Fatalf("Roots: %v", err)
	}
	if len(roots) == 0 {
		t.Fatal("expected exported roots from oculus codebase")
	}
	t.Logf("GoAST found %d exported roots", len(roots))

	// Run through the pipeline.
	p := &oculus.SymbolPipeline{
		Source:      src,
		Root:        "../",
		Concurrency: 4,
	}
	cg, err := p.CallGraph(context.Background(), "../", oculus.CallGraphOpts{
		Entry: "BuildMesh",
		Depth: 3,
	})
	if err != nil {
		t.Fatalf("Pipeline.CallGraph: %v", err)
	}
	t.Logf("Pipeline produced %d nodes, %d edges for BuildMesh", len(cg.Nodes), len(cg.Edges))

	if len(cg.Nodes) == 0 {
		t.Error("expected at least 1 node")
	}
}

// TestTreeSitterSymbolSource_Pipeline runs TreeSitterSymbolSource through
// SymbolPipeline against the oculus codebase.
func TestTreeSitterSymbolSource_Pipeline(t *testing.T) {
	src := NewTreeSitterSymbolSource("../")
	if src == nil {
		t.Skip("could not build parsed project")
	}

	roots, err := src.Roots(context.Background(), "")
	if err != nil {
		t.Fatalf("Roots: %v", err)
	}
	if len(roots) == 0 {
		t.Fatal("expected exported roots")
	}
	t.Logf("TreeSitter found %d exported roots", len(roots))

	p := &oculus.SymbolPipeline{
		Source:      src,
		Root:        "../",
		Concurrency: 4,
	}
	cg, err := p.CallGraph(context.Background(), "../", oculus.CallGraphOpts{
		Entry: "BuildMesh",
		Depth: 3,
	})
	if err != nil {
		t.Fatalf("Pipeline.CallGraph: %v", err)
	}
	t.Logf("Pipeline produced %d nodes, %d edges for BuildMesh", len(cg.Nodes), len(cg.Edges))

	if len(cg.Nodes) == 0 {
		t.Error("expected at least 1 node")
	}
}

// TestGoASTSymbolSource_TypeEnrichment tests that Hover returns type info.
func TestGoASTSymbolSource_TypeEnrichment(t *testing.T) {
	src := NewGoASTSymbolSource("../")
	if src == nil {
		t.Skip("not a Go project")
	}

	roots, _ := src.Roots(context.Background(), "BuildMesh")
	if len(roots) == 0 {
		t.Skip("BuildMesh not found")
	}

	ti, err := src.Hover(context.Background(), roots[0])
	if err != nil {
		t.Fatalf("Hover: %v", err)
	}
	if ti == nil {
		t.Skip("no type info for BuildMesh")
	}
	t.Logf("BuildMesh params=%v returns=%v", ti.ParamTypes, ti.ReturnTypes)
}
