package oculus_test

import (
	"testing"
	"time"

	"github.com/dpopsuev/oculus/v3"
	"github.com/dpopsuev/oculus/v3/testkit"
)

// TestMesh_ScalingBehavior runs BuildMesh + Boundaries at multiple scale
// points and logs the timing. Use -v to see the scaling table.
// Reveals O(n) vs O(n²) behavior empirically.
func TestMesh_ScalingBehavior(t *testing.T) {
	if testing.Short() {
		t.Skip("scaling analysis is expensive")
	}

	tiers := []testkit.ScaleTier{
		{Name: "10", Components: 10, Edges: 20},
		{Name: "25", Components: 25, Edges: 60},
		{Name: "50", Components: 50, Edges: 150},
		{Name: "100", Components: 100, Edges: 400},
		{Name: "200", Components: 200, Edges: 1000},
		{Name: "500", Components: 500, Edges: 5000},
	}

	t.Logf("%-8s %-12s %-12s %-12s %-12s", "tier", "edges", "build", "boundaries", "total")
	for _, tier := range tiers {
		sg := testkit.GenerateSymbolGraph(tier)
		names := testkit.GenerateComponentNames(tier)

		start := time.Now()
		m := oculus.BuildMesh(sg, names)
		buildTime := time.Since(start)

		start = time.Now()
		_ = m.BoundariesMinWeight(0.5)
		boundaryTime := time.Since(start)

		total := buildTime + boundaryTime
		t.Logf("%-8s %-12d %-12v %-12v %-12v",
			tier.Name, len(sg.Edges), buildTime, boundaryTime, total)
	}
}
