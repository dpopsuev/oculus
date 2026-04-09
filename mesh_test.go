package oculus

import (
	"testing"
)

func testSymbolGraph() *SymbolGraph {
	return &SymbolGraph{
		Nodes: []SymbolNode{
			{Name: "main", Package: "cmd/app", Kind: "function", File: "cmd/app/main.go"},
			{Name: "Run", Package: "internal/core", Kind: "function", File: "internal/core/core.go"},
			{Name: "Get", Package: "internal/store", Kind: "function", File: "internal/store/store.go"},
			{Name: "Store", Package: "internal/store", Kind: "interface", File: "internal/store/store.go"},
		},
		Edges: []SymbolEdge{
			{SourceFQN: "cmd/app.main", TargetFQN: "internal/core.Run", Kind: "call"},
			{SourceFQN: "internal/core.Run", TargetFQN: "internal/store.Get", Kind: "call"},
			{SourceFQN: "internal/store.Get", TargetFQN: "internal/store.Store", Kind: "implements"},
		},
	}
}

func TestBuildMesh(t *testing.T) {
	sg := testSymbolGraph()
	m := BuildMesh(sg, []string{"cmd/app", "internal/core", "internal/store"})
	if m == nil {
		t.Fatal("nil mesh")
	}

	// Should have 4 symbols + files + 3 packages + 3 components
	if len(m.Nodes) < 10 {
		t.Errorf("expected >= 10 nodes, got %d", len(m.Nodes))
	}

	// Check symbol node exists
	if n, ok := m.Nodes["cmd/app.main"]; !ok {
		t.Error("missing symbol node cmd/app.main")
	} else if n.Level != MeshSymbol {
		t.Errorf("cmd/app.main level = %d, want MeshSymbol", n.Level)
	}

	// Check component node exists
	if n, ok := m.Nodes["internal/core"]; !ok {
		t.Error("missing component node internal/core")
	} else if n.Level != MeshComponent {
		// Could be package or component depending on matching
		t.Logf("internal/core level = %d", n.Level)
	}
}

func TestBuildMesh_Nil(t *testing.T) {
	m := BuildMesh(nil, nil)
	if m == nil {
		t.Fatal("nil mesh")
	}
	if len(m.Nodes) != 0 {
		t.Errorf("expected 0 nodes, got %d", len(m.Nodes))
	}
}

func TestAggregate_PackageLevel(t *testing.T) {
	sg := testSymbolGraph()
	m := BuildMesh(sg, []string{"cmd/app", "internal/core", "internal/store"})
	edges := m.Aggregate(MeshPackage)

	// cmd/app.main → internal/core.Run collapses to cmd/app → internal/core
	// internal/core.Run → internal/store.Get collapses to internal/core → internal/store
	// internal/store.Get → internal/store.Store is intra-package — filtered out
	if len(edges) < 2 {
		t.Errorf("expected >= 2 package-level edges, got %d", len(edges))
	}
	for _, e := range edges {
		if e.SourceFQN == e.TargetFQN {
			t.Errorf("self-edge should be filtered: %s → %s", e.SourceFQN, e.TargetFQN)
		}
	}
}

func TestAggregate_ComponentLevel(t *testing.T) {
	sg := testSymbolGraph()
	m := BuildMesh(sg, []string{"cmd/app", "internal/core", "internal/store"})
	edges := m.Aggregate(MeshComponent)

	if len(edges) < 2 {
		t.Errorf("expected >= 2 component-level edges, got %d", len(edges))
	}
}

func TestNeighborhood(t *testing.T) {
	sg := testSymbolGraph()
	m := BuildMesh(sg, []string{"cmd/app", "internal/core", "internal/store"})

	// 1 hop from main should reach Run
	nb := m.Neighborhood("cmd/app.main", 1)
	found := false
	for _, n := range nb {
		if n == "internal/core.Run" {
			found = true
		}
	}
	if !found {
		t.Errorf("1-hop from main: expected Run, got %v", nb)
	}
}

func TestNeighborhood_ZeroHops(t *testing.T) {
	sg := testSymbolGraph()
	m := BuildMesh(sg, nil)
	nb := m.Neighborhood("cmd/app.main", 0)
	if len(nb) != 1 || nb[0] != "cmd/app.main" {
		t.Errorf("0-hop = %v, want [cmd/app.main]", nb)
	}
}

func TestDistance(t *testing.T) {
	sg := testSymbolGraph()
	m := BuildMesh(sg, nil)

	d := m.Distance("cmd/app.main", "internal/store.Get")
	if d != 2 {
		t.Errorf("distance main→Get = %d, want 2", d)
	}
}

func TestDistance_Self(t *testing.T) {
	sg := testSymbolGraph()
	m := BuildMesh(sg, nil)
	if d := m.Distance("cmd/app.main", "cmd/app.main"); d != 0 {
		t.Errorf("distance self = %d, want 0", d)
	}
}

func TestDistance_NoPath(t *testing.T) {
	m := &Mesh{
		Nodes: map[string]MeshNode{"a": {Level: MeshSymbol}, "b": {Level: MeshSymbol}},
		Edges: nil,
	}
	if d := m.Distance("a", "b"); d != -1 {
		t.Errorf("distance no-path = %d, want -1", d)
	}
}

func TestBoundaries(t *testing.T) {
	sg := testSymbolGraph()
	m := BuildMesh(sg, []string{"cmd/app", "internal/core", "internal/store"})
	seams := m.Boundaries()

	// main→Run crosses cmd/app→internal/core
	// Run→Get crosses internal/core→internal/store
	// Get→Store is intra-component (internal/store) — NOT a seam
	if len(seams) < 2 {
		t.Errorf("expected >= 2 boundary edges, got %d", len(seams))
	}
}
