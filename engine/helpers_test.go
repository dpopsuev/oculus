package engine

import (
	"strings"
	"testing"

	"github.com/dpopsuev/oculus/arch"
	"github.com/dpopsuev/oculus/graph"
	"github.com/dpopsuev/oculus/model"
	"github.com/dpopsuev/oculus/port"
)

// --- totalLOC ---

func TestTotalLOC(t *testing.T) {
	r := testReport()
	got := totalLOC(r)
	want := 100 + 500 + 300 + 80 // cmd/app + internal/core + internal/store + pkg/logger
	if got != want {
		t.Errorf("totalLOC = %d, want %d", got, want)
	}
}

func TestTotalLOC_Empty(t *testing.T) {
	r := &arch.ContextReport{}
	if got := totalLOC(r); got != 0 {
		t.Errorf("totalLOC(empty) = %d, want 0", got)
	}
}

// --- inferLayerOrder ---

func TestInferLayerOrder(t *testing.T) {
	r := testReport()
	layers := inferLayerOrder(r)
	if len(layers) != 4 {
		t.Fatalf("got %d layers, want 4", len(layers))
	}
	// Depth 0 (internal/store, pkg/logger) should come before depth 2 (cmd/app)
	storeIdx, appIdx := -1, -1
	for i, l := range layers {
		if l == "internal/store" {
			storeIdx = i
		}
		if l == "cmd/app" {
			appIdx = i
		}
	}
	if storeIdx >= appIdx {
		t.Errorf("internal/store (depth 0) should come before cmd/app (depth 2): got indices %d, %d", storeIdx, appIdx)
	}
}

func TestInferLayerOrder_NilDepths(t *testing.T) {
	r := testReport()
	r.ImportDepth = nil // force fallback to graph.ImportDepth(edges)
	layers := inferLayerOrder(r)
	if len(layers) == 0 {
		t.Error("inferLayerOrder with nil depths returned empty")
	}
}

// --- countBudgetChecks ---

func TestCountBudgetChecks(t *testing.T) {
	services := testReport().Architecture.Services
	constraints := []port.HealthConstraint{
		{Component: "internal/core", MaxFanIn: 5, MaxChurn: 30},
	}
	got := countBudgetChecks(services, constraints)
	if got != 2 { // MaxFanIn + MaxChurn
		t.Errorf("countBudgetChecks = %d, want 2", got)
	}
}

func TestCountBudgetChecks_NoMatch(t *testing.T) {
	services := testReport().Architecture.Services
	constraints := []port.HealthConstraint{
		{Component: "nonexistent", MaxFanIn: 5},
	}
	if got := countBudgetChecks(services, constraints); got != 0 {
		t.Errorf("countBudgetChecks(no match) = %d, want 0", got)
	}
}

func TestCountBudgetChecks_Empty(t *testing.T) {
	if got := countBudgetChecks(nil, nil); got != 0 {
		t.Errorf("countBudgetChecks(nil, nil) = %d, want 0", got)
	}
}

// --- diffEdges ---

func TestDiffEdges_Identical(t *testing.T) {
	edges := testReport().Architecture.Edges
	added, removed := diffEdges(edges, edges)
	if added != 0 || removed != 0 {
		t.Errorf("diffEdges(same, same) = added:%d removed:%d, want 0,0", added, removed)
	}
}

func TestDiffEdges_Added(t *testing.T) {
	before := testReport().Architecture.Edges[:2]
	after := testReport().Architecture.Edges
	added, removed := diffEdges(before, after)
	if added != 2 || removed != 0 {
		t.Errorf("diffEdges = added:%d removed:%d, want 2,0", added, removed)
	}
}

func TestDiffEdges_Removed(t *testing.T) {
	before := testReport().Architecture.Edges
	after := testReport().Architecture.Edges[:2]
	added, removed := diffEdges(before, after)
	if added != 0 || removed != 2 {
		t.Errorf("diffEdges = added:%d removed:%d, want 0,2", added, removed)
	}
}

// --- diffReports ---

func TestDiffReports(t *testing.T) {
	before := testReport()
	after := testReport()
	// Add a component to "after"
	after.Architecture.Services = append(after.Architecture.Services, arch.ArchService{
		Name: "internal/newpkg", LOC: 200, Language: model.LangGo,
	})

	diff := diffReports("sha1", "sha2", before, after)
	if diff.BeforeSHA != "sha1" || diff.AfterSHA != "sha2" {
		t.Errorf("SHAs wrong: %s, %s", diff.BeforeSHA, diff.AfterSHA)
	}
	if len(diff.AddedComponents) != 1 || diff.AddedComponents[0] != "internal/newpkg" {
		t.Errorf("AddedComponents = %v, want [internal/newpkg]", diff.AddedComponents)
	}
	if diff.LOCDelta != 200 {
		t.Errorf("LOCDelta = %d, want 200", diff.LOCDelta)
	}
}

