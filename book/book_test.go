package book

import (
	"path/filepath"
	"runtime"
	"testing"

	"github.com/dpopsuev/oculus/v3/graph"
)

func bookDir(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot determine test file path")
	}
	return filepath.Dir(file)
}

func TestLoadEmbedded(t *testing.T) {
	g, err := LoadEmbedded()
	if err != nil {
		t.Fatalf("LoadEmbedded: %v", err)
	}
	if len(g.Nodes) < 20 {
		t.Errorf("expected at least 20 nodes, got %d", len(g.Nodes))
	}
	result := g.Query([]string{"god", "component"}, 0)
	if len(result.Entries) == 0 {
		t.Fatal("embedded query returned no entries")
	}
	for _, e := range result.Entries {
		if e.ID == "god-component" && e.Content == "" {
			t.Error("expected content loaded from embedded FS")
		}
	}
}

func TestLoad(t *testing.T) {
	g, err := Load(bookDir(t))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(g.Nodes) == 0 {
		t.Fatal("expected nodes, got 0")
	}
	if len(g.Edges) == 0 {
		t.Fatal("expected edges, got 0")
	}
}

func TestLoad_NodeCount(t *testing.T) {
	g, err := Load(bookDir(t))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(g.Nodes) < 20 {
		t.Errorf("expected at least 20 nodes, got %d", len(g.Nodes))
	}
}

func TestLoad_EdgeCount(t *testing.T) {
	g, err := Load(bookDir(t))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(g.Edges) < 25 {
		t.Errorf("expected at least 25 edges, got %d", len(g.Edges))
	}
}

func TestLoad_NodeHasKeywords(t *testing.T) {
	g, err := Load(bookDir(t))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	node, ok := g.Nodes["god-component"]
	if !ok {
		t.Fatal("expected god-component node")
	}
	if len(node.Keywords) == 0 {
		t.Error("god-component should have keywords")
	}
}

func TestBookEdge_GraphCompatibility(t *testing.T) {
	edges := []BookEdge{
		{From: "a", To: "b", Kind: "violates"},
		{From: "c", To: "b", Kind: "measured_by"},
		{From: "a", To: "c", Kind: "feeds"},
	}
	fanIn := graph.FanIn(edges)
	if fanIn["b"] != 2 {
		t.Errorf("expected fan-in of 2 for b, got %d", fanIn["b"])
	}
}

func TestQuery_Keywords(t *testing.T) {
	g, err := Load(bookDir(t))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	result := g.Query([]string{"god", "high", "fan-in", "large"}, 0)
	if len(result.Entries) == 0 {
		t.Fatal("expected entries for 'god high fan-in large', got 0")
	}

	foundGod := false
	for _, e := range result.Entries {
		if e.ID == "god-component" {
			foundGod = true
		}
	}
	if !foundGod {
		t.Error("expected god-component in results")
	}
}

func TestQuery_GraphTraversal(t *testing.T) {
	g, err := Load(bookDir(t))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	result := g.Query([]string{"god", "component"}, 1)

	ids := make(map[string]bool)
	for _, e := range result.Entries {
		ids[e.ID] = true
	}

	// god-component has edges to srp (violates), fan-in/fan-out/loc (measured_by), facade/mediator (confused_with)
	for _, expected := range []string{"srp", "fan-in", "fan-out"} {
		if !ids[expected] {
			t.Errorf("expected %s in 1-hop neighborhood of god-component", expected)
		}
	}
}

func TestQuery_ContentLoading(t *testing.T) {
	g, err := Load(bookDir(t))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	result := g.Query([]string{"god", "component"}, 0)
	if len(result.Entries) == 0 {
		t.Fatal("expected entries")
	}
	for _, e := range result.Entries {
		if e.ID == "god-component" && e.Content == "" {
			t.Error("expected content loaded for god-component")
		}
	}
}

func TestQuery_ReturnsEdges(t *testing.T) {
	g, err := Load(bookDir(t))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	result := g.Query([]string{"god", "component"}, 1)
	if len(result.Edges) == 0 {
		t.Error("expected edges in result subgraph")
	}
}

func TestQuery_EmptyKeywords(t *testing.T) {
	g, err := Load(bookDir(t))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	result := g.Query(nil, 0)
	if len(result.Entries) != 0 {
		t.Errorf("expected 0 entries for empty keywords, got %d", len(result.Entries))
	}
}

func TestQuery_UnknownKeywords(t *testing.T) {
	g, err := Load(bookDir(t))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	result := g.Query([]string{"xyzzy", "plugh", "zyzzyx"}, 0)
	if len(result.Entries) != 0 {
		t.Errorf("expected 0 entries for unknown keywords, got %d", len(result.Entries))
	}
}

func TestQuery_Roots(t *testing.T) {
	g, err := Load(bookDir(t))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	result := g.Query([]string{"god", "component"}, 1)
	if len(result.Roots) == 0 {
		t.Error("expected roots in result")
	}
	if result.Roots[0] != "god-component" {
		t.Errorf("expected god-component as first root, got %s", result.Roots[0])
	}
}
