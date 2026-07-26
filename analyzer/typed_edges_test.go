package analyzer

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/dpopsuev/oculus/v3"
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

	cg, err := a.CallGraph(context.Background(), dir, oculus.CallGraphOpts{Entry: "main", Depth: 5})
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

	cg, err := ts.CallGraph(context.Background(), dir, oculus.CallGraphOpts{Entry: "main", Depth: 5})
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
	cg, err := da.CallGraph(context.Background(), dir, oculus.CallGraphOpts{Entry: "main", Depth: 5})
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

func TestSyntheticRepository_TypedEdgeCoverage(t *testing.T) {
	root := t.TempDir()
	if err := buildFixture(root, typedEdgeFixture); err != nil {
		t.Fatal(err)
	}
	deepAnalyzer := NewDeepFallback(root, nil)
	callGraph, err := deepAnalyzer.CallGraph(context.Background(), root, oculus.CallGraphOpts{Entry: "main", Depth: 5})
	if err != nil {
		t.Fatalf("CallGraph: %v", err)
	}

	typed := 0
	for _, edge := range callGraph.Edges {
		if len(edge.ParamTypes) > 0 || len(edge.ReturnTypes) > 0 {
			typed++
		}
	}
	if callGraph.Layer == oculus.LayerRegex {
		return
	}
	if len(callGraph.Edges) == 0 {
		t.Fatal("expected call edges")
	}
	coverage := float64(typed) / float64(len(callGraph.Edges)) * 100
	if coverage < 50 {
		t.Errorf("typed edge coverage %.0f%% < 50%%", coverage)
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
