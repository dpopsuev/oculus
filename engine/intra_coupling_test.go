package engine

import (
	"context"
	"testing"
)

// TestGetIntraCoupling_WithinComponent verifies that coupling view=intra
// returns file-to-file coupling edges within one component rather than
// cross-component edges.
//
// Given a report with file-level services within "agents/"
// When GetIntraCoupling("agents") is called
// Then file-to-file edges within agents/ are returned with weights
func TestGetIntraCoupling_WithinComponent(t *testing.T) {
	report := testReportWithFileGranularity()
	store := newMockStore(report)
	eng := New(store, []string{"/tmp"})

	r, err := eng.GetIntraCoupling(context.Background(), "/tmp", "agents")
	if err != nil {
		t.Fatalf("GetIntraCoupling: %v", err)
	}
	if r == nil {
		t.Fatal("expected non-nil result")
	}
	if len(r.Edges) == 0 {
		t.Error("expected intra-component coupling edges")
	}
	for _, e := range r.Edges {
		if len(e.From) < 7 || e.From[:7] != "agents/" {
			t.Errorf("edge From=%q is outside agents/ component", e.From)
		}
	}
}

// TestGetIntraCoupling_UnknownComponent returns error.
func TestGetIntraCoupling_UnknownComponent(t *testing.T) {
	eng, _ := newTestEngine()
	_, err := eng.GetIntraCoupling(context.Background(), "/tmp", "nonexistent")
	if err == nil {
		t.Error("expected error for unknown component")
	}
}
