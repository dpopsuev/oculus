package lang_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/dpopsuev/oculus/v3/lang"
)

func TestDetectLanguages_CoLocatedRustAndTS(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "Cargo.toml"), []byte("[package]\nname=\"x\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{"name":"x"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	langs := lang.DetectLanguages(dir)
	if len(langs) < 2 {
		t.Fatalf("DetectLanguages=%v, want rust+typescript", langs)
	}
	if lang.DetectLanguage(dir) != lang.Rust {
		t.Fatalf("DetectLanguage first-wins want Rust, got %v", lang.DetectLanguage(dir))
	}
}
