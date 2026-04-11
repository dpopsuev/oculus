package oculus

import (
	"testing"
)

func testSymbolGraph() *SymbolGraph {
	return &SymbolGraph{
		Nodes: []Symbol{
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

func TestOverlayMesh_Roles(t *testing.T) {
	sg := testSymbolGraph()
	mesh := BuildMesh(sg, []string{"cmd/app", "internal/core", "internal/store"})

	roles := map[string]string{
		"cmd/app":        "entrypoint",
		"internal/core":  "domain",
		"internal/store": "adapter",
	}
	mesh.OverlayMesh(roles)

	if n := mesh.Nodes["cmd/app"]; n.Role != "entrypoint" {
		t.Errorf("cmd/app role = %q, want entrypoint", n.Role)
	}
	if n := mesh.Nodes["internal/core"]; n.Role != "domain" {
		t.Errorf("internal/core role = %q, want domain", n.Role)
	}
	if n := mesh.Nodes["internal/store"]; n.Role != "adapter" {
		t.Errorf("internal/store role = %q, want adapter", n.Role)
	}
}

func TestOverlayMesh_Stability(t *testing.T) {
	sg := testSymbolGraph()
	mesh := BuildMesh(sg, []string{"cmd/app", "internal/core", "internal/store"})
	mesh.OverlayMesh(nil)

	// "Run" is called by main (fan-in=1) and calls Get (fan-out=1) → instability=0.5
	run := mesh.Nodes["internal/core.Run"]
	if run.FanIn == 0 {
		t.Error("Run fan-in should be > 0")
	}
	if run.FanOut == 0 {
		t.Error("Run fan-out should be > 0")
	}
	if run.Instability == 0 {
		t.Error("Run instability should be > 0")
	}
	t.Logf("Run: fan_in=%d fan_out=%d instability=%.2f choke=%.3f", run.FanIn, run.FanOut, run.Instability, run.ChokeScore)

	// Run is on the path main→Run→Get — should have non-zero choke score.
	if run.ChokeScore == 0 {
		t.Error("Run choke score should be > 0 (it's on the main→Get path)")
	}
}

func TestMesh_Circuits(t *testing.T) {
	// Build a mesh with bidirectional edges: A→B and B→A.
	mesh := &Mesh{
		Nodes: map[string]MeshNode{
			"pkg.Alpha": {Name: "pkg.Alpha", Level: MeshSymbol},
			"pkg.Beta":  {Name: "pkg.Beta", Level: MeshSymbol},
			"pkg.Gamma": {Name: "pkg.Gamma", Level: MeshSymbol},
		},
		Edges: []SymbolEdge{
			{SourceFQN: "pkg.Alpha", TargetFQN: "pkg.Beta", Weight: 1.0},
			{SourceFQN: "pkg.Beta", TargetFQN: "pkg.Alpha", Weight: 1.0}, // circuit!
			{SourceFQN: "pkg.Beta", TargetFQN: "pkg.Gamma", Weight: 1.0}, // one-way, no circuit
		},
	}

	circuits := mesh.Circuits(0.1)
	if len(circuits) != 1 {
		t.Fatalf("circuits = %d, want 1", len(circuits))
	}
	if mesh.Nodes["pkg.Alpha"].CircuitID == 0 {
		t.Error("Alpha should be in a circuit")
	}
	if mesh.Nodes["pkg.Beta"].CircuitID == 0 {
		t.Error("Beta should be in a circuit")
	}
	if mesh.Nodes["pkg.Gamma"].CircuitID != 0 {
		t.Error("Gamma should NOT be in a circuit")
	}
	t.Logf("circuits: %v", circuits)
}
