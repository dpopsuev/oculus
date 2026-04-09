package oculus

import (
	"sort"
	"testing"
)

// TestSymbolEdge_HasWeight verifies that SymbolEdge carries a Weight field.
func TestSymbolEdge_HasWeight(t *testing.T) {
	e := SymbolEdge{
		SourceFQN: "pkg.Foo",
		TargetFQN: "pkg.Bar",
		Kind:      "call",
		Weight:    1.0,
	}
	if e.Weight != 1.0 {
		t.Errorf("expected weight 1.0, got %f", e.Weight)
	}
}

// TestClassifyEdgeWeight verifies the weight classification model.
func TestClassifyEdgeWeight(t *testing.T) {
	components := []string{"analyzer", "engine", "diagram", "arch"}

	tests := []struct {
		name       string
		source     string
		target     string
		wantWeight float64
	}{
		{
			"internal cross-component",
			"analyzer.DeepFallback", "engine.Store",
			1.0,
		},
		{
			"internal same-component",
			"analyzer.DeepFallback", "analyzer.Registry",
			0.5,
		},
		{
			"stdlib plumbing — fmt",
			"analyzer.DeepFallback", "fmt.Sprintf",
			0.01,
		},
		{
			"stdlib plumbing — strings",
			"engine.New", "strings.Contains",
			0.01,
		},
		{
			"stdlib plumbing — errors",
			"engine.New", "errors.New",
			0.01,
		},
		{
			"external meaningful — net/http",
			"engine.Serve", "net/http.ListenAndServe",
			0.3,
		},
		{
			"external meaningful — database/sql",
			"engine.Query", "database/sql.Open",
			0.3,
		},
		{
			"external meaningful — encoding/json",
			"engine.Marshal", "encoding/json.Marshal",
			0.3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ClassifyEdgeWeight(tt.source, tt.target, components)
			if got != tt.wantWeight {
				t.Errorf("ClassifyEdgeWeight(%q, %q) = %f, want %f",
					tt.source, tt.target, got, tt.wantWeight)
			}
		})
	}
}

// TestMesh_Boundaries_MinWeight verifies that Boundaries filters by weight.
func TestMesh_Boundaries_MinWeight(t *testing.T) {
	sg := &SymbolGraph{
		Nodes: []SymbolNode{
			{Name: "Foo", Package: "analyzer", Kind: "function"},
			{Name: "Bar", Package: "engine", Kind: "function"},
			{Name: "Sprintf", Package: "fmt", Kind: "function"},
		},
		Edges: []SymbolEdge{
			{SourceFQN: "analyzer.Foo", TargetFQN: "engine.Bar", Kind: "call", Weight: 1.0},
			{SourceFQN: "analyzer.Foo", TargetFQN: "fmt.Sprintf", Kind: "call", Weight: 0.01},
		},
	}
	mesh := BuildMesh(sg, []string{"analyzer", "engine"})

	// No filter — both edges cross boundaries
	all := mesh.Boundaries()
	if len(all) != 2 {
		t.Errorf("Boundaries() = %d edges, want 2", len(all))
	}

	// With min weight — only the internal edge survives
	filtered := mesh.BoundariesMinWeight(0.1)
	if len(filtered) != 1 {
		t.Errorf("BoundariesMinWeight(0.1) = %d edges, want 1", len(filtered))
	}
	if len(filtered) > 0 && filtered[0].TargetFQN != "engine.Bar" {
		t.Errorf("expected engine.Bar, got %s", filtered[0].TargetFQN)
	}
}

// TestMesh_Aggregate_SumsWeights verifies that Aggregate sums weights.
func TestMesh_Aggregate_SumsWeights(t *testing.T) {
	sg := &SymbolGraph{
		Nodes: []SymbolNode{
			{Name: "Foo", Package: "analyzer", Kind: "function"},
			{Name: "Bar", Package: "engine", Kind: "function"},
			{Name: "Baz", Package: "engine", Kind: "function"},
		},
		Edges: []SymbolEdge{
			{SourceFQN: "analyzer.Foo", TargetFQN: "engine.Bar", Kind: "call", Weight: 1.0},
			{SourceFQN: "analyzer.Foo", TargetFQN: "engine.Baz", Kind: "call", Weight: 0.8},
		},
	}
	mesh := BuildMesh(sg, []string{"analyzer", "engine"})

	agg := mesh.Aggregate(MeshComponent)
	// Both edges roll up to analyzer → engine, weights should sum
	for _, e := range agg {
		if e.SourceFQN == "analyzer" && e.TargetFQN == "engine" {
			if e.Weight != 1.8 {
				t.Errorf("expected aggregated weight 1.8, got %f", e.Weight)
			}
			return
		}
	}
	t.Error("expected analyzer → engine edge in aggregate")
}

// TestMesh_Neighborhood_SortsByWeight verifies that Neighborhood returns
// neighbors sorted by edge weight (highest first).
func TestMesh_Neighborhood_SortsByWeight(t *testing.T) {
	sg := &SymbolGraph{
		Nodes: []SymbolNode{
			{Name: "Foo", Package: "analyzer", Kind: "function"},
			{Name: "Low", Package: "fmt", Kind: "function"},
			{Name: "High", Package: "engine", Kind: "function"},
			{Name: "Mid", Package: "arch", Kind: "function"},
		},
		Edges: []SymbolEdge{
			{SourceFQN: "analyzer.Foo", TargetFQN: "fmt.Low", Kind: "call", Weight: 0.01},
			{SourceFQN: "analyzer.Foo", TargetFQN: "engine.High", Kind: "call", Weight: 1.0},
			{SourceFQN: "analyzer.Foo", TargetFQN: "arch.Mid", Kind: "call", Weight: 0.5},
		},
	}
	mesh := BuildMesh(sg, []string{"analyzer", "engine", "arch"})

	neighbors := mesh.NeighborhoodWeighted("analyzer.Foo", 1)
	if len(neighbors) < 3 {
		t.Fatalf("expected >= 3 neighbors, got %d", len(neighbors))
	}

	// Should be sorted: engine.High (1.0), arch.Mid (0.5), fmt.Low (0.01)
	weights := make([]float64, 0, len(neighbors))
	for _, n := range neighbors {
		weights = append(weights, n.Weight)
	}
	if !sort.Float64sAreSorted(reverseFloat64(weights)) {
		t.Errorf("neighbors not sorted by weight descending: %v", weights)
	}
}

func reverseFloat64(s []float64) []float64 {
	r := make([]float64, len(s))
	for i, v := range s {
		r[len(s)-1-i] = v
	}
	return r
}
