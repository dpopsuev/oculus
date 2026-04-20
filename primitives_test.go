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
	r := oculus.TraceScenario(sg, "pkg1.A", 10, false)
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
	r := oculus.TraceScenario(sg, "pkg2.D", 10, false)
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
	r := oculus.TraceScenario(sg, "pkg1.B", 10, false)
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
	r := oculus.TraceScenario(sg, "pkg1.A", 1, false)
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
	r := oculus.TraceScenario(sg, "pkg1.A", 10, true)
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
	r := oculus.TraceScenario(sg, "pkg1.A", 10, false)
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
	r := oculus.FindConvergence(sg, []string{"pkg1.A", "pkg4.G"})
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
	r := oculus.FindConvergence(sg, []string{"pkg1.A", "pkg4.G", "pkg2.D"})
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
	r := oculus.FindConvergence(sg, []string{"pkg1.A", "pkg5.H"})
	if r == nil {
		t.Fatal("expected non-nil ConvergenceResult")
	}
	if len(r.Nodes) != 0 {
		t.Errorf("expected 0 convergence nodes for disjoint symbols, got %d", len(r.Nodes))
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
