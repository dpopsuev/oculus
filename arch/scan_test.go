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
			name: "high_fanin_without_nesting_does_not_qualify",
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
