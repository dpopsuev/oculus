package survey

import (
	"bufio"
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/BurntSushi/toml"

	"github.com/dpopsuev/oculus/v3/model"
)

// Slog attribute keys.
const (
	logKeyPath    = "path"
	logKeyMember  = "member"
	logKeyDirs    = "dirs"
	logKeyName    = "name"
	logKeyDir     = "dir"
	logKeyCrates  = "crates"
	logKeyMembers = "members"
	logKeyError   = "error"
)

// RustScanner extracts structural metadata from Rust projects by parsing
// Cargo.toml manifests and scanning source files for pub declarations.
// It handles both single-crate and workspace layouts.
type RustScanner struct {
	// Granularity controls crate-level (default) vs per-.rs-file namespaces.
	// FileLevel is required for useful architecture graphs on single-crate repos.
	Granularity Granularity
}

type cargoWorkspace struct {
	Members []string       `toml:"members"`
	Deps    map[string]any `toml:"dependencies"` // [workspace.dependencies]
}

type cargoPackage struct {
	Name string `toml:"name"`
}

type cargoDep struct {
	Path    string
	Version string
}

type cargoManifest struct {
	Workspace *cargoWorkspace        `toml:"workspace"`
	Package   *cargoPackage          `toml:"package"`
	Deps      map[string]any `toml:"dependencies"`
}

func (s *RustScanner) Scan(root string) (*model.Project, error) {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}

	cargoPath := filepath.Join(absRoot, "Cargo.toml")
	slog.LogAttrs(context.Background(), slog.LevelDebug, "rust scanner: reading manifest", slog.String(logKeyPath, cargoPath))

	var manifest cargoManifest
	if _, err := toml.DecodeFile(cargoPath, &manifest); err != nil {
		slog.LogAttrs(context.Background(), slog.LevelDebug, "rust scanner: failed to read Cargo.toml", slog.String(logKeyPath, cargoPath), slog.Any(logKeyError, err))
		return nil, err
	}

	proj := &model.Project{
		Path:            projectName(manifest, absRoot),
		Language:        model.LangRust,
		DependencyGraph: model.NewDependencyGraph(),
	}

	if manifest.Workspace != nil {
		slog.LogAttrs(context.Background(), slog.LevelDebug, "rust scanner: workspace detected", slog.Any(logKeyMembers, manifest.Workspace.Members))
		return s.scanWorkspace(absRoot, manifest, proj)
	}
	slog.LogAttrs(context.Background(), slog.LevelDebug, "rust scanner: single crate mode", slog.String(logKeyName, proj.Path))
	return s.scanSingleCrate(absRoot, manifest, proj)
}

// ScanDirs resurveys Rust crates touched by dirs/changedPaths.
func (s *RustScanner) ScanDirs(root string, dirs, changedPaths []string) (*model.Project, error) {
	if s.Granularity == FileLevel {
		// File-level graphs are denser; full rescan keeps mod/use edges consistent.
		return s.Scan(root)
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	crateDirs := map[string]bool{}
	for _, p := range changedPaths {
		if c := findCrateRoot(absRoot, filepath.Join(absRoot, p)); c != "" {
			crateDirs[c] = true
		}
	}
	for _, d := range dirs {
		cand := d
		if d == nsRoot || d == "." || d == "" {
			cand = absRoot
		} else {
			cand = filepath.Join(absRoot, filepath.FromSlash(d))
		}
		if c := findCrateRoot(absRoot, cand); c != "" {
			crateDirs[c] = true
		}
	}
	if len(crateDirs) == 0 {
		return s.Scan(root)
	}

	proj := &model.Project{
		Path:            filepath.Base(absRoot),
		Language:        model.LangRust,
		DependencyGraph: model.NewDependencyGraph(),
	}
	for crateDir := range crateDirs {
		var cm cargoManifest
		if _, err := toml.DecodeFile(filepath.Join(crateDir, "Cargo.toml"), &cm); err != nil || cm.Package == nil {
			continue
		}
		name := cm.Package.Name
		ns := model.NewNamespace(name, name)
		s.extractRustSymbols(crateDir, ns)
		proj.AddNamespace(ns)
		for depName, depVal := range cm.Deps {
			dep := parseCargoDep(depVal)
			proj.DependencyGraph.AddEdge(name, depName, dep.Path == "")
		}
	}
	return proj, nil
}

func findCrateRoot(absRoot, start string) string {
	dir := start
	if fi, err := os.Stat(dir); err == nil && !fi.IsDir() {
		dir = filepath.Dir(dir)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "Cargo.toml")); err == nil {
			return dir
		}
		if dir == absRoot || dir == "/" || dir == "." || dir == filepath.Dir(dir) {
			return ""
		}
		dir = filepath.Dir(dir)
	}
}

