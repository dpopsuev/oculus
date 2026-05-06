package oculus

import "testing"

func TestProbe_LeafDetection(t *testing.T) {
	sg := &SymbolGraph{
		Nodes: []Symbol{
			{Name: "main", Package: "main", Kind: "function", Exported: true},
			{Name: "process", Package: "main", Kind: "function", Exported: true},
			{Name: "leaf", Package: "main", Kind: "function", Exported: true},
			{Name: "orphan", Package: "main", Kind: "function", Exported: true},
		},
		Edges: []SymbolEdge{
			{SourceFQN: "main.main", TargetFQN: "main.process", Kind: "call"},
			{SourceFQN: "main.process", TargetFQN: "main.leaf", Kind: "call"},
		},
	}

	cases := []struct {
		symbol   string
		wantLeaf bool
		wantFanIn  int
		wantFanOut int
	}{
		{"main.main", false, 0, 1},      // entry point: fan_out>0, fan_in=0 → not leaf (it's a root)
		{"main.process", false, 1, 1},    // middle: has both callers and callees
		{"main.leaf", true, 1, 0},        // leaf: has callers but no callees
		{"main.orphan", false, 0, 0},     // orphan: no edges at all — not a leaf, just disconnected
	}

	for _, tc := range cases {
		t.Run(tc.symbol, func(t *testing.T) {
			r := Probe(sg, tc.symbol)
			if r == nil {
				t.Fatalf("Probe(%s) returned nil", tc.symbol)
			}
			if r.Leaf != tc.wantLeaf {
				t.Errorf("Leaf=%v, want %v (fan_in=%d, fan_out=%d)", r.Leaf, tc.wantLeaf, r.FanIn, r.FanOut)
			}
			if r.FanIn != tc.wantFanIn {
				t.Errorf("FanIn=%d, want %d", r.FanIn, tc.wantFanIn)
			}
			if r.FanOut != tc.wantFanOut {
				t.Errorf("FanOut=%d, want %d", r.FanOut, tc.wantFanOut)
			}
		})
	}
}

func TestScenario_GraphEdgeCount(t *testing.T) {
	sg := &SymbolGraph{
		Nodes: []Symbol{
			{Name: "main", Package: "main", Kind: "function"},
			{Name: "process", Package: "main", Kind: "function"},
		},
		Edges: []SymbolEdge{
			{SourceFQN: "main.main", TargetFQN: "main.process", Kind: "call"},
		},
	}

	r := TraceScenario(sg, "main", 5, false, 0)
	if r == nil {
		t.Fatal("TraceScenario returned nil")
	}
	if r.GraphEdgeCount != 1 {
		t.Errorf("GraphEdgeCount=%d, want 1", r.GraphEdgeCount)
	}

	// Empty graph
	emptySG := &SymbolGraph{
		Nodes: []Symbol{{Name: "lonely", Package: "main", Kind: "function"}},
	}
	r2 := TraceScenario(emptySG, "lonely", 5, false, 0)
	if r2 != nil {
		t.Logf("scenario on empty graph: edges=%d, GraphEdgeCount=%d", len(r2.Edges), r2.GraphEdgeCount)
		if r2.GraphEdgeCount != 0 {
			t.Errorf("GraphEdgeCount=%d on empty graph, want 0", r2.GraphEdgeCount)
		}
	}
}

func TestConvergence_GraphEdgeCount(t *testing.T) {
	sg := &SymbolGraph{
		Nodes: []Symbol{
			{Name: "a", Package: "main", Kind: "function"},
			{Name: "b", Package: "main", Kind: "function"},
			{Name: "shared", Package: "main", Kind: "function"},
		},
		Edges: []SymbolEdge{
			{SourceFQN: "main.a", TargetFQN: "main.shared", Kind: "call"},
			{SourceFQN: "main.b", TargetFQN: "main.shared", Kind: "call"},
		},
	}

	r := FindConvergence(sg, []string{"a", "b"}, 0)
	if r == nil {
		t.Fatal("FindConvergence returned nil")
	}
	if r.GraphEdgeCount != 2 {
		t.Errorf("GraphEdgeCount=%d, want 2", r.GraphEdgeCount)
	}
}
