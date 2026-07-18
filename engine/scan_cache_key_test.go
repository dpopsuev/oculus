package engine

// scan_cache_key_test.go verifies invariants of ScanProject's cache key contract:
//
//   1. The returned CacheKey includes intent + scanner suffix so analysis tools
//      calling getOrScan find the same DB entry ScanProject wrote.
//   2. Monoglot scanner overrides do not share / stomp the auto plain-sha slot.
//   3. The survey scanner is never forced to "lsp" regardless of intent.

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

func reportWithScanner(scanner, from, to string) *arch.ContextReport {
	r := reportWithEdges(from, to)
	r.Scanner = scanner
	return r
}

// TestScanProject_CacheKey_IncludesIntentAndScanner verifies that the CacheKey
// returned by ScanProject(intent=X) encodes sha-intent-sc:auto.
func TestScanProject_CacheKey_IncludesIntentAndScanner(t *testing.T) {
	const sha, intent = "deadbeef", "full"
	storedKey := buildScanCacheKey(sha, intent, false, "")

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
		t.Errorf("CacheKey sha-part = %q, want %q", shaPart, storedKey)
	}
	if !strings.Contains(shaPart, "-sc:auto") {
		t.Errorf("CacheKey missing scanner suffix: %q", shaPart)
	}
}

// TestScanProject_CacheKey_NoIntentStillHasScannerSuffix verifies that even
// without intent, the key is sha-sc:auto (not bare sha). Plain sha remains a
// warm lookup slot written separately for auto scans.
func TestScanProject_CacheKey_NoIntentStillHasScannerSuffix(t *testing.T) {
	const sha = "deadbeef"
	storedKey := buildScanCacheKey(sha, "", false, "")

	ms := &mockStore{
		headSHA:      sha,
		reportsBySHA: map[string]*arch.ContextReport{storedKey: reportWithSymbol("svc", "Hello")},
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
	if got := result.CacheKey[idx+1:]; got != storedKey {
		t.Errorf("CacheKey sha-part = %q, want %q", got, storedKey)
	}
}

// TestScanProject_CacheKey_RoundtripsToAnalysisTools is the end-to-end
// contract: the CacheKey returned by ScanProject must allow any downstream
// analysis tool to locate the same cached report without re-scanning.
func TestScanProject_CacheKey_RoundtripsToAnalysisTools(t *testing.T) {
	const sha, intent = "deadbeef", "full"
	storedKey := buildScanCacheKey(sha, intent, false, "")

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
// value causes the engine to override the survey scanner.
func TestScanProject_SurveyScanner_NotForcedByIntent(t *testing.T) {
	const sha = "deadbeef"
	ms := &mockStore{
		headSHA: sha,
		reportsBySHA: map[string]*arch.ContextReport{
			buildScanCacheKey(sha, "full", false, ""): reportWithEdges("api", "domain"),
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

// TestScanProject_ScannerOverride_SeparateCacheSlot verifies that scanner=typescript
// and scanner=auto (empty) use different cache keys and do not overwrite each other.
func TestScanProject_ScannerOverride_SeparateCacheSlot(t *testing.T) {
	const sha = "deadbeef"
	autoKey := buildScanCacheKey(sha, "full", false, "")
	tsKey := buildScanCacheKey(sha, "full", false, "typescript")

	autoReport := reportWithScanner("composite", "rust_crate", "ts_pkg")
	tsReport := reportWithScanner("typescript", "src", "lib")

	ms := &mockStore{
		headSHA: sha,
		reportsBySHA: map[string]*arch.ContextReport{
			autoKey: autoReport,
			tsKey:   tsReport,
		},
	}
	eng := New(ms, nil)

	autoHit, err := eng.ScanProject(context.Background(), ".", ScanOpts{Intent: "full"})
	if err != nil {
		t.Fatalf("auto ScanProject: %v", err)
	}
	tsHit, err := eng.ScanProject(context.Background(), ".", ScanOpts{Intent: "full", Scanner: "typescript"})
	if err != nil {
		t.Fatalf("typescript ScanProject: %v", err)
	}

	if autoHit.CacheKey == tsHit.CacheKey {
		t.Fatalf("auto and typescript share CacheKey %q — scanner must be part of the key", autoHit.CacheKey)
	}
	if !strings.Contains(autoHit.CacheKey, "-sc:auto") {
		t.Errorf("auto CacheKey missing -sc:auto: %q", autoHit.CacheKey)
	}
	if !strings.Contains(tsHit.CacheKey, "-sc:typescript") {
		t.Errorf("typescript CacheKey missing -sc:typescript: %q", tsHit.CacheKey)
	}
	if autoHit.Report.Scanner != "composite" {
		t.Errorf("auto hit Scanner=%q, want composite", autoHit.Report.Scanner)
	}
	if tsHit.Report.Scanner != "typescript" {
		t.Errorf("typescript hit Scanner=%q, want typescript", tsHit.Report.Scanner)
	}
}

func TestBuildScanCacheKey(t *testing.T) {
	cases := []struct {
		sha, intent, scanner string
		file                 bool
		want                 string
	}{
		{"abc", "full", "", false, "abc-full-sc:auto"},
		{"abc", "full", "auto", false, "abc-full-sc:auto"},
		{"abc", "full", "typescript", false, "abc-full-sc:typescript"},
		{"abc", "", "rust", true, "abc-file-sc:rust"},
	}
	for _, tc := range cases {
		got := buildScanCacheKey(tc.sha, tc.intent, tc.file, tc.scanner)
		if got != tc.want {
			t.Errorf("buildScanCacheKey(%q,%q,%v,%q)=%q want %q",
				tc.sha, tc.intent, tc.file, tc.scanner, got, tc.want)
		}
	}
}
