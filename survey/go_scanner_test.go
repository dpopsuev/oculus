package survey_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dpopsuev/oculus/v3/model"
	"github.com/dpopsuev/oculus/v3/survey"
)

func setupModule(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, content := range files {
		p := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestScanExtractsPackages(t *testing.T) {
	dir := setupModule(t, map[string]string{
		"go.mod":      "module example.com/test\n\ngo 1.21\n",
		"main.go":     "package main\n\nfunc main() {}\n",
		"lib/lib.go":  "package lib\n\nfunc Hello() string { return \"hi\" }\n",
		"lib/util.go": "package lib\n\nvar Version = \"1.0\"\n",
	})

	sc := &survey.GoScanner{}
	mod, err := sc.Scan(dir)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}

	if mod.Path != "example.com/test" {
		t.Errorf("path = %q, want example.com/test", mod.Path)
	}

	if len(mod.Namespaces) != 2 {
		t.Fatalf("namespaces = %d, want 2", len(mod.Namespaces))
	}

	pkgMap := make(map[string]*model.Namespace)
	for _, p := range mod.Namespaces {
		pkgMap[p.ImportPath] = p
	}

	lib, ok := pkgMap["example.com/test/lib"]
	if !ok {
		t.Fatal("missing package example.com/test/lib")
	}
	if lib.Name != "lib" {
		t.Errorf("lib.name = %q, want lib", lib.Name)
	}
	if len(lib.Files) != 2 {
		t.Errorf("lib.files = %d, want 2", len(lib.Files))
	}
}

func TestScanExtractsSymbols(t *testing.T) {
	dir := setupModule(t, map[string]string{
		"go.mod": "module example.com/sym\n\ngo 1.21\n",
		"pkg/pkg.go": `package pkg

type Server struct{}
type Handler interface{ Handle() }
func NewServer() *Server { return nil }
var DefaultTimeout = 30
const MaxRetries = 3
func helper() {}
`,
	})

	sc := &survey.GoScanner{}
	mod, err := sc.Scan(dir)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}

	if len(mod.Namespaces) != 1 {
		t.Fatalf("namespaces = %d, want 1", len(mod.Namespaces))
	}

	pkg := mod.Namespaces[0]
	symMap := make(map[string]*model.Symbol)
	for _, s := range pkg.Symbols {
		symMap[s.Name] = s
	}

	tests := []struct {
		name     string
		kind     model.SymbolKind
		exported bool
	}{
		{"Server", model.SymbolStruct, true},
		{"Handler", model.SymbolInterface, true},
		{"NewServer", model.SymbolFunction, true},
		{"DefaultTimeout", model.SymbolVariable, true},
		{"MaxRetries", model.SymbolConstant, true},
		{"helper", model.SymbolFunction, false},
	}

	for _, tt := range tests {
		s, ok := symMap[tt.name]
		if !ok {
			t.Errorf("missing symbol %q", tt.name)
			continue
		}
		if s.Kind != tt.kind {
			t.Errorf("%s.kind = %v, want %v", tt.name, s.Kind, tt.kind)
		}
		if s.Exported != tt.exported {
			t.Errorf("%s.exported = %v, want %v", tt.name, s.Exported, tt.exported)
		}
	}

	if s := symMap["NewServer"]; s.Signature == "" {
		t.Error("NewServer should have a signature")
	} else if !strings.Contains(s.Signature, "func NewServer()") {
		t.Errorf("NewServer.Signature = %q, want contains 'func NewServer()'", s.Signature)
	}
	if s := symMap["Handler"]; s.Signature == "" {
		t.Error("Handler should have a signature")
	} else if !strings.Contains(s.Signature, "Handle()") {
		t.Errorf("Handler.Signature = %q, want contains 'Handle()'", s.Signature)
	}
}

func TestScanBuildsImportGraph(t *testing.T) {
	dir := setupModule(t, map[string]string{
		"go.mod": "module example.com/graph\n\ngo 1.21\n",
		"a/a.go": `package a

import "example.com/graph/b"

var _ = b.Hello
`,
		"b/b.go": `package b

import "fmt"

func Hello() { fmt.Println("hi") }
`,
	})

	sc := &survey.GoScanner{}
	mod, err := sc.Scan(dir)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}

	if mod.DependencyGraph == nil {
		t.Fatal("dependency graph is nil")
	}

	aEdges := mod.DependencyGraph.EdgesFrom("example.com/graph/a")
	found := false
	for _, e := range aEdges {
		if e.To == "example.com/graph/b" {
			if e.External {
				t.Error("a->b should be internal")
			}
			found = true
		}
	}
	if !found {
		t.Error("missing edge a -> b")
	}

	bEdges := mod.DependencyGraph.EdgesFrom("example.com/graph/b")
	foundFmt := false
	for _, e := range bEdges {
		if e.To == "fmt" {
			if !e.External {
				t.Error("b->fmt should be external")
			}
			foundFmt = true
		}
	}
	if !foundFmt {
		t.Error("missing edge b -> fmt")
	}
}

