package analyzer

import (
	"os"
	"path/filepath"
	"testing"
)

// TestCallChain_NilContextDoesNotPanic guards the locus diagram CLI path that
// historically omitted Input.Ctx (sequence diagrams panicked on ctx.Err()).
func TestCallChain_NilContextDoesNotPanic(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\nfunc main() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	fa := NewQuickFallback(dir)
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("CallChain(nil ctx) panicked: %v", r)
		}
	}()
	_, _ = fa.CallChain(nil, dir, "main", 2)
}
