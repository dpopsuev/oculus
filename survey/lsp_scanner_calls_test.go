package survey_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/dpopsuev/oculus/v3/lsp/mockserver"
	"github.com/dpopsuev/oculus/v3/survey"
)

// TestLSPScanner_CallHierarchyEdges verifies that LSPScanner extracts call
// edges from callHierarchy/outgoingCalls and populates DependencyGraph.
//
// Given a workspace with main() in the root calling Hello() in lib/
// And an LSP server that reports the call edge
// When LSPScanner.Scan is called
// Then DependencyGraph contains an edge from "(root)" to "lib"
func TestLSPScanner_CallHierarchyEdges(t *testing.T) {
	dir := makeFixture(t, map[string]string{
		"main.go":    "package main\n\nfunc main() { Hello() }\n",
		"lib/lib.go": "package lib\n\nfunc Hello() string { return \"hi\" }\n",
	})

	mainURI := "file://" + filepath.ToSlash(filepath.Join(dir, "main.go"))
	libURI := "file://" + filepath.ToSlash(filepath.Join(dir, "lib/lib.go"))

	cfg := mockserver.Config{
		Symbols: []mockserver.Symbol{
			{Name: "main", Kind: 12, URI: mainURI, Line: 2, Col: 5},
			{Name: "Hello", Kind: 12, URI: libURI, Line: 2, Col: 5},
		},
		Edges: []mockserver.CallEdge{
			{FromName: "main", ToName: "Hello", ToURI: libURI, ToLine: 2, ToCol: 5},
		},
	}

	cfgData, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	cfgFile := filepath.Join(dir, ".mock-config.json")
	if err := os.WriteFile(cfgFile, cfgData, 0o644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("MOCK_LSP_CONFIG", cfgFile)

	sc := &survey.LSPScanner{
		ServerCmd: os.Args[0],
		Timeout:   5 * time.Second,
	}

	proj, err := sc.Scan(dir)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}

	if proj.DependencyGraph == nil {
		t.Fatal("no dependency graph")
	}

	foundEdge := false
	for _, e := range proj.DependencyGraph.Edges {
		if e.From == "(root)" && e.To == "lib" {
			foundEdge = true
			break
		}
	}
	if !foundEdge {
		t.Errorf("expected edge from (root) to lib, got edges: %v", proj.DependencyGraph.Edges)
	}
}

// TestLSPScanner_CallHierarchyNoEdgesForNonCallable verifies that non-callable
// symbols (variables, constants, types) are not sent to prepareCallHierarchy.
//
// Given a workspace with only a variable declaration
// And an LSP server with no call edges configured
// When LSPScanner.Scan is called
// Then DependencyGraph has no edges
func TestLSPScanner_CallHierarchyNoEdgesForNonCallable(t *testing.T) {
	dir := makeFixture(t, map[string]string{
		"main.go": "package main\n\nvar Version = \"1.0\"\n",
	})

	mainURI := "file://" + filepath.ToSlash(filepath.Join(dir, "main.go"))

	cfg := mockserver.Config{
		Symbols: []mockserver.Symbol{
			{Name: "Version", Kind: 13, URI: mainURI, Line: 2, Col: 4},
		},
	}

	cfgData, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	cfgFile := filepath.Join(dir, ".mock-config.json")
	if err := os.WriteFile(cfgFile, cfgData, 0o644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("MOCK_LSP_CONFIG", cfgFile)

	sc := &survey.LSPScanner{
		ServerCmd: os.Args[0],
		Timeout:   5 * time.Second,
	}

	proj, err := sc.Scan(dir)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}

	if proj.DependencyGraph != nil && len(proj.DependencyGraph.Edges) > 0 {
		t.Errorf("expected no edges for non-callable symbols, got: %v", proj.DependencyGraph.Edges)
	}
}

