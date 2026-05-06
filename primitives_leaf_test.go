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
		symbol     string
		wantLeaf   bool
		wantStatus CallGraphStatus
		wantFanIn  int
		wantFanOut int
	}{
		{"main.main", false, CallGraphCovered, 0, 1},
		{"main.process", false, CallGraphCovered, 1, 1},
		{"main.leaf", true, CallGraphCovered, 1, 0},
		{"main.orphan", false, CallGraphNotCovered, 0, 0},
	}

	for _, tc := range cases {
		t.Run(tc.symbol, func(t *testing.T) {
			r := Probe(sg, tc.symbol)
			if r == nil {
				t.Fatalf("Probe(%s) returned nil", tc.symbol)
			}
			if r.Leaf != tc.wantLeaf {
				t.Errorf("Leaf=%v, want %v", r.Leaf, tc.wantLeaf)
			}
			if r.CallGraphStatus != tc.wantStatus {
				t.Errorf("CallGraphStatus=%s, want %s", r.CallGraphStatus, tc.wantStatus)
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

func TestProbe_CallGraphStatus_NoGraph(t *testing.T) {
	sg := &SymbolGraph{
		Nodes: []Symbol{
			{Name: "lonely", Package: "main", Kind: "function", Exported: true},
		},
		Edges: nil,
	}

	r := Probe(sg, "main.lonely")
	if r == nil {
		t.Fatal("Probe returned nil")
	}
	if r.CallGraphStatus != CallGraphNone {
		t.Errorf("CallGraphStatus=%s, want %s", r.CallGraphStatus, CallGraphNone)
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
