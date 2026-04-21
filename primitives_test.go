package oculus_test

import (
	"testing"

	oculus "github.com/dpopsuev/oculus/v3"
	"github.com/dpopsuev/oculus/v3/testkit"
)

// --- Probe ---

func TestProbe_Identity(t *testing.T) {
	sg := testkit.FixtureGraph()
	r := oculus.Probe(sg, "pkg1.A")
	if r == nil {
		t.Fatal("expected non-nil ProbeResult")
	}
	if r.FQN != "pkg1.A" {
		t.Errorf("FQN = %q, want pkg1.A", r.FQN)
	}
	if r.Package != "pkg1" {
		t.Errorf("Package = %q, want pkg1", r.Package)
	}
	if r.Kind != "function" {
		t.Errorf("Kind = %q, want function", r.Kind)
	}
	if !r.Exported {
		t.Error("expected Exported = true")
	}
	if r.File != "pkg1/a.go" {
		t.Errorf("File = %q, want pkg1/a.go", r.File)
	}
}

func TestProbe_Coupling(t *testing.T) {
	sg := testkit.FixtureGraph()
	r := oculus.Probe(sg, "pkg1.B")
	if r == nil {
		t.Fatal("expected non-nil ProbeResult")
	}
	if r.FanIn != 2 {
		t.Errorf("FanIn = %d, want 2 (A and G call B)", r.FanIn)
	}
	if r.FanOut != 1 {
		t.Errorf("FanOut = %d, want 1 (B calls C)", r.FanOut)
	}
	if r.CrossPkg != 1 {
		t.Errorf("CrossPkg = %d, want 1 (B→C crosses pkg1→pkg2)", r.CrossPkg)
	}
}

func TestProbe_UnknownSymbol(t *testing.T) {
	sg := testkit.FixtureGraph()
	r := oculus.Probe(sg, "pkg99.DoesNotExist")
	if r != nil {
		t.Error("expected nil for unknown symbol")
	}
}

// --- Scenario ---

func TestScenario_Downstream(t *testing.T) {
	sg := testkit.FixtureGraph()
	r := oculus.TraceScenario(sg, "pkg1.A", 10, false, 0)
	if r == nil {
		t.Fatal("expected non-nil ScenarioResult")
	}
	downs := make(map[string]bool)
	for _, n := range r.Downstream {
		downs[n.FQN] = true
	}
	for _, want := range []string{"pkg1.B", "pkg2.C", "pkg2.D", "pkg3.E", "pkg3.F"} {
		if !downs[want] {
			t.Errorf("expected %s in downstream", want)
		}
	}
}

func TestScenario_Upstream(t *testing.T) {
	sg := testkit.FixtureGraph()
	r := oculus.TraceScenario(sg, "pkg2.D", 10, false, 0)
	if r == nil {
		t.Fatal("expected non-nil ScenarioResult")
	}
	ups := make(map[string]bool)
	for _, n := range r.Upstream {
		ups[n.FQN] = true
	}
	for _, want := range []string{"pkg2.C", "pkg1.B", "pkg1.A", "pkg4.G"} {
		if !ups[want] {
			t.Errorf("expected %s in upstream", want)
		}
	}
}

func TestScenario_Bidirectional(t *testing.T) {
	sg := testkit.FixtureGraph()
	r := oculus.TraceScenario(sg, "pkg1.B", 10, false, 0)
	if r == nil {
		t.Fatal("expected non-nil ScenarioResult")
	}
	if len(r.Upstream) == 0 {
		t.Error("expected upstream nodes for B")
	}
	if len(r.Downstream) == 0 {
		t.Error("expected downstream nodes for B")
	}
}

func TestScenario_DepthLimit(t *testing.T) {
	sg := testkit.FixtureGraph()
	r := oculus.TraceScenario(sg, "pkg1.A", 1, false, 0)
	if r == nil {
		t.Fatal("expected non-nil ScenarioResult")
	}
	for _, n := range r.Downstream {
		if n.Depth > 1 {
			t.Errorf("depth %d exceeds limit 1 for %s", n.Depth, n.FQN)
		}
	}
}

func TestScenario_Stress(t *testing.T) {
	sg := testkit.FixtureGraph()
	r := oculus.TraceScenario(sg, "pkg1.A", 10, true, 0)
	if r == nil {
		t.Fatal("expected non-nil ScenarioResult")
	}
	for _, n := range r.Downstream {
		if n.FQN == "pkg1.B" && n.FanOut == 0 {
			t.Error("stress mode should populate FanOut for B")
		}
	}
}

func TestScenario_Edges(t *testing.T) {
	sg := testkit.FixtureGraph()
	r := oculus.TraceScenario(sg, "pkg1.A", 10, false, 0)
	if r == nil {
		t.Fatal("expected non-nil ScenarioResult")
	}
	if len(r.Edges) == 0 {
		t.Error("expected edges in scenario result")
	}
}

// --- Convergence ---

