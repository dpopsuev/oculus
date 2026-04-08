package constraint_test

import (
	"testing"

	"github.com/dpopsuev/oculus/arch"
	"github.com/dpopsuev/oculus/constraint"
	"github.com/dpopsuev/oculus/graph"
)

func TestComputeImportDirection_External(t *testing.T) {
	edges := []arch.ArchEdge{{From: "a", To: "b", Weight: 1}}
	depths := graph.ImportDepth(edges)
	report := constraint.ComputeImportDirection(edges, depths)
	if report == nil {
		t.Fatal("nil report")
	}
}
