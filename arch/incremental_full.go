package arch

import (
	"context"
	"path/filepath"
	"strings"

	oculus "github.com/dpopsuev/oculus/v3"
	archgit "github.com/dpopsuev/oculus/v3/arch/git"
	"github.com/dpopsuev/oculus/v3/model"
	"golang.org/x/sync/errgroup"
)

// applyIncrementalFull refreshes coverage/authors/anchors for changedPkgs and
// merges them into baseline extras. Unchanged packages keep baseline values.
func applyIncrementalFull(ctx context.Context, root, modPath string, opts ScanOpts, proj *model.Project, changedPkgs []string, baseline, report *ContextReport) {
	var (
		coverage []archgit.CoverageResult
		authors  map[string][]archgit.Author
		anchors  []oculus.SemanticAnchor
	)
	g, _ := errgroup.WithContext(ctx)

	lang := model.LangUnknown
	if proj != nil {
		lang = proj.Language
	} else if baseline != nil && baseline.Project != nil {
		lang = baseline.Project.Language
	}

	if lang == model.LangGo || lang == model.LangUnknown {
		patterns := PackageDirsToPatterns(changedPkgs)
		if opts.IncludeCoverage {
			g.Go(func() error {
				coverage, _ = archgit.RunGoCoveragePatterns(root, modPath, patterns)
				return nil
			})
		}
		g.Go(func() error {
			anchors = extractAnchorsForPackages(root, proj, modPath, changedPkgs)
			return nil
		})
	}
	if opts.Authors {
		g.Go(func() error {
			authors = archgit.AuthorOwnershipForPackages(root, changedPkgs)
			return nil
		})
	}
	_ = g.Wait()

	baseCov, baseAuth, baseAnch := []archgit.CoverageResult(nil), map[string][]archgit.Author(nil), []oculus.SemanticAnchor(nil)
	if baseline != nil {
		baseCov, baseAuth, baseAnch = baseline.Coverage, baseline.Authors, baseline.Anchors
	}
	if opts.IncludeCoverage {
		report.Coverage = mergeCoverage(baseCov, coverage, changedPkgs)
	} else {
		report.Coverage = baseCov
	}
	if opts.Authors {
		report.Authors = mergeAuthors(baseAuth, authors, changedPkgs)
	} else {
		report.Authors = baseAuth
	}
	report.Anchors = mergeAnchors(baseAnch, anchors, changedPkgs)
}

func extractAnchorsForPackages(root string, proj *model.Project, modPath string, pkgs []string) []oculus.SemanticAnchor {
	if proj == nil {
		return nil
	}
	allow := pkgKeySet(pkgs)
	absRoot, _ := filepath.Abs(root)
	var all []oculus.SemanticAnchor
	for _, ns := range proj.Namespaces {
		rel := shortImportPath(modPath, ns.ImportPath)
		if len(allow) > 0 && !pkgKeyMatches(rel, allow) {
			continue
		}
		pkgDir := filepath.Join(absRoot, rel)
		if rel == "." || rel == "(root)" {
			pkgDir = absRoot
			rel = "."
		}
		all = append(all, oculus.ExtractAnchors(pkgDir, rel)...)
	}
	return all
}

func mergeCoverage(baseline, fresh []archgit.CoverageResult, changedPkgs []string) []archgit.CoverageResult {
	allow := pkgKeySet(changedPkgs)
	byComp := map[string]archgit.CoverageResult{}
	for _, c := range baseline {
		if pkgKeyMatches(c.Component, allow) {
			continue
		}
		byComp[c.Component] = c
	}
	for _, c := range fresh {
		byComp[c.Component] = c
	}
	out := make([]archgit.CoverageResult, 0, len(byComp))
	for _, c := range byComp {
		out = append(out, c)
	}
	return out
}

func mergeAuthors(baseline, fresh map[string][]archgit.Author, changedPkgs []string) map[string][]archgit.Author {
	if baseline == nil && fresh == nil {
		return nil
	}
	out := map[string][]archgit.Author{}
	allow := pkgKeySet(changedPkgs)
	for k, v := range baseline {
		if pkgKeyMatches(k, allow) {
			continue
		}
		out[k] = v
	}
	for k, v := range fresh {
		out[k] = v
	}
	return out
}

func mergeAnchors(baseline, fresh []oculus.SemanticAnchor, changedPkgs []string) []oculus.SemanticAnchor {
	allow := pkgKeySet(changedPkgs)
	var out []oculus.SemanticAnchor
	for _, a := range baseline {
		if pkgKeyMatches(a.Package, allow) {
			continue
		}
		out = append(out, a)
	}
	return append(out, fresh...)
}

func pkgKeySet(pkgs []string) map[string]bool {
	m := map[string]bool{}
	for _, p := range pkgs {
		m[normalizePkgKey(p)] = true
		if b := filepath.Base(p); b != "" && b != "." && b != "(root)" {
			m[b] = true
		}
	}
	return m
}

func pkgKeyMatches(key string, allow map[string]bool) bool {
	if len(allow) == 0 {
		return false
	}
	k := normalizePkgKey(key)
	if allow[k] || allow[filepath.Base(k)] {
		return true
	}
	for a := range allow {
		if strings.HasPrefix(k, a+"/") || strings.HasPrefix(a, k+"/") {
			return true
		}
	}
	return false
}

func normalizePkgKey(p string) string {
	p = filepath.ToSlash(strings.Trim(p, "/"))
	if p == "" || p == "." || p == "(root)" {
		return "."
	}
	return p
}
