package arch

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"

	oculus "github.com/dpopsuev/oculus/v3"
	"github.com/dpopsuev/oculus/v3/graph"
	"github.com/dpopsuev/oculus/v3/model"
	"github.com/dpopsuev/oculus/v3/survey"
)

const (
	ScanModeFull  = "full"
	ScanModeMerge = "merge"
)

// ForceFullScanReasons that RequireFullScan may return.
const (
	ReasonNoBaseline     = "no_baseline"
	ReasonManifestChange = "manifest_change"
	ReasonTooManyPkgs    = "too_many_packages"
	ReasonUnsupportedLang = "unsupported_lang"
	ReasonEmptyChanged   = "empty_changed"
)

// RequireFullScan returns true when package-merge is unsafe.
func RequireFullScan(baseline *ContextReport, changedPaths, changedPkgs []string) (bool, string) {
	if baseline == nil || baseline.Project == nil {
		return true, ReasonNoBaseline
	}
	if len(changedPkgs) == 0 && len(changedPaths) == 0 {
		return true, ReasonEmptyChanged
	}
	if !mergeableLanguage(baseline.Project.Language, changedPaths) {
		return true, ReasonUnsupportedLang
	}
	for _, p := range changedPaths {
		base := filepath.Base(p)
		switch base {
		case "go.mod", "go.sum", "package.json", "package-lock.json", "yarn.lock", "pnpm-lock.yaml",
			"Cargo.toml", "Cargo.lock", "pyproject.toml", "poetry.lock", "requirements.txt":
			return true, ReasonManifestChange
		}
	}
	nSvc := len(baseline.Architecture.Services)
	limit := 8
	if nSvc > 0 {
		pct := nSvc / 4 // 25%
		if pct > limit {
			limit = pct
		}
	}
	if len(changedPkgs) > limit {
		return true, ReasonTooManyPkgs
	}
	return false, ""
}

func mergeableLanguage(lang model.Language, changedPaths []string) bool {
	switch lang {
	case model.LangGo, model.LangTypeScript, model.LangPython, model.LangRust, model.LangUnknown:
		return true
	}
	// Baseline language exotic — still mergeable if changed files are known langs.
	for _, p := range changedPaths {
		if survey.LangFromFilename(p) != model.LangUnknown {
			return true
		}
	}
	return false
}

// PackageDirsToPatterns maps slash package dirs from cache.Packages to go/packages patterns.
func PackageDirsToPatterns(pkgs []string) []string {
	var out []string
	for _, p := range pkgs {
		p = filepath.ToSlash(p)
		if p == "" || p == "(root)" {
			out = append(out, ".")
			continue
		}
		out = append(out, "./"+strings.TrimPrefix(p, "./"))
	}
	return out
}

