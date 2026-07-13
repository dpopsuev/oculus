package engine

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dpopsuev/oculus/v3/arch"
	"github.com/dpopsuev/oculus/v3/lsp"
	"github.com/dpopsuev/oculus/v3/lsp/mockserver"
	"github.com/dpopsuev/oculus/v3/model"
)

func TestGetDefinition_AmbiguousReturnsEscalations(t *testing.T) {
	rep := testReportForLocator()
	for i := range rep.Architecture.Services {
		if rep.Architecture.Services[i].Name == "internal/store" {
			rep.Architecture.Services[i].Symbols = append(rep.Architecture.Services[i].Symbols, model.Symbol{
				Name: "Config", Exported: true, File: "internal/store/cfg.go", Line: 1,
			})
		}
	}
	eng := New(newMockStore(rep), []string{"/tmp"})

	r, err := eng.GetDefinition(context.Background(), "/tmp", "Config")
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Definitions) != 0 || len(r.Escalations) == 0 {
		t.Fatalf("want escalations, got %+v", r)
	}
}

func TestGetDefinition_StubPoolDegrades(t *testing.T) {
	dir := t.TempDir()
	mustWriteGoMod(t, dir)
	src := filepath.Join(dir, "pkg", "a.go")
	_ = os.MkdirAll(filepath.Dir(src), 0o755)
	_ = os.WriteFile(src, []byte("package pkg\n\nfunc Hello() {}\n"), 0o644)

	rep := &arch.ContextReport{
		ScanCore: arch.ScanCore{
			Architecture: arch.ArchModel{
				Services: []arch.ArchService{{
					Name: "pkg", Package: "example.com/m/pkg", Language: model.LangGo,
					Symbols: []model.Symbol{{Name: "Hello", Exported: true, File: "pkg/a.go", Line: 3}},
				}},
			},
		},
	}
	eng := New(newMockStore(rep), []string{dir}) // StubPool by default when no pool arg

	r, err := eng.GetDefinition(context.Background(), dir, "pkg/a.go:3:Hello")
	if err != nil {
		t.Fatal(err)
	}
	if r.Summary == "" {
		t.Fatal("expected summary")
	}
	// StubPool → unavailable or empty definitions (not an error)
	t.Logf("summary=%s defs=%d", r.Summary, len(r.Definitions))
}

func TestGetReferencesByLocator_StubPool(t *testing.T) {
	dir := t.TempDir()
	mustWriteGoMod(t, dir)
	src := filepath.Join(dir, "pkg", "a.go")
	_ = os.MkdirAll(filepath.Dir(src), 0o755)
	_ = os.WriteFile(src, []byte("package pkg\n\nfunc Hello() {}\n"), 0o644)

	rep := &arch.ContextReport{
		ScanCore: arch.ScanCore{
			Architecture: arch.ArchModel{
				Services: []arch.ArchService{{
					Name: "pkg", Package: "example.com/m/pkg", Language: model.LangGo,
					Symbols: []model.Symbol{{Name: "Hello", Exported: true, File: "pkg/a.go", Line: 3}},
				}},
			},
		},
	}
	eng := New(newMockStore(rep), []string{dir})
	r, err := eng.GetReferencesByLocator(context.Background(), dir, "Hello")
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("summary=%s refs=%d", r.Summary, len(r.References))
}

func TestGetShow_StubPoolDegrades(t *testing.T) {
	dir := t.TempDir()
	mustWriteGoMod(t, dir)
	src := filepath.Join(dir, "pkg", "a.go")
	_ = os.MkdirAll(filepath.Dir(src), 0o755)
	_ = os.WriteFile(src, []byte("package pkg\n\nfunc Hello() string {\n\treturn \"hi\"\n}\n"), 0o644)

	rep := &arch.ContextReport{
		ScanCore: arch.ScanCore{
			Architecture: arch.ArchModel{
				Services: []arch.ArchService{{
					Name: "pkg", Package: "example.com/m/pkg", Language: model.LangGo,
					Symbols: []model.Symbol{{Name: "Hello", Exported: true, File: "pkg/a.go", Line: 3}},
				}},
			},
		},
	}
	eng := New(newMockStore(rep), []string{dir})
	r, err := eng.GetShow(context.Background(), dir, "pkg/a.go:3:Hello")
	if err != nil {
		t.Fatal(err)
	}
	if r.Body != "" {
		t.Fatalf("stub pool should not return body, got %q", r.Body)
	}
	if r.Summary == "" {
		t.Fatal("expected summary")
	}
	t.Logf("show stub: %s", r.Summary)
}

func TestMatchDocSymbol_PrefersNameAndLine(t *testing.T) {
	syms := []lsp.DocSymbol{{
		Name: "Other", Kind: 12, StartLine: 1, EndLine: 10, SelectionLine: 1,
	}, {
		Name: "Hello", Kind: 12, StartLine: 3, EndLine: 5, SelectionLine: 3,
		Children: []lsp.DocSymbol{{Name: "inner", Kind: 13, StartLine: 4, EndLine: 4, SelectionLine: 4}},
	}}
	m := matchDocSymbol(syms, "Hello", 3)
	if m == nil || m.Name != "Hello" || m.EndLine != 5 {
		t.Fatalf("got %+v", m)
	}
}

func TestReadLineRange(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "f.go")
	if err := os.WriteFile(p, []byte("a\nb\nc\nd\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := readLineRange(p, 2, 3)
	if err != nil {
		t.Fatal(err)
	}
	if got != "b\nc" {
		t.Fatalf("got %q", got)
	}
}

func TestGetShow_MockPoolBodySlice(t *testing.T) {
	dir := t.TempDir()
	mustWriteGoMod(t, dir)
	src := filepath.Join(dir, "pkg", "a.go")
	_ = os.MkdirAll(filepath.Dir(src), 0o755)
	body := "package pkg\n\nfunc Hello() string {\n\treturn \"hi\"\n}\n"
	if err := os.WriteFile(src, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	uri := "file://" + filepath.ToSlash(src)

	rep := &arch.ContextReport{
		ScanCore: arch.ScanCore{
			Architecture: arch.ArchModel{
				Services: []arch.ArchService{{
					Name: "pkg", Package: "example.com/m/pkg", Language: model.LangGo,
					Symbols: []model.Symbol{{Name: "Hello", Exported: true, File: "pkg/a.go", Line: 3}},
				}},
			},
		},
	}
	pool := lsp.NewMockPool(mockserver.Config{
		Symbols: []mockserver.Symbol{{
			Name: "Hello", Kind: 12, URI: uri, Line: 2, Col: 5, // 0-based line
		}},
	})
	t.Cleanup(func() { _ = pool.Shutdown(context.Background()) })
	eng := New(newMockStore(rep), []string{dir}, pool)

	r, err := eng.GetShow(context.Background(), dir, "pkg/a.go:3:Hello")
	if err != nil {
		t.Fatal(err)
	}
	if r.Body == "" {
		t.Fatalf("expected body, summary=%s", r.Summary)
	}
	if !strings.Contains(r.Body, "func Hello") {
		t.Fatalf("body missing Hello: %q", r.Body)
	}
	if r.StartLine != 3 {
		t.Fatalf("start_line=%d want 3", r.StartLine)
	}
	t.Logf("show: %s\n%s", r.Summary, r.Body)
}

func mustWriteGoMod(t *testing.T, dir string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/m\n\ngo 1.22\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}
