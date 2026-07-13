package survey

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/dpopsuev/oculus/v3/model"
)

// PartialScan resurveys only the given package directories for merge.
// lang may be LangUnknown to infer from changedPaths extensions (polyglot).
func PartialScan(root string, lang model.Language, dirs, changedPaths []string) (*model.Project, error) {
	if lang == model.LangUnknown {
		return partialByPathLang(root, dirs, changedPaths)
	}
	return partialOneLang(root, lang, dirs, changedPaths)
}

func partialByPathLang(root string, dirs, changedPaths []string) (*model.Project, error) {
	byLang := map[model.Language][]string{}
	for _, p := range changedPaths {
		l := LangFromFilename(p)
		if l == model.LangUnknown {
			continue
		}
		dir := filepath.ToSlash(filepath.Dir(p))
		if dir == "." || dir == "" {
			dir = nsRoot
		}
		byLang[l] = appendUnique(byLang[l], dir)
	}
	if len(byLang) == 0 {
		// Fall back to dirs with Go (legacy merge callers).
		return partialOneLang(root, model.LangGo, dirs, changedPaths)
	}
	var merged *model.Project
	for l, d := range byLang {
		part, err := partialOneLang(root, l, d, nil)
		if err != nil {
			return nil, err
		}
		merged = mergePartialProjects(merged, part)
	}
	if merged == nil {
		return nil, fmt.Errorf("partial scan: no language matched")
	}
	merged.Language = model.LangUnknown // polyglot
	return merged, nil
}

func partialOneLang(root string, lang model.Language, dirs, changedPaths []string) (*model.Project, error) {
	switch lang {
	case model.LangGo:
		patterns := packageDirsToPatterns(dirs)
		return (&PackagesScanner{}).ScanPatterns(root, patterns)
	case model.LangTypeScript:
		return (&TypeScriptScanner{}).ScanDirs(root, dirs)
	case model.LangPython:
		return (&PythonScanner{}).ScanDirs(root, dirs)
	case model.LangRust:
		return (&RustScanner{}).ScanDirs(root, dirs, changedPaths)
	default:
		return nil, fmt.Errorf("partial scan: unsupported language %s", lang)
	}
}

// LangFromFilename maps a source path to a survey language.
func LangFromFilename(path string) model.Language {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".go":
		return model.LangGo
	case ".ts", ".tsx", ".js", ".jsx", ".mjs", ".cjs":
		return model.LangTypeScript
	case ".py":
		return model.LangPython
	case ".rs":
		return model.LangRust
	default:
		return model.LangUnknown
	}
}

func packageDirsToPatterns(pkgs []string) []string {
	var out []string
	for _, p := range pkgs {
		p = filepath.ToSlash(p)
		if p == "" || p == nsRoot {
			out = append(out, ".")
			continue
		}
		out = append(out, "./"+strings.TrimPrefix(p, "./"))
	}
	return out
}

func appendUnique(slice []string, v string) []string {
	for _, s := range slice {
		if s == v {
			return slice
		}
	}
	return append(slice, v)
}

func mergePartialProjects(a, b *model.Project) *model.Project {
	if a == nil {
		return b
	}
	if b == nil {
		return a
	}
	for _, ns := range b.Namespaces {
		a.AddNamespace(ns)
	}
	if b.DependencyGraph != nil {
		if a.DependencyGraph == nil {
			a.DependencyGraph = model.NewDependencyGraph()
		}
		for _, e := range b.DependencyGraph.Edges {
			a.DependencyGraph.AddEdge(e.From, e.To, e.External)
			if e.CallSites > 0 || e.LOCSurface > 0 {
				a.DependencyGraph.SetEdgeCoupling(e.From, e.To, e.CallSites, e.LOCSurface)
			}
		}
	}
	return a
}

// DirAllowed reports whether rel path (file or dir) falls under any allow dir.
func DirAllowed(rel string, allow map[string]bool) bool {
	rel = filepath.ToSlash(rel)
	if rel == "." {
		rel = nsRoot
	}
	if allow[rel] || allow[nsRoot] && (rel == nsRoot || rel == ".") {
		return true
	}
	for d := range allow {
		if d == "" || d == nsRoot {
			if !strings.Contains(rel, "/") {
				return true // root-level file/dir
			}
			continue
		}
		if rel == d || strings.HasPrefix(rel, d+"/") {
			return true
		}
	}
	return false
}

func allowSet(dirs []string) map[string]bool {
	m := map[string]bool{}
	for _, d := range dirs {
		d = filepath.ToSlash(strings.Trim(d, "/"))
		if d == "" || d == "." {
			d = nsRoot
		}
		m[d] = true
	}
	return m
}
