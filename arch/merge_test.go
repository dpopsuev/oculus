package arch

import (
	"context"
	"os"
	"path/filepath"
	"strings"
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

func TestRequireFullScan_TypeScriptAllowed(t *testing.T) {
	r := &ContextReport{}
	r.Project = model.NewProject("tsapp")
	r.Project.Language = model.LangTypeScript
	r.Architecture.Services = make([]ArchService, 4)
	if force, reason := RequireFullScan(r, []string{"src/a.ts"}, []string{"src"}); force {
		t.Fatalf("TS edit should allow merge; force=%v reason=%q", force, reason)
	}
}

func TestMergeScan_TypeScriptPackageEdit(t *testing.T) {
	dir := t.TempDir()
	files := map[string]string{
		"package.json": `{"name":"tsmerge"}`,
		"alpha/a.ts":   "export function a(): string { return 'a' }\n",
		"beta/b.ts":    "import { a } from '../alpha/a'\nexport function b() { return a() }\n",
		"gamma/g.ts":   "export function g() {}\n",
	}
	for name, body := range files {
		p := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	baseline, err := ScanAndBuild(ctx, dir, ScanOpts{Intent: IntentArchitecture, Depth: 0, ScannerOverride: "typescript"})
	if err != nil {
		t.Fatalf("baseline: %v", err)
	}
	if baseline.Project == nil || baseline.Project.Language != model.LangTypeScript {
		t.Fatalf("want TypeScript project, got %+v", baseline.Project)
	}
	if len(baseline.Architecture.Services) < 2 {
		t.Fatalf("want ≥2 services, got %d: %v", len(baseline.Architecture.Services), mergeServiceNames(baseline))
	}

	if err := os.WriteFile(filepath.Join(dir, "alpha/a.ts"),
		[]byte("export function a(): string { return 'a2' }\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	merged, err := MergeScan(ctx, dir, baseline, []string{"alpha/a.ts"}, ScanOpts{Intent: IntentArchitecture, Depth: 0})
	if err != nil {
		t.Fatalf("merge: %v", err)
	}
	if merged.ScanMode != ScanModeMerge {
		t.Fatalf("ScanMode=%q, want merge", merged.ScanMode)
	}
	var alphaChanged bool
	for _, s := range merged.Architecture.Services {
		if (s.Name == "alpha" || strings.HasSuffix(s.Name, "/alpha") || s.Name == "alpha") && s.Changed {
			alphaChanged = true
		}
		if s.Name == "alpha" && s.Changed {
			alphaChanged = true
		}
	}
	if !alphaChanged {
		// Component names may be dir basenames or paths.
		for _, s := range merged.Architecture.Services {
			if s.Changed && (s.Name == "alpha" || strings.Contains(s.Name, "alpha")) {
				alphaChanged = true
			}
		}
	}
	if !alphaChanged {
		t.Fatalf("alpha should be Changed; services=%v", mergeServiceNames(merged))
	}
	if len(merged.Architecture.Services) != len(baseline.Architecture.Services) {
		t.Fatalf("service count %d → %d (%v → %v)",
			len(baseline.Architecture.Services), len(merged.Architecture.Services),
			mergeServiceNames(baseline), mergeServiceNames(merged))
	}
}

func TestMergeScan_PackageJSONForcesFull(t *testing.T) {
	dir := t.TempDir()
	files := map[string]string{
		"package.json": `{"name":"tsmerge"}`,
		"alpha/a.ts":   "export function a() { return 1 }\n",
		"beta/b.ts":    "export function b() { return 2 }\n",
	}
	for name, body := range files {
		p := filepath.Join(dir, name)
		_ = os.MkdirAll(filepath.Dir(p), 0o755)
		_ = os.WriteFile(p, []byte(body), 0o644)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	baseline, err := ScanAndBuild(ctx, dir, ScanOpts{Intent: IntentArchitecture, ScannerOverride: "typescript"})
	if err != nil {
		t.Fatal(err)
	}
	merged, err := MergeScan(ctx, dir, baseline, []string{"package.json"}, ScanOpts{Intent: IntentArchitecture})
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