func TestConvergence_TwoSymbols(t *testing.T) {
	sg := testkit.FixtureGraph()
	r := oculus.FindConvergence(sg, []string{"pkg1.A", "pkg4.G"}, 0)
	if r == nil {
		t.Fatal("expected non-nil ConvergenceResult")
	}
	found := false
	for _, n := range r.Nodes {
		if n.FQN == "pkg1.B" && n.Converges == 2 {
			found = true
		}
	}
	if !found {
		t.Error("expected B as convergence point with converges=2 (A and G both reach B)")
	}
}

func TestConvergence_NSymbols(t *testing.T) {
	sg := testkit.FixtureGraph()
	r := oculus.FindConvergence(sg, []string{"pkg1.A", "pkg4.G", "pkg2.D"}, 0)
	if r == nil {
		t.Fatal("expected non-nil ConvergenceResult")
	}
	eMap := make(map[string]int)
	for _, n := range r.Nodes {
		eMap[n.FQN] = n.Converges
	}
	if eMap["pkg3.E"] < 2 {
		t.Errorf("expected E to converge from at least 2 sources, got %d", eMap["pkg3.E"])
	}
}

func TestConvergence_NoOverlap(t *testing.T) {
	sg := testkit.FixtureGraph()
	r := oculus.FindConvergence(sg, []string{"pkg1.A", "pkg5.H"}, 0)
	if r == nil {
		t.Fatal("expected non-nil ConvergenceResult")
	}
	if len(r.Nodes) != 0 {
		t.Errorf("expected 0 convergence nodes for disjoint symbols, got %d", len(r.Nodes))
	}
}

// --- Islands ---

func TestFindIslands_WithIsolatedNode(t *testing.T) {
	sg := testkit.FixtureGraph()
	entries := []string{"pkg1.A", "pkg4.G"}
	r := oculus.FindIslands(sg, entries)
	if r == nil {
		t.Fatal("expected non-nil IslandResult")
	}
	found := false
	for _, fqn := range r.Unreachable {
		if fqn == "pkg5.H" {
			found = true
		}
	}
	if !found {
		t.Error("expected H to be unreachable from entry points A and G")
	}
}

func TestFindIslands_AllReachable(t *testing.T) {
	sg := testkit.FixtureGraph()
	entries := []string{"pkg1.A", "pkg4.G", "pkg5.H"}
	r := oculus.FindIslands(sg, entries)
	if r == nil {
		t.Fatal("expected non-nil IslandResult")
	}
	if len(r.Unreachable) != 0 {
		t.Errorf("expected 0 unreachable, got %d: %v", len(r.Unreachable), r.Unreachable)
	}
}

func TestFindIslands_EmptyEntries(t *testing.T) {
	sg := testkit.FixtureGraph()
	r := oculus.FindIslands(sg, nil)
	if r == nil {
		t.Fatal("expected non-nil IslandResult")
	}
	// With auto-detected entry points, only truly isolated nodes are unreachable.
	if len(r.EntryPoints) == 0 {
		t.Error("expected auto-detected entry points when nil is passed")
	}
}

// --- Isolate ---

func TestIsolate_Leaf(t *testing.T) {
	sg := testkit.FixtureGraph()
	r := oculus.Isolate(sg, "pkg5.H")
	if r == nil {
		t.Fatal("expected non-nil IsolateResult")
	}
	// H is already isolated (no edges), so removing it reduces component count by 1.
	if r.ComponentsAfter != r.ComponentsBefore-1 {
		t.Errorf("removing isolated node H: before=%d, after=%d, want after=%d",
			r.ComponentsBefore, r.ComponentsAfter, r.ComponentsBefore-1)
	}
}

func TestIsolate_Bridge(t *testing.T) {
	sg := testkit.FixtureGraph()
	r := oculus.Isolate(sg, "pkg1.B")
	if r == nil {
		t.Fatal("expected non-nil IsolateResult")
	}
	if r.ComponentsAfter <= r.ComponentsBefore {
		t.Errorf("removing bridge B should increase components: before=%d, after=%d",
			r.ComponentsBefore, r.ComponentsAfter)
	}
}

func TestIsolate_Unknown(t *testing.T) {
	sg := testkit.FixtureGraph()
	r := oculus.Isolate(sg, "pkg99.Nope")
	if r == nil {
		t.Fatal("expected non-nil IsolateResult")
	}
	if r.ComponentsAfter != r.ComponentsBefore {
		t.Error("removing unknown symbol should not change components")
	}
}

// --- Fuzzy Symbol Lookup (TSK-165) ---

func TestProbe_PartialName(t *testing.T) {
	sg := testkit.FixtureGraph()
	r := oculus.Probe(sg, "A")
	if r == nil {
		t.Fatal("expected Probe to resolve partial name 'A' to 'pkg1.A'")
	}
	if r.FQN != "pkg1.A" {
		t.Errorf("FQN = %q, want pkg1.A", r.FQN)
	}
}

func TestScenario_PartialName(t *testing.T) {
	sg := testkit.FixtureGraph()
	r := oculus.TraceScenario(sg, "B", 10, false, 0)
	if r == nil {
		t.Fatal("expected Scenario to resolve partial name 'B'")
	}
	if r.Symbol != "pkg1.B" {
		t.Errorf("Symbol = %q, want pkg1.B", r.Symbol)
	}
}

