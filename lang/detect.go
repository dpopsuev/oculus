package lang

import (
	"os"
	"path/filepath"
)

// LanguageMarker maps a project manifest file to its language.
type LanguageMarker struct {
	File string
	Lang Language
}

// LanguageMarkers is the canonical list of file→language mappings,
// ordered by specificity (most specific first, ambiguous markers last).
var LanguageMarkers = []LanguageMarker{
	{"go.mod", Go},
	{"Cargo.toml", Rust},
	{"CMakeLists.txt", Cpp},
	{"pyproject.toml", Python},
	{"setup.py", Python},
	{"tsconfig.json", TypeScript},
	{"package.json", TypeScript},
	{"pom.xml", Java},
	{"build.gradle", Java},
	{"build.gradle.kts", Kotlin},
	{"Package.swift", Swift},
	{"build.zig", Zig},
	{"Makefile", C},
}

// RootProjectMarkers is the subset of LanguageMarkers used for
// discovering sub-projects at the root of a polyglot repo.
// TypeScript is excluded here because it's discovered via directory walk.
var RootProjectMarkers = []LanguageMarker{
	{"go.mod", Go},
	{"Cargo.toml", Rust},
	{"pyproject.toml", Python},
	{"setup.py", Python},
}

// DetectLanguage inspects marker files in root to determine the project language.
// First-wins order follows LanguageMarkers (most specific first).
func DetectLanguage(root string) Language {
	langs := DetectLanguages(root)
	if len(langs) == 0 {
		return Unknown
	}
	return langs[0]
}

// DetectLanguages returns every language signaled by root marker files,
// preserving LanguageMarkers order and de-duplicating. Used for polyglot
// detection (e.g. Cargo.toml + package.json at the same root).
func DetectLanguages(root string) []Language {
	seen := make(map[Language]bool)
	var out []Language
	for _, m := range LanguageMarkers {
		if _, err := os.Stat(filepath.Join(root, m.File)); err != nil {
			continue
		}
		if seen[m.Lang] {
			continue
		}
		seen[m.Lang] = true
		out = append(out, m.Lang)
	}
	globs := []struct {
		pattern string
		lang    Language
	}{
		{"*.csproj", CSharp},
		{"*.sln", CSharp},
	}
	for _, g := range globs {
		if seen[g.lang] {
			continue
		}
		matches, _ := filepath.Glob(filepath.Join(root, g.pattern))
		if len(matches) > 0 {
			seen[g.lang] = true
			out = append(out, g.lang)
		}
	}
	return out
}
