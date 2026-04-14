package analyzer

import (
	"context"
	"testing"

	"github.com/dpopsuev/oculus/v3"
)

// TestGoAST_FuncIndexSource runs GoAST through FuncIndexSource + Pipeline.
func TestGoAST_FuncIndexSource(t *testing.T) {
	funcs := ParseGoASTFunctions("../")
	if len(funcs) == 0 {
		t.Skip("not a Go project")
	}
	t.Logf("GoAST found %d functions", len(funcs))

	src := oculus.NewFuncIndexSource(funcs)
	p := &oculus.SymbolPipeline{Source: src, Root: "../", Concurrency: 4}

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

// TestTreeSitter_FuncIndexSource runs TreeSitter through FuncIndexSource + Pipeline.
func TestTreeSitter_FuncIndexSource(t *testing.T) {
	funcs := ParseTreeSitterFunctions("../")
	if len(funcs) == 0 {
		t.Skip("could not parse project")
	}
	t.Logf("TreeSitter found %d functions", len(funcs))

	src := oculus.NewFuncIndexSource(funcs)
	p := &oculus.SymbolPipeline{Source: src, Root: "../", Concurrency: 4}

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

// TestGoAST_TypeEnrichment tests that Hover returns type info via FuncIndexSource.
func TestGoAST_TypeEnrichment(t *testing.T) {
	funcs := ParseGoASTFunctions("../")
	if len(funcs) == 0 {
		t.Skip("not a Go project")
	}

	src := oculus.NewFuncIndexSource(funcs)
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
