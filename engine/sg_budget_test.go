package engine

import (
	"testing"

	oculus "github.com/dpopsuev/oculus/v3"
)

func TestOverlayCallGraph_MergesLSPEdges(t *testing.T) {
	sg := &oculus.SymbolGraph{
		QualityTier: "ast",
		Nodes: []oculus.Symbol{
			{Name: "normalizeSessionTags", Package: "session"},
		},
		Edges: []oculus.SymbolEdge{
			{SourceFQN: "session.normalizeSessionTags", TargetFQN: "session.normalizeSessionTag", Kind: "call"},
		},
	}
	cg := &oculus.CallGraph{
		Nodes: []oculus.Symbol{
			{Name: "normalizeSessionTags", Package: "session"},
			{Name: "helper", Package: "session"},
		},
		Edges: []oculus.CallEdge{
			{Caller: "normalizeSessionTags", CallerPkg: "session", Callee: "helper", CalleePkg: "session", Kind: "call", File: "tags.ts", Line: 76},
		},
	}
	overlayCallGraph(sg, cg)
	if len(sg.Edges) != 2 {
		t.Fatalf("edges=%d want 2", len(sg.Edges))
	}
	found := false
	for _, e := range sg.Edges {
		if e.TargetFQN == "session.helper" && e.Layer == "lsp" && e.Line == 76 {
			found = true
		}
	}
	if !found {
		t.Fatalf("missing LSP overlay edge: %+v", sg.Edges)
	}
}

func TestNormalize_AllowLSP(t *testing.T) {
	o := SymbolGraphOpts{AllowLSP: true, FocusEntry: "Foo"}.normalize()
	if !o.AllowLSP || o.Quick {
		t.Fatalf("got %+v", o)
	}
	if !o.Interactive {
		t.Fatal("focus+AllowLSP must be interactive")
	}
}
