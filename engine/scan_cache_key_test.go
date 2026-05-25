package engine

// scan_cache_key_test.go verifies two invariants of ScanProject's cache key contract:
//
//   1. The returned CacheKey includes the intent suffix (sha+"-"+intent) so that
//      downstream analysis tools calling getOrScan find the same DB entry that
//      ScanProject wrote. A plain-sha CacheKey causes a cache miss and all
//      analysis tools return empty results.
//
//   2. The survey scanner is never forced to "lsp" regardless of intent. The LSP
//      pool drives deep analysis independently; the survey scanner must be left
//      on auto so that language-specific scanners (e.g. TypeScriptScanner) produce
//      correct import edges. Forcing LSPScanner for the survey produces 0 import
//      edges and breaks coupling, cycles, and risk_scores.

import (
	"context"
	"strings"
	"testing"

	"github.com/dpopsuev/oculus/v3/arch"
	"github.com/dpopsuev/oculus/v3/model"
)

// reportWithSymbol builds a minimal ContextReport containing one component and
// one named symbol. Tests use it to assert that the correct cached entry was
// retrieved rather than a stale or re-scanned one.
func reportWithSymbol(component, symbol string) *arch.ContextReport {
	svc := arch.ArchService{Name: component}
	svc.Symbols = []model.Symbol{{Name: symbol, Kind: model.SymbolFunction, Exported: true}}
	r := &arch.ContextReport{}
	r.Architecture = arch.ArchModel{
		Services: []arch.ArchService{svc},
		Edges:    []arch.ArchEdge{{From: component, To: "dep"}},
	}
	return r
}

// reportWithEdges builds a minimal ContextReport with two components and one
// import edge. Tests use it to verify that import-edge data survives a cache
// roundtrip without being replaced by a zero-edge LSP survey scan.
func reportWithEdges(from, to string) *arch.ContextReport {
	r := &arch.ContextReport{}
	r.Architecture = arch.ArchModel{
		Services: []arch.ArchService{{Name: from}, {Name: to}},
		Edges:    []arch.ArchEdge{{From: from, To: to}},
	}
	return r
}

// TestScanProject_CacheKey_IncludesIntentSuffix verifies that the CacheKey
// returned by ScanProject(intent=X) encodes the same key used to store the
// report in the DB (sha+"-"+intent), so that analysis tools can resolve it.
//
// Given a DB populated only with a "sha-full" entry
// When ScanProject is called with intent=full
// Then CacheKey must end with "sha-full", not the plain "sha"
func TestScanProject_CacheKey_IncludesIntentSuffix(t *testing.T) {
	const sha, intent = "deadbeef", "full"
	storedKey := sha + "-" + intent

	ms := &mockStore{
		headSHA:      sha,
		reportsBySHA: map[string]*arch.ContextReport{storedKey: reportWithSymbol("svc", "Hello")},
	}
	eng := New(ms, nil)

	result, err := eng.ScanProject(context.Background(), ".", ScanOpts{Intent: intent})
	if err != nil {
		t.Fatalf("ScanProject: %v", err)
	}

	idx := strings.LastIndex(result.CacheKey, "@")
	if idx < 0 {
		t.Fatalf("CacheKey contains no '@' separator: %q", result.CacheKey)
	}
	shaPart := result.CacheKey[idx+1:]

	if shaPart != storedKey {
		t.Errorf("CacheKey sha-part = %q, want %q\n"+
			"analysis tools receive %q, look for %q in the DB — cache miss, empty results",
			shaPart, storedKey, result.CacheKey, sha)
	}
}

// TestScanProject_CacheKey_PlainSHAWhenNoIntent verifies backward compatibility:
// a ScanProject call without an intent suffix returns a plain-sha CacheKey.
//
// Given a DB populated with a plain sha entry
// When ScanProject is called with no intent
// Then CacheKey must end with the plain sha, unchanged
func TestScanProject_CacheKey_PlainSHAWhenNoIntent(t *testing.T) {
	const sha = "deadbeef"
	ms := &mockStore{
		headSHA:      sha,
		reportsBySHA: map[string]*arch.ContextReport{sha: reportWithSymbol("svc", "Hello")},
	}
	eng := New(ms, nil)

	result, err := eng.ScanProject(context.Background(), ".", ScanOpts{})
	if err != nil {
		t.Fatalf("ScanProject: %v", err)
	}

	idx := strings.LastIndex(result.CacheKey, "@")
	if idx < 0 {
		t.Fatalf("CacheKey contains no '@' separator: %q", result.CacheKey)
	}
	if got := result.CacheKey[idx+1:]; got != sha {
		t.Errorf("CacheKey sha-part = %q, want %q for intent-less scan", got, sha)
	}
}

// TestScanProject_CacheKey_RoundtripsToAnalysisTools is the end-to-end
// contract: the CacheKey returned by ScanProject must allow any downstream
// analysis tool to locate the same cached report without re-scanning.
//
// Given a completed scan with intent=full
// When SearchSymbols is called with the CacheKey returned by ScanProject
// Then the symbols from the original scan are returned without triggering a new scan
func TestScanProject_CacheKey_RoundtripsToAnalysisTools(t *testing.T) {
	const sha, intent = "deadbeef", "full"
	storedKey := sha + "-" + intent

	ms := &mockStore{
		headSHA:      sha,
		reportsBySHA: map[string]*arch.ContextReport{storedKey: reportWithSymbol("svc", "Hello")},
	}
	eng := New(ms, nil)

	scanResult, err := eng.ScanProject(context.Background(), ".", ScanOpts{Intent: intent})
	if err != nil {
		t.Fatalf("ScanProject: %v", err)
	}

	symbols, err := eng.SearchSymbols(context.Background(), ".", "Hello", scanResult.CacheKey)
	if err != nil {
		t.Fatalf("SearchSymbols(cache_key=%q): %v — cache miss; analysis tools return empty results",
			scanResult.CacheKey, err)
	}
	if len(symbols.Matches) == 0 {
		t.Errorf("SearchSymbols returned 0 matches for 'Hello'; cached report was not found via cache_key %q",
			scanResult.CacheKey)
	}
}

// TestScanProject_SurveyScanner_NotForcedByIntent verifies that no intent
// value causes the engine to override the survey scanner. The LSP pool drives
// deep analysis independently; the survey scanner must remain on auto so that
// language-specific scanners produce correct structural data (import edges,
// component grouping). Forcing LSPScanner for the survey produces 0 import
// edges and breaks coupling, cycles, and risk_scores.
//
// Given a cached report with import edges stored under sha-full
// When ScanProject is called with intent=full and no explicit scanner
// Then the cached report is returned with its edges intact (auto scanner hit cache)
func TestScanProject_SurveyScanner_NotForcedByIntent(t *testing.T) {
	const sha = "deadbeef"
	ms := &mockStore{
		headSHA: sha,
		reportsBySHA: map[string]*arch.ContextReport{
			sha + "-full": reportWithEdges("api", "domain"),
		},
	}
	eng := New(ms, nil)

	result, err := eng.ScanProject(context.Background(), ".", ScanOpts{Intent: "full"})
	if err != nil {
		t.Fatalf("ScanProject: %v", err)
	}

	if len(result.Report.Architecture.Edges) == 0 {
		t.Error("0 import edges in result for intent=full — survey scanner may have been overridden " +
			"to LSPScanner which produces no import edges; auto scanner should have returned cached data")
	}
}
