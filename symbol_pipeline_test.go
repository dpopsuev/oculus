package oculus

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// --- MockSymbolSource: in-memory SymbolSource for testing ---

type mockSymbolSource struct {
	roots    []SourceSymbol
	children map[string][]SourceRelation // keyed by symbol name
	hovers   map[string]*SourceTypeInfo  // keyed by symbol name
	latency  time.Duration
	calls    atomic.Int64 // counts Children calls for concurrency verification
}

func (m *mockSymbolSource) Roots(ctx context.Context, query string) ([]SourceSymbol, error) {
	if m.latency > 0 {
		select {
		case <-time.After(m.latency):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	if query != "" {
		for _, r := range m.roots {
			if r.Name == query {
				return []SourceSymbol{r}, nil
			}
		}
		return nil, nil
	}
	return m.roots, nil
}

func (m *mockSymbolSource) Children(ctx context.Context, sym SourceSymbol) ([]SourceRelation, error) {
	m.calls.Add(1)
	if m.latency > 0 {
		select {
		case <-time.After(m.latency):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return m.children[sym.Name], nil
}

func (m *mockSymbolSource) Hover(ctx context.Context, sym SourceSymbol) (*SourceTypeInfo, error) {
	if m.latency > 0 {
		select {
		case <-time.After(m.latency):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return m.hovers[sym.Name], nil
}

// --- Tests ---

// TestSymbolPipeline_InterfaceCompliance verifies SymbolPipeline satisfies DeepAnalyzer.
func TestSymbolPipeline_InterfaceCompliance(t *testing.T) {
	var da DeepAnalyzer = &SymbolPipeline{}
	if da == nil {
		t.Fatal("SymbolPipeline should satisfy DeepAnalyzer")
	}
}

// TestSymbolPipeline_CallGraph_Basic tests the fundamental walk: single root with children.
//
//	Main → Foo → Baz
//	Main → Bar
//
// Expect 4 nodes, 3 edges.
func TestSymbolPipeline_CallGraph_Basic(t *testing.T) {
	src := &mockSymbolSource{
		roots: []SourceSymbol{
			{Name: "Main", Package: "cmd", File: "main.go", Line: 1},
		},
		children: map[string][]SourceRelation{
			"Main": {
				{Target: SourceSymbol{Name: "Foo", Package: "pkg", File: "foo.go", Line: 10}, Kind: "call", InWorkspace: true},
				{Target: SourceSymbol{Name: "Bar", Package: "pkg", File: "bar.go", Line: 20}, Kind: "call", InWorkspace: true},
			},
			"Foo": {
				{Target: SourceSymbol{Name: "Baz", Package: "util", File: "baz.go", Line: 5}, Kind: "call", InWorkspace: true},
			},
		},
	}

	p := &SymbolPipeline{Source: src, Root: "/workspace"}
	cg, err := p.CallGraph(context.Background(), "/workspace", CallGraphOpts{Depth: 10})
	if err != nil {
		t.Fatalf("CallGraph: %v", err)
	}

	if len(cg.Nodes) != 4 {
		t.Errorf("nodes = %d, want 4 (Main, Foo, Bar, Baz)", len(cg.Nodes))
	}
	if len(cg.Edges) != 3 {
		t.Errorf("edges = %d, want 3 (Main→Foo, Main→Bar, Foo→Baz)", len(cg.Edges))
	}
}

// TestSymbolPipeline_CallGraph_DepthLimit tests that the walk respects depth limits.
// Chain: A → B → C → D → E. Depth 2 should produce A, B, C (not D, E).
func TestSymbolPipeline_CallGraph_DepthLimit(t *testing.T) {
	src := &mockSymbolSource{
		roots: []SourceSymbol{
			{Name: "A", Package: "pkg", File: "a.go", Line: 1},
		},
		children: map[string][]SourceRelation{
			"A": {{Target: SourceSymbol{Name: "B", Package: "pkg", File: "b.go", Line: 1}, Kind: "call", InWorkspace: true}},
			"B": {{Target: SourceSymbol{Name: "C", Package: "pkg", File: "c.go", Line: 1}, Kind: "call", InWorkspace: true}},
			"C": {{Target: SourceSymbol{Name: "D", Package: "pkg", File: "d.go", Line: 1}, Kind: "call", InWorkspace: true}},
			"D": {{Target: SourceSymbol{Name: "E", Package: "pkg", File: "e.go", Line: 1}, Kind: "call", InWorkspace: true}},
		},
	}

	p := &SymbolPipeline{Source: src, Root: "/workspace"}
	cg, err := p.CallGraph(context.Background(), "/workspace", CallGraphOpts{Depth: 2})
	if err != nil {
		t.Fatalf("CallGraph: %v", err)
	}

	// Depth 2 from A: A (depth 0) → B (depth 1) → C (depth 2). D and E pruned.
	if len(cg.Nodes) != 3 {
		t.Errorf("nodes = %d, want 3 (A, B, C)", len(cg.Nodes))
	}
	if len(cg.Edges) != 2 {
		t.Errorf("edges = %d, want 2 (A→B, B→C)", len(cg.Edges))
	}

	// Verify D and E are NOT present.
	for _, n := range cg.Nodes {
		if n.Name == "D" || n.Name == "E" {
			t.Errorf("unexpected node %s beyond depth limit", n.Name)
		}
	}
}

// TestSymbolPipeline_CallGraph_BoundedConcurrency tests that multiple roots
// are walked concurrently with bounded parallelism.
func TestSymbolPipeline_CallGraph_BoundedConcurrency(t *testing.T) {
	const numRoots = 8
	latency := 50 * time.Millisecond

	roots := make([]SourceSymbol, numRoots)
	children := make(map[string][]SourceRelation)
	for i := range numRoots {
		name := "Root" + string(rune('A'+i))
		roots[i] = SourceSymbol{Name: name, Package: "pkg", File: "root.go", Line: i + 1}
		childName := "Child" + string(rune('A'+i))
		children[name] = []SourceRelation{
			{Target: SourceSymbol{Name: childName, Package: "pkg", File: "child.go", Line: i + 1}, Kind: "call", InWorkspace: true},
		}
	}

	src := &mockSymbolSource{
		roots:    roots,
		children: children,
		latency:  latency,
	}

	p := &SymbolPipeline{Source: src, Root: "/workspace", Concurrency: 4}

	start := time.Now()
	cg, err := p.CallGraph(context.Background(), "/workspace", CallGraphOpts{Depth: 5})
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("CallGraph: %v", err)
	}

	// 8 roots + 8 children = 16 nodes, 8 edges
	if len(cg.Nodes) < 16 {
		t.Errorf("nodes = %d, want 16", len(cg.Nodes))
	}

	// Sequential would take 8 * 2 * 50ms = 800ms (roots + children).
	// With concurrency 4, expect ~200-400ms range.
	sequential := time.Duration(numRoots*2) * latency
	if elapsed > sequential {
		t.Errorf("elapsed %v >= sequential %v — no concurrency benefit", elapsed, sequential)
	}
	t.Logf("elapsed: %v, sequential estimate: %v, speedup: %.1fx", elapsed, sequential, float64(sequential)/float64(elapsed))
}

// TestSymbolPipeline_CallGraph_ContextCancel tests graceful return on context cancellation.
func TestSymbolPipeline_CallGraph_ContextCancel(t *testing.T) {
	src := &mockSymbolSource{
		roots: []SourceSymbol{
			{Name: "Slow", Package: "pkg", File: "slow.go", Line: 1},
		},
		children: map[string][]SourceRelation{
			"Slow": {
				{Target: SourceSymbol{Name: "A", Package: "pkg"}, Kind: "call", InWorkspace: true},
				{Target: SourceSymbol{Name: "B", Package: "pkg"}, Kind: "call", InWorkspace: true},
			},
			"A": {
				{Target: SourceSymbol{Name: "C", Package: "pkg"}, Kind: "call", InWorkspace: true},
			},
		},
		latency: 200 * time.Millisecond,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	p := &SymbolPipeline{Source: src, Root: "/workspace"}
	start := time.Now()
	_, err := p.CallGraph(ctx, "/workspace", CallGraphOpts{Depth: 10})
	elapsed := time.Since(start)

	// Should return quickly (within ~200ms, not hang forever).
	if elapsed > 500*time.Millisecond {
		t.Errorf("took %v, expected early return on context cancel", elapsed)
	}
	// Either returns an error or partial results — both acceptable.
	_ = err
}

// TestSymbolPipeline_CallGraph_TypeEnrichment tests that Hover is called
// to populate ParamTypes and ReturnTypes on call edges.
func TestSymbolPipeline_CallGraph_TypeEnrichment(t *testing.T) {
	src := &mockSymbolSource{
		roots: []SourceSymbol{
			{Name: "Handler", Package: "http", File: "handler.go", Line: 1},
		},
		children: map[string][]SourceRelation{
			"Handler": {
				{Target: SourceSymbol{Name: "Query", Package: "db", File: "query.go", Line: 10}, Kind: "call", InWorkspace: true},
			},
		},
		hovers: map[string]*SourceTypeInfo{
			"Query": {ParamTypes: []string{"string", "[]any"}, ReturnTypes: []string{"*sql.Rows", "error"}},
		},
	}

	p := &SymbolPipeline{Source: src, Root: "/workspace"}
	cg, err := p.CallGraph(context.Background(), "/workspace", CallGraphOpts{Depth: 5})
	if err != nil {
		t.Fatalf("CallGraph: %v", err)
	}

	if len(cg.Edges) != 1 {
		t.Fatalf("edges = %d, want 1", len(cg.Edges))
	}

	edge := cg.Edges[0]
	if len(edge.ParamTypes) == 0 {
		t.Error("edge.ParamTypes is empty, want [string, []any]")
	}
	if len(edge.ReturnTypes) == 0 {
		t.Error("edge.ReturnTypes is empty, want [*sql.Rows, error]")
	}
}

// TestSymbolPipeline_CallGraph_CrossPkg tests that cross-package edges are marked.
func TestSymbolPipeline_CallGraph_CrossPkg(t *testing.T) {
	src := &mockSymbolSource{
		roots: []SourceSymbol{
			{Name: "Start", Package: "cmd", File: "main.go", Line: 1},
		},
		children: map[string][]SourceRelation{
			"Start": {
				{Target: SourceSymbol{Name: "Same", Package: "cmd", File: "same.go", Line: 5}, Kind: "call", InWorkspace: true},
				{Target: SourceSymbol{Name: "Other", Package: "lib", File: "other.go", Line: 5}, Kind: "call", InWorkspace: true},
			},
		},
	}

	p := &SymbolPipeline{Source: src, Root: "/workspace"}
	cg, err := p.CallGraph(context.Background(), "/workspace", CallGraphOpts{Depth: 5})
	if err != nil {
		t.Fatalf("CallGraph: %v", err)
	}

	var crossPkg, samePkg int
	for _, e := range cg.Edges {
		if e.CrossPkg {
			crossPkg++
		} else {
			samePkg++
		}
	}
	if crossPkg != 1 {
		t.Errorf("cross-pkg edges = %d, want 1", crossPkg)
	}
	if samePkg != 1 {
		t.Errorf("same-pkg edges = %d, want 1", samePkg)
	}
}

// TestSymbolPipeline_CallGraph_ExternalSkipped tests that non-workspace
// callees produce edges but don't recurse.
func TestSymbolPipeline_CallGraph_ExternalSkipped(t *testing.T) {
	src := &mockSymbolSource{
		roots: []SourceSymbol{
			{Name: "Main", Package: "cmd", File: "main.go", Line: 1},
		},
		children: map[string][]SourceRelation{
			"Main": {
				{Target: SourceSymbol{Name: "Println", Package: "fmt"}, Kind: "call", InWorkspace: false},
				{Target: SourceSymbol{Name: "Helper", Package: "pkg", File: "h.go", Line: 1}, Kind: "call", InWorkspace: true},
			},
			// Println has children, but since it's external they should NOT be walked.
			"Println": {
				{Target: SourceSymbol{Name: "Fprintln", Package: "fmt"}, Kind: "call", InWorkspace: false},
			},
		},
	}

	p := &SymbolPipeline{Source: src, Root: "/workspace"}
	cg, err := p.CallGraph(context.Background(), "/workspace", CallGraphOpts{Depth: 10})
	if err != nil {
		t.Fatalf("CallGraph: %v", err)
	}

	// Edge to Println exists, but Fprintln should NOT appear.
	if len(cg.Edges) != 2 {
		t.Errorf("edges = %d, want 2 (Main→Println, Main→Helper)", len(cg.Edges))
	}
	for _, n := range cg.Nodes {
		if n.Name == "Fprintln" {
			t.Error("Fprintln should not appear — external callees are not recursed")
		}
	}
}

// TestSymbolPipeline_CallGraph_EntryOpt tests that Entry option limits to a single root.
func TestSymbolPipeline_CallGraph_EntryOpt(t *testing.T) {
	src := &mockSymbolSource{
		roots: []SourceSymbol{
			{Name: "Alpha", Package: "pkg", File: "a.go", Line: 1},
			{Name: "Beta", Package: "pkg", File: "b.go", Line: 1},
		},
		children: map[string][]SourceRelation{
			"Alpha": {{Target: SourceSymbol{Name: "X", Package: "pkg"}, Kind: "call", InWorkspace: true}},
			"Beta":  {{Target: SourceSymbol{Name: "Y", Package: "pkg"}, Kind: "call", InWorkspace: true}},
		},
	}

	p := &SymbolPipeline{Source: src, Root: "/workspace"}
	cg, err := p.CallGraph(context.Background(), "/workspace", CallGraphOpts{Entry: "Alpha", Depth: 5})
	if err != nil {
		t.Fatalf("CallGraph: %v", err)
	}

	// Only Alpha's subtree: Alpha → X. Beta and Y absent.
	for _, n := range cg.Nodes {
		if n.Name == "Beta" || n.Name == "Y" {
			t.Errorf("unexpected node %s — Entry=Alpha should exclude Beta's subtree", n.Name)
		}
	}
	if len(cg.Edges) != 1 {
		t.Errorf("edges = %d, want 1 (Alpha→X)", len(cg.Edges))
	}
}

// TestSymbolPipeline_DataFlowTrace_Basic tests basic data flow tracing.
func TestSymbolPipeline_DataFlowTrace_Basic(t *testing.T) {
	src := &mockSymbolSource{
		roots: []SourceSymbol{
			{Name: "HandleRequest", Package: "api", File: "handler.go", Line: 1},
		},
		children: map[string][]SourceRelation{
			"HandleRequest": {
				{Target: SourceSymbol{Name: "ValidateInput", Package: "api"}, Kind: "call", InWorkspace: true},
				{Target: SourceSymbol{Name: "QueryDB", Package: "db"}, Kind: "data_store", InWorkspace: true},
			},
			"ValidateInput": {
				{Target: SourceSymbol{Name: "ParseJSON", Package: "util"}, Kind: "call", InWorkspace: true},
			},
		},
	}

	p := &SymbolPipeline{Source: src, Root: "/workspace"}
	df, err := p.DataFlowTrace(context.Background(), "/workspace", "HandleRequest", 5)
	if err != nil {
		t.Fatalf("DataFlowTrace: %v", err)
	}

	if len(df.Nodes) == 0 {
		t.Error("expected data flow nodes")
	}
	if len(df.Edges) == 0 {
		t.Error("expected data flow edges")
	}

	// Check that data_store kind is detected.
	hasStore := false
	for _, n := range df.Nodes {
		if n.Kind == "data_store" {
			hasStore = true
			break
		}
	}
	if !hasStore {
		t.Error("expected at least one data_store node")
	}
}

// TestSymbolPipeline_CallGraph_Progress tests that OnProgress fires after each root.
func TestSymbolPipeline_CallGraph_Progress(t *testing.T) {
	src := &mockSymbolSource{
		roots: []SourceSymbol{
			{Name: "Alpha", Package: "pkg", File: "a.go", Line: 1},
			{Name: "Beta", Package: "pkg", File: "b.go", Line: 1},
			{Name: "Gamma", Package: "pkg", File: "c.go", Line: 1},
			{Name: "Delta", Package: "pkg", File: "d.go", Line: 1},
		},
		children: map[string][]SourceRelation{
			"Alpha": {{Target: SourceSymbol{Name: "A1", Package: "pkg"}, Kind: "call", InWorkspace: true}},
			"Beta":  {{Target: SourceSymbol{Name: "B1", Package: "pkg"}, Kind: "call", InWorkspace: true}},
			"Gamma": {{Target: SourceSymbol{Name: "C1", Package: "pkg"}, Kind: "call", InWorkspace: true}},
			"Delta": {{Target: SourceSymbol{Name: "D1", Package: "pkg"}, Kind: "call", InWorkspace: true}},
		},
		latency: 10 * time.Millisecond,
	}

	var updates []ProgressUpdate
	var mu sync.Mutex

	p := &SymbolPipeline{Source: src, Root: "/workspace", Concurrency: 2}
	cg, err := p.CallGraph(context.Background(), "/workspace", CallGraphOpts{
		Depth: 5,
		OnProgress: func(u ProgressUpdate) {
			mu.Lock()
			updates = append(updates, u)
			mu.Unlock()
		},
	})
	if err != nil {
		t.Fatalf("CallGraph: %v", err)
	}

	if len(cg.Edges) == 0 {
		t.Fatal("expected edges")
	}

	// Must receive at least 4 progress updates (one per root).
	if len(updates) < 4 {
		t.Fatalf("progress updates = %d, want >= 4", len(updates))
	}

	// Last update should have RootsResolved == RootsTotal.
	last := updates[len(updates)-1]
	if last.RootsResolved != last.RootsTotal {
		t.Errorf("last update: resolved=%d total=%d, want equal", last.RootsResolved, last.RootsTotal)
	}
	if last.RootsTotal != 4 {
		t.Errorf("total = %d, want 4", last.RootsTotal)
	}

	// EdgesFound should be > 0 in the last update.
	if last.EdgesFound == 0 {
		t.Error("last update EdgesFound = 0, want > 0")
	}

	t.Logf("received %d progress updates, last: resolved=%d/%d edges=%d nodes=%d",
		len(updates), last.RootsResolved, last.RootsTotal, last.EdgesFound, last.NodesFound)
}
