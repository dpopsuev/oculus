package preset

import (
	"context"
	"strings"
	"testing"

	"github.com/dpopsuev/oculus/v3/arch"
	"github.com/dpopsuev/oculus/v3/graph"
	"github.com/dpopsuev/oculus/v3/model"
	"github.com/dpopsuev/oculus/v3/port"
)

func testReport() *arch.ContextReport {
	return &arch.ContextReport{
		ScanCore: arch.ScanCore{
			ModulePath:     "github.com/example/project",
			Scanner:        "test",
			SuggestedDepth: 1,
			Architecture: arch.ArchModel{
				Title: "project",
				Services: []arch.ArchService{
					{Name: "cmd/app", Package: "github.com/example/project/cmd/app", LOC: 100, Churn: 5, Language: model.LangGo, Symbols: model.SymbolsFromNames("main")},
					{Name: "internal/core", Package: "github.com/example/project/internal/core", LOC: 500, Churn: 20, Language: model.LangGo, Symbols: model.SymbolsFromNames("Run", "Config")},
					{Name: "internal/store", Package: "github.com/example/project/internal/store", LOC: 300, Churn: 8, Language: model.LangGo, Symbols: model.SymbolsFromNames("DB", "Get")},
				},
				Edges: []arch.ArchEdge{
					{From: "cmd/app", To: "internal/core", Weight: 1, CallSites: 3, LOCSurface: 10},
					{From: "internal/core", To: "internal/store", Weight: 1, CallSites: 5, LOCSurface: 15},
				},
			},
		},
		GraphMetrics: arch.GraphMetrics{
			HotSpots: []arch.HotSpot{
				{Component: "internal/core", FanIn: 2, Churn: 20},
			},
			ImportDepth: graph.DepthMap{
				"cmd/app":        0,
				"internal/core":  1,
				"internal/store": 2,
			},
		},
	}
}

func testDeps() Deps {
	return Deps{
		DesiredState: func(_ context.Context, _ string) (*port.DesiredState, error) {
			return port.NewDesiredState("domain", "application", "infrastructure"), nil
		},
	}
}

func TestNames(t *testing.T) {
	names := Names()
	if len(names) != 8 {
		t.Errorf("Names() returned %d, want 8", len(names))
	}
	expected := map[string]bool{
		ArchReview: true, HealthCheck: true, Onboarding: true, PrePR: true,
		Normative: true, PreRefactor: true, FullClinic: true, CodeHealth: true,
	}
	for _, n := range names {
		if !expected[n] {
			t.Errorf("unexpected preset name %q", n)
		}
	}
}

func TestPresetConstants(t *testing.T) {
	// Verify all constants are non-empty and distinct
	consts := []string{ArchReview, HealthCheck, Onboarding, PrePR, Normative, PreRefactor, FullClinic, CodeHealth}
	seen := make(map[string]bool)
	for _, c := range consts {
		if c == "" {
			t.Error("preset constant is empty")
		}
		if seen[c] {
			t.Errorf("duplicate preset constant %q", c)
		}
		seen[c] = true
	}
}

func TestRunUnknownPreset(t *testing.T) {
	_, err := Run(context.Background(), testReport(), "/tmp", "nonexistent", testDeps())
	if err == nil {
		t.Fatal("expected error for unknown preset")
	}
	if !strings.Contains(err.Error(), "unknown preset") {
		t.Errorf("error = %v, want 'unknown preset'", err)
	}
}

func TestRunArchReview(t *testing.T) {
	out, err := Run(context.Background(), testReport(), "/tmp", ArchReview, testDeps())
	if err != nil {
		t.Fatal(err)
	}
	assertContains(t, out, "Architecture Review")
	assertContains(t, out, "3 components")
}

func TestRunHealthCheck(t *testing.T) {
	out, err := Run(context.Background(), testReport(), "/tmp", HealthCheck, testDeps())
	if err != nil {
		t.Fatal(err)
	}
	assertContains(t, out, "Health Check")
	assertContains(t, out, "internal/core")
}

func TestRunOnboarding(t *testing.T) {
	out, err := Run(context.Background(), testReport(), "/tmp", Onboarding, testDeps())
	if err != nil {
		t.Fatal(err)
	}
	assertContains(t, out, "Onboarding")
	assertContains(t, out, "3 components")
}

func TestRunPrePR(t *testing.T) {
	out, err := Run(context.Background(), testReport(), "/tmp", PrePR, testDeps())
	if err != nil {
		t.Fatal(err)
	}
	assertContains(t, out, "Pre-PR Review")
	assertContains(t, out, "3 components")
}

func TestRunNormative(t *testing.T) {
	out, err := Run(context.Background(), testReport(), "/tmp", Normative, testDeps())
	if err != nil {
		t.Fatal(err)
	}
	assertContains(t, out, "Normative Analysis")
	assertContains(t, out, "Import Direction")
}

func TestRunPreRefactor(t *testing.T) {
	out, err := Run(context.Background(), testReport(), "/tmp", PreRefactor, testDeps())
	if err != nil {
		t.Fatal(err)
	}
	assertContains(t, out, "Pre-Refactor")
	assertContains(t, out, "Hot Spots")
}

func TestRunFullClinic(t *testing.T) {
	out, err := Run(context.Background(), testReport(), "/tmp", FullClinic, testDeps())
	if err != nil {
		t.Fatal(err)
	}
	assertContains(t, out, "Full Clinic")
	assertContains(t, out, "3 components")
}

func TestRunCodeHealth(t *testing.T) {
	out, err := Run(context.Background(), testReport(), "/tmp", CodeHealth, testDeps())
	if err != nil {
		t.Fatal(err)
	}
	assertContains(t, out, "Code Health")
	assertContains(t, out, "3 components")
}

func TestRunHealthCheckNoHotSpots(t *testing.T) {
	report := testReport()
	report.HotSpots = nil
	out, err := Run(context.Background(), report, "/tmp", HealthCheck, testDeps())
	if err != nil {
		t.Fatal(err)
	}
	assertContains(t, out, "No hot spots detected")
}

func TestResolveRolesAndAccepted_NilInputs(t *testing.T) {
	roles, accepted := ResolveRolesAndAccepted(nil, nil)
	if roles != nil {
		t.Error("expected nil roles")
	}
	if accepted != nil {
		t.Error("expected nil accepted")
	}
}

func TestRulesFromServices_Empty(t *testing.T) {
	rules := RulesFromServices(nil)
	if rules != nil {
		t.Error("expected nil rules for empty services")
	}
}

func TestRulesFromServices_Go(t *testing.T) {
	services := []arch.ArchService{
		{Name: "pkg", Language: model.LangGo},
	}
	rules := RulesFromServices(services)
	// Go should have rules
	if rules == nil {
		t.Error("expected non-nil rules for Go")
	}
}

// --- helpers ---

func assertContains(t *testing.T, output, want string) {
	t.Helper()
	if !strings.Contains(output, want) {
		t.Errorf("output missing %q\nfirst 500 chars:\n%s", want, truncate(output, 500))
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
