package survey_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/dpopsuev/oculus/v3/model"
	"github.com/dpopsuev/oculus/v3/survey"
)

func TestPartialScan_TypeScriptDirs(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{"name":"p"}`), 0o644)
	_ = os.MkdirAll(filepath.Join(dir, "alpha"), 0o755)
	_ = os.MkdirAll(filepath.Join(dir, "beta"), 0o755)
	_ = os.WriteFile(filepath.Join(dir, "alpha/a.ts"), []byte("export const A = 1\n"), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "beta/b.ts"), []byte("export const B = 2\n"), 0o644)

	proj, err := survey.PartialScan(dir, model.LangTypeScript, []string{"alpha"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(proj.Namespaces) != 1 {
		t.Fatalf("namespaces=%d want 1", len(proj.Namespaces))
	}
	if proj.Namespaces[0].ImportPath != "alpha" {
		t.Fatalf("import=%q", proj.Namespaces[0].ImportPath)
	}
}

func TestLangFromFilename(t *testing.T) {
	cases := map[string]model.Language{
		"a.go":   model.LangGo,
		"a.ts":   model.LangTypeScript,
		"a.tsx":  model.LangTypeScript,
		"a.py":   model.LangPython,
		"a.rs":   model.LangRust,
		"a.txt":  model.LangUnknown,
	}
	for path, want := range cases {
		if got := survey.LangFromFilename(path); got != want {
			t.Errorf("%s: got %v want %v", path, got, want)
		}
	}
}
