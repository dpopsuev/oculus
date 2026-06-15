package engine

import (
	"context"
	"testing"
)

// TestGetIntraPackageDeps_ReturnsFileEdges verifies that querying
// dependencies scoped to a single component returns file-to-file edges
// within that component, not just cross-component edges.
//
// Given a report where file-granularity services exist within one component
// When GetIntraPackageDeps is called with that component
// Then file-to-file edges within the component are returned
func TestGetIntraPackageDeps_ReturnsFileEdges(t *testing.T) {
	report := testReportWithFileGranularity()
	store := newMockStore(report)
	eng := New(store, []string{"/tmp"})

	r, err := eng.GetIntraPackageDeps(context.Background(), "/tmp", "agents")
	if err != nil {
		t.Fatalf("GetIntraPackageDeps: %v", err)
	}
	if r == nil {
		t.Fatal("expected non-nil result")
	}
	if len(r.Edges) == 0 {
		t.Error("expected intra-package edges between files within agents/")
	}
	for _, e := range r.Edges {
		if len(e.From) < 7 || e.From[:7] != "agents/" {
			t.Errorf("edge From=%q is outside agents/ component", e.From)
		}
		if len(e.To) < 7 || e.To[:7] != "agents/" {
			t.Errorf("edge To=%q is outside agents/ component", e.To)
		}
	}
}

// TestGetIntraPackageDeps_UnknownComponent returns error for missing component.
func TestGetIntraPackageDeps_UnknownComponent(t *testing.T) {
	eng, _ := newTestEngine()

	_, err := eng.GetIntraPackageDeps(context.Background(), "/tmp", "nonexistent")
	if err == nil {
		t.Error("expected error for unknown component")
	}
}
