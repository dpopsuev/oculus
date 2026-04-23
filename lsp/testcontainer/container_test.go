//go:build integration

package testcontainer

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dpopsuev/oculus/v3/lang"
	"github.com/dpopsuev/oculus/v3/lsp"
)

type langTestConfig struct {
	name        string
	testkitDir  string        // directory name under testdata/testkit/
	lang        lang.Language
	indexWait   time.Duration
	queries     []string      // workspace/symbol queries to try (first non-empty wins)
	wantSymbols []string      // at least one of these must appear (bare or FQN)
	entry       string        // function to find for callHierarchy tests
}

var containerLangs = []langTestConfig{
	{"Go", "go", lang.Go, 8 * time.Second, []string{"."}, []string{"Entity", "Service"}, "main"},
	{"Python", "python", lang.Python, 5 * time.Second, []string{"", "Entity"}, []string{"Entity"}, "get_entity"},
	{"TypeScript", "typescript", lang.TypeScript, 5 * time.Second, []string{"", "Entity"}, []string{"Entity", "Service"}, "findById"},
	{"JavaScript", "javascript", lang.JavaScript, 5 * time.Second, []string{"", "Entity"}, []string{"Entity", "Service"}, "createRouter"},
	{"Rust", "rust", lang.Rust, 15 * time.Second, []string{"", "Entity"}, []string{"Entity", "Service"}, "get_entity"},
	{"C", "c", lang.C, 3 * time.Second, []string{"", "main", "entity"}, []string{"main"}, "main"},
	{"C++", "cpp", lang.Cpp, 3 * time.Second, []string{"", "main", "Entity"}, []string{"Entity", "main"}, "main"},
}

func TestContainerPool_Smoke(t *testing.T) {
	if err := Available(""); err != nil {
		t.Skipf("skipping: %v", err)
	}

	pool := NewPool("")
	defer pool.Shutdown(context.Background())

	client, err := pool.Get(lang.Go, t.TempDir())
	if err != nil {
		t.Fatalf("Get(Go): %v", err)
	}
	if client == nil {
		t.Fatal("Get(Go) returned nil client")
	}

	status := pool.Status()
	if status.Active != 1 {
		t.Errorf("expected 1 active connection, got %d", status.Active)
	}
	t.Logf("pool status: %+v", status)
}

// TestContainerPool_WorkspaceSymbol_EmptyQueryReturnsNull documents LCS-BUG-54 root cause:
// gopls returns null for workspace/symbol with empty query "".
// This behavior is permanent — it is NOT indexing delay. The fix is to
// use query "." instead.
func TestContainerPool_WorkspaceSymbol_EmptyQueryReturnsNull(t *testing.T) {
	if err := Available(""); err != nil {
		t.Skipf("skipping: %v", err)
	}

	workdir := copyTestkitLang(t, "go")

	pool := NewPool("")
	defer pool.Shutdown(context.Background())

	client, err := pool.Get(lang.Go, workdir)
	if err != nil {
		t.Fatalf("Get(Go): %v", err)
	}

	time.Sleep(8 * time.Second)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	result, err := client.RequestContext(ctx, "workspace/symbol", map[string]any{"query": ""})
	if err != nil {
		t.Fatalf("workspace/symbol request failed: %v", err)
	}
	if string(result) != "null" {
		t.Errorf("expected null for empty query, got: %s", truncate(string(result), 200))
	}
	t.Logf("empty query returns: %s (confirmed null — LCS-BUG-54 root cause)", string(result))

	result, err = client.RequestContext(ctx, "workspace/symbol", map[string]any{"query": "."})
	if err != nil {
		t.Fatalf("workspace/symbol with dot query: %v", err)
	}
	if result == nil || string(result) == "null" {
		t.Fatal("dot query also returned null — fix doesn't work")
	}
	var symbols []workspaceSymbolResult
	if err := json.Unmarshal(result, &symbols); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(symbols) == 0 {
		t.Fatal("dot query returned empty array")
	}
	t.Logf("dot query returned %d symbols (fix works)", len(symbols))
}