func TestScanSkipsVendorAndHiddenDirs(t *testing.T) {
	dir := setupModule(t, map[string]string{
		"go.mod":            "module example.com/skip\n\ngo 1.21\n",
		"main.go":           "package main\n",
		"vendor/v/v.go":     "package v\n",
		".hidden/h.go":      "package h\n",
		"testdata/td/td.go": "package td\n",
	})

	sc := &survey.GoScanner{}
	mod, err := sc.Scan(dir)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}

	if len(mod.Namespaces) != 1 {
		t.Errorf("namespaces = %d, want 1 (only main)", len(mod.Namespaces))
	}
}

// --- ScanFile interface tests ---

// TestGoScanner_ScanFile verifies that GoScanner implements FileScanner and
// returns only the symbols from the specified file.
//
// Given a Go module with two files in the same package
// When ScanFile is called on one file
// Then the result contains only symbols from that file
func TestGoScanner_ScanFile(t *testing.T) {
	dir := setupModule(t, map[string]string{
		"go.mod":    "module example.com/scan\n\ngo 1.21\n",
		"foo.go":    "package main\n\nfunc Alpha() {}\nfunc Beta() {}\n",
		"bar.go":    "package main\n\nfunc Gamma() {}\n",
	})

	sc := &survey.GoScanner{}
	fs, ok := any(sc).(survey.FileScanner)
	if !ok {
		t.Fatal("GoScanner does not implement FileScanner")
	}

	proj, err := fs.ScanFile(filepath.Join(dir, "foo.go"))
	if err != nil {
		t.Fatalf("ScanFile: %v", err)
	}
	if len(proj.Namespaces) == 0 {
		t.Fatal("expected at least one namespace")
	}

	symNames := make(map[string]bool)
	for _, ns := range proj.Namespaces {
		for _, s := range ns.Symbols {
			symNames[s.Name] = true
		}
	}
	if !symNames["Alpha"] || !symNames["Beta"] {
		t.Errorf("expected Alpha and Beta from foo.go; got: %v", symNames)
	}
	if symNames["Gamma"] {
		t.Error("Gamma from bar.go should not appear in ScanFile(foo.go)")
	}
}

// TestRustScanner_ScanFile verifies that RustScanner implements FileScanner.
//
// Given a Rust file with pub symbols
// When ScanFile is called
// Then the result contains those symbols
func TestRustScanner_ScanFile(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "lib.rs")
	if err := os.WriteFile(filePath, []byte("pub fn serve() {}\npub struct Config {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	sc := &survey.RustScanner{}
	fs, ok := any(sc).(survey.FileScanner)
	if !ok {
		t.Fatal("RustScanner does not implement FileScanner")
	}

	proj, err := fs.ScanFile(filePath)
	if err != nil {
		t.Fatalf("ScanFile: %v", err)
	}

	symNames := make(map[string]bool)
	for _, ns := range proj.Namespaces {
		for _, s := range ns.Symbols {
			symNames[s.Name] = true
		}
	}
	if !symNames["serve"] {
		t.Errorf("expected 'serve'; got: %v", symNames)
	}
	if !symNames["Config"] {
		t.Errorf("expected 'Config'; got: %v", symNames)
	}
}

// TestTSScanner_ScanFile verifies that TypeScriptScanner implements FileScanner.
func TestTSScanner_ScanFile(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "index.ts")
	if err := os.WriteFile(filePath, []byte("export function render() {}\nexport class Widget {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	sc := &survey.TypeScriptScanner{}
	fs, ok := any(sc).(survey.FileScanner)
	if !ok {
		t.Fatal("TypeScriptScanner does not implement FileScanner")
	}

	proj, err := fs.ScanFile(filePath)
	if err != nil {
		t.Fatalf("ScanFile: %v", err)
	}

	symNames := make(map[string]bool)
	for _, ns := range proj.Namespaces {
		for _, s := range ns.Symbols {
			symNames[s.Name] = true
		}
	}
	if !symNames["render"] {
		t.Errorf("expected 'render'; got: %v", symNames)
	}
	if !symNames["Widget"] {
		t.Errorf("expected 'Widget'; got: %v", symNames)
	}
}

// TestPythonScanner_ScanFile verifies that PythonScanner implements FileScanner.
func TestPythonScanner_ScanFile(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "api.py")
	if err := os.WriteFile(filePath, []byte("def handle():\n    pass\nclass Router:\n    pass\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	sc := &survey.PythonScanner{}
	fs, ok := any(sc).(survey.FileScanner)
	if !ok {
		t.Fatal("PythonScanner does not implement FileScanner")
	}

	proj, err := fs.ScanFile(filePath)
	if err != nil {
		t.Fatalf("ScanFile: %v", err)
	}

	symNames := make(map[string]bool)
	for _, ns := range proj.Namespaces {
		for _, s := range ns.Symbols {
			symNames[s.Name] = true
		}
	}
	if !symNames["handle"] {
		t.Errorf("expected 'handle'; got: %v", symNames)
	}
	if !symNames["Router"] {
		t.Errorf("expected 'Router'; got: %v", symNames)
	}
}
