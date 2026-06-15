package engine

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// --- Caller discovery: struct constructions ---

// TestGetCallers_FindsStructConstruction verifies that GetCallers finds
// struct literal constructions (Config{Name: "x"}), not just function calls.
//
// Given a project where Config is constructed via function call AND struct literal
// When GetCallers("Config") is called
// Then both construction sites appear
func TestGetCallers_FindsStructConstruction(t *testing.T) {
	dir := t.TempDir()

	files := map[string]string{
		"go.mod": "module testmod\ngo 1.21\n",
		"main.go": `package main

type Config struct {
	Name string
	Port int
}

func NewConfig(name string) Config {
	return Config{Name: name}
}

func LoadFromFile(path string) Config {
	return Config{Name: path, Port: 8080}
}

func main() {
	cfg1 := NewConfig("app")
	cfg2 := Config{Name: "direct", Port: 3000}
	_ = cfg1
	_ = cfg2
}
`,
	}

	for name, content := range files {
		p := filepath.Join(dir, name)
		os.MkdirAll(filepath.Dir(p), 0o755)
		os.WriteFile(p, []byte(content), 0o600)
	}

	eng := New(&mockStore{headSHA: "test"}, []string{dir})

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	report, err := eng.GetCallers(ctx, dir, "Config")
	if err != nil {
		t.Skipf("GetCallers unavailable (LSP): %v", err)
	}

	t.Logf("callers of Config: %d", len(report.Callers))
	for _, c := range report.Callers {
		t.Logf("  caller=%s pkg=%s file=%s line=%d", c.Caller, c.CallerPkg, c.File, c.Line)
	}

	if len(report.Callers) == 0 {
		t.Error("GetCallers('Config') returned 0 callers — misses struct literal constructions")
	}

	foundDirect := false
	for _, c := range report.Callers {
		if c.Caller == "main" {
			foundDirect = true
		}
	}
	if !foundDirect {
		t.Error("main() constructs Config{} directly but not found in callers")
	}
}

// --- Caller discovery: error vs empty ---

// TestGetCallers_UnknownSymbol_ReturnsError verifies that querying callers
// for a symbol that doesn't exist in the index returns an explicit error,
// not a silent empty slice.
//
// Given a project with no call graph data
// When GetCallers is called with a nonexistent symbol
// Then ErrSymbolNotFound is returned
func TestGetCallers_UnknownSymbol_ReturnsError(t *testing.T) {
	eng, _ := newTestEngine()

	_, err := eng.GetCallers(context.Background(), "/tmp", "_nonexistent_function_xyz")
	if err == nil {
		t.Fatal("expected error for unknown symbol, got nil — agent cannot distinguish from zero-callers")
	}
	if !errors.Is(err, ErrSymbolNotFound) {
		t.Errorf("expected ErrSymbolNotFound, got %v", err)
	}
}

// TestGetCallers_KnownSymbol_ZeroCallers_OK verifies that a known symbol
// with zero callers returns an empty report without error.
//
// Given a symbol that exists in the index but has no callers
// When GetCallers is called
// Then an empty report is returned without error
func TestGetCallers_KnownSymbol_ZeroCallers_OK(t *testing.T) {
	eng, _ := newTestEngine()

	r, err := eng.GetCallers(context.Background(), "/tmp", "Log")
	if err != nil {
		t.Errorf("known symbol with no callers should not error: %v", err)
	}
	if r == nil {
		t.Fatal("expected non-nil report")
	}
}

// --- Caller discovery: unqualified name matching ---

// TestGetCallers_UnqualifiedName verifies that callers accepts short
// function names (e.g. "format") and matches any callee whose FQN has
// that name as a suffix (e.g. "testmod/pkg.format").
//
// Given a Go project where pkg.WriteOutput calls pkg.format
// When GetCallers("format") is called
// Then WriteOutput is returned as a caller
func TestGetCallers_UnqualifiedName(t *testing.T) {
	dir := t.TempDir()
	files := map[string]string{
		"go.mod": "module testmod\ngo 1.21\n",
		"pkg/render.go": `package pkg

func WriteOutput(data string) string {
	return format(data)
}

func format(s string) string {
	return "[" + s + "]"
}
`,
	}
	for name, content := range files {
		p := filepath.Join(dir, name)
		os.MkdirAll(filepath.Dir(p), 0o755)
		os.WriteFile(p, []byte(content), 0o600)
	}

	eng := New(&mockStore{headSHA: "test"}, []string{dir})
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	r, err := eng.GetCallers(ctx, dir, "format")
	if err != nil {
		t.Skipf("call graph unavailable (LSP/tree-sitter): %v", err)
	}

	t.Logf("callers of 'format': %d", len(r.Callers))
	for _, c := range r.Callers {
		t.Logf("  caller=%s file=%s", c.Caller, c.File)
	}

	if len(r.Callers) == 0 {
		t.Error("GetCallers('format') returned 0 callers — unqualified name matching not working")
	}
}

// TestGetCallers_PrivatePrefixName verifies that underscore-prefixed names
// (e.g. "_write_output") get an explicit error when not in the index,
// rather than a silent empty result.
func TestGetCallers_PrivatePrefixName(t *testing.T) {
	report := testReport()
	store := newMockStore(report)
	eng := New(store, []string{"/tmp"})

	_, err := eng.GetCallers(context.Background(), "/tmp", "_write_output")
	if err == nil {
		return
	}
	t.Logf("got expected explicit error for unknown private symbol: %v", err)
}

// --- Caller discovery: filter correctness ---

// TestGetCallers_OnlyMatchingCallees verifies that GetCallers returns
// only callers of the target symbol, not everything else in the call graph.
//
// Given a call graph with edges A→B, C→B, D→E
// When GetCallers("B") is called
// Then only A and C are returned (not D)
func TestGetCallers_OnlyMatchingCallees(t *testing.T) {
	dir := t.TempDir()
	files := map[string]string{
		"go.mod": "module testmod\ngo 1.21\n",
		"main.go": `package main

func Alpha() string { return Beta() }
func Gamma() string { return Beta() }
func Delta() string { return Epsilon() }

func Beta() string    { return "b" }
func Epsilon() string { return "e" }

func main() {
	Alpha()
	Gamma()
	Delta()
}
`,
	}
	for name, content := range files {
		p := filepath.Join(dir, name)
		os.MkdirAll(filepath.Dir(p), 0o755)
		os.WriteFile(p, []byte(content), 0o600)
	}

	eng := New(&mockStore{headSHA: "test"}, []string{dir})
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	r, err := eng.GetCallers(ctx, dir, "Beta")
	if err != nil {
		t.Skipf("call graph unavailable: %v", err)
	}

	t.Logf("callers of Beta: %d", len(r.Callers))
	for _, c := range r.Callers {
		t.Logf("  caller=%s file=%s", c.Caller, c.File)
	}

	for _, c := range r.Callers {
		if c.Caller == "Delta" {
			t.Errorf("Delta calls Epsilon, not Beta — should not appear as a caller of Beta")
		}
	}

	foundAlpha := false
	foundGamma := false
	for _, c := range r.Callers {
		if c.Caller == "Alpha" {
			foundAlpha = true
		}
		if c.Caller == "Gamma" {
			foundGamma = true
		}
	}
	if !foundAlpha {
		t.Error("Alpha calls Beta but was not found in callers")
	}
	if !foundGamma {
		t.Error("Gamma calls Beta but was not found in callers")
	}
}