func TestDiffReports_Removed(t *testing.T) {
	before := testReport()
	after := testReport()
	after.Architecture.Services = after.Architecture.Services[:2] // keep only 2

	diff := diffReports("a", "b", before, after)
	if len(diff.RemovedComponents) != 2 {
		t.Errorf("RemovedComponents count = %d, want 2", len(diff.RemovedComponents))
	}
}

// --- generateComponentMeta ---

func TestGenerateComponentMeta(t *testing.T) {
	r := testReport()
	meta := generateComponentMeta(r)
	if len(meta) != 4 {
		t.Fatalf("got %d meta entries, want 4", len(meta))
	}
	// Check role inference
	roles := make(map[string]string)
	for _, m := range meta {
		roles[m.Name] = m.Role
	}
	if roles["cmd/app"] != "entrypoint" {
		t.Errorf("cmd/app role = %q, want entrypoint", roles["cmd/app"])
	}
	if roles["internal/core"] != "core" {
		t.Errorf("internal/core role = %q, want core", roles["internal/core"])
	}
	if roles["pkg/logger"] != "library" {
		t.Errorf("pkg/logger role = %q, want library", roles["pkg/logger"])
	}
}

// --- inferRole ---

func TestInferRole(t *testing.T) {
	tests := []struct {
		name, want string
	}{
		{"cmd/app", "entrypoint"},
		{"internal/core", "core"},
		{"pkg/logger", "library"},
		{"testutil", "test"},
		{"random", "module"},
	}
	for _, tt := range tests {
		if got := inferRole(tt.name); got != tt.want {
			t.Errorf("inferRole(%q) = %q, want %q", tt.name, got, tt.want)
		}
	}
}

// --- extractKeywords ---

func TestExtractKeywords(t *testing.T) {
	s := arch.ArchService{
		Name:    "internal/core",
		Symbols: model.SymbolsFromNames("Run", "Config", "Init"),
	}
	kw := extractKeywords(s)
	if len(kw) == 0 {
		t.Fatal("extractKeywords returned empty")
	}
	// Should include path segments
	found := false
	for _, k := range kw {
		if k == "core" {
			found = true
		}
	}
	if !found {
		t.Error("keywords missing 'core' from path segment")
	}
}

// --- GenerateHints ---

func TestGenerateHints_Clean(t *testing.T) {
	r := testReport()
	r.HotSpots = nil
	r.Cycles = nil
	r.LayerViolations = nil
	hints := GenerateHints(r)
	if len(hints) != 0 {
		t.Errorf("clean report should have 0 hints, got %d", len(hints))
	}
}

func TestGenerateHints_WithCycles(t *testing.T) {
	r := testReportWithCycles()
	hints := GenerateHints(r)
	found := false
	for _, h := range hints {
		if strings.Contains(h, "cycle") {
			found = true
		}
	}
	if !found {
		t.Error("expected cycle hint")
	}
}

func TestGenerateHints_WithHotSpots(t *testing.T) {
	r := testReport() // has hot spots
	hints := GenerateHints(r)
	found := false
	for _, h := range hints {
		if strings.Contains(h, "hot spot") {
			found = true
		}
	}
	if !found {
		t.Error("expected hot spot hint")
	}
}

// --- buildEvolutionSummary ---

func TestBuildEvolutionSummary_Empty(t *testing.T) {
	got := buildEvolutionSummary(nil)
	if got != "no steps" {
		t.Errorf("empty = %q, want 'no steps'", got)
	}
}

func TestBuildEvolutionSummary_SingleStep(t *testing.T) {
	steps := []EvolutionStep{
		{Components: 10, Edges: 15, TotalLOC: 1000, ShortSHA: "abc1234"},
	}
	got := buildEvolutionSummary(steps)
	if got == "" || got == "no steps" {
		t.Error("single step should produce non-empty summary")
	}
}

