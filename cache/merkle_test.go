package cache

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func writeTree(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for rel, body := range files {
		p := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestBuildMerkle_StableAndOrderIndependent(t *testing.T) {
	dir := t.TempDir()
	writeTree(t, dir, map[string]string{
		"go.mod":      "module example.com/m\n",
		"pkg/a.go":    "package pkg\n",
		"pkg/b.go":    "package pkg\nfunc B() {}\n",
		"README.md":   "ignored non-source\n",
		"vendor/x.go": "package vendor\n", // skipped dir
	})

	a, err := BuildMerkle(dir)
	if err != nil {
		t.Fatalf("BuildMerkle: %v", err)
	}
	if a.Root == "" {
		t.Fatal("empty Merkle root")
	}
	if _, ok := a.Leaves["pkg/a.go"]; !ok {
		t.Fatalf("missing leaf pkg/a.go: %#v", a.Leaves)
	}
	if _, ok := a.Leaves["vendor/x.go"]; ok {
		t.Fatal("vendor/ should be skipped")
	}
	if _, ok := a.Leaves["README.md"]; ok {
		t.Fatal("non-source README.md should be skipped")
	}

	b, err := BuildMerkle(dir)
	if err != nil {
		t.Fatalf("BuildMerkle again: %v", err)
	}
	if a.Root != b.Root {
		t.Fatalf("root not stable: %q vs %q", a.Root, b.Root)
	}
}

func TestBuildMerkle_ContentChangeMovesRoot(t *testing.T) {
	dir := t.TempDir()
	writeTree(t, dir, map[string]string{
		"go.mod":   "module example.com/m\n",
		"main.go":  "package main\n",
	})
	before, err := BuildMerkle(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\nfunc main() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	after, err := BuildMerkle(dir)
	if err != nil {
		t.Fatal(err)
	}
	if before.Root == after.Root {
		t.Fatal("content change should move Merkle root")
	}
}

func TestDiff_EditAddDelete(t *testing.T) {
	a := Index{
		Root: "a",
		Leaves: map[string]string{
			"pkg/a.go": "h1",
			"pkg/b.go": "h2",
			"gone.go":  "h3",
		},
	}
	b := Index{
		Root: "b",
		Leaves: map[string]string{
			"pkg/a.go": "h1",      // unchanged
			"pkg/b.go": "h2-new",  // edited
			"new.go":   "h4",      // added
		},
	}
	got := Diff(a, b)
	slices.Sort(got)
	want := []string{"gone.go", "new.go", "pkg/b.go"}
	if !slices.Equal(got, want) {
		t.Fatalf("Diff = %v, want %v", got, want)
	}
}

func TestPackages_FromPaths(t *testing.T) {
	got := Packages([]string{
		"engine/protocol.go",
		"engine/foo.go",
		"cache/merkle.go",
		"main.go",
	})
	slices.Sort(got)
	want := []string{"(root)", "cache", "engine"}
	if !slices.Equal(got, want) {
		t.Fatalf("Packages = %v, want %v", got, want)
	}
}
