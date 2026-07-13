package engine

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/dpopsuev/oculus/v3/arch"
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

func mustWriteGoMod(t *testing.T, dir string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/m\n\ngo 1.22\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}
