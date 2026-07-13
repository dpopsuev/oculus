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
type RustScanner struct{}

type cargoWorkspace struct {
	Members []string `toml:"members"`
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
