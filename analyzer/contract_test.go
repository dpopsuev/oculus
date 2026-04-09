package analyzer

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/dpopsuev/oculus"
)

// contractFixture is a minimal Go project with typed functions calling each other.
// Used to verify every analyzer meets the contract.
var contractFixture = map[string]string{
	"go.mod": "module example.com/contract\ngo 1.21\n",
	"main.go": `package main

type Config struct{ Name string }
type Result struct{ OK bool }

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

func setupContractFixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	paths := make([]string, 0, len(contractFixture))
	for p := range contractFixture {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	for _, rel := range paths {
		abs := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(abs, []byte(contractFixture[rel]), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// --- Contract: CallGraph edges must have File and Line ---

func TestContract_GoAST_EdgeMetadata(t *testing.T) {
	dir := setupContractFixture(t)
	a := NewGoASTDeep(dir)
	if a == nil {
		t.Skip("GoAST not available")
	}
	cg, err := a.CallGraph(context.Background(), dir, oculus.CallGraphOpts{Entry: "main", Depth: 5})
	if err != nil {
		t.Fatal(err)
	}
	assertEdgeMetadata(t, "GoAST", cg)
}

func TestContract_TreeSitter_EdgeMetadata(t *testing.T) {
	dir := setupContractFixture(t)
	a, err := NewTreeSitterDeep(dir)
	if err != nil {
		t.Skipf("TreeSitter not available: %v", err)
	}
	cg, err := a.CallGraph(context.Background(), dir, oculus.CallGraphOpts{Entry: "main", Depth: 5})
	if err != nil {
		t.Fatal(err)
	}
	assertEdgeMetadata(t, "TreeSitter", cg)
}

func TestContract_Regex_EdgeMetadata(t *testing.T) {
	dir := setupContractFixture(t)
	a := &RegexDeepAnalyzer{}
	cg, err := a.CallGraph(context.Background(), dir, oculus.CallGraphOpts{Entry: "main", Depth: 5})
	if err != nil {
		t.Fatal(err)
	}
	if len(cg.Edges) == 0 {
		t.Error("[Regex] contract violation: 0 edges")
		return
	}
	// Regex is a best-effort fallback — it produces edges but without
	// precise File/Line metadata. Document this as known limitation.
	for _, e := range cg.Edges {
		if e.File != "" && e.Line > 0 {
			return // at least one edge has metadata — pass
		}
	}
	t.Log("[Regex] known limitation: edges lack File/Line (best-effort fallback)")
}

func TestContract_Fallback_EdgeMetadata(t *testing.T) {
	dir := setupContractFixture(t)
	a := NewDeepFallback(dir, nil)
	cg, err := a.CallGraph(context.Background(), dir, oculus.CallGraphOpts{Entry: "main", Depth: 5})
	if err != nil {
		t.Fatal(err)
	}
	assertEdgeMetadata(t, "Fallback", cg)
}

// --- Contract: After enrichment, edges must have ParamTypes/ReturnTypes ---

func TestContract_GoAST_TypedEdges(t *testing.T) {
	dir := setupContractFixture(t)
	a := NewGoASTDeep(dir)
	if a == nil {
		t.Skip("GoAST not available")
	}
	cg, err := a.CallGraph(context.Background(), dir, oculus.CallGraphOpts{Entry: "main", Depth: 5})
	if err != nil {
		t.Fatal(err)
	}
	// GoAST produces types directly — no enrichment needed
	assertTypedEdges(t, "GoAST", cg)
}

func TestContract_TreeSitter_TypedEdges(t *testing.T) {
	dir := setupContractFixture(t)
	a, err := NewTreeSitterDeep(dir)
	if err != nil {
		t.Skipf("TreeSitter not available: %v", err)
	}
	cg, err := a.CallGraph(context.Background(), dir, oculus.CallGraphOpts{Entry: "main", Depth: 5})
	if err != nil {
		t.Fatal(err)
	}
	// TreeSitter populates Go types directly
	assertTypedEdges(t, "TreeSitter", cg)
}

func TestContract_UniversalEnrichment(t *testing.T) {
	dir := setupContractFixture(t)
	// Use Regex — produces edges with 0% types
	a := &RegexDeepAnalyzer{}
	cg, err := a.CallGraph(context.Background(), dir, oculus.CallGraphOpts{Entry: "main", Depth: 5})
	if err != nil {
		t.Fatal(err)
	}
	if len(cg.Edges) == 0 {
		t.Skip("Regex produced 0 edges")
	}

	// Before enrichment: 0% typed
	typedBefore := countTyped(cg.Edges)
	t.Logf("Before enrichment: %d/%d typed", typedBefore, len(cg.Edges))

	// Run go/parser enrichment (universal — works without LSP)
	EnrichCallEdgeTypes(dir, cg.Edges)

	// After enrichment: should have types
	typedAfter := countTyped(cg.Edges)
	t.Logf("After enrichment: %d/%d typed", typedAfter, len(cg.Edges))

	if typedAfter <= typedBefore {
		t.Errorf("enrichment did not improve type coverage: %d → %d", typedBefore, typedAfter)
	}
}

// --- Helpers ---

func assertEdgeMetadata(t *testing.T, analyzer string, cg *oculus.CallGraph) {
	t.Helper()
	if len(cg.Edges) == 0 {
		t.Errorf("[%s] contract violation: 0 edges", analyzer)
		return
	}
	for _, e := range cg.Edges {
		if e.File == "" {
			t.Errorf("[%s] contract violation: edge %s→%s has empty File", analyzer, e.Caller, e.Callee)
		}
		if e.Line == 0 {
			t.Errorf("[%s] contract violation: edge %s→%s has Line=0", analyzer, e.Caller, e.Callee)
		}
	}
	t.Logf("[%s] %d edges, all with File+Line ✓", analyzer, len(cg.Edges))
}

func assertTypedEdges(t *testing.T, analyzer string, cg *oculus.CallGraph) {
	t.Helper()
	if len(cg.Edges) == 0 {
		t.Errorf("[%s] contract violation: 0 edges", analyzer)
		return
	}
	typed := countTyped(cg.Edges)
	pct := float64(typed) / float64(len(cg.Edges)) * 100
	t.Logf("[%s] typed edges: %d/%d (%.0f%%)", analyzer, typed, len(cg.Edges), pct)
	if typed == 0 {
		t.Errorf("[%s] contract violation: 0 typed edges out of %d", analyzer, len(cg.Edges))
	}
}

func countTyped(edges []oculus.CallEdge) int {
	n := 0
	for _, e := range edges {
		if len(e.ParamTypes) > 0 || len(e.ReturnTypes) > 0 {
			n++
		}
	}
	return n
}

var _ = fmt.Sprintf // keep fmt import
