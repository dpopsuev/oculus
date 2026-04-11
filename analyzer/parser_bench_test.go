package analyzer

import (
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/dpopsuev/oculus"
)

func benchRoot(b *testing.B) string {
	b.Helper()
	dir, _ := filepath.Abs("..")
	if _, err := os.Stat(filepath.Join(dir, "go.mod")); err != nil {
		b.Skip("oculus root not found")
	}
	return dir
}

func BenchmarkParseGoAST(b *testing.B) {
	root := benchRoot(b)
	b.ResetTimer()
	for range b.N {
		funcs := ParseGoASTFunctions(root)
		if len(funcs) == 0 {
			b.Fatal("no functions")
		}
	}
}

func BenchmarkParseTreeSitter(b *testing.B) {
	root := benchRoot(b)
	b.ResetTimer()
	for range b.N {
		funcs := ParseTreeSitterFunctions(root)
		if len(funcs) == 0 {
			b.Fatal("no functions")
		}
	}
}

func benchFixture(b *testing.B, files map[string]string) string {
	b.Helper()
	dir := b.TempDir()
	paths := make([]string, 0, len(files))
	for p := range files {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	for _, rel := range paths {
		abs := filepath.Join(dir, rel)
		os.MkdirAll(filepath.Dir(abs), 0o755)
		os.WriteFile(abs, []byte(files[rel]), 0o600)
	}
	return dir
}

func BenchmarkParsePython(b *testing.B) {
	dir := benchFixture(b, languageFixtures[0].files)
	b.ResetTimer()
	for range b.N {
		ParsePythonFunctions(dir)
	}
}

func BenchmarkParseTypeScript(b *testing.B) {
	dir := benchFixture(b, languageFixtures[1].files)
	b.ResetTimer()
	for range b.N {
		ParseTypeScriptFunctions(dir)
	}
}

func BenchmarkParseRust(b *testing.B) {
	dir := benchFixture(b, languageFixtures[3].files)
	b.ResetTimer()
	for range b.N {
		ParseRustFunctions(dir)
	}
}

func BenchmarkParseJava(b *testing.B) {
	dir := benchFixture(b, languageFixtures[4].files)
	b.ResetTimer()
	for range b.N {
		ParseJavaFunctions(dir)
	}
}

func BenchmarkParseC(b *testing.B) {
	dir := benchFixture(b, languageFixtures[5].files)
	b.ResetTimer()
	for range b.N {
		ParseCFunctions(dir)
	}
}

func BenchmarkFuncIndexSource_Construction(b *testing.B) {
	root := benchRoot(b)
	funcs := ParseGoASTFunctions(root)
	b.ResetTimer()
	for range b.N {
		oculus.NewFuncIndexSource(funcs)
	}
}