func (s *RustScanner) scanWorkspace(root string, manifest cargoManifest, proj *model.Project) (*model.Project, error) {
	crateNames := make(map[string]bool)
	type crateInfo struct {
		name string
		dir  string
	}
	crates := make([]crateInfo, 0, len(manifest.Workspace.Members))

	for _, member := range manifest.Workspace.Members {
		// Resolve globs (e.g., "crates/*") to actual directories.
		memberDirs, err := resolveWorkspaceMember(root, member)
		if err != nil {
			slog.LogAttrs(context.Background(), slog.LevelDebug, "rust scanner: failed to resolve member", slog.String(logKeyMember, member), slog.Any(logKeyError, err))
			continue
		}
		slog.LogAttrs(context.Background(), slog.LevelDebug, "rust scanner: member resolved", slog.String(logKeyMember, member), slog.Int(logKeyDirs, len(memberDirs)))
		for _, memberDir := range memberDirs {
			var cm cargoManifest
			if _, err := toml.DecodeFile(filepath.Join(memberDir, "Cargo.toml"), &cm); err != nil {
				slog.LogAttrs(context.Background(), slog.LevelDebug, "rust scanner: failed to read member Cargo.toml", slog.String(logKeyDir, memberDir), slog.Any(logKeyError, err))
				continue
			}
			if cm.Package == nil {
				slog.LogAttrs(context.Background(), slog.LevelDebug, "rust scanner: member has no [package] section", slog.String(logKeyDir, memberDir))
				continue
			}
			slog.LogAttrs(context.Background(), slog.LevelDebug, "rust scanner: discovered crate", slog.String(logKeyName, cm.Package.Name), slog.String(logKeyDir, memberDir))
			crateNames[cm.Package.Name] = true
			crates = append(crates, crateInfo{name: cm.Package.Name, dir: memberDir})
		}
	}
	slog.LogAttrs(context.Background(), slog.LevelDebug, "rust scanner: workspace scan complete", slog.Int(logKeyCrates, len(crates)))

	for _, c := range crates {
		var cm cargoManifest
		_, _ = toml.DecodeFile(filepath.Join(c.dir, "Cargo.toml"), &cm)

		if s.Granularity == FileLevel {
			prefix, _ := filepath.Rel(root, c.dir)
			prefix = filepath.ToSlash(prefix)
			if err := s.addFileLevelCrate(c.dir, prefix, &cm, crateNames, proj); err != nil {
				return nil, err
			}
			continue
		}

		ns := model.NewNamespace(c.name, c.name)
		s.extractRustSymbols(c.dir, ns)
		proj.AddNamespace(ns)

		for depName, depVal := range cm.Deps {
			dep := parseCargoDep(depVal)
			if dep.Path != "" || crateNames[depName] {
				proj.DependencyGraph.AddEdge(c.name, depName, false)
			} else {
				proj.DependencyGraph.AddEdge(c.name, depName, true)
			}
		}
	}

	sort.Slice(proj.Namespaces, func(i, j int) bool {
		return proj.Namespaces[i].Name < proj.Namespaces[j].Name
	})
	return proj, nil
}

func (s *RustScanner) scanSingleCrate(root string, manifest cargoManifest, proj *model.Project) (*model.Project, error) {
	if s.Granularity == FileLevel {
		if err := s.addFileLevelCrate(root, "", &manifest, nil, proj); err != nil {
			return nil, err
		}
		sort.Slice(proj.Namespaces, func(i, j int) bool {
			return proj.Namespaces[i].Name < proj.Namespaces[j].Name
		})
		return proj, nil
	}

	name := proj.Path
	ns := model.NewNamespace(name, name)
	s.extractRustSymbols(root, ns)
	proj.AddNamespace(ns)

	for depName, depVal := range manifest.Deps {
		dep := parseCargoDep(depVal)
		if dep.Path != "" {
			proj.DependencyGraph.AddEdge(name, depName, false)
		} else {
			proj.DependencyGraph.AddEdge(name, depName, true)
		}
	}

	return proj, nil
}

