package arch

import (
	"testing"

	oculus "github.com/dpopsuev/oculus/v3"
	archgit "github.com/dpopsuev/oculus/v3/arch/git"
)

func TestMergeCoverage_ReplacesChangedOnly(t *testing.T) {
	base := []archgit.CoverageResult{
		{Component: "alpha", CoveragePct: 10},
		{Component: "beta", CoveragePct: 90},
	}
	fresh := []archgit.CoverageResult{
		{Component: "alpha", CoveragePct: 80},
	}
	got := mergeCoverage(base, fresh, []string{"alpha"})
	by := map[string]float64{}
	for _, c := range got {
		by[c.Component] = c.CoveragePct
	}
	if by["alpha"] != 80 {
		t.Fatalf("alpha=%v want 80", by["alpha"])
	}
	if by["beta"] != 90 {
		t.Fatalf("beta=%v want 90", by["beta"])
	}
}

func TestMergeAnchors_ReplacesChangedPackage(t *testing.T) {
	base := []oculus.SemanticAnchor{
		{Kind: oculus.AnchorEntryPoint, Name: "main", Package: "cmd"},
		{Kind: oculus.AnchorHTTPHandler, Name: "Handle", Package: "alpha"},
	}
	fresh := []oculus.SemanticAnchor{
		{Kind: oculus.AnchorHTTPHandler, Name: "HandleV2", Package: "alpha"},
	}
	got := mergeAnchors(base, fresh, []string{"alpha"})
	if len(got) != 2 {
		t.Fatalf("len=%d want 2: %+v", len(got), got)
	}
	var sawCmd, sawV2 bool
	for _, a := range got {
		if a.Package == "cmd" {
			sawCmd = true
		}
		if a.Name == "HandleV2" {
			sawV2 = true
		}
		if a.Name == "Handle" {
			t.Fatal("stale alpha Handle should be dropped")
		}
	}
	if !sawCmd || !sawV2 {
		t.Fatalf("got %+v", got)
	}
}

func TestMergeAuthors_ReplacesChangedOnly(t *testing.T) {
	base := map[string][]archgit.Author{
		"alpha": {{Name: "Old", Commits: 1}},
		"beta":  {{Name: "Keep", Commits: 5}},
	}
	fresh := map[string][]archgit.Author{
		"alpha": {{Name: "New", Commits: 3}},
	}
	got := mergeAuthors(base, fresh, []string{"alpha"})
	if got["alpha"][0].Name != "New" {
		t.Fatalf("alpha=%v", got["alpha"])
	}
	if got["beta"][0].Name != "Keep" {
		t.Fatalf("beta=%v", got["beta"])
	}
}
