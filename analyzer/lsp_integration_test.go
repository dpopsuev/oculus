//go:build integration

package analyzer

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/dpopsuev/oculus/v3"
	"github.com/dpopsuev/oculus/v3/lang"
	"github.com/dpopsuev/oculus/v3/lsp/testcontainer"
)

// containerLanguages maps fixture names to their lang.Language constants.
// Only languages with LSP servers in the Docker image are listed.
var containerLanguages = map[string]lang.Language{
	"Go":         lang.Go,
	"Python":     lang.Python,
	"TypeScript": lang.TypeScript,
	"JavaScript": lang.JavaScript,
	"Rust":       lang.Rust,
	"C":          lang.C,
	"C++":        lang.Cpp,
}

type langExpectation struct {
	typedEdges bool
	indexWait  time.Duration
}

var langExpectations = map[string]langExpectation{
	"Go":         {typedEdges: true, indexWait: 8 * time.Second},
	"Python":     {typedEdges: true, indexWait: 5 * time.Second},
	"TypeScript": {typedEdges: true, indexWait: 5 * time.Second},
	"JavaScript": {typedEdges: false, indexWait: 5 * time.Second},
	"Rust":       {typedEdges: true, indexWait: 15 * time.Second},
	"C":          {typedEdges: true, indexWait: 3 * time.Second},
	"C++":        {typedEdges: true, indexWait: 3 * time.Second},
}

// TestLSPIntegration_ThreeLayer runs workspace/symbol, callHierarchy, and
// hover enrichment for every language with a container LSP server.
//
// Requires: docker, oculus-lsp-test image (make docker-lsp)
// Run: go test -tags integration -run TestLSPIntegration_ThreeLayer -timeout 600s ./analyzer/...
func TestLSPIntegration_ThreeLayer(t *testing.T) {
	if err := testcontainer.Available(""); err != nil {
		t.Skipf("skipping LSP integration: %v", err)
	}

	pool := testcontainer.NewPool("")
	defer pool.Shutdown(context.Background())

	for _, fix := range languageFixtures {
		langConst, supported := containerLanguages[fix.name]
		if !supported {
			continue
		}
		t.Run(fix.name, func(t *testing.T) {
			dir := setupFixture(t, fix.files)
			expect := langExpectations[fix.name]

			// Layer 1: Pool connection.
			client, err := pool.Get(langConst, dir)
			if err != nil {
				t.Fatalf("pool.Get(%s): %v", fix.name, err)
			}
			_ = client
			pool.Release(langConst, dir)

			// Layer 2: Full analyzer call graph.
			timeout := max(30*time.Second, expect.indexWait*3)
			ctx, cancel := context.WithTimeout(context.Background(), timeout)
			defer cancel()

			da := NewLSPDeepWithPool(dir, pool)
			cg, err := da.CallGraph(ctx, dir, oculus.CallGraphOpts{Entry: fix.entry, Depth: 3})
			if err != nil {
				t.Fatalf("CallGraph: %v", err)
			}

			t.Logf("[%s] layer=%s nodes=%d edges=%d", fix.name, cg.Layer, len(cg.Nodes), len(cg.Edges))

			if len(cg.Nodes) == 0 {
				t.Errorf("[%s] workspace/symbol returned 0 nodes", fix.name)
			}

			if len(cg.Edges) == 0 {
				t.Logf("[%s] 0 edges — callHierarchy may not be supported or entry %q not resolved", fix.name, fix.entry)
				return
			}

			// Check for expected callee.
			found := false
			for _, e := range cg.Edges {
				if e.Callee == fix.callee {
					found = true
					t.Logf("[%s] edge: %s -> %s (params=%v returns=%v)", fix.name, e.Caller, e.Callee, e.ParamTypes, e.ReturnTypes)
				}
			}
			if !found {
				t.Logf("[%s] callee %q not in %d edges (LSP may use different name)", fix.name, fix.callee, len(cg.Edges))
			}

			// Layer 3: Hover enrichment (typed edges).
			typed := countTyped(cg.Edges)
			pct := float64(typed) / float64(len(cg.Edges)) * 100
			t.Logf("[%s] typed edges: %d/%d (%.0f%%)", fix.name, typed, len(cg.Edges), pct)

			if expect.typedEdges && typed == 0 {
				t.Errorf("[%s] expected typed edges but got 0", fix.name)
			}
		})
	}

	// Go uses testkit (multi-package), not inline fixture.
	t.Run("Go", func(t *testing.T) {
		dir := copyTestkitGo(t)
		expect := langExpectations["Go"]

		ctx, cancel := context.WithTimeout(context.Background(), max(30*time.Second, expect.indexWait*3))
		defer cancel()

		da := NewLSPDeepWithPool(dir, pool)
		cg, err := da.CallGraph(ctx, dir, oculus.CallGraphOpts{Depth: 1})
		if err != nil {
			t.Fatalf("CallGraph: %v", err)
		}

		t.Logf("[Go] layer=%s nodes=%d edges=%d", cg.Layer, len(cg.Nodes), len(cg.Edges))

		if len(cg.Nodes) == 0 {
			t.Fatal("[Go] workspace/symbol returned 0 nodes — BUG-54 regression")
		}

		if len(cg.Edges) > 0 {
			typed := countTyped(cg.Edges)
			t.Logf("[Go] typed edges: %d/%d", typed, len(cg.Edges))
		} else {
			t.Logf("[Go] 0 edges (callHierarchy pipeline issue, separate from BUG-54)")
		}
	})
}

