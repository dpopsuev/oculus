package lang

import (
	"os"
	"path/filepath"
	"sort"
)

// MinExtensionFiles is the minimum number of source files (by extension)
// required to treat a language as present when it has no project marker.
// Keeps one-off scripts from flipping a Go repo into composite.
const MinExtensionFiles = 3

// Inventory is the result of a language first-pass over a repository root.
type Inventory struct {
	// Languages is the ordered set of languages to scan (markers first,
	// then extension-discovered languages by descending file count).
	Languages []Language
	// Markers lists languages signaled by root manifests.
	Markers []Language
	// ExtCounts is the per-language source-file census from the tree walk.
	ExtCounts map[Language]int
}

// ExtensionLanguage maps a source file extension (with leading dot) to a
// Language. .js/.jsx count as TypeScript for scanner selection (same survey
// backend); call sites that need JS-vs-TS distinction can inspect ExtCounts.
var ExtensionLanguage = map[string]Language{
	".go":   Go,
	".rs":   Rust,
	".py":   Python,
	".pyi":  Python,
	".ts":   TypeScript,
	".tsx":  TypeScript,
	".mts":  TypeScript,
	".cts":  TypeScript,
	".js":   TypeScript,
	".jsx":  TypeScript,
	".mjs":  TypeScript,
	".cjs":  TypeScript,
	".c":    C,
	".h":    C,
	".cc":   Cpp,
	".cpp":  Cpp,
	".cxx":  Cpp,
	".hpp":  Cpp,
	".hxx":  Cpp,
	".java": Java,
	".kt":   Kotlin,
	".kts":  Kotlin,
	".swift": Swift,
	".cs":   CSharp,
	".zig":  Zig,
	".lua":  Lua,
	".proto": Proto,
	".sh":   Shell,
	".bash": Shell,
}

// InventoryLanguages performs the auto-scan first pass:
//  1. root project markers (Cargo.toml, package.json, …) — Makefile excluded
//  2. extension census under root (skipping vendor/node_modules/…)
//  3. merge — markers always win a seat; extension-only langs need MinExtensionFiles
func InventoryLanguages(root string) Inventory {
	inv := Inventory{
		Markers:   detectInventoryMarkers(root),
		ExtCounts: countExtensionLanguages(root),
	}

	seen := make(map[Language]bool)
	for _, l := range inv.Markers {
		if l == Unknown || seen[l] {
			continue
		}
		seen[l] = true
		inv.Languages = append(inv.Languages, l)
	}

	type ranked struct {
		lang  Language
		count int
	}
	var extra []ranked
	for lang, n := range inv.ExtCounts {
		if lang == Unknown || seen[lang] {
			continue
		}
		if n < MinExtensionFiles {
			continue
		}
		extra = append(extra, ranked{lang, n})
	}
	sort.Slice(extra, func(i, j int) bool {
		if extra[i].count != extra[j].count {
			return extra[i].count > extra[j].count
		}
		return extra[i].lang < extra[j].lang
	})
	for _, e := range extra {
		seen[e.lang] = true
		inv.Languages = append(inv.Languages, e.lang)
	}

	return inv
}

// IsMultiLanguage reports whether inventory found two or more languages.
func (inv Inventory) IsMultiLanguage() bool {
	return len(inv.Languages) > 1
}

// Primary returns the first inventory language, or Unknown.
func (inv Inventory) Primary() Language {
	if len(inv.Languages) == 0 {
		return Unknown
	}
	return inv.Languages[0]
}

// detectInventoryMarkers is DetectLanguages minus ubiquitous false friends.
// Makefile alone must not mark a repo as C (every Go tree has one).
func detectInventoryMarkers(root string) []Language {
	seen := make(map[Language]bool)
	var out []Language
	for _, m := range LanguageMarkers {
		if m.File == "Makefile" {
			continue
		}
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

func countExtensionLanguages(root string) map[Language]int {
	counts := make(map[Language]int)
	abs, err := filepath.Abs(root)
	if err != nil {
		abs = root
	}
	_ = filepath.WalkDir(abs, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if ShouldSkipDir(d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		ext := filepath.Ext(d.Name())
		lang, ok := ExtensionLanguage[ext]
		if !ok || lang == Unknown {
			return nil
		}
		counts[lang]++
		return nil
	})
	return counts
}
