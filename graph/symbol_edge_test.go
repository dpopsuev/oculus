package graph_test

import (
	"testing"

	"github.com/dpopsuev/oculus"
	"github.com/dpopsuev/oculus/graph"
)

// symbolEdges returns a small acyclic graph for contract testing.
// Topology: cmd/app.main → internal/core.Run → internal/store.Get
//           internal/core.Service --implements--> internal/core.Runner
func symbolEdges() []oculus.SymbolEdge {
	return []oculus.SymbolEdge{
		{SourceFQN: "cmd/app.main", TargetFQN: "internal/core.Run", Kind: "call"},
		{SourceFQN: "internal/core.Run", TargetFQN: "internal/store.Get", Kind: "call"},
		{SourceFQN: "internal/core.Service", TargetFQN: "internal/core.Runner", Kind: "implements"},
	}
}

func TestSymbolEdge_SatisfiesEdge(t *testing.T) {
	var e graph.Edge = oculus.SymbolEdge{SourceFQN: "a.X", TargetFQN: "b.Y"}
	if e.Source() != "a.X" || e.Target() != "b.Y" {
		t.Errorf("Source=%q Target=%q", e.Source(), e.Target())
	}
}

func TestSymbolEdge_FanIn(t *testing.T) {
	fi := graph.FanIn(symbolEdges())
	if fi["internal/core.Run"] != 1 {
		t.Errorf("FanIn(Run) = %d, want 1", fi["internal/core.Run"])
	}
	if fi["internal/store.Get"] != 1 {
		t.Errorf("FanIn(Get) = %d, want 1", fi["internal/store.Get"])
	}
}

func TestSymbolEdge_FanOut(t *testing.T) {
	fo := graph.FanOut(symbolEdges())
	if fo["cmd/app.main"] != 1 {
		t.Errorf("FanOut(main) = %d, want 1", fo["cmd/app.main"])
	}
	if fo["internal/core.Run"] != 1 {
		t.Errorf("FanOut(Run) = %d, want 1", fo["internal/core.Run"])
	}
}

func TestSymbolEdge_DetectCycles(t *testing.T) {
	cycles := graph.DetectCycles(symbolEdges())
	if len(cycles) != 0 {
		t.Errorf("expected 0 cycles in acyclic graph, got %d", len(cycles))
	}
}

func TestSymbolEdge_DetectCycles_WithCycle(t *testing.T) {
	edges := append(symbolEdges(), oculus.SymbolEdge{
		SourceFQN: "internal/store.Get", TargetFQN: "cmd/app.main", Kind: "call",
	})
	cycles := graph.DetectCycles(edges)
	if len(cycles) == 0 {
		t.Error("expected cycle in circular graph")
	}
}

func TestSymbolEdge_ImportDepth(t *testing.T) {
	depths := graph.ImportDepth(symbolEdges())
	// ImportDepth: roots (no incoming) = 0, downstream = higher depth
	if depths["cmd/app.main"] != 0 {
		t.Errorf("depth(main) = %d, want 0 (root)", depths["cmd/app.main"])
	}
	if depths["internal/store.Get"] < 1 {
		t.Errorf("depth(Get) = %d, want >= 1 (downstream)", depths["internal/store.Get"])
	}
}

func TestSymbolEdge_ShortestPath(t *testing.T) {
	path, found := graph.ShortestPath(symbolEdges(), "cmd/app.main", "internal/store.Get")
	if !found {
		t.Fatal("expected path from main to Get")
	}
	if len(path) != 3 {
		t.Errorf("path length = %d, want 3 (main→Run→Get)", len(path))
	}
}

func TestSymbolEdge_ShortestPath_NoPath(t *testing.T) {
	_, found := graph.ShortestPath(symbolEdges(), "internal/store.Get", "cmd/app.main")
	if found {
		t.Error("expected no path from Get to main in acyclic graph")
	}
}

func TestSymbolEdge_TopologicalSort(t *testing.T) {
	order, err := graph.TopologicalSort(symbolEdges())
	if err != nil {
		t.Fatal(err)
	}
	if len(order) == 0 {
		t.Error("expected non-empty topological order")
	}
}

func TestSymbolEdge_ConnectedComponents(t *testing.T) {
	components := graph.ConnectedComponents(symbolEdges())
	// All nodes are connected (call chain + implements share internal/core)
	if len(components) == 0 {
		t.Error("expected at least 1 connected component")
	}
}

func TestSymbolEdge_Partition(t *testing.T) {
	order := graph.Partition(symbolEdges())
	if len(order) == 0 {
		t.Error("expected non-empty partition order")
	}
}

func TestSymbolEdge_CheckLayerPurity(t *testing.T) {
	layers := []string{"internal/store.Get", "internal/core.Run", "cmd/app.main"}
	violations := graph.CheckLayerPurity(symbolEdges(), layers)
	// All edges go from higher layer to lower — no violations
	if len(violations) != 0 {
		t.Errorf("expected 0 violations in correctly-layered graph, got %d", len(violations))
	}
}

func TestSymbolNode_FQN(t *testing.T) {
	tests := []struct {
		node oculus.Symbol
		want string
	}{
		{oculus.Symbol{Name: "Run", Package: "internal/core"}, "internal/core.Run"},
		{oculus.Symbol{Name: "main", Package: ""}, "main"},
	}
	for _, tt := range tests {
		if got := tt.node.FQN(); got != tt.want {
			t.Errorf("FQN() = %q, want %q", got, tt.want)
		}
	}
}