// addFileLevelCrate promotes each .rs file under crateDir to its own namespace.
// nsPrefix is prepended to relative paths (workspace member path); empty for
// single-crate roots. crateNames, when non-nil, marks path deps as internal.
func (s *RustScanner) addFileLevelCrate(crateDir, nsPrefix string, manifest *cargoManifest, crateNames map[string]bool, proj *model.Project) error {
	files, err := listRustFiles(crateDir)
	if err != nil {
		return err
	}
	fileSet := make(map[string]bool, len(files))
	nsMap := make(map[string]*model.Namespace, len(files))
	seenSym := make(map[string]map[string]bool, len(files))

	for _, rel := range files {
		nsKey := rel
		if nsPrefix != "" && nsPrefix != "." {
			nsKey = nsPrefix + "/" + rel
		}
		fileSet[rel] = true
		ns := model.NewNamespace(nsKey, nsKey)
		nsMap[rel] = ns
		seenSym[rel] = make(map[string]bool)
		proj.AddNamespace(ns)
	}

	for _, rel := range files {
		ns := nsMap[rel]
		abs := filepath.Join(crateDir, filepath.FromSlash(rel))
		f, err := os.Open(abs)
		if err != nil {
			continue
		}
		lineCount := 0
		sc := bufio.NewScanner(f)
		for sc.Scan() {
			lineCount++
			line := sc.Text()
			matchSymbolPatterns(line, rustSymbolPatterns, ns, seenSym[rel], true, rel, lineCount)
			s.extractRustFileEdges(line, rel, fileSet, nsPrefix, manifest, crateNames, proj.DependencyGraph)
		}
		_ = f.Close()
		fileObj := model.NewFile(rel, ns.Name)
		fileObj.Lines = lineCount
		ns.AddFile(fileObj)
	}
	return nil
}

var (
	reRustModDecl = regexp.MustCompile(`^\s*(?:pub(?:\([^)]*\))?\s+)?mod\s+(\w+)\s*;`)
	reRustUseCrate = regexp.MustCompile(`^\s*(?:pub\s+)?use\s+crate::([\w:]+)`)
	reRustUseSuper = regexp.MustCompile(`^\s*(?:pub\s+)?use\s+super::([\w:]+)`)
	reRustUseExt   = regexp.MustCompile(`^\s*(?:pub\s+)?use\s+([a-z][\w]*)::`)
)

func (s *RustScanner) extractRustFileEdges(line, fromRel string, fileSet map[string]bool, nsPrefix string, manifest *cargoManifest, crateNames map[string]bool, graph *model.DependencyGraph) {
	fromKey := nsKey(nsPrefix, fromRel)

	if m := reRustModDecl.FindStringSubmatch(line); m != nil {
		if target := resolveRustMod(fromRel, m[1], fileSet); target != "" && target != fromRel {
			graph.AddEdge(fromKey, nsKey(nsPrefix, target), false)
		}
		return
	}
	if m := reRustUseCrate.FindStringSubmatch(line); m != nil {
		if target := resolveRustCratePath(m[1], fileSet); target != "" && target != fromRel {
			graph.AddEdge(fromKey, nsKey(nsPrefix, target), false)
		}
		return
	}
	if m := reRustUseSuper.FindStringSubmatch(line); m != nil {
		if target := resolveRustSuperPath(fromRel, m[1], fileSet); target != "" && target != fromRel {
			graph.AddEdge(fromKey, nsKey(nsPrefix, target), false)
		}
		return
	}
	if m := reRustUseExt.FindStringSubmatch(line); m != nil {
		dep := m[1]
		if dep == "crate" || dep == "super" || dep == "self" {
			return
		}
		if target := resolveRustCratePath(dep, fileSet); target != "" {
			if target != fromRel {
				graph.AddEdge(fromKey, nsKey(nsPrefix, target), false)
			}
			return
		}
		external := true
		if crateNames != nil && crateNames[dep] {
			external = false
		} else if manifest != nil {
			if depVal, ok := manifest.Deps[dep]; ok {
				if parseCargoDep(depVal).Path != "" {
					external = false
				}
			}
		}
		graph.AddEdge(fromKey, dep, external)
	}
}

func nsKey(prefix, rel string) string {
	if prefix == "" || prefix == "." {
		return rel
	}
	return prefix + "/" + rel
}