// TestContainerPool_WorkspaceSymbol_PerLanguage verifies that workspace/symbol
// returns results for each language's LSP server. Uses query "." (the BUG-54 fix).
func TestContainerPool_WorkspaceSymbol_PerLanguage(t *testing.T) {
	if err := Available(""); err != nil {
		t.Skipf("skipping: %v", err)
	}

	pool := NewPool("")
	defer pool.Shutdown(context.Background())

	for _, tc := range containerLangs {
		t.Run(tc.name, func(t *testing.T) {
			workdir := copyTestkitLang(t, tc.testkitDir)

			client, err := pool.Get(tc.lang, workdir)
			if err != nil {
				t.Fatalf("Get(%s): %v", tc.name, err)
			}
			defer pool.Release(tc.lang, workdir)

			// Some servers need didOpen before workspace/symbol works:
			// clangd indexes only opened files, tsserver needs a file to find the project.
			if tc.lang == lang.C || tc.lang == lang.Cpp || tc.lang == lang.TypeScript || tc.lang == lang.JavaScript {
				openSourceFiles(t, client, workdir)
			}

			time.Sleep(tc.indexWait)

			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			symbols := queryWorkspaceSymbols(t, client, ctx, tc.queries)

			if len(symbols) == 0 {
				t.Errorf("workspace/symbol returned no symbols (queries: %v)", tc.queries)
				return
			}

			t.Logf("workspace/symbol returned %d symbols", len(symbols))

			found := make(map[string]bool)
			for _, s := range symbols {
				found[s.Name] = true
				if dot := strings.LastIndex(s.Name, "."); dot >= 0 {
					found[s.Name[dot+1:]] = true
				}
			}
			for _, want := range tc.wantSymbols {
				if !found[want] {
					t.Errorf("missing expected symbol %q", want)
				}
			}
		})
	}
}

// TestContainerPool_CallHierarchy_PerLanguage verifies prepareCallHierarchy
// and outgoingCalls work for each language's LSP server.
func TestContainerPool_CallHierarchy_PerLanguage(t *testing.T) {
	if err := Available(""); err != nil {
		t.Skipf("skipping: %v", err)
	}

	pool := NewPool("")
	defer pool.Shutdown(context.Background())

	for _, tc := range containerLangs {
		t.Run(tc.name, func(t *testing.T) {
			workdir := copyTestkitLang(t, tc.testkitDir)

			client, err := pool.Get(tc.lang, workdir)
			if err != nil {
				t.Fatalf("Get(%s): %v", tc.name, err)
			}
			defer pool.Release(tc.lang, workdir)

			if tc.lang == lang.C || tc.lang == lang.Cpp || tc.lang == lang.TypeScript || tc.lang == lang.JavaScript {
				openSourceFiles(t, client, workdir)
			}

			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel()

			time.Sleep(tc.indexWait)

			// Find the entry function via workspace/symbol.
			var entryURI string
			var entryLine, entryChar int
			for attempt := range 10 {
				if attempt > 0 {
					time.Sleep(2 * time.Second)
				}

				result, err := client.RequestContext(ctx, "workspace/symbol", map[string]any{"query": tc.entry})
				if err != nil {
					t.Logf("workspace/symbol(%q): %v", tc.entry, err)
					continue
				}
				if result == nil || string(result) == "null" {
					continue
				}
				var symbols []workspaceSymbolResult
				_ = json.Unmarshal(result, &symbols)
				for _, s := range symbols {
					name := s.Name
					if dot := strings.LastIndex(name, "."); dot >= 0 {
						name = name[dot+1:]
					}
					if name == tc.entry {
						entryURI = s.Location.URI
						entryLine = s.Location.Range.Start.Line
						entryChar = s.Location.Range.Start.Character
						break
					}
				}
				if entryURI != "" {
					break
				}
			}

			if entryURI == "" {
				t.Fatalf("could not find %q via workspace/symbol", tc.entry)
			}

			t.Logf("found %s at %s:%d", tc.entry, entryURI, entryLine)

			prepResult, err := client.RequestContext(ctx, "textDocument/prepareCallHierarchy", map[string]any{
				"textDocument": map[string]any{"uri": entryURI},
				"position":     map[string]int{"line": entryLine, "character": entryChar},
			})
			if err != nil {
				t.Fatalf("prepareCallHierarchy: %v", err)
			}

			if prepResult == nil || string(prepResult) == "null" || string(prepResult) == "[]" {
				t.Logf("prepareCallHierarchy returned null/empty — %s may not support callHierarchy", tc.name)
				return
			}

			var items []json.RawMessage
			if err := json.Unmarshal(prepResult, &items); err != nil {
				t.Fatalf("unmarshal prepareCallHierarchy: %v", err)
			}
			if len(items) == 0 {
				t.Logf("prepareCallHierarchy returned empty array")
				return
			}

			t.Logf("prepareCallHierarchy: %d items", len(items))

			outResult, err := client.RequestContext(ctx, "callHierarchy/outgoingCalls", map[string]any{
				"item": json.RawMessage(items[0]),
			})
			if err != nil {
				t.Logf("outgoingCalls not supported by %s: %v", tc.name, err)
				return
			}

			var outgoing []json.RawMessage
			if outResult != nil && string(outResult) != "null" {
				_ = json.Unmarshal(outResult, &outgoing)
			}

			t.Logf("%s() has %d outgoing calls", tc.entry, len(outgoing))
			if len(outgoing) == 0 {
				t.Logf("0 outgoing calls — callHierarchy may not be fully supported by %s", tc.name)
			}
		})
	}
}

