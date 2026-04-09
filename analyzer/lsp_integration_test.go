//go:build integration

package analyzer

import (
	"context"
	"testing"

	"github.com/dpopsuev/oculus"
	"github.com/dpopsuev/oculus/lang"
	"github.com/dpopsuev/oculus/lsp/testcontainer"
)

// containerLanguages maps fixture names to their lang.Language constants.
// Only languages with LSP servers in the Docker image are listed.
var containerLanguages = map[string]lang.Language{
	"Python":     lang.Python,
	"TypeScript": lang.TypeScript,
	"JavaScript": lang.JavaScript,
	"Rust":       lang.Rust,
	"C":          lang.C,
	"C++":        lang.Cpp,
}

// Languages where we expect typed edges from LSP (proven working).
// Others are measured but not gated.
var typedEdgeExpected = map[string]bool{
	"Python": true,
}

// TestLSPIntegration_CallGraph runs each language fixture through the full
// analyzer chain backed by a Docker container pool. Verifies that LSP servers
// produce call graphs with typed edges.
//
// Requires: docker, oculus-lsp-test image (make docker-lsp)
// Run: go test -tags integration -run TestLSPIntegration -timeout 300s ./analyzer/...
func TestLSPIntegration_CallGraph(t *testing.T) {
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

			// Verify pool can connect for this language
			client, err := pool.Get(langConst, dir)
			if err != nil {
				t.Fatalf("pool.Get(%v): %v", fix.name, err)
			}
			_ = client
			pool.Release(langConst, dir)

			// Run through the full analyzer chain with container pool
			da := NewDeepFallback(dir, pool)
			cg, err := da.CallGraph(context.Background(), dir, oculus.CallGraphOpts{Entry: fix.entry, Depth: 5})
			if err != nil {
				t.Fatalf("CallGraph: %v", err)
			}

			t.Logf("[%s] layer=%s, %d nodes, %d edges", fix.name, cg.Layer, len(cg.Nodes), len(cg.Edges))

			if len(cg.Edges) == 0 {
				t.Logf("[%s] 0 edges (layer=%s) — LSP or fallback didn't produce call graph", fix.name, cg.Layer)
				return
			}

			// Check for expected callee
			found := false
			for _, e := range cg.Edges {
				if e.Callee == fix.callee {
					found = true
					t.Logf("[%s] found edge: %s -> %s (params=%v returns=%v)",
						fix.name, e.Caller, e.Callee, e.ParamTypes, e.ReturnTypes)
				}
			}
			if !found {
				t.Logf("[%s] callee %q not found in %d edges (may use different name)", fix.name, fix.callee, len(cg.Edges))
			}

			// Check typed edges
			typed := countTyped(cg.Edges)
			pct := float64(typed) / float64(len(cg.Edges)) * 100
			t.Logf("[%s] typed edges: %d/%d (%.0f%%)", fix.name, typed, len(cg.Edges), pct)

			// Only gate languages where we've proven typed edges work
			if typedEdgeExpected[fix.name] && typed == 0 {
				t.Errorf("[%s] expected typed edges but got 0", fix.name)
			}
		})
	}
}

// TestLSPIntegration_HoverEnrichment tests the hover → signature → types
// pipeline for each language independently of the call graph.
func TestLSPIntegration_HoverEnrichment(t *testing.T) {
	if err := testcontainer.Available(""); err != nil {
		t.Skipf("skipping LSP integration: %v", err)
	}

	pool := testcontainer.NewPool("")
	defer pool.Shutdown(context.Background())

	for _, fix := range languageFixtures {
		_, supported := containerLanguages[fix.name]
		if !supported {
			continue
		}
		t.Run(fix.name, func(t *testing.T) {
			dir := setupFixture(t, fix.files)

			// Use LSP analyzer directly (not fallback chain)
			a := NewLSPDeepWithPool(dir, pool)

			// Get call graph — this exercises callHierarchy + hover enrichment
			cg, err := a.CallGraph(context.Background(), dir, oculus.CallGraphOpts{Entry: fix.entry, Depth: 5})
			if err != nil {
				t.Fatalf("LSP CallGraph: %v", err)
			}

			t.Logf("[%s] LSP layer=%s, %d edges", fix.name, cg.Layer, len(cg.Edges))

			if len(cg.Edges) == 0 {
				t.Logf("[%s] LSP produced 0 edges — callHierarchy may not be supported", fix.name)
				return
			}

			typed := countTyped(cg.Edges)
			pct := float64(typed) / float64(len(cg.Edges)) * 100
			t.Logf("[%s] hover enrichment: %d/%d typed (%.0f%%)", fix.name, typed, len(cg.Edges), pct)

			if typedEdgeExpected[fix.name] && typed == 0 {
				t.Errorf("[%s] expected typed edges from hover but got 0", fix.name)
			}
		})
	}
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

	// Container LSP
	lspDA := NewLSPDeepWithPool(dir, pool)
	lspCG, err := lspDA.CallGraph(context.Background(), dir, oculus.CallGraphOpts{Entry: "main", Depth: 5})
	if err != nil {
		t.Fatalf("LSP CallGraph: %v", err)
	}

	// Local GoAST
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
