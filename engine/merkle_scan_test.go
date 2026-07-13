package engine

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/dpopsuev/oculus/v3/arch"
	"github.com/dpopsuev/oculus/v3/cache"
)

func initMerkleFixture(t *testing.T) (dir, sha string) {
	t.Helper()
	dir = t.TempDir()
	files := map[string]string{
		"go.mod":       "module merkletest\ngo 1.21\n",
		"pkg/a.go":     "package pkg\n\nfunc A() {}\n",
		"pkg/b.go":     "package pkg\n\nfunc B() {}\n",
		"cmd/main.go":  "package main\n\nfunc main() {}\n",
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
	for _, args := range [][]string{
		{"init", "-q"},
		{"config", "user.email", "t@t"},
		{"config", "user.name", "t"},
		{"add", "-A"},
		{"commit", "-q", "-m", "init"},
	} {
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %s: %v", args, out, err)
		}
	}
	sha = cache.ResolveHEAD(dir)
	if sha == "" {
		t.Fatal("empty HEAD")
	}
	return dir, sha
}

func TestScanProject_StampsMerkleRoot(t *testing.T) {
	dir, sha := initMerkleFixture(t)
	ms := &mockStore{headSHA: sha, reportsBySHA: map[string]*arch.ContextReport{}}
	eng := New(ms, []string{dir})
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	r1, err := eng.ScanProject(ctx, dir, ScanOpts{Intent: "architecture"})
	if err != nil {
		t.Fatalf("scan1: %v", err)
	}
	if r1.Report.MerkleRoot == "" {
		t.Fatal("expected MerkleRoot stamped on first scan")
	}
	if len(r1.Report.MerkleLeaves) == 0 {
		t.Fatal("expected MerkleLeaves stamped on first scan")
	}

	// Clean tree, same SHA → cache hit, same root.
	r2, err := eng.ScanProject(ctx, dir, ScanOpts{Intent: "architecture"})
	if err != nil {
		t.Fatalf("scan2: %v", err)
	}
	if r2.Report.MerkleRoot != r1.Report.MerkleRoot {
		t.Fatalf("clean hit root moved: %q → %q", r1.Report.MerkleRoot, r2.Report.MerkleRoot)
	}
	if ms.putReportCalls < 1 {
		t.Fatal("expected PutReport on first scan")
	}
	putsAfterHit := ms.putReportCalls
	r3, err := eng.ScanProject(ctx, dir, ScanOpts{Intent: "architecture"})
	if err != nil {
		t.Fatalf("scan3: %v", err)
	}
	_ = r3
	if ms.putReportCalls != putsAfterHit {
		t.Fatalf("clean cache hit should not PutReport again; puts %d → %d", putsAfterHit, ms.putReportCalls)
	}
}

func TestScanProject_DirtyTreeInvalidatesSHACache(t *testing.T) {
	dir, sha := initMerkleFixture(t)
	ms := &mockStore{headSHA: sha, reportsBySHA: map[string]*arch.ContextReport{}}
	eng := New(ms, []string{dir})
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	r1, err := eng.ScanProject(ctx, dir, ScanOpts{Intent: "architecture"})
	if err != nil {
		t.Fatalf("scan1: %v", err)
	}
	root1 := r1.Report.MerkleRoot
	puts := ms.putReportCalls

	// Dirty without commit — HEAD SHA unchanged.
	if err := os.WriteFile(filepath.Join(dir, "pkg/a.go"), []byte("package pkg\n\nfunc A() { /* dirty */ }\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	r2, err := eng.ScanProject(ctx, dir, ScanOpts{Intent: "architecture"})
	if err != nil {
		t.Fatalf("scan2: %v", err)
	}
	if r2.Report.MerkleRoot == "" || r2.Report.MerkleRoot == root1 {
		t.Fatalf("dirty rescan MerkleRoot=%q, want changed from %q", r2.Report.MerkleRoot, root1)
	}
	if ms.putReportCalls <= puts {
		t.Fatal("dirty tree should rescan and PutReport")
	}
	if r2.Report.ScanMode != arch.ScanModeMerge && r2.Report.ScanMode != arch.ScanModeFull {
		t.Fatalf("ScanMode=%q, want merge or full", r2.Report.ScanMode)
	}
	// Prefer merge for single-package dirty edit.
	if r2.Report.ScanMode != arch.ScanModeMerge {
		t.Logf("ScanMode=%s (merge preferred for single-pkg dirty)", r2.Report.ScanMode)
	}
	// pkg/ component should be marked Changed when service names match.
	var sawChanged bool
	for _, s := range r2.Report.Architecture.Services {
		if s.Changed {
			sawChanged = true
			t.Logf("changed service: %s", s.Name)
		}
	}
	if !sawChanged {
		// Soft if grouping depths rename packages — still require new MerkleRoot.
		t.Logf("no Changed flags (component names may not match pkg/); services=%v",
			serviceNames(r2.Report))
	}
}

func serviceNames(r *arch.ContextReport) []string {
	out := make([]string, len(r.Architecture.Services))
	for i, s := range r.Architecture.Services {
		out[i] = s.Name
	}
	return out
}

func TestMarkChangedPackages(t *testing.T) {
	r := &arch.ContextReport{}
	r.Architecture.Services = []arch.ArchService{
		{Name: "pkg"},
		{Name: "engine"},
		{Name: "other"},
	}
	MarkChangedPackages(r, []string{"pkg/a.go", "engine/protocol.go"})
	if !r.Architecture.Services[0].Changed || !r.Architecture.Services[1].Changed {
		t.Fatalf("want pkg+engine Changed, got %+v", r.Architecture.Services)
	}
	if r.Architecture.Services[2].Changed {
		t.Fatal("other should not be Changed")
	}
}
