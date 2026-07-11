package arch

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestScanAndBuild_IntentLevels(t *testing.T) {
	// Use the locus repo itself as a fixture.
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Skip("cannot resolve repo root")
	}
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		t.Skip("not in a Go repo")
	}

	tests := []struct {
		intent         ScanIntent
		wantCycles     bool
		wantHotSpots   bool
		wantNesting    bool
		wantGitHistory bool
	}{
		{IntentArchitecture, false, false, false, false},
		{IntentCoupling, true, true, false, false},
		{IntentHealth, true, true, true, true},
	}

	for _, tt := range tests {
		t.Run(string(tt.intent), func(t *testing.T) {
			report, err := ScanAndBuild(context.Background(), root, ScanOpts{
				ExcludeTests: true,
				Depth:        2,
				ChurnDays:    7,
				Intent:       tt.intent,
			})
			if err != nil {
				t.Fatalf("scan: %v", err)
			}
			if len(report.Architecture.Services) == 0 {
				t.Error("expected at least one service")
			}
			if report.ModulePath == "" {
				t.Error("expected non-empty module path")
			}

			hasCycles := report.Cycles != nil
			if hasCycles != tt.wantCycles {
				t.Errorf("cycles: got %v, want %v", hasCycles, tt.wantCycles)
			}

			hasHotSpots := report.HotSpots != nil
			if hasHotSpots != tt.wantHotSpots {
				t.Errorf("hot spots: got %v, want %v", hasHotSpots, tt.wantHotSpots)
			}

			hasNesting := hasAnyNesting(report)
			if hasNesting != tt.wantNesting {
				t.Errorf("nesting: got %v, want %v", hasNesting, tt.wantNesting)
			}

			hasGitHistory := len(report.RecentCommits) > 0
			if hasGitHistory != tt.wantGitHistory {
				t.Errorf("git history: got %v, want %v", hasGitHistory, tt.wantGitHistory)
			}
		})
	}
}

func hasAnyNesting(r *ContextReport) bool {
	for i := range r.Architecture.Services {
		if r.Architecture.Services[i].MaxNesting > 0 {
			return true
		}
	}
	return false
}

func TestScanIntentLevel(t *testing.T) {
	tests := []struct {
		intent ScanIntent
		level  int
	}{
		{IntentArchitecture, 0},
		{IntentCoupling, 1},
		{IntentHealth, 2},
		{IntentFull, 3},
		{"", 2},        // default
		{"unknown", 2}, // unknown defaults to health
	}
	for _, tt := range tests {
		got := tt.intent.ScanLevel()
		if got != tt.level {
			t.Errorf("ScanIntent(%q).ScanLevel() = %d, want %d", tt.intent, got, tt.level)
		}
	}
}

func TestComputeHotSpots(t *testing.T) {
	tests := []struct {
		name      string
		services  []ArchService
		edges     []ArchEdge
		wantNames []string
	}{
		{
			name: "churn_only_qualifies_without_fanin",
			services: []ArchService{
				{Name: "hot", Churn: MinChurnHotSpot},
				{Name: "cold", Churn: MinChurnHotSpot - 1},
			},
			wantNames: []string{"hot"},
		},
		{
			name: "structural_hub_with_deep_nesting_qualifies",
			services: []ArchService{
				{Name: "hub", MaxNesting: MinNestingHotSpot},
				{Name: "leaf", MaxNesting: MinNestingHotSpot}, // high nesting but low fan-in
			},
			edges: []ArchEdge{
				{From: "a", To: "hub"},
				{From: "b", To: "hub"},
				{From: "c", To: "hub"},
			},
			wantNames: []string{"hub"},
		},
		{
			name: "moderate_fanin_without_nesting_does_not_qualify",
			services: []ArchService{
				{Name: "hub", MaxNesting: MinNestingHotSpot - 1, Churn: MinChurnHotSpot - 1},
			},
			edges: []ArchEdge{
				{From: "a", To: "hub"},
				{From: "b", To: "hub"},
				{From: "c", To: "hub"},
			},
			wantNames: nil,
		},
		{
			name: "central_hub_fanin_alone_qualifies",
			services: []ArchService{
				{Name: "core", MaxNesting: 0, Churn: 0},
			},
			edges: []ArchEdge{
				{From: "a", To: "core"},
				{From: "b", To: "core"},
				{From: "c", To: "core"},
				{From: "d", To: "core"},
				{From: "e", To: "core"},
			},
			wantNames: []string{"core"},
		},
		{
			name:      "nothing_qualifies",
			services:  []ArchService{{Name: "fine"}},
			wantNames: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := ArchModel{Services: tt.services, Edges: tt.edges}
			got := computeHotSpots(m)
			if len(got) != len(tt.wantNames) {
				t.Fatalf("got %d hot spots, want %d: %+v", len(got), len(tt.wantNames), got)
			}
			for i, want := range tt.wantNames {
				if got[i].Component != want {
					t.Errorf("spot[%d] = %q, want %q", i, got[i].Component, want)
				}
			}
		})
	}
}

// TestProjectToArch_TestCoverageMetadata verifies that ArchService.HasTests and
// CoverageRatio are populated from the namespace file list.
//
// Given a Go project where one package has *_test.go files and another does not
// When ScanAndBuild is called
// Then the tested package has HasTests=true and CoverageRatio > 0
// And the untested package has HasTests=false and CoverageRatio=0
func TestProjectToArch_TestCoverageMetadata(t *testing.T) {
	dir := t.TempDir()
	files := map[string]string{
		"go.mod":                 "module example.com/cov\n\ngo 1.21\n",
		"api/api.go":             "package api\n\nfunc Handle() {}\n",
		"api/api_test.go":        "package api\n\nimport \"testing\"\n\nfunc TestHandle(t *testing.T) {}\n",
		"store/store.go":         "package store\n\nfunc Get() string { return \"\" }\n",
		// store has no test file
	}
	for name, content := range files {
		p := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	report, err := ScanAndBuild(context.Background(), dir, ScanOpts{Intent: IntentArchitecture})
	if err != nil {
		t.Fatalf("ScanAndBuild: %v", err)
	}

	svcMap := make(map[string]ArchService)
	for _, svc := range report.Architecture.Services {
		svcMap[svc.Name] = svc
	}

	api, ok := svcMap["example.com/cov/api"]
	if !ok {
		// Try short name.
		api, ok = svcMap["api"]
	}
	if !ok {
		t.Fatalf("api component not found; have: %v", func() []string {
			names := make([]string, 0, len(svcMap))
			for k := range svcMap {
				names = append(names, k)
			}
			return names
		}())
	}
	if !api.HasTests {
		t.Errorf("api: HasTests=false, want true (has api_test.go)")
	}
	if api.CoverageRatio <= 0 {
		t.Errorf("api: CoverageRatio=%f, want > 0", api.CoverageRatio)
	}

	store, ok := svcMap["example.com/cov/store"]
	if !ok {
		store, ok = svcMap["store"]
	}
	if !ok {
		t.Fatalf("store component not found")
	}
	if store.HasTests {
		t.Errorf("store: HasTests=true, want false (no test files)")
	}
	if store.CoverageRatio != 0 {
		t.Errorf("store: CoverageRatio=%f, want 0", store.CoverageRatio)
	}
}
