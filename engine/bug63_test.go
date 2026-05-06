package engine

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// TestBug63_SilentEmptyCallGraph reproduces LCS-BUG-63:
// When gopls is unavailable, callers returns an explicit error but
// scenario/convergence/callees silently return empty results.
//
// Root cause: GetSymbolGraph builds a partial graph (types from TreeSitter,
// no call edges) and caches it. Primitives return empty data on the
// partial graph without indicating that the call graph is incomplete.
func TestBug63_SilentEmptyCallGraph(t *testing.T) {
	dir := t.TempDir()
	files := map[string]string{
		"go.mod": "module bug63test\ngo 1.21\n",
		"main.go": `package main

func main() {
	result := process("input")
	_ = format(result)
}

func process(s string) string { return transform(s) }
func transform(s string) string { return s + "_done" }
func format(s string) string { return "[" + s + "]" }
`,
	}
	for name, content := range files {
		p := filepath.Join(dir, name)
		os.MkdirAll(filepath.Dir(p), 0o755)
		os.WriteFile(p, []byte(content), 0o644)
	}
	exec.Command("git", "-C", dir, "init", "-q").Run()
	exec.Command("git", "-C", dir, "add", "-A").Run()
	exec.Command("git", "-C", dir, "commit", "-q", "-m", "init").Run()

	// No LSP pool — forces call graph failure
	eng := New(&mockStore{headSHA: "test"}, []string{dir})

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// callers should error explicitly
	_, errCallers := eng.GetCallers(ctx, dir, "process")

	// scenario should ALSO error, not return empty
	resultScenario, errScenario := eng.GetScenario(ctx, dir, "process", 5, false)

	// convergence should ALSO error, not return empty
	resultConvergence, errConvergence := eng.GetConvergence(ctx, dir, []string{"process", "format"})

	// callees should ALSO error, not return empty
	_, errCallees := eng.GetCallees(ctx, dir, "main")

	t.Logf("callers err: %v", errCallers)
	t.Logf("scenario err: %v, result nil: %v", errScenario, resultScenario == nil)
	t.Logf("convergence err: %v, result nil: %v", errConvergence, resultConvergence == nil)
	t.Logf("callees err: %v", errCallees)

	// BUG-63: These should all error consistently
	if errCallers == nil {
		t.Error("expected callers to error without LSP")
	}
	if errScenario == nil && resultScenario != nil && resultScenario.Upstream == nil && resultScenario.Downstream == nil {
		t.Error("BUG-63: scenario returned empty result without error — silent failure")
	}
	if errConvergence == nil && resultConvergence != nil && resultConvergence.Nodes == nil {
		t.Error("BUG-63: convergence returned empty result without error — silent failure")
	}
	if errCallees == nil {
		t.Error("expected callees to error without LSP")
	}
}

// TestBug63_CachedEmptyGraph reproduces the cache path:
// GetSymbolGraph builds a graph with types but no call edges,
// caches it, then primitives query the cached empty graph.
func TestBug63_CachedEmptyGraph(t *testing.T) {
	dir := t.TempDir()
	files := map[string]string{
		"go.mod": "module bug63cache\ngo 1.21\n",
		"main.go": `package main

func main() { process("x") }
func process(s string) string { return s }
`,
	}
	for name, content := range files {
		p := filepath.Join(dir, name)
		os.MkdirAll(filepath.Dir(p), 0o755)
		os.WriteFile(p, []byte(content), 0o644)
	}
	exec.Command("git", "-C", dir, "init", "-q").Run()
	exec.Command("git", "-C", dir, "add", "-A").Run()
	exec.Command("git", "-C", dir, "commit", "-q", "-m", "init").Run()

	// Use a store that returns a real SHA so the symbol graph gets cached
	store := &mockStore{headSHA: "abc123"}
	eng := New(store, []string{dir}) // no pool = no LSP

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// Phase 1: Build symbol graph — this should fail or produce an incomplete graph
	sg, errSG := eng.GetSymbolGraph(ctx, dir)
	t.Logf("GetSymbolGraph: err=%v, nodes=%d, edges=%d", errSG, 0, 0)
	if errSG != nil {
		t.Logf("GetSymbolGraph errored (correct): %v", errSG)
	}
	if sg != nil {
		t.Logf("GetSymbolGraph returned graph: nodes=%d edges=%d", len(sg.Nodes), len(sg.Edges))
	}

	// Phase 2: Call scenario — if graph was cached empty, this returns null silently
	resultScenario, errScenario := eng.GetScenario(ctx, dir, "process", 5, false)
	t.Logf("scenario: err=%v, result nil=%v", errScenario, resultScenario == nil)

	if errScenario == nil && resultScenario != nil && resultScenario.Upstream == nil && resultScenario.Downstream == nil {
		// Check if edges exist — if no call edges, this is the silent failure
		if sg != nil && len(sg.Edges) == 0 {
			t.Error("BUG-63: scenario returned empty on cached graph with 0 call edges — silent failure")
		}
	}
}
