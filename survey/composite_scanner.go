package survey

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"

	"github.com/dpopsuev/oculus/v3/lang"
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

	repairMergedDependencyGraph(proj, absRoot, subs)
	return proj, nil
}

// repairMergedDependencyGraph fixes post-merge coupling gaps common in polyglot
// repos (ShojiWM-style): Cargo path-deps remapped away from nsSet, file-ish
// TypeScript targets that should collapse to dir namespaces, and external edges
// whose To already exists as a merged namespace.
func repairMergedDependencyGraph(proj *model.Project, absRoot string, subs []subProject) {
	if proj == nil || proj.DependencyGraph == nil {
		return
	}
	nsSet := make(map[string]bool, len(proj.Namespaces))
	for _, ns := range proj.Namespaces {
		nsSet[ns.ImportPath] = true
	}

	crateToIP := buildRustCrateImportPathMap(absRoot, subs, nsSet)
	emitRustCargoPathEdges(proj, absRoot, subs, crateToIP, nsSet)
	normalizeMergedEdges(proj, nsSet)
}

// buildRustCrateImportPathMap maps Cargo package.name → final merged ImportPath.
func buildRustCrateImportPathMap(absRoot string, subs []subProject, nsSet map[string]bool) map[string]string {
	out := make(map[string]string)
	for _, sub := range subs {
		if sub.lang != model.LangRust {
			continue
		}
		var cm cargoManifest
		cargoPath := filepath.Join(absRoot, sub.relPath, "Cargo.toml")
		if _, err := toml.DecodeFile(cargoPath, &cm); err != nil {
			continue
		}
		registerRustManifestCrates(absRoot, sub.relPath, &cm, nsSet, out)
	}
	return out
}

func registerRustManifestCrates(absRoot, subRel string, cm *cargoManifest, nsSet map[string]bool, out map[string]string) {
	if cm.Package != nil && cm.Package.Name != "" {
		pickRustCrateImportPath(cm.Package.Name, subRel, nsSet, out)
	}
	if cm.Workspace == nil {
		return
	}
	for _, member := range cm.Workspace.Members {
		memberDirs, err := resolveWorkspaceMember(filepath.Join(absRoot, subRel), member)
		if err != nil {
			continue
		}
		for _, memberDir := range memberDirs {
			var mcm cargoManifest
			if _, err := toml.DecodeFile(filepath.Join(memberDir, "Cargo.toml"), &mcm); err != nil || mcm.Package == nil {
				continue
			}
			rel, err := filepath.Rel(absRoot, memberDir)
			if err != nil {
				continue
			}
			pickRustCrateImportPath(mcm.Package.Name, filepath.ToSlash(rel), nsSet, out)
		}
	}
}

func pickRustCrateImportPath(crateName, subRel string, nsSet map[string]bool, out map[string]string) {
	if crateName == "" {
		return
	}
	candidates := []string{crateName, rustImportPath(subRel, crateName)}
	for _, c := range candidates {
		if nsSet[c] {
			out[crateName] = c
			return
		}
	}
	// Prefer crate-name when present after workspace scan; else remapped path.
	if nsSet[crateName] {
		out[crateName] = crateName
		return
	}
	out[crateName] = rustImportPath(subRel, crateName)
}

func emitRustCargoPathEdges(proj *model.Project, absRoot string, subs []subProject, crateToIP map[string]string, nsSet map[string]bool) {
	add := func(fromCrate, toCrate string) {
		from, okFrom := crateToIP[fromCrate]
		to, okTo := crateToIP[toCrate]
		if !okFrom || !okTo {
			return
		}
		if !nsSet[from] || !nsSet[to] || from == to {
			return
		}
		proj.DependencyGraph.AddEdge(from, to, false)
	}

	for _, sub := range subs {
		if sub.lang != model.LangRust {
			continue
		}
		var cm cargoManifest
		cargoPath := filepath.Join(absRoot, sub.relPath, "Cargo.toml")
		if _, err := toml.DecodeFile(cargoPath, &cm); err != nil {
			continue
		}
		emitManifestPathDeps(&cm, absRoot, sub.relPath, crateToIP, add)

		if cm.Workspace == nil {
			continue
		}
		for _, member := range cm.Workspace.Members {
			memberDirs, err := resolveWorkspaceMember(filepath.Join(absRoot, sub.relPath), member)
			if err != nil {
				continue
			}
			for _, memberDir := range memberDirs {
				var mcm cargoManifest
				if _, err := toml.DecodeFile(filepath.Join(memberDir, "Cargo.toml"), &mcm); err != nil {
					continue
				}
				rel, _ := filepath.Rel(absRoot, memberDir)
				emitManifestPathDeps(&mcm, absRoot, filepath.ToSlash(rel), crateToIP, add)
			}
		}
	}
}

