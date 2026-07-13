package oculus

import (
	"strings"
	"testing"
)

// RED→GREEN: Go receiver FQN forms must aggregate under the type pivot.
// Call-graph edges often use (*T).M or *T.M while type nodes are pkg.T.
func TestProbe_GoReceiverFQN_AggregatesMethods(t *testing.T) {
	sg := &SymbolGraph{
		Nodes: []Symbol{
			{Name: "Engine", Package: "engine", Kind: "struct", Exported: true},
			{Name: "Engine.ScanProject", Package: "engine", Kind: "method", Exported: true},
			{Name: "(*Engine).GetSymbolGraph", Package: "engine", Kind: "method", Exported: true},
			{Name: "*Engine.WarmLSP", Package: "engine", Kind: "method", Exported: true},
			{Name: "helper", Package: "engine", Kind: "function"},
			{Name: "caller", Package: "cli", Kind: "function"},
		},
		Edges: []SymbolEdge{
			{SourceFQN: "cli.caller", TargetFQN: "engine.Engine.ScanProject", Kind: "call"},
			{SourceFQN: "engine.(*Engine).GetSymbolGraph", TargetFQN: "engine.helper", Kind: "call"},
			{SourceFQN: "engine.*Engine.WarmLSP", TargetFQN: "engine.helper", Kind: "call"},
		},
	}

	r := Probe(sg, "engine.Engine")
	if r == nil {
		t.Fatal("Probe(engine.Engine) returned nil")
	}
	if r.CallGraphStatus != CallGraphCovered {
		t.Errorf("CallGraphStatus=%s, want covered (aggregate methods)", r.CallGraphStatus)
	}
	if r.FanIn < 1 {
		t.Errorf("FanIn=%d, want ≥1 from ScanProject caller", r.FanIn)
	}
	if r.FanOut < 2 {
		t.Errorf("FanOut=%d, want ≥2 from GetSymbolGraph+WarmLSP → helper", r.FanOut)
	}
}

func TestMergeSymbolGraph_ParenPointerReceiverNormalized(t *testing.T) {
	cg := &CallGraph{
		Nodes: []Symbol{
			{Name: "(*Engine).GetSymbolGraph", Package: "engine"},
			{Name: "helper", Package: "engine"},
		},
		Edges: []CallEdge{
			{Caller: "(*Engine).GetSymbolGraph", CallerPkg: "engine", Callee: "helper", CalleePkg: "engine"},
		},
	}
	classes := []ClassInfo{{
		Name: "Engine", Package: "engine", Kind: "struct",
		Methods: []MethodInfo{{Name: "GetSymbolGraph", Exported: true}},
	}}
	sg := MergeSymbolGraph(cg, classes, nil, nil)

	for _, e := range sg.Edges {
		if strings.Contains(e.SourceFQN, "*") || strings.Contains(e.TargetFQN, "*") {
			t.Errorf("edge still has receiver form: %s → %s", e.SourceFQN, e.TargetFQN)
		}
	}
	r := Probe(sg, "engine.Engine")
	if r == nil || r.CallGraphStatus != CallGraphCovered {
		t.Fatalf("after merge, probe Engine want covered, got %+v", r)
	}
}
