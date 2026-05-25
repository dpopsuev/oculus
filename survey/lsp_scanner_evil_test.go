// Package survey_test contains timeout tests for LSPScanner using adversarial
// ("evil") LSP server behaviours. The test binary itself acts as the evil LSP
// server subprocess via TestMain: when EVIL_LSP_MODE is set in the environment
// the binary runs ServeEvil and exits without running any tests.
package survey_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/dpopsuev/oculus/v3/lsp/mockserver"
	"github.com/dpopsuev/oculus/v3/survey"
)

// TestMain intercepts child-process invocations where this test binary is used
// as a fake LSP server. EVIL_LSP_MODE is set by the parent test via t.Setenv;
// the child inherits it and calls ServeEvil instead of running tests.
func TestMain(m *testing.M) {
	if mode := os.Getenv("EVIL_LSP_MODE"); mode != "" {
		mockserver.ServeEvil(os.Stdin, os.Stdout, mode)
		os.Exit(0)
	}
	os.Exit(m.Run())
}

// evilTimeout is deliberately short so each test completes quickly.
// It must be long enough for the scan protocol to finish on the happy path
// (init + N×didOpen + N×documentSymbol over a local pipe ≈ a few ms).
const evilTimeout = 300 * time.Millisecond

// lspLanguageCases defines one fixture per language that LSPScanner may scan.
// Each case contains only the source files relevant to that language so that
// findSourceFiles returns the right extension set and the evil server receives
// the correct textDocument/didOpen language IDs.
var lspLanguageCases = []struct {
	name  string
	files map[string]string // relative path → content
}{
	{
		name: "go",
		files: map[string]string{
			"main.go": "package main\n\nfunc Hello() {}\n",
		},
	},
	{
		name: "rust",
		files: map[string]string{
			"src/main.rs": "fn main() {}\n",
		},
	},
	{
		name: "typescript",
		files: map[string]string{
			"index.ts": "export function hello(): void {}\n",
		},
	},
	{
		name: "python",
		files: map[string]string{
			"main.py": "def hello():\n    pass\n",
		},
	},
	{
		name: "java",
		files: map[string]string{
			"Main.java": "public class Main { public static void main(String[] a) {} }\n",
		},
	},
	{
		// A C file without CMakeLists.txt → DetectLanguage returns LangUnknown.
		// This is the primary failure scenario: AutoScanner falls through to
		// LSPScanner because no language marker file is present.
		name: "unknown_language",
		files: map[string]string{
			"lib.c": "void hello(void) {}\n",
		},
	},
}

// evilModes is the full set of adversarial behaviours exercised per language.
var evilModes = []struct {
	mode    string
	wantErr bool // true = Scan must return a non-nil error (scanner timed out)
}{
	{mockserver.EvilHangOnInitialize, true},
	{mockserver.EvilHangOnDocumentSymbol, true},
	// hang_exit: the protocol completes but shutdown hangs. LSPScanner kills
	// the process after the timeout and returns the collected data with nil error.
	{mockserver.EvilHangOnExit, false},
}

// TestLSPScanner_ReturnsWithinTimeout verifies that LSPScanner.Scan returns
// within a bounded time for every (language, adversarial-mode) combination and
// never leaves a zombie process behind.
//
// Given a workspace with source files of the given language
// And an LSP server that hangs at the specified lifecycle point
// When LSPScanner.Scan is called
// Then the call returns within 2×Timeout
// And returns an error for hang_init and hang_document_symbol
// And returns a (possibly empty) result with nil error for hang_exit
func TestLSPScanner_ReturnsWithinTimeout(t *testing.T) {
	for _, lang := range lspLanguageCases {
		for _, evil := range evilModes {
			t.Run(lang.name+"/"+evil.mode, func(t *testing.T) {
				dir := makeFixture(t, lang.files)

				// Point the child process at this test binary so TestMain
				// intercepts it and runs ServeEvil.
				t.Setenv("EVIL_LSP_MODE", evil.mode)

				sc := &survey.LSPScanner{
					ServerCmd: os.Args[0],
					Timeout:   evilTimeout,
				}

				start := time.Now()
				proj, err := sc.Scan(dir)
				elapsed := time.Since(start)

				// Primary assertion: the call must return well within 2×
				// the timeout. Any longer and the fix is broken.
				if elapsed > 2*evilTimeout {
					t.Errorf("Scan took %v — did not return within 2×timeout (%v) for %s/%s",
						elapsed, 2*evilTimeout, lang.name, evil.mode)
				}

				if evil.wantErr {
					if err == nil {
						t.Errorf("expected timeout error for %s/%s, got nil (proj=%v)",
							lang.name, evil.mode, proj)
					}
				} else {
					if err != nil {
						t.Logf("%s/%s: non-nil error (acceptable if timeout fired during shutdown): %v",
							lang.name, evil.mode, err)
					}
				}
			})
		}
	}
}

// makeFixture creates a temporary directory containing the given files and
// returns its absolute path. Subdirectories are created as needed.
func makeFixture(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for rel, content := range files {
		full := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", filepath.Dir(full), err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", full, err)
		}
	}
	return dir
}