func emitManifestPathDeps(cm *cargoManifest, absRoot, crateRel string, crateToIP map[string]string, add func(from, to string)) {
	fromName := ""
	if cm.Package != nil {
		fromName = cm.Package.Name
	}
	if fromName == "" {
		return
	}
	deps := cm.Deps
	if deps == nil && cm.Workspace != nil {
		deps = cm.Workspace.Deps
	}
	for depName, depVal := range deps {
		dep := parseCargoDep(depVal)
		if dep.Path == "" {
			// Workspace crate referenced by name (no path=) still couples.
			if _, ok := crateToIP[depName]; ok {
				add(fromName, depName)
			}
			continue
		}
		// Resolve path dep to a package name when possible.
		depDir := filepath.Clean(filepath.Join(absRoot, crateRel, dep.Path))
		var dcm cargoManifest
		if _, err := toml.DecodeFile(filepath.Join(depDir, "Cargo.toml"), &dcm); err == nil && dcm.Package != nil {
			add(fromName, dcm.Package.Name)
			continue
		}
		if _, ok := crateToIP[depName]; ok {
			add(fromName, depName)
		}
	}
}

// normalizeMergedEdges collapses file-ish targets onto dir namespaces and
// promotes external edges whose endpoints both exist in nsSet to internal.
func normalizeMergedEdges(proj *model.Project, nsSet map[string]bool) {
	old := proj.DependencyGraph.Edges
	ng := model.NewDependencyGraph()
	for _, e := range old {
		from := normalizeEdgeEndpoint(e.From, nsSet)
		to := e.To
		external := e.External
		if norm := normalizeEdgeEndpoint(e.To, nsSet); nsSet[norm] {
			to = norm
		}
		if nsSet[from] && nsSet[to] {
			external = false
		}
		if !external && (!nsSet[from] || !nsSet[to]) {
			continue // orphan internal — drop
		}
		for w := 0; w < e.Weight; w++ {
			ng.AddEdge(from, to, external)
		}
	}
	proj.DependencyGraph = ng
}

func normalizeEdgeEndpoint(ep string, nsSet map[string]bool) string {
	if nsSet[ep] {
		return ep
	}
	cur := ep
	for {
		i := strings.LastIndex(cur, "/")
		if i <= 0 {
			return ep
		}
		cur = cur[:i]
		if nsSet[cur] {
			return cur
		}
	}
}

func discoverSubProjects(root string) []subProject {
	var subs []subProject
	// Key by path+language so co-located manifests (Cargo.toml + package.json
	// at ".") both survive — path-only seen dropped the second language.
	seen := make(map[string]bool)
	mark := func(rel string, lang model.Language) bool {
		key := fmt.Sprintf("%s\x00%d", rel, lang)
		if seen[key] {
			return false
		}
		seen[key] = true
		return true
	}

	for _, m := range RootProjectMarkers {
		if _, err := os.Stat(filepath.Join(root, m.File)); err == nil {
			lang := ToModelLanguage(m.Lang)
			if mark(".", lang) {
				subs = append(subs, subProject{relPath: ".", lang: lang})
			}
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
		if hasNodeModulesParent(rel) {
			return nil
		}
		if !mark(rel, lang) {
			return nil
		}
		subs = append(subs, subProject{relPath: rel, lang: lang})
		return nil
	})

	return subs
}

// IsPolyglot reports whether auto-scan would select CompositeScanner for root:
// language inventory finds ≥2 languages, or discoverSubProjects finds ≥2 subs.
func IsPolyglot(root string) bool {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		absRoot = root
	}
	if lang.InventoryLanguages(absRoot).IsMultiLanguage() {
		return true
	}
	return len(discoverSubProjects(absRoot)) > 1
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