// resolveRustMod maps `mod name;` in fromRel to a sibling .rs path.
func resolveRustMod(fromRel, name string, fileSet map[string]bool) string {
	childDir := rustChildDir(fromRel)
	for _, cand := range []string{
		filepath.ToSlash(filepath.Join(childDir, name+".rs")),
		filepath.ToSlash(filepath.Join(childDir, name, "mod.rs")),
	} {
		if fileSet[cand] {
			return cand
		}
	}
	return ""
}

// rustChildDir is the directory where child modules of fromRel live.
func rustChildDir(fromRel string) string {
	base := filepath.Base(fromRel)
	dir := filepath.ToSlash(filepath.Dir(fromRel))
	if dir == "." {
		dir = ""
	}
	if base == "mod.rs" || base == "lib.rs" || base == "main.rs" {
		return dir
	}
	stem := strings.TrimSuffix(base, ".rs")
	if dir == "" {
		return stem
	}
	return dir + "/" + stem
}

// resolveRustCratePath maps crate::a::b::c to the deepest existing file module.
func resolveRustCratePath(path string, fileSet map[string]bool) string {
	segs := strings.Split(path, "::")
	prefix := "src"
	if !hasSrcRoot(fileSet) {
		prefix = ""
	}
	var best string
	cur := prefix
	for _, seg := range segs {
		if seg == "" {
			continue
		}
		var candidates []string
		if cur == "" {
			candidates = []string{seg + ".rs", seg + "/mod.rs"}
		} else {
			candidates = []string{cur + "/" + seg + ".rs", cur + "/" + seg + "/mod.rs"}
		}
		found := false
		for _, c := range candidates {
			if !fileSet[c] {
				continue
			}
			best = c
			if strings.HasSuffix(c, "/mod.rs") {
				cur = strings.TrimSuffix(c, "/mod.rs")
			} else {
				cur = strings.TrimSuffix(c, ".rs")
			}
			found = true
			break
		}
		if !found {
			break
		}
	}
	return best
}

func hasSrcRoot(fileSet map[string]bool) bool {
	for p := range fileSet {
		if p == "src/lib.rs" || p == "src/main.rs" || strings.HasPrefix(p, "src/") {
			return true
		}
	}
	return false
}

func resolveRustSuperPath(fromRel, path string, fileSet map[string]bool) string {
	var parent string
	for _, c := range rustParentCandidates(fromRel) {
		if fileSet[c] {
			parent = c
			break
		}
	}
	if parent == "" {
		return ""
	}
	if path == "" {
		return parent
	}
	// Resolve remaining path as modules under the parent's child directory,
	// expressed as a crate path relative to src/.
	childDir := rustChildDir(parent)
	rel := strings.TrimPrefix(childDir, "src")
	rel = strings.TrimPrefix(rel, "/")
	full := rel
	for _, seg := range strings.Split(path, "::") {
		if seg == "" {
			continue
		}
		if full == "" {
			full = seg
		} else {
			full += "::" + seg
		}
	}
	if full == "" {
		return parent
	}
	if hit := resolveRustCratePath(full, fileSet); hit != "" {
		return hit
	}
	return parent
}

func rustParentCandidates(fromRel string) []string {
	base := filepath.Base(fromRel)
	dir := filepath.ToSlash(filepath.Dir(fromRel))
	if dir == "." {
		dir = ""
	}
	switch {
	case base == "lib.rs" || base == "main.rs":
		return nil
	case base == "mod.rs":
		// src/foo/mod.rs → src/foo.rs, or src/lib.rs / src/main.rs
		parentDir := filepath.ToSlash(filepath.Dir(dir))
		if parentDir == "." {
			parentDir = ""
		}
		var out []string
		if dir != "" {
			out = append(out, dir+".rs")
		}
		for _, root := range []string{"lib.rs", "main.rs", "mod.rs"} {
			if parentDir == "" {
				out = append(out, "src/"+root, root)
			} else {
				out = append(out, parentDir+"/"+root)
			}
		}
		return out
	default:
		// src/foo/bar.rs → src/foo/mod.rs, src/foo.rs
		if dir == "" {
			return []string{"src/lib.rs", "src/main.rs", "lib.rs", "main.rs"}
		}
		return []string{dir + "/mod.rs", dir + ".rs", "src/lib.rs", "src/main.rs"}
	}
}

