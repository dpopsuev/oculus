package lang_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/dpopsuev/oculus/v3/lang"
)

func TestInventoryLanguages_MarkersWin(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "Cargo.toml", "[package]\nname=\"x\"\n")
	write(t, dir, "package.json", `{"name":"x"}`)
	write(t, dir, "src/lib.rs", "pub fn f() {}\n")
	write(t, dir, "packages/a/index.ts", "export const a = 1;\n")

	inv := lang.InventoryLanguages(dir)
	if !inv.IsMultiLanguage() {
		t.Fatalf("Languages=%v, want multi", inv.Languages)
	}
	has := map[lang.Language]bool{}
	for _, l := range inv.Languages {
		has[l] = true
	}
	if !has[lang.Rust] || !has[lang.TypeScript] {
		t.Fatalf("want rust+typescript, got %v", inv.Languages)
	}
}

func TestInventoryLanguages_ExtensionCensus(t *testing.T) {
	dir := t.TempDir()
	// No markers — pure extension discovery.
	for _, name := range []string{"a.go", "b.go", "c.go", "d.go"} {
		write(t, dir, name, "package main\n")
	}
	inv := lang.InventoryLanguages(dir)
	if inv.Primary() != lang.Go {
		t.Fatalf("Primary=%v, want go (ext census)", inv.Primary())
	}
	if inv.IsMultiLanguage() {
		t.Fatalf("unexpected multi: %v", inv.Languages)
	}
}

func TestInventoryLanguages_IgnoresLoneScript(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "go.mod", "module example.com/m\n")
	write(t, dir, "main.go", "package main\nfunc main() {}\n")
	write(t, dir, "hack.py", "print(1)\n") // one file < MinExtensionFiles

	inv := lang.InventoryLanguages(dir)
	if inv.IsMultiLanguage() {
		t.Fatalf("lone .py must not flip polyglot: %v counts=%v", inv.Languages, inv.ExtCounts)
	}
	if inv.Primary() != lang.Go {
		t.Fatalf("Primary=%v, want go", inv.Primary())
	}
}

func TestInventoryLanguages_MakefileDoesNotImplyC(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "go.mod", "module example.com/m\n")
	write(t, dir, "main.go", "package main\nfunc main() {}\n")
	write(t, dir, "Makefile", "all:\n\tgo build\n")

	inv := lang.InventoryLanguages(dir)
	if inv.IsMultiLanguage() {
		t.Fatalf("Makefile must not add C: %v markers=%v", inv.Languages, inv.Markers)
	}
	if inv.Primary() != lang.Go {
		t.Fatalf("Primary=%v, want go", inv.Primary())
	}
}

func TestInventoryLanguages_ExtensionPolyglot(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"a.rs", "b.rs", "c.rs"} {
		write(t, dir, name, "fn main() {}\n")
	}
	for _, name := range []string{"a.ts", "b.ts", "c.ts"} {
		write(t, dir, "web/"+name, "export const x = 1;\n")
	}
	inv := lang.InventoryLanguages(dir)
	if !inv.IsMultiLanguage() {
		t.Fatalf("Languages=%v, want rust+typescript from extensions", inv.Languages)
	}
}

func write(t *testing.T, root, rel, content string) {
	t.Helper()
	p := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
