package analyzer

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/dpopsuev/oculus"
)

// Fixture: two Go files with typed functions calling each other.
var typedEdgeFixture = map[string]string{
	"go.mod": "module example.com/typed\ngo 1.21\n",
	"main.go": `package main

type Config struct {
	Name string
}

type Result struct {
	OK bool
}

func LoadConfig(path string) *Config {
	return &Config{Name: path}
}

func Transform(cfg *Config) (*Result, error) {
	return &Result{OK: true}, nil
}

func main() {
	cfg := LoadConfig("app.yaml")
	result, _ := Transform(cfg)
	_ = result
}
`,
}

// TestGoAST_TypedEdges verifies GoAST produces ParamTypes/ReturnTypes on edges.
func TestGoAST_TypedEdges(t *testing.T) {
	dir := t.TempDir()
	if err := buildFixture(dir, typedEdgeFixture); err != nil {
		t.Fatal(err)
	}

	a := NewGoASTDeep(dir)
	if a == nil {
		t.Skip("not detected as Go project")
	}

	cg, err := a.CallGraph(dir, oculus.CallGraphOpts{Entry: "main", Depth: 5})
	if err != nil {
		t.Fatal(err)
	}

	assertEdgeHasTypes(t, cg, "main", "LoadConfig", []string{"string"}, []string{"*Config"})
	assertEdgeHasTypes(t, cg, "main", "Transform", []string{"*Config"}, []string{"*Result", "error"})
}

// TestTreeSitter_TypedEdges verifies TreeSitter produces ParamTypes/ReturnTypes.
func TestTreeSitter_TypedEdges(t *testing.T) {
	dir := t.TempDir()
	if err := buildFixture(dir, typedEdgeFixture); err != nil {
		t.Fatal(err)
	}

	ts, err := NewTreeSitterDeep(dir)
	if err != nil {
		t.Skipf("tree-sitter not available: %v", err)
	}

	cg, err := ts.CallGraph(dir, oculus.CallGraphOpts{Entry: "main", Depth: 5})
	if err != nil {
		t.Fatal(err)
	}

	assertEdgeHasTypes(t, cg, "main", "LoadConfig", []string{"string"}, []string{"*Config"})
	assertEdgeHasTypes(t, cg, "main", "Transform", []string{"*Config"}, []string{"*Result", "error"})
}

// TestFallback_TypedEdges verifies the fallback chain produces typed edges.
func TestFallback_TypedEdges(t *testing.T) {
	dir := t.TempDir()
	if err := buildFixture(dir, typedEdgeFixture); err != nil {
		t.Fatal(err)
	}

	da := NewDeepFallback(dir, nil)
	cg, err := da.CallGraph(dir, oculus.CallGraphOpts{Entry: "main", Depth: 5})
	if err != nil {
		t.Fatal(err)
	}

	// At minimum, the fallback should produce SOME typed edges.
	typed := 0
	for _, e := range cg.Edges {
		if len(e.ParamTypes) > 0 || len(e.ReturnTypes) > 0 {
			typed++
		}
	}
	if len(cg.Edges) > 0 && typed == 0 {
		t.Errorf("fallback produced %d edges but 0 with types (layer=%s)", len(cg.Edges), cg.Layer)
	}
	t.Logf("Fallback typed edges: %d/%d (layer=%s)", typed, len(cg.Edges), cg.Layer)
}

// TestDogfood_TypedEdgeCoverage runs on the Oculus repo and asserts
// minimum type coverage. This is the regression gate for OCL-BUG-2.
func TestDogfood_TypedEdgeCoverage(t *testing.T) {
	if testing.Short() {
		t.Skip("dogfood: skipping in -short mode")
	}
	root, err := filepath.Abs("..")
	if err != nil {
		t.Skip("cannot resolve repo root")
	}
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		t.Skip("not in a Go repo")
	}

	da := NewDeepFallback(root, nil)
	cg, err := da.CallGraph(root, oculus.CallGraphOpts{Entry: "main", Depth: 5})
	if err != nil {
		t.Fatalf("CallGraph: %v", err)
	}

	typed := 0
	for _, e := range cg.Edges {
		if len(e.ParamTypes) > 0 || len(e.ReturnTypes) > 0 {
			typed++
		}
	}

	pct := 0.0
	if len(cg.Edges) > 0 {
		pct = float64(typed) / float64(len(cg.Edges)) * 100
	}
	t.Logf("Dogfood typed edge coverage: %d/%d (%.0f%%, layer=%s)", typed, len(cg.Edges), pct, cg.Layer)

	// GoAST layer should have 100% type coverage.
	// LSP layer should have >80% (hover enrichment).
	// Regex layer has 0% (expected — no AST).
	// Gate: >50% for GoAST/LSP, skip for regex.
	if cg.Layer == oculus.LayerRegex {
		t.Logf("regex layer — typed edges not expected, skipping gate")
		return
	}
	if pct < 50 {
		t.Errorf("typed edge coverage %.0f%% < 50%% minimum (OCL-BUG-2 regression)", pct)
	}
}

// --- helpers ---

func assertEdgeHasTypes(t *testing.T, cg *oculus.CallGraph, caller, callee string, wantParams, wantReturns []string) {
	t.Helper()
	for _, e := range cg.Edges {
		if e.Caller == caller && e.Callee == callee {
			if !sliceEqual(e.ParamTypes, wantParams) {
				t.Errorf("edge %s→%s: ParamTypes=%v, want %v", caller, callee, e.ParamTypes, wantParams)
			}
			if !sliceEqual(e.ReturnTypes, wantReturns) {
				t.Errorf("edge %s→%s: ReturnTypes=%v, want %v", caller, callee, e.ReturnTypes, wantReturns)
			}
			return
		}
	}
	t.Errorf("edge %s→%s not found in %d edges", caller, callee, len(cg.Edges))
}

// buildFixture writes test files to a directory. Inline copy from testkit
// to avoid import cycle (analyzer → testkit → oculus → analyzer).
func buildFixture(dir string, files map[string]string) error {
	paths := make([]string, 0, len(files))
	for p := range files {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	for _, rel := range paths {
		abs := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
			return fmt.Errorf("mkdir %s: %w", filepath.Dir(abs), err)
		}
		if err := os.WriteFile(abs, []byte(files[rel]), 0o600); err != nil {
			return fmt.Errorf("write %s: %w", rel, err)
		}
	}
	return nil
}

func sliceEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
