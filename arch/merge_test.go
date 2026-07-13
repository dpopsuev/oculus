package arch

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/dpopsuev/oculus/v3/model"
)

func writeMergeFixture(t *testing.T, root string) {
	t.Helper()
	files := map[string]string{
		"go.mod":     "module mergetest\ngo 1.21\n",
		"alpha/a.go": "package alpha\n\nfunc A() string { return \"a\" }\n",
		"beta/b.go":  "package beta\n\nimport \"mergetest/alpha\"\n\nfunc B() string { return alpha.A() }\n",
		"gamma/g.go": "package gamma\n\nfunc G() {}\n",
	}
	for name, body := range files {
		p := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestRequireFullScan_ManifestAndThreshold(t *testing.T) {
	r := &ContextReport{}
	r.Project = model.NewProject("mergetest")
	r.Project.Language = model.LangGo
	r.Architecture.Services = make([]ArchService, 10)
	if force, reason := RequireFullScan(r, []string{"go.mod"}, []string{"(root)"}); !force || reason != ReasonManifestChange {
		t.Fatalf("go.mod: force=%v reason=%q", force, reason)
	}
	many := make([]string, 20)
	for i := range many {
		many[i] = "pkg" + string(rune('a'+i%26))
	}
	if force, reason := RequireFullScan(r, nil, many); !force || reason != ReasonTooManyPkgs {
		t.Fatalf("too many: force=%v reason=%q", force, reason)
	}
	if force, _ := RequireFullScan(r, []string{"alpha/a.go"}, []string{"alpha"}); force {
		t.Fatal("single package edit should allow merge")
	}
}

func TestMergeScan_SinglePackageEdit(t *testing.T) {
	dir := t.TempDir()
	writeMergeFixture(t, dir)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	baseline, err := ScanAndBuild(ctx, dir, ScanOpts{Intent: IntentArchitecture, Depth: 0})
	if err != nil {
		t.Fatalf("baseline: %v", err)
	}
	if len(baseline.Architecture.Services) < 3 {
		t.Fatalf("want ≥3 services, got %d: %v", len(baseline.Architecture.Services), mergeServiceNames(baseline))
	}
	baseline.ScanMode = ScanModeFull

	if err := os.WriteFile(filepath.Join(dir, "alpha/a.go"),
		[]byte("package alpha\n\nfunc A() string { return \"a2\" }\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	merged, err := MergeScan(ctx, dir, baseline, []string{"alpha/a.go"}, ScanOpts{Intent: IntentArchitecture, Depth: 0})
	if err != nil {
		t.Fatalf("merge: %v", err)
	}
	if merged.ScanMode != ScanModeMerge {
		t.Fatalf("ScanMode=%q, want merge", merged.ScanMode)
	}
	var alphaChanged bool
	for _, s := range merged.Architecture.Services {
		if s.Name == "alpha" && s.Changed {
			alphaChanged = true
		}
	}
	if !alphaChanged {
		t.Fatalf("alpha should be Changed; services=%v", mergeServiceNames(merged))
	}
	if len(merged.Architecture.Services) != len(baseline.Architecture.Services) {
		t.Fatalf("service count %d → %d", len(baseline.Architecture.Services), len(merged.Architecture.Services))
	}
}

func TestMergeScan_GoModForcesFull(t *testing.T) {
	dir := t.TempDir()
	writeMergeFixture(t, dir)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	baseline, err := ScanAndBuild(ctx, dir, ScanOpts{Intent: IntentArchitecture})
	if err != nil {
		t.Fatal(err)
	}
	merged, err := MergeScan(ctx, dir, baseline, []string{"go.mod"}, ScanOpts{Intent: IntentArchitecture})
	if err != nil {
		t.Fatal(err)
	}
	if merged.ScanMode != ScanModeFull {
		t.Fatalf("ScanMode=%q, want full", merged.ScanMode)
	}
}

func mergeServiceNames(r *ContextReport) []string {
	out := make([]string, len(r.Architecture.Services))
	for i, s := range r.Architecture.Services {
		out[i] = s.Name
	}
	return out
}