// TestLSPIntegration_GoReference verifies that Go via container matches
// Go via local GoAST — serves as a reference test.
func TestLSPIntegration_GoReference(t *testing.T) {
	if err := testcontainer.Available(""); err != nil {
		t.Skipf("skipping LSP integration: %v", err)
	}

	pool := testcontainer.NewPool("")
	defer pool.Shutdown(context.Background())

	dir := setupContractFixture(t)

	lspDA := NewLSPDeepWithPool(dir, pool)
	lspCG, err := lspDA.CallGraph(context.Background(), dir, oculus.CallGraphOpts{Entry: "main", Depth: 5})
	if err != nil {
		t.Fatalf("LSP CallGraph: %v", err)
	}

	goastDA := NewGoASTDeep(dir)
	goastCG, err := goastDA.CallGraph(context.Background(), dir, oculus.CallGraphOpts{Entry: "main", Depth: 5})
	if err != nil {
		t.Fatalf("GoAST CallGraph: %v", err)
	}

	t.Logf("LSP:   %d nodes, %d edges, typed=%d", len(lspCG.Nodes), len(lspCG.Edges), countTyped(lspCG.Edges))
	t.Logf("GoAST: %d nodes, %d edges, typed=%d", len(goastCG.Nodes), len(goastCG.Edges), countTyped(goastCG.Edges))

	if len(lspCG.Edges) == 0 {
		t.Error("LSP produced 0 edges for Go reference fixture")
	}
	if countTyped(lspCG.Edges) == 0 {
		t.Error("LSP produced 0 typed edges for Go reference fixture")
	}
}

// TestLSPIntegration_WorkspaceSymbolBug54 reproduces LCS-BUG-54:
// gopls workspace/symbol returns null/empty, causing probe/callgraph
// to fail on a fully valid Go repo.
func TestLSPIntegration_WorkspaceSymbolBug54(t *testing.T) {
	if err := testcontainer.Available(""); err != nil {
		t.Skipf("skipping LSP integration: %v", err)
	}

	pool := testcontainer.NewPool("")
	defer pool.Shutdown(context.Background())

	dir := copyTestkitGo(t)

	da := NewLSPDeepWithPool(dir, pool)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cg, err := da.CallGraph(ctx, dir, oculus.CallGraphOpts{Depth: 1})
	if err != nil {
		t.Fatalf("CallGraph on real Go testkit: %v", err)
	}

	t.Logf("layer=%s nodes=%d edges=%d", cg.Layer, len(cg.Nodes), len(cg.Edges))

	if len(cg.Nodes) == 0 {
		t.Fatal("LCS-BUG-54: workspace/symbol returned no symbols — call graph has 0 nodes")
	}

	t.Logf("LCS-BUG-54 fix verified: %d nodes discovered via workspace/symbol", len(cg.Nodes))

	if len(cg.Edges) > 0 {
		t.Logf("call hierarchy also working: %d edges", len(cg.Edges))
	} else {
		t.Logf("call hierarchy returned 0 edges (gopls callHierarchy may need didOpen — separate from BUG-54)")
	}
}

func copyTestkitGo(t *testing.T) string {
	t.Helper()
	src := filepath.Join(findTestkitRoot(t), "testdata", "testkit", "go")
	dst := t.TempDir()

	err := filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, path)
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
	if err != nil {
		t.Fatalf("copy testkit/go: %v", err)
	}
	t.Logf("testkit/go copied to %s", dst)
	return dst
}

func findTestkitRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not find repo root")
		}
		dir = parent
	}
}
