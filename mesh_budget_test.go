package oculus_test

import (
	"testing"
	"time"

	"github.com/dpopsuev/oculus/v3"
	"github.com/dpopsuev/oculus/v3/testkit"
)

func TestMeshTimeBudget_Medium(t *testing.T) {
	sg := testkit.GenerateSymbolGraph(testkit.TierMedium)
	names := testkit.GenerateComponentNames(testkit.TierMedium)

	start := time.Now()
	m := oculus.BuildMesh(sg, names)
	_ = m.BoundariesMinWeight(0.5)
	elapsed := time.Since(start)

	t.Logf("medium (%d edges): BuildMesh + Boundaries = %v", len(sg.Edges), elapsed)
	if elapsed > 5*time.Second {
		t.Errorf("medium mesh took %v, budget is 5s", elapsed)
	}
}

func TestMeshTimeBudget_Large(t *testing.T) {
	sg := testkit.GenerateSymbolGraph(testkit.TierLarge)
	names := testkit.GenerateComponentNames(testkit.TierLarge)

	start := time.Now()
	m := oculus.BuildMesh(sg, names)
	_ = m.BoundariesMinWeight(0.5)
	elapsed := time.Since(start)

	t.Logf("large (%d edges): BuildMesh + Boundaries = %v", len(sg.Edges), elapsed)
	if elapsed > 30*time.Second {
		t.Errorf("large mesh took %v, budget is 30s", elapsed)
	}
}
