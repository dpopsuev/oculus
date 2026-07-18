package arch

import "testing"

func TestShortImportPath_RootLabel(t *testing.T) {
	if got := shortImportPath("example.com/mod", "example.com/mod"); got != "(root)" {
		t.Fatalf("got %q", got)
	}
	if got := shortImportPath("example.com/mod", "."); got != "(root)" {
		t.Fatalf("got %q", got)
	}
	if got := shortImportPath("example.com/mod", "example.com/mod/pkg"); got != "pkg" {
		t.Fatalf("got %q", got)
	}
}
