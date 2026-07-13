package lsp

import (
	"os"
	"path/filepath"
	"testing"
)

func TestApplyWorkspaceEdit_BottomUp(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "a.go")
	if err := os.WriteFile(p, []byte("aaa Hello bbb Hello ccc\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	edit := &WorkspaceEdit{Files: []FileEdit{{
		File: p,
		Edits: []TextEdit{
			{StartLine: 0, StartChar: 4, EndLine: 0, EndChar: 9, NewText: "Howdy"},
			{StartLine: 0, StartChar: 14, EndLine: 0, EndChar: 19, NewText: "Howdy"},
		},
	}}}
	if err := ApplyWorkspaceEdit(edit); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(p)
	want := "aaa Howdy bbb Howdy ccc\n"
	if string(got) != want {
		t.Fatalf("got %q want %q", got, want)
	}
}
