package engine

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/dpopsuev/oculus/v3/lang"
	"github.com/dpopsuev/oculus/v3/lsp"
)

// --- Polyglot E2E tests: scan → warm → probe across languages ---
//
// These tests run against real repos on the host. They skip gracefully
// when the repo or LSP server is unavailable.
//
// Run: go test ./engine/... -run TestPolyglot -count=1 -v -timeout 300s

func requireRepo(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); err != nil {
		t.Skipf("repo not found: %s", path)
	}
}

func requireLSP(t *testing.T, cmd string) {
	t.Helper()
	bin := cmd
	if i := len(bin); i > 0 {
		// Take first word (e.g. "gopls serve" → "gopls")
		for j, c := range bin {
			if c == ' ' {
				bin = bin[:j]
				break
			}
		}
	}
	if _, err := exec.LookPath(bin); err != nil {
		t.Skipf("LSP server not found: %s", cmd)
	}
}

func polyglotProbe(t *testing.T, repoPath, symbol string) {
	t.Helper()

	detected := lang.DetectLanguage(repoPath)
	server := lang.DefaultLSPServer(detected)
	t.Logf("repo=%s detected=%s server=%s", repoPath, detected, server)

	if server == "" {
		t.Skipf("no LSP server configured for %s", detected)
	}
	requireLSP(t, server)

	pool := lsp.NewPool()
	defer pool.Shutdown(context.Background())

	eng := New(&mockStore{headSHA: "test"}, []string{repoPath}, pool)

	// Warm
	if err := eng.WarmLSP(context.Background(), repoPath); err != nil {
		t.Skipf("WarmLSP failed (LSP server not functional): %v", err)
	}
	time.Sleep(3 * time.Second)

	// Probe
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	result, err := eng.ProbeSymbol(ctx, repoPath, symbol)
	if err != nil {
		t.Fatalf("ProbeSymbol(%s): %v", symbol, err)
	}
	if result == nil {
		t.Fatal("expected non-nil probe result")
	}
	t.Logf("probe: fqn=%s kind=%s fan_in=%d fan_out=%d", result.FQN, result.Kind, result.FanIn, result.FanOut)
}

// --- Test 1: Hegemony Rust ---

func TestPolyglot_Hegemony_Rust(t *testing.T) {
	repoPath := "/home/dpopsuev/Projects/hegemony"
	requireRepo(t, repoPath)
	requireRepo(t, filepath.Join(repoPath, "Cargo.toml"))
	polyglotProbe(t, repoPath, "Archon")
}

// --- Test 2: Hegemony TypeScript (admin frontend) ---

func TestPolyglot_Hegemony_TypeScript(t *testing.T) {
	repoPath := "/home/dpopsuev/Projects/hegemony/admin"
	requireRepo(t, repoPath)
	requireRepo(t, filepath.Join(repoPath, "package.json"))
	polyglotProbe(t, repoPath, "fetchHealth")
}

// --- Test 3: Origami Go ---

func TestPolyglot_Origami_Go(t *testing.T) {
	repoPath := "/home/dpopsuev/Workspace/origami"
	requireRepo(t, repoPath)
	requireRepo(t, filepath.Join(repoPath, "go.mod"))
	polyglotProbe(t, repoPath, "main")
}

// --- Test 4: Kilocode TypeScript (large, 716 components) ---

func TestPolyglot_Kilocode_TypeScript(t *testing.T) {
	repoPath := "/home/dpopsuev/Workspace/kilocode"
	requireRepo(t, repoPath)
	requireRepo(t, filepath.Join(repoPath, "package.json"))
	polyglotProbe(t, repoPath, "main")
}

// --- Test 6: Language detection correctness ---

func TestPolyglot_LanguageDetection(t *testing.T) {
	cases := []struct {
		name   string
		path   string
		expect lang.Language
	}{
		{"oculus_go", "/home/dpopsuev/Workspace/oculus", lang.Go},
		{"locus_go", "/home/dpopsuev/Workspace/locus", lang.Go},
		{"battery_go", "/home/dpopsuev/Workspace/battery", lang.Go},
		{"hegemony_rust", "/home/dpopsuev/Projects/hegemony", lang.Rust},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			requireRepo(t, tc.path)
			detected := lang.DetectLanguage(tc.path)
			if detected != tc.expect {
				t.Errorf("expected %s, got %s", tc.expect, detected)
			}
		})
	}
}

// --- Test 7: Python probe ---

func TestPolyglot_Python_Probe(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "pyproject.toml"), []byte("[project]\nname = \"polytest\"\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "main.py"), []byte(`
def process(data):
    return transform(data)

def transform(data):
    return data.upper()

def main():
    result = process("hello")
    print(result)
`), 0o644)
	exec.Command("git", "-C", dir, "init", "-q").Run()
	exec.Command("git", "-C", dir, "add", "-A").Run()
	exec.Command("git", "-C", dir, "commit", "-q", "-m", "init").Run()

	polyglotProbe(t, dir, "process")
}

// --- Test 8: C probe ---

func TestPolyglot_C_Probe(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "main.c"), []byte(`
#include <stdio.h>

int add(int a, int b) { return a + b; }
int multiply(int a, int b) { return a * b; }

int main() {
    int sum = add(1, 2);
    int product = multiply(3, 4);
    printf("%d %d\n", sum, product);
    return 0;
}
`), 0o644)
	os.WriteFile(filepath.Join(dir, "compile_commands.json"), []byte(`[{"directory":"`+dir+`","command":"cc -c main.c","file":"main.c"}]`), 0o644)
	exec.Command("git", "-C", dir, "init", "-q").Run()
	exec.Command("git", "-C", dir, "add", "-A").Run()
	exec.Command("git", "-C", dir, "commit", "-q", "-m", "init").Run()

	polyglotProbe(t, dir, "add")
}
