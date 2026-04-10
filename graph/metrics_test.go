package graph

import (
	"testing"
)

func TestCohesion_FullyConnected(t *testing.T) {
	adj := map[string]map[string]bool{
		"a": {"b": true, "c": true},
		"b": {"a": true, "c": true},
		"c": {"a": true, "b": true},
	}
	c := Cohesion([]string{"a", "b", "c"}, adj)
	if c != 1.0 {
		t.Errorf("expected 1.0 for fully connected, got %f", c)
	}
}

func TestCohesion_Sparse(t *testing.T) {
	adj := map[string]map[string]bool{
		"a": {"b": true},
		"b": {"a": true},
		"c": {},
	}
	// 3 nodes, 1 edge out of 3 possible = 0.333
	c := Cohesion([]string{"a", "b", "c"}, adj)
	if c < 0.3 || c > 0.4 {
		t.Errorf("expected ~0.33, got %f", c)
	}
}

func TestCohesion_SingleNode(t *testing.T) {
	c := Cohesion([]string{"a"}, nil)
	if c != 1.0 {
		t.Errorf("expected 1.0 for single node, got %f", c)
	}
}

func TestBFSGroup_Basic(t *testing.T) {
	adj := map[string]map[string]bool{
		"a": {"b": true},
		"b": {"a": true, "c": true},
		"c": {"b": true},
		"d": {},
	}
	visited := make(map[string]bool)
	group := BFSGroup("a", adj, visited)
	if len(group) != 3 {
		t.Errorf("expected 3 nodes, got %d: %v", len(group), group)
	}
	if visited["d"] {
		t.Error("d should not be visited")
	}
}

func TestBetweennessCentrality_Star(t *testing.T) {
	// Star topology: center connects to 4 leaves. Center should have highest centrality.
	edges := []testEdge{
		{"center", "a"}, {"center", "b"}, {"center", "c"}, {"center", "d"},
	}
	bc := BetweennessCentrality(edges)

	if bc["center"] == 0 {
		t.Error("center should have highest centrality")
	}
	for _, leaf := range []string{"a", "b", "c", "d"} {
		if bc[leaf] >= bc["center"] {
			t.Errorf("leaf %s centrality %.3f >= center %.3f", leaf, bc[leaf], bc["center"])
		}
	}
	t.Logf("centrality: center=%.3f a=%.3f b=%.3f c=%.3f d=%.3f",
		bc["center"], bc["a"], bc["b"], bc["c"], bc["d"])
}

func TestBetweennessCentrality_Chain(t *testing.T) {
	// Chain: A → B → C → D. B and C are on all shortest paths — highest centrality.
	edges := []testEdge{
		{"A", "B"}, {"B", "C"}, {"C", "D"},
	}
	bc := BetweennessCentrality(edges)

	if bc["B"] == 0 || bc["C"] == 0 {
		t.Error("B and C should have non-zero centrality")
	}
	if bc["A"] > bc["B"] {
		t.Errorf("A (%.3f) should have lower centrality than B (%.3f)", bc["A"], bc["B"])
	}
	t.Logf("centrality: A=%.3f B=%.3f C=%.3f D=%.3f", bc["A"], bc["B"], bc["C"], bc["D"])
}
