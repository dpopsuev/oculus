package impact_test

import (
	"testing"

	"github.com/dpopsuev/oculus/arch"
	"github.com/dpopsuev/oculus/impact"
)

func TestComputeRiskScores_External(t *testing.T) {
	services := []arch.ArchService{{Name: "a", LOC: 100, Churn: 5}}
	edges := []arch.ArchEdge{{From: "b", To: "a", Weight: 1}}
	report := impact.ComputeRiskScores(services, edges, nil)
	if report == nil {
		t.Fatal("nil report")
	}
}
