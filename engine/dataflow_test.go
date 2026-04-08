package engine

import (
	"context"
	"testing"
)

func TestGetDataFlow_Dogfood(t *testing.T) {
	if testing.Short() {
		t.Skip("dogfood: skipping in -short mode")
	}
	root := oculusRoot(t)
	eng := New(nil, []string{root})

	report, err := eng.GetDataFlow(context.Background(), root, "main", 5)
	if err != nil {
		t.Fatalf("GetDataFlow: %v", err)
	}
	if report == nil {
		t.Fatal("GetDataFlow returned nil report")
	}
	if report.Flow == nil {
		t.Fatal("GetDataFlow returned nil flow")
	}
	if report.Entry != "main" {
		t.Errorf("entry = %q, want %q", report.Entry, "main")
	}
	if report.Summary == "" {
		t.Error("summary is empty")
	}
	t.Logf("DataFlow: %s", report.Summary)
}

func TestGetDataFlow_DefaultEntry(t *testing.T) {
	if testing.Short() {
		t.Skip("dogfood: skipping in -short mode")
	}
	root := oculusRoot(t)
	eng := New(nil, []string{root})

	report, err := eng.GetDataFlow(context.Background(), root, "", 0)
	if err != nil {
		t.Fatalf("GetDataFlow (default entry): %v", err)
	}
	if report.Entry != "main" {
		t.Errorf("default entry = %q, want %q", report.Entry, "main")
	}
}

func TestGetSymbolGraph_Dogfood(t *testing.T) {
	if testing.Short() {
		t.Skip("dogfood: skipping in -short mode")
	}
	root := oculusRoot(t)
	eng := New(nil, []string{root})

	sg, err := eng.GetSymbolGraph(context.Background(), root)
	if err != nil {
		t.Fatalf("GetSymbolGraph: %v", err)
	}
	if sg == nil {
		t.Fatal("GetSymbolGraph returned nil")
	}
	if len(sg.Nodes) == 0 {
		t.Error("expected nodes")
	}
	if len(sg.Edges) == 0 {
		t.Error("expected edges")
	}
	t.Logf("SymbolGraph: %d nodes, %d edges", len(sg.Nodes), len(sg.Edges))

	// Verify call edges exist
	callEdges := 0
	for _, e := range sg.Edges {
		if e.Kind == "call" {
			callEdges++
		}
	}
	if callEdges == 0 {
		t.Error("expected call edges")
	}
	t.Logf("  %d call edges", callEdges)
}

func TestDetectPipelines_Dogfood(t *testing.T) {
	if testing.Short() {
		t.Skip("dogfood: skipping in -short mode")
	}
	root := oculusRoot(t)
	eng := New(nil, []string{root})

	report, err := eng.DetectPipelines(context.Background(), root, 2)
	if err != nil {
		t.Fatalf("DetectPipelines: %v", err)
	}
	t.Logf("Pipelines: %s", report.Summary)
	for i, p := range report.Pipelines {
		if i >= 5 {
			t.Logf("  ... and %d more", len(report.Pipelines)-5)
			break
		}
		steps := make([]string, len(p.Steps))
		for j, s := range p.Steps {
			steps[j] = s.FQN
		}
		t.Logf("  [%d steps] %s (types: %v)", p.Length, steps, p.TypeChain)
	}
}

func TestDetectStateMachines_Dogfood(t *testing.T) {
	if testing.Short() {
		t.Skip("dogfood: skipping in -short mode")
	}
	root := oculusRoot(t)
	eng := New(nil, []string{root})

	report, err := eng.DetectStateMachines(context.Background(), root)
	if err != nil {
		t.Fatalf("DetectStateMachines: %v", err)
	}
	if report == nil {
		t.Fatal("DetectStateMachines returned nil report")
	}
	if report.Summary == "" {
		t.Error("summary is empty")
	}
	t.Logf("StateMachines: %s", report.Summary)
	for _, m := range report.Machines {
		t.Logf("  %s (%s): %d states, %d transitions", m.Name, m.Package, len(m.States), len(m.Transitions))
	}
}
