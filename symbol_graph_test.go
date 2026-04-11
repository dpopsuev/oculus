package oculus

import "testing"

func TestMergeSymbolGraph_CallEdgesOnly(t *testing.T) {
	cg := &CallGraph{
		Nodes: []Symbol{
			{Name: "main", Package: "cmd/app"},
			{Name: "Run", Package: "internal/core"},
		},
		Edges: []CallEdge{
			{Caller: "main", Callee: "Run", CallerPkg: "cmd/app", CalleePkg: "internal/core"},
		},
	}
	sg := MergeSymbolGraph(cg, nil, nil, nil)
	if len(sg.Nodes) != 2 {
		t.Errorf("nodes = %d, want 2", len(sg.Nodes))
	}
	if len(sg.Edges) != 1 {
		t.Errorf("edges = %d, want 1", len(sg.Edges))
	}
	if sg.Edges[0].Kind != "call" {
		t.Errorf("kind = %q, want call", sg.Edges[0].Kind)
	}
}

func TestMergeSymbolGraph_ImplEdges(t *testing.T) {
	classes := []ClassInfo{
		{Name: "Service", Package: "core", Kind: "struct"},
		{Name: "Runner", Package: "core", Kind: "interface"},
	}
	impls := []ImplEdge{
		{From: "core.Service", To: "core.Runner", Kind: "implements"},
	}
	sg := MergeSymbolGraph(nil, classes, impls, nil)
	if len(sg.Nodes) != 2 {
		t.Errorf("nodes = %d, want 2", len(sg.Nodes))
	}
	if len(sg.Edges) != 1 {
		t.Errorf("edges = %d, want 1", len(sg.Edges))
	}
	if sg.Edges[0].Kind != "implements" {
		t.Errorf("kind = %q, want implements", sg.Edges[0].Kind)
	}
}

func TestMergeSymbolGraph_FieldRefs(t *testing.T) {
	refs := []FieldRef{
		{Owner: "core.Config", Field: "DB", RefType: "store.Store"},
	}
	sg := MergeSymbolGraph(nil, nil, nil, refs)
	if len(sg.Edges) != 1 {
		t.Errorf("edges = %d, want 1", len(sg.Edges))
	}
	if sg.Edges[0].Kind != "field_ref" {
		t.Errorf("kind = %q, want field_ref", sg.Edges[0].Kind)
	}
}

func TestMergeSymbolGraph_Combined(t *testing.T) {
	cg := &CallGraph{
		Nodes: []Symbol{
			{Name: "main", Package: "cmd"},
			{Name: "Run", Package: "core"},
		},
		Edges: []CallEdge{
			{Caller: "main", Callee: "Run", CallerPkg: "cmd", CalleePkg: "core"},
		},
	}
	classes := []ClassInfo{
		{Name: "Service", Package: "core", Kind: "struct"},
	}
	impls := []ImplEdge{
		{From: "core.Service", To: "core.Runner", Kind: "implements"},
	}
	refs := []FieldRef{
		{Owner: "core.Config", Field: "DB", RefType: "store.Store"},
	}
	sg := MergeSymbolGraph(cg, classes, impls, refs)
	// Nodes: main, Run (from cg) + Service (from classes) = 3
	if len(sg.Nodes) < 3 {
		t.Errorf("nodes = %d, want >= 3", len(sg.Nodes))
	}
	// Edges: 1 call + 1 implements + 1 field_ref = 3
	if len(sg.Edges) != 3 {
		t.Errorf("edges = %d, want 3", len(sg.Edges))
	}
}

func TestMergeSymbolGraph_NilCallGraph(t *testing.T) {
	sg := MergeSymbolGraph(nil, nil, nil, nil)
	if sg == nil {
		t.Fatal("expected non-nil SymbolGraph")
	}
	if len(sg.Nodes) != 0 {
		t.Errorf("nodes = %d, want 0", len(sg.Nodes))
	}
}

func TestMergeSymbolGraph_FQNFormat(t *testing.T) {
	cg := &CallGraph{
		Nodes: []Symbol{{Name: "Run", Package: "core"}},
		Edges: []CallEdge{
			{Caller: "Run", Callee: "Get", CallerPkg: "core", CalleePkg: "store"},
		},
	}
	sg := MergeSymbolGraph(cg, nil, nil, nil)
	if len(sg.Edges) != 1 {
		t.Fatal("expected 1 edge")
	}
	if sg.Edges[0].Source() != "core.Run" {
		t.Errorf("Source() = %q, want core.Run", sg.Edges[0].Source())
	}
	if sg.Edges[0].Target() != "store.Get" {
		t.Errorf("Target() = %q, want store.Get", sg.Edges[0].Target())
	}
}

func TestMergeSymbolGraph_Dedup(t *testing.T) {
	cg := &CallGraph{
		Nodes: []Symbol{
			{Name: "Run", Package: "core"},
			{Name: "Run", Package: "core"}, // duplicate node
			{Name: "Get", Package: "store"},
		},
		Edges: []CallEdge{
			{Caller: "Run", Callee: "Get", CallerPkg: "core", CalleePkg: "store"},
			{Caller: "Run", Callee: "Get", CallerPkg: "core", CalleePkg: "store"}, // duplicate edge
		},
	}
	sg := MergeSymbolGraph(cg, nil, nil, nil)
	if len(sg.Nodes) != 2 { // Run + Get (deduped)
		t.Errorf("nodes = %d, want 2 (deduped)", len(sg.Nodes))
	}
	if len(sg.Edges) != 1 {
		t.Errorf("edges = %d, want 1 (deduped)", len(sg.Edges))
	}
}