// TestContainerPool_WorkspaceSymbolNonEmpty tests if gopls needs
// a non-empty query (e.g. ".") to return workspace/symbol results.
func TestContainerPool_WorkspaceSymbolNonEmpty(t *testing.T) {
	if err := Available(""); err != nil {
		t.Skipf("skipping: %v", err)
	}

	workdir := copyTestkitLang(t, "go")

	pool := NewPool("")
	defer pool.Shutdown(context.Background())

	client, err := pool.Get(lang.Go, workdir)
	if err != nil {
		t.Fatalf("Get(Go): %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	time.Sleep(5 * time.Second)

	queries := []string{"", ".", "New", "Entity"}
	for _, q := range queries {
		result, err := client.RequestContext(ctx, "workspace/symbol", map[string]any{"query": q})
		if err != nil {
			t.Errorf("query=%q: request failed: %v", q, err)
			continue
		}

		isNull := result == nil || string(result) == "null"
		var symbols []workspaceSymbolResult
		count := 0
		if !isNull {
			_ = json.Unmarshal(result, &symbols)
			count = len(symbols)
		}

		t.Logf("query=%q: null=%v count=%d raw=%s", q, isNull, count, truncate(string(result), 200))
	}
}

// TestContainerPool_CallGraph does a full call graph walk with a real gopls.
func TestContainerPool_CallGraph(t *testing.T) {
	if err := Available(""); err != nil {
		t.Skipf("skipping: %v", err)
	}

	workdir := copyTestkitLang(t, "go")

	pool := NewPool("")
	defer pool.Shutdown(context.Background())

	client, err := pool.Get(lang.Go, workdir)
	if err != nil {
		t.Fatalf("Get(Go): %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	var mainURI string
	var mainLine int
	for attempt := range 30 {
		if attempt > 0 {
			time.Sleep(2 * time.Second)
		}

		result, err := client.RequestContext(ctx, "workspace/symbol", map[string]any{"query": "main"})
		if err != nil {
			t.Fatalf("workspace/symbol: %v", err)
		}
		if result == nil || string(result) == "null" {
			continue
		}
		var symbols []workspaceSymbolResult
		_ = json.Unmarshal(result, &symbols)
		for _, s := range symbols {
			if s.Name == "main" && s.Kind == 12 {
				mainURI = s.Location.URI
				mainLine = s.Location.Range.Start.Line
				break
			}
		}
		if mainURI != "" {
			break
		}
	}

	if mainURI == "" {
		t.Fatal("could not find main function via workspace/symbol")
	}

	t.Logf("found main at %s:%d", mainURI, mainLine)

	prepResult, err := client.RequestContext(ctx, "textDocument/prepareCallHierarchy", map[string]any{
		"textDocument": map[string]any{"uri": mainURI},
		"position":     map[string]int{"line": mainLine, "character": 5},
	})
	if err != nil {
		t.Fatalf("prepareCallHierarchy: %v", err)
	}

	t.Logf("prepareCallHierarchy raw: %s", truncate(string(prepResult), 500))

	if prepResult == nil || string(prepResult) == "null" || string(prepResult) == "[]" {
		t.Fatal("prepareCallHierarchy returned null/empty for main")
	}

	var items []json.RawMessage
	if err := json.Unmarshal(prepResult, &items); err != nil {
		t.Fatalf("unmarshal prepareCallHierarchy: %v", err)
	}
	if len(items) == 0 {
		t.Fatal("prepareCallHierarchy returned empty array")
	}

	outResult, err := client.RequestContext(ctx, "callHierarchy/outgoingCalls", map[string]any{
		"item": json.RawMessage(items[0]),
	})
	if err != nil {
		t.Fatalf("outgoingCalls: %v", err)
	}

	t.Logf("outgoingCalls raw: %s", truncate(string(outResult), 500))

	var outgoing []json.RawMessage
	if outResult != nil && string(outResult) != "null" {
		_ = json.Unmarshal(outResult, &outgoing)
	}

	t.Logf("main() has %d outgoing calls", len(outgoing))
	if len(outgoing) == 0 {
		t.Error("expected outgoing calls from main() — callHierarchy may not be working")
	}
}

// queryWorkspaceSymbols tries each query in order, retrying up to 10 times.
// Returns the first non-empty result. Different LSP servers need different
// queries: gopls needs ".", pyright/tsserver need "", clangd needs symbol names.
func queryWorkspaceSymbols(t *testing.T, client *lsp.Client, ctx context.Context, queries []string) []workspaceSymbolResult {
	t.Helper()
	for _, q := range queries {
		for attempt := range 10 {
			if attempt > 0 {
				time.Sleep(2 * time.Second)
			}

			result, err := client.RequestContext(ctx, "workspace/symbol", map[string]any{"query": q})
			if err != nil {
				t.Logf("query=%q attempt=%d: %v", q, attempt+1, err)
				break
			}

			if result == nil || string(result) == "null" {
				t.Logf("query=%q attempt=%d: null", q, attempt+1)
				continue
			}

			var symbols []workspaceSymbolResult
			if err := json.Unmarshal(result, &symbols); err != nil {
				t.Logf("query=%q attempt=%d: unmarshal error: %v", q, attempt+1, err)
				break
			}

			if len(symbols) > 0 {
				t.Logf("query=%q: %d symbols (attempt %d)", q, len(symbols), attempt+1)
				return symbols
			}
			t.Logf("query=%q attempt=%d: empty array", q, attempt+1)
		}
	}
	return nil
}

type workspaceSymbolResult struct {
	Name     string `json:"name"`
	Kind     int    `json:"kind"`
	Location struct {
		URI   string `json:"uri"`
		Range struct {
			Start struct {
				Line      int `json:"line"`
				Character int `json:"character"`
			} `json:"start"`
		} `json:"range"`
	} `json:"location"`
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// copyTestkitLang copies testdata/testkit/<lang> to a temp directory.
// Uses manual cleanup because Docker containers write root-owned files
// that t.TempDir() can't remove.
func copyTestkitLang(t *testing.T, language string) string {
	t.Helper()

	src := filepath.Join(findRepoRoot(t), "testdata", "testkit", language)
	if _, err := os.Stat(src); err != nil {
		t.Fatalf("testkit %q not found at %s", language, src)
	}

	dst, err := os.MkdirTemp("", "testkit-"+language+"-*")
	if err != nil {
		t.Fatalf("mkdirtemp: %v", err)
	}
	t.Cleanup(func() {
		// Container writes root-owned files; chmod everything before removing.
		_ = filepath.Walk(dst, func(path string, _ os.FileInfo, _ error) error {
			_ = os.Chmod(path, 0o777)
			return nil
		})
		os.RemoveAll(dst)
	})

	err = filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, path)
		target := filepath.Join(dst, rel)

		if info.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
	if err != nil {
		t.Fatalf("copy testkit/%s: %v", language, err)
	}

	// clangd needs compile_commands.json with absolute paths.
	if language == "c" || language == "cpp" {
		generateCompileCommands(t, dst, language)
	}

	t.Logf("testkit/%s copied to %s", language, dst)
	return dst
}

var extToLangID = map[string]string{
	".c": "c", ".h": "c",
	".cpp": "cpp", ".cc": "cpp", ".cxx": "cpp", ".hpp": "cpp",
	".ts": "typescript", ".tsx": "typescriptreact",
	".js": "javascript", ".jsx": "javascriptreact",
	".py": "python",
	".rs": "rust",
	".go": "go",
}

// openSourceFiles sends textDocument/didOpen for all source files in the workspace.
// clangd requires this — it only indexes files that have been opened.
func openSourceFiles(t *testing.T, client *lsp.Client, dir string) {
	t.Helper()
	_ = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		ext := filepath.Ext(path)
		langID, ok := extToLangID[ext]
		if !ok {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		_ = client.Notify("textDocument/didOpen", map[string]any{
			"textDocument": map[string]any{
				"uri":        "file://" + path,
				"languageId": langID,
				"version":    1,
				"text":       string(data),
			},
		})
		return nil
	})
}

func generateCompileCommands(t *testing.T, dir, language string) {
	t.Helper()
	var entries []string
	srcDir := filepath.Join(dir, "src")
	files, _ := os.ReadDir(srcDir)
	for _, f := range files {
		if f.IsDir() {
			continue
		}
		name := f.Name()
		compiler := "gcc"
		if language == "cpp" {
			compiler = "g++ -std=c++17"
		}
		entries = append(entries, fmt.Sprintf(
			`{"directory": %q, "command": "%s -I%s/include -c src/%s", "file": "src/%s"}`,
			dir, compiler, dir, name, name,
		))
	}
	content := "[\n  " + strings.Join(entries, ",\n  ") + "\n]\n"
	if err := os.WriteFile(filepath.Join(dir, "compile_commands.json"), []byte(content), 0o644); err != nil {
		t.Fatalf("write compile_commands.json: %v", err)
	}
}

func findRepoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not find repo root (no go.mod found)")
		}
		dir = parent
	}
}
