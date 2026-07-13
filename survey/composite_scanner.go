package survey

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/dpopsuev/oculus/v3/model"
)

// CompositeScanner detects multiple sub-projects within a root directory
// and merges their scan results into a single Project. This handles
// polyglot repositories (e.g. Rust backend + TypeScript frontend).
type CompositeScanner struct {
	// TSFileGranularity propagates to TypeScript and Rust sub-project scans.
	TSFileGranularity bool
}

type subProject struct {
	relPath string
	lang    model.Language
}

func (s *CompositeScanner) Scan(root string) (*model.Project, error) {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}

	subs := discoverSubProjects(absRoot)
	if len(subs) == 0 {
		return (&AutoScanner{}).Scan(root)
	}

	proj := &model.Project{
		Path:            filepath.Base(absRoot),
		DependencyGraph: model.NewDependencyGraph(),
	}

	if len(subs) == 1 && subs[0].relPath == "." {
		proj.Language = subs[0].lang
	}

	// seenNS tracks namespace ImportPaths that have already been added to
	// the merged project. The root '.' sub-project is scanned first and
	// produces monorepo-root-relative names (e.g. 'packages/spine/src').
	// Individual package sub-projects produce the same names after
	// prefixImportPath is applied. Without deduplication every component
	// appears twice — once from each scan — doubling risk scores, coupling
	// metrics, and component counts.
	seenNS := make(map[string]bool)

	for _, sub := range subs {
		subRoot := filepath.Join(absRoot, sub.relPath)
		sc := ScannerFromRegistry(sub.lang, subRoot)
		if sub.lang == model.LangTypeScript && s.TSFileGranularity {
			sc = &TypeScriptScanner{Granularity: FileLevel}
		}
		if sub.lang == model.LangRust && s.TSFileGranularity {
			sc = &RustScanner{Granularity: FileLevel}
		}
		subProj, err := sc.Scan(subRoot)
		if err != nil {
			continue
		}

		prefix := sub.relPath
		applyPrefix := prefixImportPath
		if sub.lang == model.LangRust {
			applyPrefix = rustImportPath
		}
		for _, ns := range subProj.Namespaces {
			merged := &model.Namespace{
				Name:       ns.Name,
				ImportPath: applyPrefix(prefix, ns.ImportPath),
				Files:      ns.Files,
				Symbols:    ns.Symbols,
			}
			if seenNS[merged.ImportPath] {
				continue // first (root-scan) entry wins; skip sub-project duplicate
			}
			seenNS[merged.ImportPath] = true
			proj.AddNamespace(merged)
		}

		if subProj.DependencyGraph != nil {
			for _, edge := range subProj.DependencyGraph.Edges {
				proj.DependencyGraph.AddEdge(
					applyPrefix(prefix, edge.From),
					applyPrefix(prefix, edge.To),
					edge.External,
				)
			}
		}
	}

	return proj, nil
}

func discoverSubProjects(root string) []subProject {
	var subs []subProject
	seen := make(map[string]bool)

	for _, m := range RootProjectMarkers {
		if _, err := os.Stat(filepath.Join(root, m.File)); err == nil {
			subs = append(subs, subProject{relPath: ".", lang: ToModelLanguage(m.Lang)})
			seen["."] = true
		}
	}

	// subProjectMarkers maps a marker filename to the language it signals.
	// These are walked recursively so polyglot monorepos (e.g. deepagents
	// with pyproject.toml inside libs/) are discovered correctly (LCS-BUG-74).
	subProjectMarkers := map[string]model.Language{
		"package.json":   model.LangTypeScript,
		"tsconfig.json":  model.LangTypeScript,
		"pyproject.toml": model.LangPython,
		"setup.py":       model.LangPython,
		"Cargo.toml":     model.LangRust,
	}

	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if ShouldSkipDir(d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}

		lang, ok := subProjectMarkers[d.Name()]
		if !ok {
			return nil
		}

		subDir := filepath.Dir(path)
		rel, relErr := filepath.Rel(root, subDir)
		if relErr != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		if seen[rel] {
			return nil
		}
		if hasNodeModulesParent(rel) {
			return nil
		}
		seen[rel] = true
		subs = append(subs, subProject{relPath: rel, lang: lang})
		return nil
	})

	return subs
}

func hasNodeModulesParent(rel string) bool {
	for _, seg := range strings.Split(rel, "/") {
		if seg == "node_modules" {
			return true
		}
	}
	return false
}

func prefixImportPath(prefix, importPath string) string {
	if prefix == "." || prefix == "" {
		return importPath
	}
	return prefix + "/" + importPath
}

// rustImportPath computes the namespace import path for a Rust sub-project.
// The Rust scanner uses the crate name (from Cargo.toml) as the import path,
// which equals the directory name for single-crate layouts. Prefixing would
// double the segment ("backend/backend"). We use the sub-project relPath
// directly in that case.
func rustImportPath(subRelPath, nsImportPath string) string {
	if subRelPath == "." || subRelPath == "" {
		return nsImportPath
	}
	if nsImportPath == filepath.Base(subRelPath) {
		return subRelPath
	}
	return subRelPath + "/" + nsImportPath
}
