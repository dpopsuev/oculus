package engine

import (
	"context"
	"testing"

	"github.com/dpopsuev/oculus/v3/internal/testfixture"
)

func TestGetDataFlow_SyntheticRepository(t *testing.T) {
	root := testfixture.Repository(t, "go")
	eng := New(nil, []string{root})

	report, err := eng.GetDataFlow(context.Background(), root, "main", 5)
	if err != nil {
		t.Fatalf("GetDataFlow: %v", err)
	}
	if report == nil || report.Flow == nil {
		t.Fatal("GetDataFlow returned no flow")
	}
	if report.Entry != "main" {
		t.Errorf("entry = %q, want main", report.Entry)
	}
	if report.Summary == "" {
		t.Error("summary is empty")
	}
}

func TestGetDataFlow_DefaultEntry(t *testing.T) {
	root := testfixture.Repository(t, "go")
	eng := New(nil, []string{root})

	report, err := eng.GetDataFlow(context.Background(), root, "", 0)
	if err != nil {
		t.Fatalf("GetDataFlow: %v", err)
	}
	if report.Entry != "main" {
		t.Errorf("default entry = %q, want main", report.Entry)
	}
}

func TestGetSymbolGraph_SyntheticRepository(t *testing.T) {
	root := testfixture.Repository(t, "go")
	eng := New(nil, []string{root})

	symbolGraph, err := eng.GetSymbolGraph(context.Background(), root)
	if err != nil {
		t.Fatalf("GetSymbolGraph: %v", err)
	}
	if symbolGraph == nil || len(symbolGraph.Nodes) == 0 {
		t.Fatal("expected symbol nodes")
	}
	callEdges := 0
	for _, edge := range symbolGraph.Edges {
		if edge.Kind == "call" {
			callEdges++
		}
	}
	if callEdges == 0 {
		t.Error("expected call edges")
	}
}

func TestDetectPipelines_SyntheticRepository(t *testing.T) {
	root := testfixture.Repository(t, "go")
	eng := New(nil, []string{root})

	report, err := eng.DetectPipelines(context.Background(), root, 2)
	if err != nil {
		t.Fatalf("DetectPipelines: %v", err)
	}
	if report == nil || report.Summary == "" {
		t.Fatal("expected pipeline summary")
	}
}

func TestDetectStateMachines_SyntheticRepository(t *testing.T) {
	root := testfixture.Repository(t, "go")
	eng := New(nil, []string{root})

	report, err := eng.DetectStateMachines(context.Background(), root)
	if err != nil {
		t.Fatalf("DetectStateMachines: %v", err)
	}
	if report == nil || report.Summary == "" {
		t.Fatal("expected state-machine summary")
	}
}
