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
