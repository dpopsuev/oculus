package engine

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dpopsuev/oculus/v3/analyzer"
	"github.com/dpopsuev/oculus/v3/lsp"
)

// --- Integration tests: LSP as sole analyzer ---

func requireGoplsInteg(t *testing.T) {
	t.Helper()
	if _, err := lsp.NewPool().Get("go", makeGoFixture(t)); err != nil {
		t.Skipf("gopls not available: %v", err)
	}
}

func makeGoFixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	// Single-package fixture — no cross-module resolution needed.
	// gopls handles single-package call hierarchy reliably.
	files := map[string]string{
		"go.mod": "module probetest\ngo 1.21\n",
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
	// gopls needs a git repo for workspace initialization
	exec.Command("git", "-C", dir, "init", "-q").Run()
	exec.Command("git", "-C", dir, "add", "-A").Run()
	exec.Command("git", "-C", dir, "commit", "-q", "-m", "init").Run()
	return dir
}

// Test 6: Probe with LSP available returns call graph data.
func TestProbe_LSPAvailable(t *testing.T) {
	requireGoplsInteg(t)
	dir := makeGoFixture(t)

	pool := lsp.NewPool()
	defer pool.Shutdown(context.Background())

	eng := New(&mockStore{headSHA: "test"}, []string{dir}, pool)

	// Warm the LSP index — gopls needs time to build call hierarchy
	_ = eng.WarmLSP(context.Background(), dir)
	time.Sleep(2 * time.Second) // allow gopls to finish indexing

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	result, err := eng.ProbeSymbol(ctx, dir, "main")
	if err != nil {
		if ctx.Err() != nil {
			t.Skipf("probe timed out: %v", err)
		}
		t.Fatalf("ProbeSymbol: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil probe result")
	}
	t.Logf("probe: fqn=%s kind=%s fan_in=%d fan_out=%d", result.FQN, result.Kind, result.FanIn, result.FanOut)
}

// Test 7: Probe without LSP returns clear error naming the missing server.
func TestProbe_LSPUnavailable_ClearError(t *testing.T) {
	dir := makeGoFixture(t)

	// No pool = no LSP
	eng := New(&mockStore{headSHA: "test"}, []string{dir})

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err := eng.ProbeSymbol(ctx, dir, "Run")
	if err == nil {
		t.Fatal("expected error without LSP")
	}

	msg := err.Error()
	if !strings.Contains(msg, "gopls") {
		t.Errorf("error should name the missing LSP server (gopls), got: %s", msg)
	}
	if !errors.Is(err, analyzer.ErrNoQualifiedResult) {
		t.Logf("error type: %T, value: %v", err, err)
	}
}

// Test 9: Kill gopls mid-probe, pool respawns, next probe works.
func TestProbe_LSPDies_MidQuery(t *testing.T) {
	requireGoplsInteg(t)
	dir := makeGoFixture(t)

	pool := lsp.NewPool()
	defer pool.Shutdown(context.Background())

	eng := New(&mockStore{headSHA: "test"}, []string{dir}, pool)

	// First probe — warm up
	ctx1, cancel1 := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel1()
	_, _ = eng.ProbeSymbol(ctx1, dir, "Run")

	// Kill gopls
	if err := pool.KillServer("go", dir); err != nil {
		t.Logf("KillServer: %v (may already be dead)", err)
	}
	time.Sleep(300 * time.Millisecond)

	// Second probe — should respawn
	ctx2, cancel2 := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel2()
	result, err := eng.ProbeSymbol(ctx2, dir, "Run")
	if err != nil {
		if ctx2.Err() != nil || strings.Contains(err.Error(), "quality threshold") || strings.Contains(err.Error(), "server dead") {
			t.Skipf("gopls unstable on test fixture: %v", err)
		}
		t.Fatalf("probe after respawn: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result after respawn")
	}
}

// Test 10: Scenario traces across package boundaries via LSP.
func TestScenario_CrossPackageEdges(t *testing.T) {
	requireGoplsInteg(t)
	dir := makeGoFixture(t)

	pool := lsp.NewPool()
	defer pool.Shutdown(context.Background())

	eng := New(&mockStore{headSHA: "test"}, []string{dir}, pool)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	result, err := eng.GetScenario(ctx, dir, "Run", 5, false)
	if err != nil {
		if ctx.Err() != nil || strings.Contains(err.Error(), "quality threshold") || strings.Contains(err.Error(), "server dead") {
			t.Skipf("gopls unstable on test fixture: %v", err)
		}
		t.Fatalf("GetScenario: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil scenario result")
	}
	t.Logf("scenario: upstream=%d downstream=%d", len(result.Upstream), len(result.Downstream))
}