// TestLSPScanner_CallBudgetLimitsRoundtrips verifies that CallBudget caps
// the number of callHierarchy roundtrips.
//
// Given 5 callable symbols and a CallBudget of 2
// When LSPScanner.Scan is called
// Then at most 2 symbols produce call edges (budget limits roundtrips)
func TestLSPScanner_CallBudgetLimitsRoundtrips(t *testing.T) {
	dir := makeFixture(t, map[string]string{
		"main.go": "package main\n\nfunc A(){}\nfunc B(){}\nfunc C(){}\nfunc D(){}\nfunc E(){}\n",
	})

	mainURI := "file://" + filepath.ToSlash(filepath.Join(dir, "main.go"))

	cfg := mockserver.Config{
		Symbols: []mockserver.Symbol{
			{Name: "A", Kind: 12, URI: mainURI, Line: 2, Col: 5},
			{Name: "B", Kind: 12, URI: mainURI, Line: 3, Col: 5},
			{Name: "C", Kind: 12, URI: mainURI, Line: 4, Col: 5},
			{Name: "D", Kind: 12, URI: mainURI, Line: 5, Col: 5},
			{Name: "E", Kind: 12, URI: mainURI, Line: 6, Col: 5},
		},
		Edges: []mockserver.CallEdge{
			{FromName: "A", ToName: "B", ToURI: mainURI, ToLine: 3, ToCol: 5},
			{FromName: "B", ToName: "C", ToURI: mainURI, ToLine: 4, ToCol: 5},
			{FromName: "C", ToName: "D", ToURI: mainURI, ToLine: 5, ToCol: 5},
			{FromName: "D", ToName: "E", ToURI: mainURI, ToLine: 6, ToCol: 5},
			{FromName: "E", ToName: "A", ToURI: mainURI, ToLine: 2, ToCol: 5},
		},
	}

	cfgData, _ := json.Marshal(cfg)
	cfgFile := filepath.Join(dir, ".mock-config.json")
	os.WriteFile(cfgFile, cfgData, 0o644)
	t.Setenv("MOCK_LSP_CONFIG", cfgFile)

	sc := &survey.LSPScanner{
		ServerCmd:  os.Args[0],
		Timeout:    5 * time.Second,
		CallBudget: 2,
	}

	proj, err := sc.Scan(dir)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}

	edgeCount := 0
	if proj.DependencyGraph != nil {
		edgeCount = len(proj.DependencyGraph.Edges)
	}
	if edgeCount > 2 {
		t.Errorf("CallBudget=2 but got %d edges; budget should cap roundtrips", edgeCount)
	}
}

// TestLSPScanner_EntryPointsAlwaysCrawled verifies that main/init are always
// included in callHierarchy crawling even when they are unexported.
func TestLSPScanner_EntryPointsAlwaysCrawled(t *testing.T) {
	dir := makeFixture(t, map[string]string{
		"main.go":    "package main\n\nfunc main() { Hello() }\n",
		"lib/lib.go": "package lib\n\nfunc Hello() string { return \"hi\" }\n",
	})

	mainURI := "file://" + filepath.ToSlash(filepath.Join(dir, "main.go"))
	libURI := "file://" + filepath.ToSlash(filepath.Join(dir, "lib/lib.go"))

	cfg := mockserver.Config{
		Symbols: []mockserver.Symbol{
			{Name: "main", Kind: 12, URI: mainURI, Line: 2, Col: 5},
			{Name: "Hello", Kind: 12, URI: libURI, Line: 2, Col: 5},
		},
		Edges: []mockserver.CallEdge{
			{FromName: "main", ToName: "Hello", ToURI: libURI, ToLine: 2, ToCol: 5},
		},
	}

	cfgData, _ := json.Marshal(cfg)
	cfgFile := filepath.Join(dir, ".mock-config.json")
	os.WriteFile(cfgFile, cfgData, 0o644)
	t.Setenv("MOCK_LSP_CONFIG", cfgFile)

	// CallBudget=1: only one symbol gets crawled. main is an entry point
	// so it must be prioritized over Hello (exported but not entry).
	sc := &survey.LSPScanner{
		ServerCmd:  os.Args[0],
		Timeout:    5 * time.Second,
		CallBudget: 1,
	}

	proj, err := sc.Scan(dir)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}

	foundEdge := false
	if proj.DependencyGraph != nil {
		for _, e := range proj.DependencyGraph.Edges {
			if e.From == "(root)" && e.To == "lib" {
				foundEdge = true
			}
		}
	}
	if !foundEdge {
		t.Error("main() is an entry point — should be crawled even with CallBudget=1")
	}
}

