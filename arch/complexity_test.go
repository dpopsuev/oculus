package arch_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/dpopsuev/oculus/v3/arch"
)

func TestAnalyzePackageComplexity_NestedAndRecursion(t *testing.T) {
	dir := t.TempDir()
	src := `package hot

func Nested(xs [][][]int) int {
	n := 0
	for i := range xs {
		for j := range xs[i] {
			for k := range xs[i][j] {
				n += xs[i][j][k]
			}
		}
	}
	return n
}

func Fact(n int) int {
	if n <= 1 {
		return 1
	}
	return n * Fact(n-1)
}
`
	if err := os.WriteFile(filepath.Join(dir, "hot.go"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	pc := arch.AnalyzePackageComplexity(dir, ".")
	if pc.MaxLoopNesting < 3 {
		t.Fatalf("MaxLoopNesting=%d want >=3", pc.MaxLoopNesting)
	}
	has := map[string]bool{}
	for _, p := range pc.Patterns {
		has[p] = true
	}
	if !has["nested_loops"] {
		t.Fatalf("patterns=%v missing nested_loops", pc.Patterns)
	}
	if !has["recursion"] {
		t.Fatalf("patterns=%v missing recursion", pc.Patterns)
	}
	if pc.ComplexityHint == "" {
		t.Fatal("expected ComplexityHint")
	}
}

func TestAnalyzePackageComplexity_Linear(t *testing.T) {
	dir := t.TempDir()
	src := `package linear

func Sum(xs []int) int {
	n := 0
	for _, v := range xs {
		n += v
	}
	return n
}
`
	if err := os.WriteFile(filepath.Join(dir, "lin.go"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	pc := arch.AnalyzePackageComplexity(dir, ".")
	if pc.MaxLoopNesting != 1 {
		t.Fatalf("MaxLoopNesting=%d want 1", pc.MaxLoopNesting)
	}
	if len(pc.Patterns) != 0 {
		t.Fatalf("patterns=%v want empty", pc.Patterns)
	}
	if pc.ComplexityHint != "" {
		t.Fatalf("hint=%q want empty", pc.ComplexityHint)
	}
}