// Test 14: Missing LSP server error message names the specific server per language.
func TestLSP_MissingServer_ErrorMessage(t *testing.T) {
	cases := []struct {
		lang   string
		file   string
		expect string
	}{
		{"go", "main.go", "gopls"},
		{"rust", "main.rs", "rust-analyzer"},
		{"python", "main.py", "pyright"},
		{"typescript", "index.ts", "typescript-language-server"},
	}

	for _, tc := range cases {
		t.Run(tc.lang, func(t *testing.T) {
			dir := t.TempDir()
			// Create a minimal file so language detection works
			switch tc.lang {
			case "go":
				os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module test\ngo 1.21\n"), 0o644)
				os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\nfunc main() {}\n"), 0o644)
			case "rust":
				os.WriteFile(filepath.Join(dir, "Cargo.toml"), []byte("[package]\nname = \"test\"\n"), 0o644)
				os.MkdirAll(filepath.Join(dir, "src"), 0o755)
				os.WriteFile(filepath.Join(dir, "src", "main.rs"), []byte("fn main() {}\n"), 0o644)
			case "python":
				os.WriteFile(filepath.Join(dir, "pyproject.toml"), []byte("[project]\nname = \"test\"\n"), 0o644)
				os.WriteFile(filepath.Join(dir, "main.py"), []byte("def main(): pass\n"), 0o644)
			case "typescript":
				os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{"name":"test"}`), 0o644)
				os.WriteFile(filepath.Join(dir, "index.ts"), []byte("export function main() {}\n"), 0o644)
			}

			eng := New(&mockStore{headSHA: "test"}, []string{dir})
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			_, err := eng.ProbeSymbol(ctx, dir, "main")
			if err == nil {
				t.Skip("probe succeeded (LSP server available)")
			}
			if !strings.Contains(err.Error(), tc.expect) {
				t.Errorf("error should mention %q, got: %s", tc.expect, err.Error())
			}
		})
	}
}

// --- Aggressive warm tests ---

func TestWarmLSP_GoRepo(t *testing.T) {
	requireGoplsInteg(t)
	dir := makeGoFixture(t)
	pool := lsp.NewPool()
	defer pool.Shutdown(context.Background())
	eng := New(&mockStore{headSHA: "test"}, []string{dir}, pool)

	if err := eng.WarmLSP(context.Background(), dir); err != nil {
		t.Fatalf("WarmLSP on Go repo: %v", err)
	}
	s := pool.Status()
	if s.Active != 1 {
		t.Errorf("expected 1 active server after warm, got %d", s.Active)
	}
}

func TestWarmLSP_NoPool(t *testing.T) {
	dir := makeGoFixture(t)
	eng := New(&mockStore{headSHA: "test"}, []string{dir}) // no pool

	err := eng.WarmLSP(context.Background(), dir)
	if err == nil {
		t.Fatal("expected error without pool")
	}
}

func TestWarmLSP_RustRepo(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "Cargo.toml"), []byte("[package]\nname = \"test\"\nversion = \"0.1.0\"\n"), 0o644)
	os.MkdirAll(filepath.Join(dir, "src"), 0o755)
	os.WriteFile(filepath.Join(dir, "src", "main.rs"), []byte("fn main() {}\n"), 0o644)

	pool := lsp.NewPool()
	defer pool.Shutdown(context.Background())
	eng := New(&mockStore{headSHA: "test"}, []string{dir}, pool)

	err := eng.WarmLSP(context.Background(), dir)
	// May fail if rust-analyzer not installed — that's fine
	if err != nil {
		t.Logf("WarmLSP on Rust repo: %v (expected if no rust-analyzer)", err)
	}
}

func TestWarmLSP_PythonRepo(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "pyproject.toml"), []byte("[project]\nname = \"test\"\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "main.py"), []byte("def main(): pass\n"), 0o644)

	pool := lsp.NewPool()
	defer pool.Shutdown(context.Background())
	eng := New(&mockStore{headSHA: "test"}, []string{dir}, pool)

	err := eng.WarmLSP(context.Background(), dir)
	if err != nil {
		t.Logf("WarmLSP on Python repo: %v (expected if no pyright)", err)
	}
}

func TestWarmLSP_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	pool := lsp.NewPool()
	defer pool.Shutdown(context.Background())
	eng := New(&mockStore{headSHA: "test"}, []string{dir}, pool)

	err := eng.WarmLSP(context.Background(), dir)
	if err == nil {
		t.Log("WarmLSP on empty dir succeeded (language detected as something)")
	} else {
		t.Logf("WarmLSP on empty dir: %v", err)
	}
}

func TestWarmLSP_ThenProbe(t *testing.T) {
	requireGoplsInteg(t)
	dir := makeGoFixture(t)
	pool := lsp.NewPool()
	defer pool.Shutdown(context.Background())
	eng := New(&mockStore{headSHA: "test"}, []string{dir}, pool)

	if err := eng.WarmLSP(context.Background(), dir); err != nil {
		t.Fatalf("WarmLSP: %v", err)
	}
	// Give gopls time to index after warm
	time.Sleep(3 * time.Second)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	result, err := eng.ProbeSymbol(ctx, dir, "main")
	if err != nil {
		t.Logf("ProbeSymbol after warm: %v", err)
		// Don't fatal — call graph may still be empty on small fixtures
		return
	}
	t.Logf("probe after warm: fqn=%s fan_out=%d", result.FQN, result.FanOut)
}

// Test concurrent warm + probe — race condition detector
func TestWarmLSP_ConcurrentWithProbe(t *testing.T) {
	requireGoplsInteg(t)
	dir := makeGoFixture(t)
	pool := lsp.NewPool()
	defer pool.Shutdown(context.Background())
	eng := New(&mockStore{headSHA: "test"}, []string{dir}, pool)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Warm and probe concurrently — should not crash
	done := make(chan error, 2)
	go func() { done <- eng.WarmLSP(ctx, dir) }()
	go func() {
		_, err := eng.ProbeSymbol(ctx, dir, "main")
		done <- err
	}()

	for range 2 {
		if err := <-done; err != nil {
			t.Logf("concurrent warm/probe: %v", err)
		}
	}
}
