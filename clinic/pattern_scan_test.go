package clinic

import (
	"testing"

	"github.com/dpopsuev/oculus/v3/arch"
	"github.com/dpopsuev/oculus/v3/graph"
	"github.com/dpopsuev/oculus/v3/port"
)

func TestComputePatternScan_CircularDependency(t *testing.T) {
	services := []arch.ArchService{
		{Name: "pkg/a", Package: "example.com/pkg/a"},
		{Name: "pkg/b", Package: "example.com/pkg/b"},
	}
	edges := []arch.ArchEdge{
		{From: "pkg/a", To: "pkg/b"},
		{From: "pkg/b", To: "pkg/a"},
	}
	cycles := []graph.Cycle{{"pkg/a", "pkg/b"}}

	report := ComputePatternScan(services, edges, cycles, nil, nil, nil, nil)

	foundA := false
	foundB := false
	for _, d := range report.Detections {
		if d.PatternID == "circular_dependency" {
			if d.Component == "pkg/a" {
				foundA = true
			}
			if d.Component == "pkg/b" {
				foundB = true
			}
		}
	}
	if !foundA {
		t.Error("circular_dependency not detected for pkg/a")
	}
	if !foundB {
		t.Error("circular_dependency not detected for pkg/b")
	}
}

func TestComputePatternScan_BidirectionalEdge(t *testing.T) {
	services := []arch.ArchService{
		{Name: "pkg/a"},
		{Name: "pkg/b"},
	}
	edges := []arch.ArchEdge{
		{From: "pkg/a", To: "pkg/b"},
		{From: "pkg/b", To: "pkg/a"},
	}

	report := ComputePatternScan(services, edges, nil, nil, nil, nil, nil)

	found := false
	for _, d := range report.Detections {
		if d.PatternID == "inappropriate_intimacy" {
			found = true
		}
	}
	if !found {
		t.Error("inappropriate_intimacy not detected for bidirectional edge")
	}
}

func TestComputePatternScan_Clean(t *testing.T) {
	services := []arch.ArchService{
		{Name: "pkg/core"},
		{Name: "pkg/util"},
	}
	edges := []arch.ArchEdge{
		{From: "pkg/util", To: "pkg/core"},
	}

	report := ComputePatternScan(services, edges, nil, nil, nil, nil, nil)

	if report.SmellsFound != 0 {
		t.Errorf("expected 0 smells for clean architecture, got %d", report.SmellsFound)
	}
	if report.Summary != "No structural issues detected" {
		t.Errorf("unexpected summary: %s", report.Summary)
	}
}

func TestComputePatternScan_EmptyInput(t *testing.T) {
	report := ComputePatternScan(nil, nil, nil, nil, nil, nil, nil)

	if report.SmellsFound != 0 {
		t.Errorf("expected 0 smells, got %d", report.SmellsFound)
	}
}

func TestComputePatternScan_AcceptedViolation(t *testing.T) {
	services := []arch.ArchService{
		{Name: "pkg/a"},
		{Name: "pkg/b"},
	}
	edges := []arch.ArchEdge{
		{From: "pkg/a", To: "pkg/b"},
		{From: "pkg/b", To: "pkg/a"},
	}
	accepted := []port.AcceptedViolation{
		{Component: "pkg/a", Principle: "inappropriate_intimacy"},
	}

	report := ComputePatternScan(services, edges, nil, nil, nil, nil, accepted)

	for _, d := range report.Detections {
		if d.Component == "pkg/a" && d.PatternID == "inappropriate_intimacy" {
			t.Error("accepted violation should be suppressed")
		}
	}
}

func TestGetPatternCatalog_All(t *testing.T) {
	report := GetPatternCatalog("")

	if len(report.Entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(report.Entries))
	}
}

func TestGetPatternCatalog_FilterSmells(t *testing.T) {
	report := GetPatternCatalog("smell")

	if len(report.Entries) != 2 {
		t.Fatalf("expected 2 smell entries, got %d", len(report.Entries))
	}
}

func TestGetPatternCatalog_FilterByName(t *testing.T) {
	report := GetPatternCatalog("circular")

	if len(report.Entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(report.Entries))
	}
	if report.Entries[0].ID != "circular_dependency" {
		t.Errorf("expected circular_dependency, got %s", report.Entries[0].ID)
	}
}
