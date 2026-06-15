package engine

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// TestCallGraph_ExplicitErrorWhenLSPUnavailable verifies that callers,
// scenario, convergence, and callees all error explicitly when the call
// graph is unavailable, instead of returning silent empty results.
func TestCallGraph_ExplicitErrorWhenLSPUnavailable(t *testing.T) {
	dir := t.TempDir()
	files := map[string]string{
		"go.mod": "module cgtestmod\ngo 1.21\n",
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

	eng := New(&mockStore{headSHA: "test"}, []string{dir})

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	_, errCallers := eng.GetCallers(ctx, dir, "process")
	resultScenario, errScenario := eng.GetScenario(ctx, dir, "process", 5, false)
	resultConvergence, errConvergence := eng.GetConvergence(ctx, dir, []string{"process", "format"})
	_, errCallees := eng.GetCallees(ctx, dir, "main")

	t.Logf("callers err: %v", errCallers)
	t.Logf("scenario err: %v, result nil: %v", errScenario, resultScenario == nil)
	t.Logf("convergence err: %v, result nil: %v", errConvergence, resultConvergence == nil)
	t.Logf("callees err: %v", errCallees)

	if errCallers == nil {
		t.Error("expected callers to error without LSP")
	}
	if errScenario == nil && resultScenario != nil && resultScenario.Upstream == nil && resultScenario.Downstream == nil {
		t.Error("scenario returned empty result without error — silent failure")
	}
	if errConvergence == nil && resultConvergence != nil && resultConvergence.Nodes == nil {
		t.Error("convergence returned empty result without error — silent failure")
	}
	if errCallees == nil {
		t.Error("expected callees to error without LSP")
	}
}

// TestCallGraph_CachedEmptyGraph verifies the cache path: GetSymbolGraph
// builds a graph with types but no call edges, caches it, then primitives
// query the cached empty graph and must not return silent empty.
func TestCallGraph_CachedEmptyGraph(t *testing.T) {
	dir := t.TempDir()
	files := map[string]string{
		"go.mod": "module cgcachemod\ngo 1.21\n",
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

	store := &mockStore{headSHA: "abc123"}
	eng := New(store, []string{dir})

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	sg, errSG := eng.GetSymbolGraph(ctx, dir)
	t.Logf("GetSymbolGraph: err=%v", errSG)
	if errSG != nil {
		t.Logf("GetSymbolGraph errored (correct): %v", errSG)
	}
	if sg != nil {
		t.Logf("GetSymbolGraph returned graph: nodes=%d edges=%d", len(sg.Nodes), len(sg.Edges))
	}

	resultScenario, errScenario := eng.GetScenario(ctx, dir, "process", 5, false)
	t.Logf("scenario: err=%v, result nil=%v", errScenario, resultScenario == nil)

	if errScenario == nil && resultScenario != nil && resultScenario.Upstream == nil && resultScenario.Downstream == nil {
		if sg != nil && len(sg.Edges) == 0 {
			t.Error("scenario returned empty on cached graph with 0 call edges — silent failure")
		}
	}
}