// --- Top-N (TSK-166) ---

func TestConvergence_TopN(t *testing.T) {
	sg := testkit.FixtureGraph()
	r := oculus.FindConvergence(sg, []string{"pkg1.A", "pkg4.G"}, 2)
	if r == nil {
		t.Fatal("expected non-nil ConvergenceResult")
	}
	if len(r.Nodes) > 2 {
		t.Errorf("expected max 2 nodes with topN=2, got %d", len(r.Nodes))
	}
}

// --- Auto Entry Points (TSK-169) ---

func TestDetectEntryPoints(t *testing.T) {
	sg := testkit.FixtureGraph()
	entries := oculus.DetectEntryPoints(sg)
	if len(entries) == 0 {
		t.Fatal("expected entry points")
	}
	entrySet := make(map[string]bool)
	for _, e := range entries {
		entrySet[e] = true
	}
	if !entrySet["pkg1.A"] {
		t.Error("expected A as entry point (no callers)")
	}
	if !entrySet["pkg4.G"] {
		t.Error("expected G as entry point (no callers)")
	}
}

func TestFindIslands_AutoEntryPoints(t *testing.T) {
	sg := testkit.FixtureGraph()
	r := oculus.FindIslands(sg, nil)
	if r == nil {
		t.Fatal("expected non-nil IslandResult")
	}
	if len(r.EntryPoints) == 0 {
		t.Error("expected auto-detected entry points")
	}
	found := false
	for _, fqn := range r.Unreachable {
		if fqn == "pkg5.H" {
			found = true
		}
	}
	if !found {
		t.Error("expected H unreachable even with auto-detected entry points")
	}
}

// --- TSK-177: Probe aggregates method metrics for structs ---

func TestProbe_StructAggregatesMethodMetrics(t *testing.T) {
	// Build a graph with struct S and methods S.M1, S.M2.
	// M1 has fan-out=2, M2 has fan-out=1.
	// Probing "mypkg.S" should show fan-out=3 (aggregated from methods).
	sg := &oculus.SymbolGraph{
		Nodes: []oculus.Symbol{
			{Name: "S", Package: "mypkg", Kind: "struct", Exported: true, File: "mypkg/s.go", Line: 1},
			{Name: "S.M1", Package: "mypkg", Kind: "method", Exported: true, File: "mypkg/s.go", Line: 10},
			{Name: "S.M2", Package: "mypkg", Kind: "method", Exported: true, File: "mypkg/s.go", Line: 20},
			{Name: "Helper1", Package: "mypkg", Kind: "function", Exported: true, File: "mypkg/h.go", Line: 1},
			{Name: "Helper2", Package: "mypkg", Kind: "function", Exported: true, File: "mypkg/h.go", Line: 10},
			{Name: "Helper3", Package: "other", Kind: "function", Exported: true, File: "other/h.go", Line: 1},
			{Name: "Caller", Package: "client", Kind: "function", Exported: true, File: "client/c.go", Line: 1},
		},
		Edges: []oculus.SymbolEdge{
			// M1 calls Helper1 and Helper2 (fan-out=2)
			{SourceFQN: "mypkg.S.M1", TargetFQN: "mypkg.Helper1", Kind: "call"},
			{SourceFQN: "mypkg.S.M1", TargetFQN: "mypkg.Helper2", Kind: "call"},
			// M2 calls Helper3 (fan-out=1, cross-pkg)
			{SourceFQN: "mypkg.S.M2", TargetFQN: "other.Helper3", Kind: "call"},
			// Caller calls M1 (fan-in=1 for M1)
			{SourceFQN: "client.Caller", TargetFQN: "mypkg.S.M1", Kind: "call"},
		},
	}

	r := oculus.Probe(sg, "mypkg.S")
	if r == nil {
		t.Fatal("expected non-nil ProbeResult for struct S")
	}
	if r.Kind != "struct" {
		t.Errorf("Kind = %q, want struct", r.Kind)
	}

	// Aggregated fan-out from methods: M1(2) + M2(1) = 3
	if r.FanOut != 3 {
		t.Errorf("FanOut = %d, want 3 (aggregated from methods M1=2, M2=1)", r.FanOut)
	}

	// Aggregated fan-in from methods: M1(1) + M2(0) = 1
	if r.FanIn != 1 {
		t.Errorf("FanIn = %d, want 1 (aggregated from methods: M1=1, M2=0)", r.FanIn)
	}

	// Aggregated cross-pkg: M2→Helper3 crosses mypkg→other = 1
	if r.CrossPkg != 1 {
		t.Errorf("CrossPkg = %d, want 1 (M2→Helper3 crosses mypkg→other)", r.CrossPkg)
	}

	// Instability = fo/(fi+fo) = 3/(1+3) = 0.75
	expectedInst := 0.75
	if r.Instability < expectedInst-0.01 || r.Instability > expectedInst+0.01 {
		t.Errorf("Instability = %f, want ~%f", r.Instability, expectedInst)
	}
}
