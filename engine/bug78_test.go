package engine

// Tests for LCS-BUG-78: symbol data pipeline broken between scan and analysis.
//
// Two root causes:
//
//  1. CacheKey mismatch — ScanProject(intent=X) stores the report under
//     "sha-X" in the DB but returns CacheKey = "path@sha" (plain sha, no
//     intent suffix). Analysis tools calling getOrScan with "path@sha" look
//     for "sha" in the DB, get a cache miss, and return ErrNoCachedReport.
//
//  2. Wrong scanner for intent=full (v0.72 regression) — setting
//     scanner=lsp for intent=full forced LSPScanner for the survey path.
//     LSPScanner extracts symbols from documentSymbol but produces 0 import
//     edges (no import statement parsing). TypeScriptScanner is correct for
//     the structural scan; the LSP pool handles deep analysis independently.

import (
	"context"
	"strings"
	"testing"

	"github.com/dpopsuev/oculus/v3/arch"
	"github.com/dpopsuev/oculus/v3/model"
)

// cannedReport is a minimal ContextReport with one service and one symbol,
// used to verify that the right report is retrieved after scanning.
func cannedReport(componentName, symbolName string) *arch.ContextReport {
	svc := arch.ArchService{Name: componentName}
	svc.Symbols = []model.Symbol{{Name: symbolName, Kind: model.SymbolFunction, Exported: true}}
	r := &arch.ContextReport{}
	r.Architecture = arch.ArchModel{
		Services: []arch.ArchService{svc},
		Edges:    []arch.ArchEdge{{From: componentName, To: "dep"}},
	}
	return r
}

// --- Root cause 1: CacheKey mismatch ---

// TestScanProject_CacheKey_IncludesIntent is the primary regression test for
// LCS-BUG-78. ScanProject must return a CacheKey whose SHA portion matches
// the key under which the report was stored in the DB (sha + "-" + intent).
// Without this, every analysis call with the returned cache_key results in a
// cache miss and ErrNoCachedReport.
func TestScanProject_CacheKey_IncludesIntent(t *testing.T) {
	const sha = "deadbeef"
	const intent = "full"
	storedKey := sha + "-" + intent // "deadbeef-full"

	// Pre-populate the store only under the intent-keyed entry.
	// If ScanProject returns CacheKey = path@sha (plain), getOrScan will miss.
	// If it returns CacheKey = path@sha-full, getOrScan hits.
	ms := &mockStore{
		headSHA:      sha,
		reportsBySHA: map[string]*arch.ContextReport{storedKey: cannedReport("svc", "Hello")},
	}
	eng := New(ms, nil)

	result, err := eng.ScanProject(context.Background(), ".", ScanOpts{Intent: intent})
	if err != nil {
		t.Fatalf("ScanProject: %v", err)
	}

	// Extract the sha portion from "path@<sha-portion>".
	idx := strings.LastIndex(result.CacheKey, "@")
	if idx < 0 {
		t.Fatalf("CacheKey has no '@': %q", result.CacheKey)
	}
	shaPart := result.CacheKey[idx+1:]

	if shaPart != storedKey {
		t.Errorf("CacheKey sha-part = %q, want %q\n"+
			"(LCS-BUG-78: analysis tools receive path@%s and look for %q in the DB,\n"+
			"but the report was stored under %q — cache miss → ErrNoCachedReport)",
			shaPart, storedKey, sha, sha, storedKey)
	}
}

// TestScanProject_CacheKey_NoIntent verifies that when no intent is set the
// CacheKey still uses the plain sha (no suffix), preserving backward compat.
func TestScanProject_CacheKey_NoIntent(t *testing.T) {
	const sha = "deadbeef"
	ms := &mockStore{
		headSHA:      sha,
		reportsBySHA: map[string]*arch.ContextReport{sha: cannedReport("svc", "Hello")},
	}
	eng := New(ms, nil)

	result, err := eng.ScanProject(context.Background(), ".", ScanOpts{})
	if err != nil {
		t.Fatalf("ScanProject: %v", err)
	}

	idx := strings.LastIndex(result.CacheKey, "@")
	if idx < 0 {
		t.Fatalf("CacheKey has no '@': %q", result.CacheKey)
	}
	shaPart := result.CacheKey[idx+1:]

	if shaPart != sha {
		t.Errorf("CacheKey sha-part = %q, want %q (no-intent path should stay plain sha)", shaPart, sha)
	}
}