func listRustFiles(crateDir string) ([]string, error) {
	var files []string
	err := filepath.WalkDir(crateDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if ShouldSkipDir(d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(d.Name(), ".rs") {
			return nil
		}
		rel, relErr := filepath.Rel(crateDir, path)
		if relErr != nil {
			return nil
		}
		files = append(files, filepath.ToSlash(rel))
		return nil
	})
	sort.Strings(files)
	return files, err
}

var rustSymbolPatterns = []symbolPattern{
	{regexp.MustCompile(`^\s*pub(?:\(crate\))?\s+(?:async\s+)?fn\s+(\w+)`), model.SymbolFunction},
	{regexp.MustCompile(`^\s*pub(?:\(crate\))?\s+struct\s+(\w+)`), model.SymbolStruct},
	{regexp.MustCompile(`^\s*pub(?:\(crate\))?\s+enum\s+(\w+)`), model.SymbolEnum},
	{regexp.MustCompile(`^\s*pub(?:\(crate\))?\s+trait\s+(\w+)`), model.SymbolInterface},
	{regexp.MustCompile(`^\s*pub(?:\(crate\))?\s+const\s+(\w+)`), model.SymbolConstant},
	{regexp.MustCompile(`^\s*pub(?:\(crate\))?\s+type\s+(\w+)`), model.SymbolTypeParameter},
}

func (s *RustScanner) extractRustSymbols(crateDir string, ns *model.Namespace) {
	seen := make(map[string]bool)
	_ = filepath.WalkDir(crateDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if ShouldSkipDir(d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(d.Name(), ".rs") {
			return nil
		}

		rel, relErr := filepath.Rel(crateDir, path)
		if relErr != nil {
			rel = path
		}
		fileObj := model.NewFile(filepath.ToSlash(rel), ns.Name)

		f, fErr := os.Open(path)
		if fErr != nil {
			return nil
		}
		defer f.Close()

		lineCount := 0
		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			lineCount++
			matchSymbolPatterns(scanner.Text(), rustSymbolPatterns, ns, seen, true, filepath.ToSlash(rel), lineCount)
		}
		fileObj.Lines = lineCount
		ns.AddFile(fileObj)
		return nil
	})
}

// resolveWorkspaceMember expands a workspace member pattern to actual directories.
// Handles both literal paths ("crate-a") and globs ("crates/*").
func resolveWorkspaceMember(root, member string) ([]string, error) {
	pattern := filepath.Join(root, member)
	// If no glob characters, return as-is (literal path).
	if !strings.ContainsAny(member, "*?[") {
		return []string{pattern}, nil
	}
	// Expand glob pattern.
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return nil, err
	}
	// Filter to directories only (Cargo members must be directories).
	dirs := make([]string, 0, len(matches))
	for _, m := range matches {
		info, statErr := os.Stat(m)
		if statErr == nil && info.IsDir() {
			dirs = append(dirs, m)
		}
	}
	sort.Strings(dirs)
	return dirs, nil
}

func parseCargoDep(v any) cargoDep {
	switch val := v.(type) {
	case string:
		return cargoDep{Version: val}
	case map[string]any:
		d := cargoDep{}
		if p, ok := val["path"].(string); ok {
			d.Path = p
		}
		if ver, ok := val["version"].(string); ok {
			d.Version = ver
		}
		return d
	default:
		return cargoDep{}
	}
}

func projectName(m cargoManifest, root string) string {
	if m.Package != nil && m.Package.Name != "" {
		return m.Package.Name
	}
	return filepath.Base(root)
}

// ScanFile implements FileScanner for a single Rust source file.
// It returns a Project with one namespace named after the file's stem.
func (s *RustScanner) ScanFile(path string) (*model.Project, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}

	stem := strings.TrimSuffix(filepath.Base(absPath), ".rs")
	ns := model.NewNamespace(stem, stem)

	f, err := os.Open(absPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	seen := make(map[string]bool)
	lineCount := 0
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		lineCount++
		matchSymbolPatterns(sc.Text(), rustSymbolPatterns, ns, seen, true, filepath.Base(absPath), lineCount)
	}
	fileObj := model.NewFile(filepath.Base(absPath), stem)
	fileObj.Lines = lineCount
	ns.AddFile(fileObj)

	proj := &model.Project{
		Path:            stem,
		Language:        model.LangRust,
		DependencyGraph: model.NewDependencyGraph(),
	}
	proj.AddNamespace(ns)
	return proj, nil
}