// MergeScan rebuilds only changed packages into baseline when safe.
// On force-full it runs ScanAndBuild and returns ScanModeFull.
func MergeScan(ctx context.Context, root string, baseline *ContextReport, changedPaths []string, opts ScanOpts) (*ContextReport, error) {
	changedPkgs := uniquePackageDirs(changedPaths)
	if force, reason := RequireFullScan(baseline, changedPaths, changedPkgs); force {
		slog.LogAttrs(ctx, slog.LevelInfo, "merge: falling back to full scan",
			slog.String("reason", reason),
			slog.Int("changed_pkgs", len(changedPkgs)),
		)
		opts.Since = ""
		report, err := ScanAndBuild(ctx, root, opts)
		if err != nil {
			return nil, err
		}
		if report.ScanMode == "" {
			report.ScanMode = ScanModeFull
		}
		markServicesChanged(report, changedPkgs)
		return report, nil
	}

	lang := baseline.Project.Language
	partial, err := survey.PartialScan(root, lang, changedPkgs, changedPaths)
	if err != nil {
		slog.LogAttrs(ctx, slog.LevelWarn, "merge: scoped survey failed, full scan",
			slog.Any("error", err))
		opts.Since = ""
		report, err2 := ScanAndBuild(ctx, root, opts)
		if err2 != nil {
			return nil, err2
		}
		report.ScanMode = ScanModeFull
		markServicesChanged(report, changedPkgs)
		return report, nil
	}

	mergedProj := mergeProjects(baseline.Project, partial, baseline.ModulePath, changedPkgs)
	intent := opts.Intent.normalize()
	modPath := baseline.ModulePath
	if modPath == "" {
		modPath = DetectProjectPath(root)
	}
	if modPath == "" {
		modPath = mergedProj.Path
	}

	syncOpts := SyncOptions{
		ModulePath:      modPath,
		ExcludeTests:    opts.ExcludeTests,
		IncludeExternal: opts.IncludeExternal,
	}
	grouped := opts.Grouped
	depth := opts.Depth
	if depth > 0 {
		grouped = true
	}
	if grouped {
		d := depth
		if d == 0 {
			d = DefaultGroupingDepth
		}
		syncOpts.Groups = InferDefaultGroups(mergedProj, modPath, d)
	}
	if intent.IncludesHealth() && opts.ChurnDays > 0 {
		syncOpts.ChurnData = nil // keep baseline churn; avoid full git walk on merge
	}

	archModel := ProjectToArchModel(mergedProj, syncOpts)
	populateLOC(root, mergedProj, modPath, &archModel)

	report := &ContextReport{
		ScanCore: ScanCore{
			Project:      mergedProj,
			Architecture: archModel,
			ModulePath:   modPath,
			Scanner:      baseline.Scanner,
			ScanMode:     ScanModeMerge,
			Languages:    inventoryLanguageNames(root),
		},
	}
	if report.Scanner == "" {
		report.Scanner = scannerNameForLang(lang)
	}
	report.SuggestedDepth = baseline.SuggestedDepth
	markServicesChanged(report, changedPkgs)

	if !intent.IncludesCoupling() {
		return report, nil
	}

	spots := computeHotSpots(archModel)
	if spots == nil {
		spots = []oculus.HotSpot{}
	}
	report.HotSpots = EnrichHotSpotsComplexity(root, spots, MaxHotSpotsMarkdown)
	cycles := graph.DetectCycles(archModel.Edges)
	if cycles == nil {
		cycles = []graph.Cycle{}
	}
	report.Cycles = cycles
	if groups := graph.StronglyConnectedComponents(archModel.Edges); len(groups) > 0 {
		report.CycleGroups = groups
	}
	report.ImportDepth = graph.ImportDepth(archModel.Edges)
	report.APISurfaces = ComputeAPISurface(archModel)
	report.BoundaryCrossings = DetectBoundaryCrossings(archModel, nil)
	report.FanIn = graph.FanIn(archModel.Edges)
	report.FanOut = graph.FanOut(archModel.Edges)

	// Preserve health extras from baseline; refresh full extras incrementally.
	if intent.IncludesHealth() {
		report.RecentCommits = baseline.RecentCommits
		report.FileHotSpots = baseline.FileHotSpots
	}
	if intent.IncludesFull() {
		fullOpts := opts
		fullOpts.Authors = true
		fullOpts.IncludeCoverage = true
		applyIncrementalFull(ctx, root, modPath, fullOpts, mergedProj, changedPkgs, baseline, report)
	} else if intent.IncludesHealth() {
		report.Authors = baseline.Authors
		report.Coverage = baseline.Coverage
		report.Anchors = baseline.Anchors
	}
	return report, nil
}

func scannerNameForLang(lang model.Language) string {
	switch lang {
	case model.LangGo:
		return "packages"
	case model.LangTypeScript:
		return "typescript"
	case model.LangPython:
		return "python"
	case model.LangRust:
		return "rust"
	default:
		return "auto"
	}
}

