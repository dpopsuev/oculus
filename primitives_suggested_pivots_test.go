package oculus

import "testing"

func TestProbe_SuggestedPivots_OnStruct(t *testing.T) {
	sg := &SymbolGraph{
		Nodes: []Symbol{
			{Name: "Engine", Package: "engine", Kind: "struct", Exported: true},
			{Name: "Engine.Hot", Package: "engine", Kind: "method", Exported: true},
			{Name: "Engine.Cold", Package: "engine", Kind: "method", Exported: true},
			{Name: "helper", Package: "engine", Kind: "function"},
			{Name: "caller", Package: "cli", Kind: "function"},
		},
		Edges: []SymbolEdge{
			{SourceFQN: "cli.caller", TargetFQN: "engine.Engine.Hot", Kind: "call"},
			{SourceFQN: "engine.Engine.Hot", TargetFQN: "engine.helper", Kind: "call"},
			// Cold has no edges
		},
	}
	r := Probe(sg, "engine.Engine")
	if r == nil {
		t.Fatal("nil probe")
	}
	if len(r.SuggestedPivots) == 0 {
		t.Fatal("expected suggested_pivots for struct with methods")
	}
	if r.SuggestedPivots[0] != "engine.Engine.Hot" {
		t.Errorf("top pivot=%q, want engine.Engine.Hot (highest degree)", r.SuggestedPivots[0])
	}
}

func TestProbe_SuggestedPivots_HollowTypeStillListsMethods(t *testing.T) {
	sg := &SymbolGraph{
		Nodes: []Symbol{
			{Name: "Empty", Package: "pkg", Kind: "struct"},
			{Name: "Empty.M", Package: "pkg", Kind: "method"},
			{Name: "other", Package: "pkg", Kind: "function"},
		},
		Edges: []SymbolEdge{
			// edges exist in graph but not involving Empty.M
			{SourceFQN: "pkg.other", TargetFQN: "pkg.other", Kind: "call"},
		},
	}
	r := Probe(sg, "pkg.Empty")
	if r == nil {
		t.Fatal("nil probe")
	}
	if r.CallGraphStatus != CallGraphNotCovered {
		t.Errorf("status=%s, want not_covered", r.CallGraphStatus)
	}
	if len(r.SuggestedPivots) == 0 {
		t.Fatal("hollow type with methods must still suggest pivots")
	}
}
