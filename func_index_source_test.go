package oculus

import (
	"context"
	"testing"
)

// pythonFixture simulates what a Python parser would produce.
// def load_config(path: str) -> dict: ...
// def transform(cfg: dict) -> list: ...
// def main(): cfg = load_config(...); result = transform(cfg)
var pythonFixture = []SourceFunc{
	{Name: "load_config", Package: ".", File: "main.py", Line: 1, EndLine: 2, Exported: true,
		ParamTypes: []string{"str"}, ReturnTypes: []string{"dict"}, Callees: nil},
	{Name: "transform", Package: ".", File: "main.py", Line: 4, EndLine: 5, Exported: true,
		ParamTypes: []string{"dict"}, ReturnTypes: []string{"list"}, Callees: nil},
	{Name: "main", Package: ".", File: "main.py", Line: 7, EndLine: 9, Exported: true,
		ParamTypes: nil, ReturnTypes: nil, Callees: []string{"load_config", "transform"}},
}

// TestFuncIndexSource_CallGraph_Basic tests basic walk through Pipeline.
func TestFuncIndexSource_CallGraph_Basic(t *testing.T) {
	src := NewFuncIndexSource(pythonFixture)

	p := &SymbolPipeline{Source: src, Root: "/workspace"}
	cg, err := p.CallGraph(context.Background(), "/workspace", CallGraphOpts{Entry: "main", Depth: 5})
	if err != nil {
		t.Fatalf("CallGraph: %v", err)
	}

	if len(cg.Nodes) != 3 {
		t.Errorf("nodes = %d, want 3 (main, load_config, transform)", len(cg.Nodes))
	}
	if len(cg.Edges) != 2 {
		t.Errorf("edges = %d, want 2 (main→load_config, main→transform)", len(cg.Edges))
	}
}

// TestFuncIndexSource_TypeEnrichment tests that Hover returns types from index.
func TestFuncIndexSource_TypeEnrichment(t *testing.T) {
	src := NewFuncIndexSource(pythonFixture)

	p := &SymbolPipeline{Source: src, Root: "/workspace"}
	cg, err := p.CallGraph(context.Background(), "/workspace", CallGraphOpts{Entry: "main", Depth: 5})
	if err != nil {
		t.Fatalf("CallGraph: %v", err)
	}

	typed := 0
	for _, e := range cg.Edges {
		if len(e.ParamTypes) > 0 || len(e.ReturnTypes) > 0 {
			typed++
		}
	}
	if typed != 2 {
		t.Errorf("typed edges = %d, want 2 (load_config has str→dict, transform has dict→list)", typed)
	}

	// Check specific types.
	for _, e := range cg.Edges {
		if e.Callee == "load_config" {
			if len(e.ParamTypes) == 0 || e.ParamTypes[0] != "str" {
				t.Errorf("load_config params = %v, want [str]", e.ParamTypes)
			}
			if len(e.ReturnTypes) == 0 || e.ReturnTypes[0] != "dict" {
				t.Errorf("load_config returns = %v, want [dict]", e.ReturnTypes)
			}
		}
	}
}

// TestFuncIndexSource_AllExported tests that empty query returns all exported functions.
func TestFuncIndexSource_AllExported(t *testing.T) {
	funcs := []SourceFunc{
		{Name: "Public", Package: "pkg", Exported: true},
		{Name: "_private", Package: "pkg", Exported: false},
		{Name: "AlsoPublic", Package: "pkg", Exported: true},
	}
	src := NewFuncIndexSource(funcs)

	roots, err := src.Roots(context.Background(), "")
	if err != nil {
		t.Fatalf("Roots: %v", err)
	}
	if len(roots) != 2 {
		t.Errorf("roots = %d, want 2 (Public, AlsoPublic)", len(roots))
	}
}

// TestFuncIndexSource_QueryByName tests single-entry lookup.
func TestFuncIndexSource_QueryByName(t *testing.T) {
	src := NewFuncIndexSource(pythonFixture)

	roots, err := src.Roots(context.Background(), "transform")
	if err != nil {
		t.Fatalf("Roots: %v", err)
	}
	if len(roots) != 1 {
		t.Fatalf("roots = %d, want 1", len(roots))
	}
	if roots[0].Name != "transform" {
		t.Errorf("root name = %q, want transform", roots[0].Name)
	}
}

// TestFuncIndexSource_DepthLimit tests depth limit through Pipeline.
func TestFuncIndexSource_DepthLimit(t *testing.T) {
	funcs := []SourceFunc{
		{Name: "A", Package: "p", Exported: true, Callees: []string{"B"}},
		{Name: "B", Package: "p", Exported: true, Callees: []string{"C"}},
		{Name: "C", Package: "p", Exported: true, Callees: []string{"D"}},
		{Name: "D", Package: "p", Exported: true, Callees: nil},
	}
	src := NewFuncIndexSource(funcs)

	p := &SymbolPipeline{Source: src, Root: "/workspace"}
	cg, err := p.CallGraph(context.Background(), "/workspace", CallGraphOpts{Entry: "A", Depth: 2})
	if err != nil {
		t.Fatalf("CallGraph: %v", err)
	}

	// Depth 2: A(0)→B(1)→C(2). D pruned.
	if len(cg.Nodes) != 3 {
		t.Errorf("nodes = %d, want 3 (A, B, C)", len(cg.Nodes))
	}
	for _, n := range cg.Nodes {
		if n.Name == "D" {
			t.Error("D should be pruned at depth 2")
		}
	}
}

// TestFuncIndexSource_CrossPkg tests cross-package edge marking.
func TestFuncIndexSource_CrossPkg(t *testing.T) {
	funcs := []SourceFunc{
		{Name: "Handler", Package: "api", Exported: true, Callees: []string{"Query", "Helper"}},
		{Name: "Query", Package: "db", Exported: true},
		{Name: "Helper", Package: "api", Exported: true},
	}
	src := NewFuncIndexSource(funcs)

	p := &SymbolPipeline{Source: src, Root: "/workspace"}
	cg, err := p.CallGraph(context.Background(), "/workspace", CallGraphOpts{Entry: "Handler", Depth: 5})
	if err != nil {
		t.Fatalf("CallGraph: %v", err)
	}

	var cross, same int
	for _, e := range cg.Edges {
		if e.CrossPkg {
			cross++
		} else {
			same++
		}
	}
	if cross != 1 {
		t.Errorf("cross-pkg = %d, want 1 (Handler→Query)", cross)
	}
	if same != 1 {
		t.Errorf("same-pkg = %d, want 1 (Handler→Helper)", same)
	}
}
