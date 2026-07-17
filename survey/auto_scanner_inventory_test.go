package survey_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dpopsuev/oculus/v3/arch"
	"github.com/dpopsuev/oculus/v3/survey"
)

// TestAutoScanner_InventorySelectsComposite verifies the default auto path
// uses language inventory — agents need not pass scanner=composite.
func TestAutoScanner_InventorySelectsComposite(t *testing.T) {
	dir := t.TempDir()
	files := map[string]string{
		"Cargo.toml":               "[package]\nname = \"core\"\nversion = \"0.1.0\"\n",
		"src/lib.rs":               "pub fn run() {}\n",
		"package.json":             `{"name":"ui"}`,
		"packages/ui/src/index.ts": "export const n = 1;\n",
	}
	for name, content := range files {
		p := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	report, err := arch.ScanAndBuild(context.Background(), dir, arch.ScanOpts{
		Intent: arch.IntentArchitecture,
	})
	if err != nil {
		t.Fatalf("ScanAndBuild: %v", err)
	}
	if report.Scanner != "composite" {
		t.Fatalf("Scanner=%q, want composite (inventory auto)", report.Scanner)
	}

	var hasRust, hasTS bool
	if report.Project != nil {
		for _, ns := range report.Project.Namespaces {
			for _, f := range ns.Files {
				if strings.HasSuffix(f.Path, ".rs") {
					hasRust = true
				}
				if strings.HasSuffix(f.Path, ".ts") {
					hasTS = true
				}
			}
		}
	}
	if !hasRust || !hasTS {
		names := make([]string, 0, len(report.Architecture.Services))
		for _, s := range report.Architecture.Services {
			names = append(names, s.Name)
		}
		t.Fatalf("expected rust+ts files (rust=%v ts=%v) services=%v", hasRust, hasTS, names)
	}
}

func TestAutoScanner_ExplicitOverrideSkipsInventory(t *testing.T) {
	dir := t.TempDir()
	files := map[string]string{
		"Cargo.toml":   "[package]\nname = \"core\"\nversion = \"0.1.0\"\n",
		"src/lib.rs":   "pub fn run() {}\n",
		"package.json": `{"name":"ui"}`,
		"ui/index.ts":  "export const n = 1;\n",
	}
	for name, content := range files {
		p := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	sc := &survey.AutoScanner{Override: "typescript"}
	proj, err := sc.Scan(dir)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	for _, ns := range proj.Namespaces {
		for _, f := range ns.Files {
			if strings.HasSuffix(f.Path, ".rs") {
				t.Fatalf("typescript override must not scan .rs; got %s", f.Path)
			}
		}
	}
}
