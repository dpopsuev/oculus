package engine

import (
	"context"
	"strings"
	"testing"

	"github.com/dpopsuev/oculus/arch"
	"github.com/dpopsuev/oculus/port"
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
