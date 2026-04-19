package constraint

import (
	"testing"

	"github.com/dpopsuev/oculus/v3/arch"
	"github.com/dpopsuev/oculus/v3/port"
)

func TestComputeBudgetViolations_NoConstraints(t *testing.T) {
	report := ComputeBudgetViolations(nil, nil, nil)
	if report.Failing != 0 {
		t.Fatalf("expected 0 failing, got %d", report.Failing)
	}
	if report.Passing != 0 {
		t.Fatalf("expected 0 passing, got %d", report.Passing)
	}
}

func TestComputeBudgetViolations_AllPassing(t *testing.T) {
	services := []arch.ArchService{
		{Name: "pkg/core", Churn: 3, MaxNesting: 2},
		{Name: "pkg/util", Churn: 1, MaxNesting: 1},
	}
	edges := []arch.ArchEdge{
		{From: "pkg/util", To: "pkg/core"},
	}
	constraints := []port.HealthConstraint{
		{Component: "pkg/core", MaxFanIn: 5, MaxChurn: 10, MaxNesting: 5},
	}

	report := ComputeBudgetViolations(services, edges, constraints)

	if report.Failing != 0 {
		t.Fatalf("expected 0 failing, got %d", report.Failing)
	}
	if report.Passing != 3 {
		t.Fatalf("expected 3 passing, got %d", report.Passing)
	}
	if len(report.Violations) != 0 {
		t.Fatalf("expected no violations, got %d", len(report.Violations))
	}
}

func TestComputeBudgetViolations_Warning(t *testing.T) {
	services := []arch.ArchService{
		{Name: "pkg/core", Churn: 8, MaxNesting: 3},
	}
	// 3 edges pointing at pkg/core => fan_in = 3
	edges := []arch.ArchEdge{
		{From: "a", To: "pkg/core"},
		{From: "b", To: "pkg/core"},
		{From: "c", To: "pkg/core"},
	}
	constraints := []port.HealthConstraint{
		{Component: "pkg/core", MaxFanIn: 2, MaxChurn: 5},
	}

	report := ComputeBudgetViolations(services, edges, constraints)

	if report.Failing != 2 {
		t.Fatalf("expected 2 failing, got %d", report.Failing)
	}
	for _, v := range report.Violations {
		if v.Severity != port.SeverityWarning {
			t.Errorf("expected warning severity for %s, got %s", v.Metric, v.Severity)
		}
	}
}

func TestComputeBudgetViolations_Error(t *testing.T) {
	services := []arch.ArchService{
		{Name: "pkg/core", Churn: 25, MaxNesting: 12},
	}
	edges := []arch.ArchEdge{
		{From: "a", To: "pkg/core"},
		{From: "b", To: "pkg/core"},
		{From: "c", To: "pkg/core"},
		{From: "d", To: "pkg/core"},
		{From: "e", To: "pkg/core"},
	}
	constraints := []port.HealthConstraint{
		{Component: "pkg/core", MaxFanIn: 2, MaxChurn: 10, MaxNesting: 5},
	}

	report := ComputeBudgetViolations(services, edges, constraints)

	if report.Failing != 3 {
		t.Fatalf("expected 3 failing, got %d", report.Failing)
	}
	for _, v := range report.Violations {
		if v.Severity != port.SeverityWarning {
			t.Errorf("expected warning severity for %s (actual=%.0f, budget=%.0f), got %s",
				v.Metric, v.Actual, v.Budget, v.Severity)
		}
	}
}

func TestComputeBudgetViolations_MixedSeverity(t *testing.T) {
	services := []arch.ArchService{
		{Name: "svc", Churn: 7, MaxNesting: 6},
	}
	edges := []arch.ArchEdge{
		{From: "x", To: "svc"},
		{From: "y", To: "svc"},
		{From: "z", To: "svc"},
	}
	// All budget violations are now warning regardless of overshoot.
	constraints := []port.HealthConstraint{
		{Component: "svc", MaxFanIn: 2, MaxChurn: 5, MaxNesting: 2},
	}

	report := ComputeBudgetViolations(services, edges, constraints)

	if report.Failing != 3 {
		t.Fatalf("expected 3 failing, got %d", report.Failing)
	}

	for _, v := range report.Violations {
		if v.Severity != port.SeverityWarning {
			t.Errorf("%s: expected warning, got %s", v.Metric, v.Severity)
		}
	}
}

func TestComputeBudgetViolations_UnknownComponent(t *testing.T) {
	services := []arch.ArchService{
		{Name: "pkg/core"},
	}
	constraints := []port.HealthConstraint{
		{Component: "nonexistent", MaxFanIn: 1},
	}

	report := ComputeBudgetViolations(services, nil, constraints)

	if report.Failing != 0 {
		t.Fatalf("expected 0 failing for unknown component, got %d", report.Failing)
	}
	if report.Passing != 0 {
		t.Fatalf("expected 0 passing for unknown component, got %d", report.Passing)
	}
}

func TestComputeBudgetViolations_Summary(t *testing.T) {
	services := []arch.ArchService{
		{Name: "a", Churn: 10},
	}
	constraints := []port.HealthConstraint{
		{Component: "a", MaxChurn: 5},
	}

	report := ComputeBudgetViolations(services, nil, constraints)
	if report.Summary == "" {
		t.Fatal("expected non-empty summary")
	}
	t.Logf("Summary: %s", report.Summary)
}