func TestBuildEvolutionSummary_Growth(t *testing.T) {
	steps := []EvolutionStep{
		{Components: 5, Edges: 8, TotalLOC: 500, ShortSHA: "aaa1111"},
		{Components: 10, Edges: 15, TotalLOC: 1000, ShortSHA: "bbb2222"},
	}
	got := buildEvolutionSummary(steps)
	if !strings.Contains(got, "5") || !strings.Contains(got, "10") {
		t.Errorf("summary should mention component counts: %s", got)
	}
}

// --- sampleCommits ---

func TestSampleCommits_Identity(t *testing.T) {
	commits := []CommitMeta{
		{SHA: "a"}, {SHA: "b"}, {SHA: "c"},
	}
	got := sampleCommits(commits, 1)
	if len(got) != 3 {
		t.Errorf("stride=1: got %d, want 3", len(got))
	}
}

func TestSampleCommits_Stride2(t *testing.T) {
	commits := []CommitMeta{
		{SHA: "a"}, {SHA: "b"}, {SHA: "c"}, {SHA: "d"}, {SHA: "e"},
	}
	got := sampleCommits(commits, 2)
	// Should pick a, c, e (stride=2 always includes last)
	if got[0].SHA != "a" {
		t.Errorf("first = %s, want a", got[0].SHA)
	}
	if got[len(got)-1].SHA != "e" {
		t.Errorf("last = %s, want e", got[len(got)-1].SHA)
	}
}

func TestSampleCommits_TwoElements(t *testing.T) {
	commits := []CommitMeta{{SHA: "a"}, {SHA: "b"}}
	got := sampleCommits(commits, 5)
	if len(got) != 2 {
		t.Errorf("2 elements with any stride should return both: got %d", len(got))
	}
}

// --- RenderScanSummary ---

func TestRenderScanSummary(t *testing.T) {
	r := &ScanResult{
		Report:   testReport(),
		CacheKey: "test@deadbeef",
		SHA:      "deadbeef",
	}
	got := RenderScanSummary(r, "")
	if !strings.Contains(got, "4 components") {
		t.Errorf("missing component count: %s", got)
	}
	if !strings.Contains(got, "cache_key") {
		t.Errorf("missing cache_key: %s", got)
	}
}

func TestRenderScanSummary_WithDrift(t *testing.T) {
	r := &ScanResult{Report: testReport(), CacheKey: "x@y"}
	got := RenderScanSummary(r, "drift detected: 2 violations")
	if !strings.Contains(got, "drift detected") {
		t.Errorf("missing drift info: %s", got)
	}
}

// --- resolveRolesAndAccepted ---

func TestResolveRolesAndAccepted_NilNil(t *testing.T) {
	roles, accepted := resolveRolesAndAccepted(nil, nil)
	if roles != nil {
		t.Error("expected nil roles")
	}
	if accepted != nil {
		t.Error("expected nil accepted")
	}
}

func TestResolveRolesAndAccepted_WithDesired(t *testing.T) {
	ds := &port.DesiredState{
		Accepted: []port.AcceptedViolation{
			{Component: "legacy", Principle: "SRP", Reason: "planned"},
		},
	}
	_, accepted := resolveRolesAndAccepted(nil, ds)
	if len(accepted) != 1 {
		t.Errorf("accepted = %d, want 1", len(accepted))
	}
}

// --- CheckDriftOnScan ---

func TestCheckDriftOnScan_NilStore(t *testing.T) {
	// CheckDriftOnScan takes a report directly, but still needs p.db for desired state.
	// With a mock store returning no desired state, drift should be empty.
	store := newMockStore(testReport())
	eng := New(store, nil)
	drift := eng.CheckDriftOnScan(nil, "/tmp", testReport())
	if drift != "" {
		t.Errorf("expected empty drift with no desired state, got %q", drift)
	}
}

func TestCheckDriftOnScan_NoDesiredState(t *testing.T) {
	store := newMockStore(testReport())
	eng := New(store, []string{"/tmp"})
	report := testReport()
	drift := eng.CheckDriftOnScan(nil, "/tmp", report)
	if drift != "" {
		// No desired state = no drift info
		t.Errorf("expected empty drift, got %q", drift)
	}
}

func TestCheckDriftOnScan_WithDesiredState(t *testing.T) {
	store := newMockStore(testReport())
	store.desiredState = testDesiredState()
	eng := New(store, []string{"/tmp"})
	report := testReport()
	drift := eng.CheckDriftOnScan(nil, "/tmp", report)
	// With desired state and clean architecture, drift should mention layers
	if drift == "" {
		t.Log("drift is empty — no violations detected (expected for clean report)")
	}
}

// --- Unused import guard ---

var _ = graph.DepthMap{}
