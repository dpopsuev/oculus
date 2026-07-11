package arch

import (
	"testing"
)

// TestComputeHotSpots_AfterNestingApplied verifies the structural path that
// was dead when HotSpots were frozen before runL2Health populated MaxNesting.
func TestComputeHotSpots_AfterNestingApplied(t *testing.T) {
	m := ArchModel{
		Services: []ArchService{
			{Name: "hub", Churn: 0, MaxNesting: 0},
		},
		Edges: []ArchEdge{
			{From: "a", To: "hub"},
			{From: "b", To: "hub"},
			{From: "c", To: "hub"},
		},
	}
	if got := computeHotSpots(m); len(got) != 0 {
		t.Fatalf("before nesting: got %d spots, want 0", len(got))
	}

	m.Services[0].MaxNesting = MinNestingHotSpot
	got := computeHotSpots(m)
	if len(got) != 1 || got[0].Component != "hub" {
		t.Fatalf("after nesting: got %+v, want hub", got)
	}
	if got[0].Nesting != MinNestingHotSpot {
		t.Errorf("Nesting = %d, want %d", got[0].Nesting, MinNestingHotSpot)
	}
}