func uniquePackageDirs(changedPaths []string) []string {
	set := map[string]bool{}
	var out []string
	for _, p := range changedPaths {
		p = filepath.ToSlash(p)
		dir := filepath.ToSlash(filepath.Dir(p))
		if dir == "." || dir == "" {
			// file at root — package is (root) unless it's a manifest
			base := filepath.Base(p)
			if base == "go.mod" || base == "go.sum" {
				continue
			}
			dir = "(root)"
		}
		if !set[dir] {
			set[dir] = true
			out = append(out, dir)
		}
	}
	return out
}

func markServicesChanged(report *ContextReport, changedPkgs []string) {
	if report == nil || len(changedPkgs) == 0 {
		return
	}
	set := map[string]bool{}
	for _, p := range changedPkgs {
		set[p] = true
		set[filepath.Base(p)] = true
	}
	for i := range report.Architecture.Services {
		name := report.Architecture.Services[i].Name
		if set[name] || set[filepath.ToSlash(name)] {
			report.Architecture.Services[i].Changed = true
		}
	}
}

// mergeProjects keeps baseline namespaces not in changedPkgs, replaces with partial.
func mergeProjects(baseline, partial *model.Project, modPath string, changedPkgs []string) *model.Project {
	if baseline == nil {
		return partial
	}
	changedSet := map[string]bool{}
	for _, p := range changedPkgs {
		changedSet[p] = true
		changedSet[filepath.Base(p)] = true
	}
	isChanged := func(importPath string) bool {
		rel := shortImportPath(modPath, importPath)
		if rel == "." || rel == "" {
			rel = "(root)"
		}
		return changedSet[rel] || changedSet[filepath.Base(rel)]
	}

	out := model.NewProject(baseline.Path)
	out.Language = baseline.Language
	if out.Language == model.LangUnknown && partial != nil && partial.Language != model.LangUnknown {
		out.Language = partial.Language
	}
	out.DependencyGraph = model.NewDependencyGraph()

	for _, ns := range baseline.Namespaces {
		if isChanged(ns.ImportPath) {
			continue
		}
		out.AddNamespace(ns)
	}
	changedImports := map[string]bool{}
	for _, ns := range partial.Namespaces {
		out.AddNamespace(ns)
		changedImports[ns.ImportPath] = true
	}

	// Edges: keep baseline edges that don't touch changed packages; add partial edges.
	if baseline.DependencyGraph != nil {
		for _, e := range baseline.DependencyGraph.Edges {
			if isChanged(e.From) || isChanged(e.To) || changedImports[e.From] || changedImports[e.To] {
				continue
			}
			out.DependencyGraph.AddEdge(e.From, e.To, e.External)
			if e.CallSites > 0 || e.LOCSurface > 0 {
				out.DependencyGraph.SetEdgeCoupling(e.From, e.To, e.CallSites, e.LOCSurface)
			}
		}
	}
	if partial.DependencyGraph != nil {
		for _, e := range partial.DependencyGraph.Edges {
			out.DependencyGraph.AddEdge(e.From, e.To, e.External)
			if e.CallSites > 0 || e.LOCSurface > 0 {
				out.DependencyGraph.SetEdgeCoupling(e.From, e.To, e.CallSites, e.LOCSurface)
			}
		}
	}
	return out
}

// MergeOrFull is used by incrementalScan / ScanProject when a baseline exists.
func MergeOrFull(ctx context.Context, root string, baseline *ContextReport, changedPaths []string, opts ScanOpts) (*ContextReport, error) {
	if baseline == nil {
		opts.Since = ""
		r, err := ScanAndBuild(ctx, root, opts)
		if err != nil {
			return nil, err
		}
		r.ScanMode = ScanModeFull
		return r, nil
	}
	return MergeScan(ctx, root, baseline, changedPaths, opts)
}

// ErrMergeUnavailable is reserved for callers.
var ErrMergeUnavailable = fmt.Errorf("merge unavailable")