// TestLSPScanner_SelectiveCrawlReportsCounts verifies that CrawlStats
// reports how many symbols were crawled vs skipped.
func TestLSPScanner_SelectiveCrawlReportsCounts(t *testing.T) {
	dir := makeFixture(t, map[string]string{
		"main.go": "package main\n\nfunc A(){}\nfunc B(){}\nfunc C(){}\n",
	})

	mainURI := "file://" + filepath.ToSlash(filepath.Join(dir, "main.go"))

	cfg := mockserver.Config{
		Symbols: []mockserver.Symbol{
			{Name: "A", Kind: 12, URI: mainURI, Line: 2, Col: 5},
			{Name: "B", Kind: 12, URI: mainURI, Line: 3, Col: 5},
			{Name: "C", Kind: 12, URI: mainURI, Line: 4, Col: 5},
		},
	}

	cfgData, _ := json.Marshal(cfg)
	cfgFile := filepath.Join(dir, ".mock-config.json")
	os.WriteFile(cfgFile, cfgData, 0o644)
	t.Setenv("MOCK_LSP_CONFIG", cfgFile)

	sc := &survey.LSPScanner{
		ServerCmd:  os.Args[0],
		Timeout:    5 * time.Second,
		CallBudget: 1,
	}

	proj, err := sc.Scan(dir)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}

	if proj.CrawlStats == nil {
		t.Fatal("expected CrawlStats to be populated")
	}
	if proj.CrawlStats.Crawled > 1 {
		t.Errorf("CrawlStats.Crawled = %d, want <= 1 (budget=1)", proj.CrawlStats.Crawled)
	}
	if proj.CrawlStats.Total != 3 {
		t.Errorf("CrawlStats.Total = %d, want 3", proj.CrawlStats.Total)
	}
	if proj.CrawlStats.Skipped < 2 {
		t.Errorf("CrawlStats.Skipped = %d, want >= 2", proj.CrawlStats.Skipped)
	}
}

// TestLSPScanner_HoverSignatures verifies that LSPScanner extracts signatures
// from textDocument/hover and populates Symbol.Signature.
//
// Given a workspace with exported functions
// And an LSP server that returns signature text via hover
// When LSPScanner.Scan is called
// Then each callable symbol has its Signature field populated
func TestLSPScanner_HoverSignatures(t *testing.T) {
	dir := makeFixture(t, map[string]string{
		"main.go": "package main\n\nfunc Hello(name string) string { return name }\n",
	})

	mainURI := "file://" + filepath.ToSlash(filepath.Join(dir, "main.go"))

	cfg := mockserver.Config{
		Symbols: []mockserver.Symbol{
			{Name: "Hello", Kind: 12, URI: mainURI, Line: 2, Col: 5},
		},
		Signatures: map[string]string{
			"Hello": "func Hello(name string) string",
		},
	}

	cfgData, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	cfgFile := filepath.Join(dir, ".mock-config.json")
	if err := os.WriteFile(cfgFile, cfgData, 0o644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("MOCK_LSP_CONFIG", cfgFile)

	sc := &survey.LSPScanner{
		ServerCmd: os.Args[0],
		Timeout:   5 * time.Second,
	}

	proj, err := sc.Scan(dir)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}

	for _, ns := range proj.Namespaces {
		for _, s := range ns.Symbols {
			if s.Name == "Hello" {
				if s.Signature != "func Hello(name string) string" {
					t.Errorf("Hello.Signature = %q, want %q", s.Signature, "func Hello(name string) string")
				}
				return
			}
		}
	}
	t.Error("missing symbol Hello")
}