// TestAnalysis_UsesCacheKeyFromScan is the end-to-end regression: after
// ScanProject with intent=full, analysis tools called with the returned
// cache_key must find the report without re-scanning.
//
// The test uses SearchSymbols as a representative analysis tool; all others
// (GetRiskScores, GetCycles, GetCouplingTable, etc.) share the same getOrScan
// path and would exhibit identical behaviour.
func TestAnalysis_UsesCacheKeyFromScan(t *testing.T) {
	const sha = "deadbeef"
	const intent = "full"
	storedKey := sha + "-" + intent

	ms := &mockStore{
		headSHA:      sha,
		reportsBySHA: map[string]*arch.ContextReport{storedKey: cannedReport("svc", "Hello")},
	}
	eng := New(ms, nil)

	// Step 1: scan.
	result, err := eng.ScanProject(context.Background(), ".", ScanOpts{Intent: intent})
	if err != nil {
		t.Fatalf("ScanProject: %v", err)
	}

	// Step 2: analysis using the returned cache_key.
	report, err := eng.SearchSymbols(context.Background(), ".", "Hello", result.CacheKey)
	if err != nil {
		t.Fatalf("SearchSymbols with cache_key=%q: %v\n"+
			"(LCS-BUG-78: cache_key does not match stored key — getOrScan returns ErrNoCachedReport)", result.CacheKey, err)
	}
	if len(report.Matches) == 0 {
		t.Errorf("SearchSymbols returned 0 matches; want symbol 'Hello'\n"+
			"cache_key used: %q", result.CacheKey)
	}
}

// --- Root cause 2: scanner regression from v0.72 ---

// TestScanProject_IntentFull_DoesNotForceScanner verifies that ScanProject
// with intent=full and no explicit scanner override leaves ScannerOverride
// empty so that AutoScanner selects the right language scanner (e.g.
// TypeScriptScanner for TS projects with 195 components and 110 edges).
//
// The v0.72.0 locus fix incorrectly set scanner=lsp for intent=full at the
// MCP layer. The LSPScanner does extract symbols via documentSymbol but
// produces 0 import edges (no import statement parsing), causing coupling,
// cycles, and risk_scores to return empty results.
//
// The LSP pool — used by GetSymbolGraph, GetMesh, risk_scores deep path —
// operates independently of the survey scanner and does not need a scanner
// override to function correctly.
func TestScanProject_IntentFull_DoesNotForceScanner(t *testing.T) {
	const sha = "deadbeef"
	storedKey := sha + "-full"

	// Report with edges — if scanner=lsp were forced for the survey,
	// LSPScanner would produce 0 edges.
	reportWithEdges := &arch.ContextReport{}
	reportWithEdges.Architecture = arch.ArchModel{
		Services: []arch.ArchService{{Name: "a"}, {Name: "b"}},
		Edges:    []arch.ArchEdge{{From: "a", To: "b"}},
	}
	ms := &mockStore{
		headSHA:      sha,
		reportsBySHA: map[string]*arch.ContextReport{storedKey: reportWithEdges},
	}
	eng := New(ms, nil)

	result, err := eng.ScanProject(context.Background(), ".", ScanOpts{Intent: "full"})
	if err != nil {
		t.Fatalf("ScanProject: %v", err)
	}

	// The engine does not expose the scanner override used internally, but we
	// can verify that the cached report with edges is retrieved — confirming
	// the survey scanner did not silently switch to LSPScanner (which would
	// produce a fresh scan with 0 edges instead of hitting the cache).
	if len(result.Report.Architecture.Edges) == 0 {
		t.Errorf("0 edges in scan result for intent=full; want edges from cached TypeScript scan\n"+
			"(if scanner was forced to lsp, LSPScanner may have re-scanned with 0 import edges)")
	}
}
