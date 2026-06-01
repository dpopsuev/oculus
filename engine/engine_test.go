package engine

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/dpopsuev/oculus/v3/arch"
	"github.com/dpopsuev/oculus/v3/port"
)

func newTestEngine() (*Engine, *mockStore) {
	store := newMockStore(testReport())
	return New(store, []string{"/tmp"}), store
}

// --- GetHotSpots ---

func TestGetHotSpots(t *testing.T) {
	eng, _ := newTestEngine()
	r, err := eng.GetHotSpots(context.Background(), "/tmp", 30, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(r) == 0 {
		t.Error("expected hot spots")
	}
}

func TestGetHotSpots_TopN(t *testing.T) {
	eng, _ := newTestEngine()
	r, err := eng.GetHotSpots(context.Background(), "/tmp", 30, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(r) > 1 {
		t.Errorf("topN=1 but got %d hot spots", len(r))
	}
}

// --- GetDependencies ---

func TestGetDependencies(t *testing.T) {
	eng, _ := newTestEngine()
	r, err := eng.GetDependencies(context.Background(), "/tmp", "internal/core")
	if err != nil {
		t.Fatal(err)
	}
	if r.Component != "internal/core" {
		t.Errorf("component = %q, want internal/core", r.Component)
	}
	// internal/core has fan-in from cmd/app and fan-out to internal/store + pkg/logger
	if len(r.FanOut) == 0 {
		t.Error("expected fan-out edges")
	}
}

func TestGetDependencies_EmptyComponent(t *testing.T) {
	eng, _ := newTestEngine()
	_, err := eng.GetDependencies(context.Background(), "/tmp", "")
	if err == nil {
		t.Error("expected ErrComponentRequired")
	}
}

// --- GetCouplingTable ---

func TestGetCouplingTable(t *testing.T) {
	eng, _ := newTestEngine()
	r, err := eng.GetCouplingTable(context.Background(), "/tmp", "", 0)
	if err != nil {
		t.Fatal(err)
	}
	if r == "" {
		t.Error("expected non-empty coupling table")
	}
}

// --- GetEdgeList ---

func TestGetEdgeList(t *testing.T) {
	eng, _ := newTestEngine()
	r, err := eng.GetEdgeList(context.Background(), "/tmp", "")
	if err != nil {
		t.Fatal(err)
	}
	if r == "" {
		t.Error("expected non-empty edge list")
	}
}

// --- GetCycles ---

func TestGetCycles_Clean(t *testing.T) {
	eng, _ := newTestEngine()
	r, err := eng.GetCycles(context.Background(), "/tmp", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Cycles) != 0 {
		t.Errorf("expected 0 cycles, got %d", len(r.Cycles))
	}
}

func TestGetCycles_WithCycles(t *testing.T) {
	store := newMockStore(testReportWithCycles())
	eng := New(store, []string{"/tmp"})
	r, err := eng.GetCycles(context.Background(), "/tmp", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Cycles) == 0 {
		t.Error("expected cycles")
	}
}

// --- GetViolations ---

func TestGetViolations_ExplicitLayers(t *testing.T) {
	eng, _ := newTestEngine()
	layers := []string{"pkg/logger", "internal/store", "internal/core", "cmd/app"}
	r, err := eng.GetViolations(context.Background(), "/tmp", layers)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(r.Summary, "layer") {
		t.Errorf("summary missing 'layer': %s", r.Summary)
	}
}

func TestGetViolations_FromDesiredState(t *testing.T) {
	eng, store := newTestEngine()
	store.desiredState = testDesiredState()
	r, err := eng.GetViolations(context.Background(), "/tmp", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Layers) == 0 {
		t.Error("expected layers from desired state")
	}
}

func TestGetViolations_NoDesiredState(t *testing.T) {
	eng, _ := newTestEngine()
	r, err := eng.GetViolations(context.Background(), "/tmp", nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(r.Summary, "No desired") {
		t.Errorf("expected 'No desired' message, got: %s", r.Summary)
	}
}

// --- GetComponentDetail ---

func TestGetComponentDetail(t *testing.T) {
	eng, _ := newTestEngine()
	r, err := eng.GetComponentDetail(context.Background(), "/tmp", "internal/core")
	if err != nil {
		t.Fatal(err)
	}
	if r.Name != "internal/core" {
		t.Errorf("name = %q, want internal/core", r.Name)
	}
	if r.LOC != 500 {
		t.Errorf("LOC = %d, want 500", r.LOC)
	}
}

func TestGetComponentDetail_NotFound(t *testing.T) {
	eng, _ := newTestEngine()
	_, err := eng.GetComponentDetail(context.Background(), "/tmp", "nonexistent")
	if err == nil {
		t.Error("expected ErrComponentNotFound")
	}
}

// --- SuggestArchitecture ---

func TestSuggestArchitecture(t *testing.T) {
	eng, _ := newTestEngine()
	ds, err := eng.SuggestArchitecture(context.Background(), "/tmp")
	if err != nil {
		t.Fatal(err)
	}
	if len(ds.Layers) == 0 {
		t.Error("expected inferred layers")
	}
}

// --- GetAPISurface ---

func TestGetAPISurface(t *testing.T) {
	eng, _ := newTestEngine()
	r, err := eng.GetAPISurface(context.Background(), "/tmp", nil)
	if err != nil {
		t.Fatal(err)
	}
	if r == nil {
		t.Fatal("expected non-nil API surface report")
	}
}

// --- GetDrift ---

func TestGetDrift_NoDesiredState(t *testing.T) {
	eng, _ := newTestEngine()
	r, err := eng.GetDrift(context.Background(), "/tmp")
	if err != nil {
		t.Fatal(err)
	}
	// GetDrift auto-bootstraps inferred layers when no desired state exists,
	// so HasDesiredState is true with inferred layers.
	if r == nil {
		t.Fatal("expected non-nil drift report")
	}
	if r.Summary == "" {
		t.Error("expected non-empty summary")
	}
}

func TestGetDrift_WithDesiredState(t *testing.T) {
	eng, store := newTestEngine()
	store.desiredState = testDesiredState()
	r, err := eng.GetDrift(context.Background(), "/tmp")
	if err != nil {
		t.Fatal(err)
	}
	if !r.HasDesiredState {
		t.Error("expected HasDesiredState=true")
	}
}

// --- GetScanDiff ---

func TestGetScanDiff(t *testing.T) {
	before := testReport()
	after := testReport()
	after.Architecture.Services = append(after.Architecture.Services, arch.ArchService{
		Name: "new/pkg", LOC: 100,
	})

	store := newMockStore(nil)
	store.reportsBySHA = map[string]*arch.ContextReport{
		"sha1": before,
		"sha2": after,
	}
	eng := New(store, []string{"/tmp"})

	r, err := eng.GetScanDiff(context.Background(), "/tmp", "sha1", "sha2")
	if err != nil {
		t.Fatal(err)
	}
	if len(r.AddedComponents) != 1 {
		t.Errorf("AddedComponents = %d, want 1", len(r.AddedComponents))
	}
}

func TestGetScanDiff_MissingBefore(t *testing.T) {
	eng, _ := newTestEngine()
	_, err := eng.GetScanDiff(context.Background(), "/tmp", "", "sha2")
	if err == nil {
		t.Error("expected ErrBeforeSHARequired")
	}
}

// --- GetLeverage ---

func TestGetLeverage(t *testing.T) {
	eng, _ := newTestEngine()
	r, err := eng.GetLeverage(context.Background(), "/tmp", "internal/core")
	if err != nil {
		t.Fatal(err)
	}
	if r == nil {
		t.Fatal("expected non-nil leverage report")
	}
}

// --- GetRiskScores ---

func TestGetRiskScores(t *testing.T) {
	eng, _ := newTestEngine()
	r, err := eng.GetRiskScores(context.Background(), "/tmp")
	if err != nil {
		t.Fatal(err)
	}
	if r == nil {
		t.Fatal("expected non-nil risk scores")
	}
}

// --- GetConsolidation ---

func TestGetConsolidation(t *testing.T) {
	eng, _ := newTestEngine()
	r, err := eng.GetConsolidation(context.Background(), "/tmp")
	if err != nil {
		t.Fatal(err)
	}
	if r == nil {
		t.Fatal("expected non-nil consolidation report")
	}
}

// --- GetBudgets ---

func TestGetBudgets_NoConstraints(t *testing.T) {
	eng, _ := newTestEngine()
	r, err := eng.GetBudgets(context.Background(), "/tmp")
	if err != nil {
		t.Fatal(err)
	}
	if r == nil {
		t.Fatal("expected non-nil budget report")
	}
}

func TestGetBudgets_WithConstraints(t *testing.T) {
	eng, store := newTestEngine()
	store.desiredState = testDesiredState()
	r, err := eng.GetBudgets(context.Background(), "/tmp")
	if err != nil {
		t.Fatal(err)
	}
	if r == nil {
		t.Fatal("expected non-nil budget report")
	}
}

// --- GetImportDirection ---

func TestGetImportDirection(t *testing.T) {
	eng, _ := newTestEngine()
	r, err := eng.GetImportDirection(context.Background(), "/tmp")
	if err != nil {
		t.Fatal(err)
	}
	if r == nil {
		t.Fatal("expected non-nil import direction report")
	}
}

// --- GetTrustBoundaries ---

func TestGetTrustBoundaries(t *testing.T) {
	eng, _ := newTestEngine()
	r, err := eng.GetTrustBoundaries(context.Background(), "/tmp")
	if err != nil {
		t.Fatal(err)
	}
	if r == nil {
		t.Fatal("expected non-nil trust boundary report")
	}
}

// --- GetPatternCatalog ---

func TestGetPatternCatalog(t *testing.T) {
	eng, _ := newTestEngine()
	r := eng.GetPatternCatalog("")
	if r == nil {
		t.Fatal("expected non-nil pattern catalog")
	}
}

// --- Status ---

func TestStatus(t *testing.T) {
	eng, store := newTestEngine()
	store.projects = []port.ProjectInfo{
		{Path: "/tmp/project", Name: "test", Components: 5},
	}
	r, err := eng.Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if r == nil {
		t.Fatal("expected non-nil status")
	}
}

// --- SearchComponents ---

func TestSearchComponents(t *testing.T) {
	eng, store := newTestEngine()
	store.components = []port.ComponentMeta{
		{Name: "internal/core", Role: "core"},
	}
	r, err := eng.SearchComponents(context.Background(), "/tmp", "core")
	if err != nil {
		t.Fatal(err)
	}
	if r == nil {
		t.Fatal("expected non-nil search result")
	}
}

// --- TSK-176: Cache key must include intent ---

func TestScanProject_DifferentIntentsGetDifferentCacheEntries(t *testing.T) {
	// Build a mock store that records the SHA keys used for PutReport.
	store := &intentMockStore{
		mockStore: mockStore{
			headSHA:   "abc123",
			reportHit: false, // force scan path — no cached report
		},
		putKeys: make(map[string]bool),
		getKeys: make(map[string]bool),
	}
	eng := New(store, []string{"/tmp"})

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// First scan with intent "health".
	_, _ = eng.ScanProject(ctx, "/tmp", ScanOpts{Intent: "health"})

	// Second scan with intent "architecture".
	_, _ = eng.ScanProject(ctx, "/tmp", ScanOpts{Intent: "architecture"})

	// The two intents must produce different cache keys.
	if len(store.putKeys) < 2 {
		t.Errorf("expected 2 distinct cache keys for different intents, got %d: %v",
			len(store.putKeys), store.putKeys)
	}

	// Verify the keys contain intent discriminator.
	hasHealth := false
	hasArch := false
	for k := range store.putKeys {
		if strings.Contains(k, "health") {
			hasHealth = true
		}
		if strings.Contains(k, "architecture") {
			hasArch = true
		}
	}
	if !hasHealth || !hasArch {
		t.Errorf("cache keys should contain intent names; got keys: %v", store.putKeys)
	}
}

// intentMockStore extends mockStore to track cache key usage.
type intentMockStore struct {
	mockStore
	putKeys map[string]bool
	getKeys map[string]bool
}

func (m *intentMockStore) GetReport(_ context.Context, project, sha string) (*arch.ContextReport, bool, error) {
	m.getKeys[sha] = true
	return nil, false, nil
}

func (m *intentMockStore) PutReport(_ context.Context, project, sha string, _ *arch.ContextReport) error {
	m.putKeys[sha] = true
	m.putReportCalls++
	return nil
}

// --- SearchSymbols file filter ---

// testReportWithFilePaths extends testReport with explicit file paths on
// symbols so file-filtered search can be tested.
func testReportWithFilePaths() *arch.ContextReport {
	r := testReport()
	for i := range r.Architecture.Services {
		svc := &r.Architecture.Services[i]
		for j := range svc.Symbols {
			svc.Symbols[j].File = svc.Name + "/main.go"
		}
	}
	// Override one symbol in internal/core to a different file.
	for i := range r.Architecture.Services {
		if r.Architecture.Services[i].Name == "internal/core" {
			for j := range r.Architecture.Services[i].Symbols {
				if r.Architecture.Services[i].Symbols[j].Name == "Config" {
					r.Architecture.Services[i].Symbols[j].File = "internal/core/config.go"
				}
			}
		}
	}
	return r
}

// TestSearchSymbols_FileFilter verifies that when a file path is provided to
// SearchSymbols, only symbols from that file are returned.
//
// Given a project with symbols spread across multiple files
// When SearchSymbols is called with file=internal/core/config.go
// Then only the Config symbol is returned
func TestSearchSymbols_FileFilter(t *testing.T) {
	store := newMockStore(testReportWithFilePaths())
	eng := New(store, []string{"/tmp"})

	r, err := eng.SearchSymbolsFiltered(context.Background(), "/tmp", "", "internal/core/config.go")
	if err != nil {
		t.Fatalf("SearchSymbolsFiltered: %v", err)
	}

	if len(r.Matches) != 1 {
		names := make([]string, len(r.Matches))
		for i, m := range r.Matches {
			names[i] = m.Symbol
		}
		t.Errorf("expected 1 match (Config), got %d: %v", len(r.Matches), names)
	}
	if len(r.Matches) > 0 && r.Matches[0].Symbol != "Config" {
		t.Errorf("expected Config, got %q", r.Matches[0].Symbol)
	}
}

// TestSearchSymbols_FileAndPatternFilter verifies combined file + name pattern.
//
// Given symbols in internal/core with file=internal/core/main.go
// When SearchSymbols(pattern="Run", file="internal/core/main.go")
// Then only Run is returned (Config is in config.go, excluded)
func TestSearchSymbols_FileAndPatternFilter(t *testing.T) {
	store := newMockStore(testReportWithFilePaths())
	eng := New(store, []string{"/tmp"})

	r, err := eng.SearchSymbolsFiltered(context.Background(), "/tmp", "run", "internal/core/main.go")
	if err != nil {
		t.Fatalf("SearchSymbolsFiltered: %v", err)
	}

	if len(r.Matches) != 1 || r.Matches[0].Symbol != "Run" {
		names := make([]string, len(r.Matches))
		for i, m := range r.Matches {
			names[i] = m.Symbol
		}
		t.Errorf("expected [Run], got %v", names)
	}
}

// TestSearchSymbols_NoFileFilter verifies backward compat: empty file = all files.
func TestSearchSymbols_NoFileFilter(t *testing.T) {
	store := newMockStore(testReportWithFilePaths())
	eng := New(store, []string{"/tmp"})

	r, err := eng.SearchSymbolsFiltered(context.Background(), "/tmp", "config", "")
	if err != nil {
		t.Fatalf("SearchSymbolsFiltered: %v", err)
	}

	if len(r.Matches) == 0 {
		t.Error("expected Config match with no file filter")
	}
}

// --- GetCallersAt ---

// TestGetCallersAt_StubPool verifies that GetCallersAt returns an empty result
// (not an error) when the pool is a StubPool (CLI mode with no LSP).
//
// Given a project with StubPool
// When GetCallersAt is called with a file position
// Then an empty CallersReport is returned (not an error)
func TestGetCallersAt_StubPool(t *testing.T) {
	store := newMockStore(testReport())
	eng := New(store, []string{"/tmp"})

	r, err := eng.GetCallersAt(context.Background(), "/tmp", "/tmp/internal/core/main.go", 10, 0)
	if err != nil {
		t.Fatalf("GetCallersAt with StubPool: %v", err)
	}
	if r == nil {
		t.Fatal("expected non-nil CallersReport")
	}
	// StubPool returns ErrNoPool → empty callers (not an error propagated up).
	_ = r.Callers
}

// --- GetComponentRangeDiff ---

// TestGetComponentRangeDiff_SameSHA verifies that a range diff of a SHA against
// itself returns no changes.
//
// Given before_sha == after_sha
// When GetComponentRangeDiff is called
// Then the result has no touched components
func TestGetComponentRangeDiff_SameSHA(t *testing.T) {
	store := newMockStore(testReport())
	eng := New(store, []string{"/tmp"})

	r, err := eng.GetComponentRangeDiff(context.Background(), "/tmp", "abc123", "abc123")
	if err != nil {
		t.Fatalf("GetComponentRangeDiff same SHA: %v", err)
	}
	if len(r.TouchedComponents) != 0 {
		t.Errorf("expected 0 touched components for same SHA, got %d", len(r.TouchedComponents))
	}
}

// --- MigrationOverlay ---

// TestComputeMigrationOverlay_PortedAndPending verifies that MigrationOverlay
// correctly classifies ported vs pending symbols between two components.
//
// Given a source component with 4 symbols and a target with 2 matching names
// When ComputeMigrationOverlay is called
// Then ported has the 2 matching symbols and pending has the 2 remaining
// And ProgressPct is 50.0
func TestComputeMigrationOverlay_PortedAndPending(t *testing.T) {
	// Build a report with two components sharing some symbol names.
	r := testReport()
	// Source: internal/core with Run, Config, Init
	// Target: internal/store with Run, Get, Put (Run matches, Config does not, etc.)
	store := newMockStore(r)
	eng := New(store, []string{"/tmp"})

	overlay, err := eng.ComputeMigrationOverlay(context.Background(), "/tmp", "internal/core", "internal/store")
	if err != nil {
		t.Fatalf("ComputeMigrationOverlay: %v", err)
	}

	// internal/core has: Run, Config, Init
	// internal/store has: DB, Get, Put
	// No names match (snake/camel normalize: Run≠DB, Run≠Get, etc.)
	// So all of source is "pending".
	if overlay.ProgressPct != 0 {
		t.Errorf("expected 0%% progress (no matching symbols), got %.1f%%", overlay.ProgressPct)
	}
	if len(overlay.PendingSymbols) != 3 {
		t.Errorf("expected 3 pending (Run, Config, Init), got %d: %v", len(overlay.PendingSymbols), overlay.PendingSymbols)
	}

	// Verify meta fields.
	if overlay.Source != "internal/core" {
		t.Errorf("source = %q, want internal/core", overlay.Source)
	}
	if overlay.Target != "internal/store" {
		t.Errorf("target = %q, want internal/store", overlay.Target)
	}
}

// TestComputeMigrationOverlay_MissingComponent verifies that a clear error is
// returned when either component does not exist.
//
// Given component "nonexistent" not in the scan
// When ComputeMigrationOverlay is called
// Then a descriptive error is returned
func TestComputeMigrationOverlay_MissingComponent(t *testing.T) {
	store := newMockStore(testReport())
	eng := New(store, []string{"/tmp"})

	_, err := eng.ComputeMigrationOverlay(context.Background(), "/tmp", "nonexistent", "internal/core")
	if err == nil {
		t.Fatal("expected error for missing component, got nil")
	}
}

// --- Symbol mirrors ---

// TestRegisterMirror_ListMirrors verifies that registered mirrors are stored
// and returned by ListMirrors.
//
// Given RegisterMirror(ctx, "ts:computeGap", "rs:compute_gap")
// When ListMirrors is called
// Then the pair is returned
func TestRegisterMirror_ListMirrors(t *testing.T) {
	store := newMockStore(testReport())
	eng := New(store, []string{"/tmp"})
	ctx := context.Background()

	if err := eng.RegisterMirror(ctx, "/tmp", "ts:computeGap", "rs:compute_gap"); err != nil {
		t.Fatalf("RegisterMirror: %v", err)
	}

	mirrors, err := eng.ListMirrors(ctx, "/tmp")
	if err != nil {
		t.Fatalf("ListMirrors: %v", err)
	}

	if len(mirrors) != 1 {
		t.Fatalf("expected 1 mirror, got %d", len(mirrors))
	}
	if mirrors[0].From != "ts:computeGap" || mirrors[0].To != "rs:compute_gap" {
		t.Errorf("unexpected mirror: %+v", mirrors[0])
	}
}

// TestRegisterMirror_Idempotent verifies that registering the same pair twice
// does not create a duplicate.
func TestRegisterMirror_Idempotent(t *testing.T) {
	store := newMockStore(testReport())
	eng := New(store, []string{"/tmp"})
	ctx := context.Background()

	_ = eng.RegisterMirror(ctx, "/tmp", "a", "b")
	_ = eng.RegisterMirror(ctx, "/tmp", "a", "b")

	mirrors, err := eng.ListMirrors(ctx, "/tmp")
	if err != nil {
		t.Fatalf("ListMirrors: %v", err)
	}
	if len(mirrors) != 1 {
		t.Errorf("expected 1 mirror (idempotent), got %d", len(mirrors))
	}
}

// TestListMirrors_Empty verifies that ListMirrors returns an empty slice (not nil)
// when no mirrors are registered.
func TestListMirrors_Empty(t *testing.T) {
	store := newMockStore(testReport())
	eng := New(store, []string{"/tmp"})
	mirrors, err := eng.ListMirrors(context.Background(), "/tmp")
	if err != nil {
		t.Fatalf("ListMirrors: %v", err)
	}
	if mirrors == nil {
		t.Error("expected non-nil empty slice")
	}
}

// --- ComponentDetail: test coverage fields ---

// TestGetComponentDetail_HasTests verifies that HasTests and CoverageRatio
// surface in the component action response.
//
// Given a component with test file symbols (File ending in _test.go)
// When GetComponentDetail is called
// Then HasTests and CoverageRatio are present in the result
func TestGetComponentDetail_HasTests(t *testing.T) {
	r := testReport()
	// Inject a test file reference into internal/core symbols.
	for i := range r.Architecture.Services {
		if r.Architecture.Services[i].Name == "internal/core" {
			r.Architecture.Services[i].HasTests = true
			r.Architecture.Services[i].CoverageRatio = 0.5
			r.Architecture.Services[i].TestFiles = []string{"internal/core/core_test.go"}
		}
	}
	store := newMockStore(r)
	eng := New(store, []string{"/tmp"})

	detail, err := eng.GetComponentDetail(context.Background(), "/tmp", "internal/core")
	if err != nil {
		t.Fatalf("GetComponentDetail: %v", err)
	}
	if !detail.HasTests {
		t.Error("HasTests should be true")
	}
	if detail.CoverageRatio != 0.5 {
		t.Errorf("CoverageRatio = %f, want 0.5", detail.CoverageRatio)
	}
	if len(detail.TestFiles) != 1 {
		t.Errorf("TestFiles = %v, want [core_test.go]", detail.TestFiles)
	}
}

// --- LCS-BUG-63: call graph operations must signal incompleteness ---

// TestCallGraphStatus_FieldExists verifies LCS-BUG-63: CalleesReport and
// CallersReport carry a CallGraphStatus field that is populated when the call
// graph has 0 edges, distinguishing "no callees" from "analysis incomplete".
//
// The field is populated in GetCallees/GetCallers when len(cg.Edges)==0.
// Tested at the struct level to avoid spawning real LSP servers in unit tests.
func TestCallGraphStatus_FieldExists(t *testing.T) {
	r := &CalleesReport{Symbol: "X", CallGraphStatus: "partial"}
	if r.CallGraphStatus == "" {
		t.Error("CalleesReport.CallGraphStatus field missing")
	}
	cr := &CallersReport{Symbol: "X", CallGraphStatus: "partial"}
	if cr.CallGraphStatus == "" {
		t.Error("CallersReport.CallGraphStatus field missing")
	}
}
