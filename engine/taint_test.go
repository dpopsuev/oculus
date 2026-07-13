package engine

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBFSCallPath_Heuristic(t *testing.T) {
	dir := t.TempDir()
	mustWrite := func(name, body string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	mustWrite("go.mod", "module example.com/taintfix\n\ngo 1.22\n")
	mustWrite("flow.go", `package taintfix

func Source() string { return Mid("x") }
func Mid(s string) string { return Sink(s) }
func Sink(s string) string { return s }
`)

	eng := New(nil, []string{dir})
	res, err := eng.TaintQuery(context.Background(), dir, "Source", "Sink")
	if err != nil {
		t.Fatalf("TaintQuery: %v", err)
	}
	if res.Engine != "heuristic" {
		t.Fatalf("engine=%q", res.Engine)
	}
	if res.Disclaimer == "" {
		t.Fatal("missing disclaimer")
	}
	if !res.Found {
		t.Fatalf("expected path found; got %+v", res)
	}
	if len(res.Path) < 2 {
		t.Fatalf("path too short: %v", res.Path)
	}
}

func TestTaintFederationEnv(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/fed\n\ngo 1.22\n"), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "x.go"), []byte("package fed\nfunc A(){}\nfunc B(){}\n"), 0o644)

	t.Setenv("LOCUS_TAINT_CMD", `printf 'federated:%s->%s' '{source}' '{sink}'`)
	eng := New(nil, []string{dir})
	res, err := eng.TaintQuery(context.Background(), dir, "A", "B")
	if err != nil {
		t.Fatalf("TaintQuery: %v", err)
	}
	if res.Engine != "federated" {
		t.Fatalf("engine=%q want federated", res.Engine)
	}
	if !strings.Contains(res.Federated, "federated:A->B") {
		t.Fatalf("federated=%q", res.Federated)
	}
}
