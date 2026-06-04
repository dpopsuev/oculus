package survey

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/BurntSushi/toml"

	"github.com/dpopsuev/oculus/v3/model"
)

// PythonScanner extracts structural metadata from Python projects by parsing
// pyproject.toml (or setup.py) for project metadata and dependencies, and
// scanning .py source files for import/def/class declarations. Zero external
// tool dependency.
type PythonScanner struct{}

func (s *PythonScanner) Scan(root string) (*model.Project, error) {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}

	projName := detectPythonProjectName(absRoot)
	proj := &model.Project{
		Path:            projName,
		Language:        model.LangPython,
		DependencyGraph: model.NewDependencyGraph(),
	}

	externalDeps := detectPythonDependencies(absRoot)

	packages := discoverPythonPackages(absRoot)

	pkgSet := make(map[string]bool, len(packages))
	for _, pkg := range packages {
		pkgSet[pkg] = true
	}

	// Build a Python-import-path → namespace-import-path index.
	// This handles nested layouts like libs/<pkg>/<pkg>/ where the filesystem
	// prefix does not match the Python import path (LCS-BUG-74).
	pyIndex := buildPyImportIndex(packages)

	sort.Strings(packages)
	for _, pkgPath := range packages {
		relPath := pkgPath
		importPath := strings.ReplaceAll(relPath, "/", ".")
		ns := model.NewNamespace(filepath.Base(pkgPath), importPath)

		fullDir := filepath.Join(absRoot, pkgPath)
		s.extractPythonSymbols(fullDir, ns)
		s.extractPythonImports(fullDir, ns, importPath, pkgSet, externalDeps, proj, pyIndex)
		proj.AddNamespace(ns)
	}

	return proj, nil
}

// --- Project metadata detection ---

type pyprojectTOML struct {
	Project struct {
		Name         string   `toml:"name"`
		Dependencies []string `toml:"dependencies"`
	} `toml:"project"`
	Tool struct {
		Poetry struct {
			Name         string                 `toml:"name"`
			Dependencies map[string]any `toml:"dependencies"`
		} `toml:"poetry"`
	} `toml:"tool"`
}

func detectPythonProjectName(root string) string {
	var pp pyprojectTOML
	if _, err := toml.DecodeFile(filepath.Join(root, "pyproject.toml"), &pp); err == nil {
		if pp.Project.Name != "" {
			return pp.Project.Name
		}
		if pp.Tool.Poetry.Name != "" {
			return pp.Tool.Poetry.Name
		}
	}

	if data, err := os.ReadFile(filepath.Join(root, "setup.py")); err == nil {
		if name := parseSetupPyName(string(data)); name != "" {
			return name
		}
	}

	return filepath.Base(root)
}

var reSetupName = regexp.MustCompile(`name\s*=\s*['"]([^'"]+)['"]`)

func parseSetupPyName(content string) string {
	if m := reSetupName.FindStringSubmatch(content); m != nil {
		return m[1]
	}
	return ""
}

func detectPythonDependencies(root string) map[string]bool {
	deps := make(map[string]bool)

	var pp pyprojectTOML
	if _, err := toml.DecodeFile(filepath.Join(root, "pyproject.toml"), &pp); err == nil {
		for _, dep := range pp.Project.Dependencies {
			name := normalizePipDep(dep)
			if name != "" {
				deps[name] = true
			}
		}
		for name := range pp.Tool.Poetry.Dependencies {
			if name != "python" {
				deps[normalizePkgName(name)] = true
			}
		}
	}

	if f, err := os.Open(filepath.Join(root, "requirements.txt")); err == nil {
		defer f.Close()
		sc := bufio.NewScanner(f)
		for sc.Scan() {
			line := strings.TrimSpace(sc.Text())
			if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "-") {
				continue
			}
			name := normalizePipDep(line)
			if name != "" {
				deps[name] = true
			}
		}
	}

	return deps
}

func normalizePipDep(dep string) string {
	dep = strings.TrimSpace(dep)
	for _, sep := range []string{">=", "<=", "==", "!=", "~=", ">", "<", "[", ";"} {
		if idx := strings.Index(dep, sep); idx > 0 {
			dep = dep[:idx]
		}
	}
	return normalizePkgName(strings.TrimSpace(dep))
}

func normalizePkgName(name string) string {
	return strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(name, "-", "_"), ".", "_"))
}

// --- Package discovery ---

func discoverPythonPackages(root string) []string {
	var packages []string

	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			return nil
		}

		base := d.Name()
		if ShouldSkipPythonDir(base) {
			return filepath.SkipDir
		}

		if path == root {
			return nil
		}

		rel, err := filepath.Rel(root, path)
		if err != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)

		if hasPythonFiles(path) {
			packages = append(packages, rel)
		}
		return nil
	})

	if len(packages) == 0 && hasPythonFiles(root) {
		packages = append(packages, ".")
	}

	return packages
}

func hasPythonFiles(dir string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".py") {
			return true
		}
	}
	return false
}

// --- Symbol extraction ---

var (
	rePyDef      = regexp.MustCompile(`^def\s+(\w+)\s*\(`)
	rePyAsyncDef = regexp.MustCompile(`^async\s+def\s+(\w+)\s*\(`)
	rePyClass    = regexp.MustCompile(`^class\s+(\w+)[\s:(]`)
)

func (s *PythonScanner) extractPythonSymbols(dir string, ns *model.Namespace) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}

	seen := make(map[string]bool)
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".py") {
			continue
		}

		fullPath := filepath.Join(dir, entry.Name())
		fileObj := model.NewFile(entry.Name(), ns.Name)

		f, err := os.Open(fullPath)
		if err != nil {
			continue
		}

		lineCount := 0
		sc := bufio.NewScanner(f)
		for sc.Scan() {
			lineCount++
			line := sc.Text()
			addPythonSymbolFromLine(ns, seen, line, entry.Name(), lineCount)
		}
		f.Close()
		fileObj.Lines = lineCount
		ns.AddFile(fileObj)
	}
}

// addPythonSymbolFromLine matches the first Python declaration on line and
// records the symbol. At most one symbol per line (def/async def/class).
func addPythonSymbolFromLine(ns *model.Namespace, seen map[string]bool, line, filePath string, lineNum int) {
	if m := rePyDef.FindStringSubmatch(line); m != nil {
		addPythonSymbol(ns, seen, m[1], model.SymbolFunction, filePath, lineNum)
		return
	}
	if m := rePyAsyncDef.FindStringSubmatch(line); m != nil {
		addPythonSymbol(ns, seen, m[1], model.SymbolFunction, filePath, lineNum)
		return
	}
	if m := rePyClass.FindStringSubmatch(line); m != nil {
		addPythonSymbol(ns, seen, m[1], model.SymbolClass, filePath, lineNum)
	}
}

func addPythonSymbol(ns *model.Namespace, seen map[string]bool, name string, kind model.SymbolKind, filePath string, lineNum int) {
	if seen[name] {
		return
	}
	seen[name] = true
	ns.AddSymbol(&model.Symbol{
		Name:     name,
		Kind:     kind,
		Exported: !strings.HasPrefix(name, "_"),
		File:     filePath,
		Line:     lineNum,
	})
}

// --- Import extraction ---

var (
	rePyImport     = regexp.MustCompile(`^import\s+(\S+)`)
	rePyFromImport = regexp.MustCompile(`^from\s+(\S+)\s+import\s+`)
)

// buildPyImportIndex constructs a mapping from Python import path (slash-separated)
// to the namespace import path (dot-separated) used in dependency edges.
//
// It detects namespace roots — packages whose parent directory is NOT itself a
// package — and maps all Python-visible paths relative to each root. This
// correctly handles nested monorepo layouts like libs/<pkg>/<pkg>/ where a
// Python import `from pkg.sub import X` must resolve to the internal namespace
// `libs.pkg.pkg.sub`, not to an external dependency named `pkg`.
func buildPyImportIndex(packages []string) map[string]string {
	pkgSet := make(map[string]bool, len(packages))
	for _, p := range packages {
		pkgSet[p] = true
	}

	index := make(map[string]string)
	for _, pkg := range packages {
		parent := filepath.Dir(pkg)
		if parent == "." || !pkgSet[parent] {
			// pkg is a namespace root; its Python name is its directory base name.
			pyRoot := filepath.Base(pkg)
			fsRoot := pkg
			index[pyRoot] = strings.ReplaceAll(pkg, "/", ".")
			for _, other := range packages {
				if other != pkg && strings.HasPrefix(other, fsRoot+"/") {
					rel := other[len(fsRoot)+1:]
					pyPath := pyRoot + "/" + rel
					index[pyPath] = strings.ReplaceAll(other, "/", ".")
				}
			}
		}
	}
	return index
}

// resolvePyImport maps an import key (slash-separated module path) to the
// corresponding namespace import path (dot-separated) using the pyIndex first
// (handles nested layouts) and the pkgSet as a fallback (flat layouts).
func resolvePyImport(importKey string, pyIndex map[string]string, pkgSet map[string]bool) string {
	// Exact match in the Python import index.
	if ns, ok := pyIndex[importKey]; ok {
		return ns
	}
	// Longest-prefix match in the Python import index.
	best, bestNs := "", ""
	for pyPath, ns := range pyIndex {
		if strings.HasPrefix(importKey, pyPath+"/") && len(pyPath) > len(best) {
			best, bestNs = pyPath, ns
		}
	}
	if bestNs != "" {
		return bestNs
	}
	// Fallback: plain pkgSet matching (flat layouts already indexed above,
	// but kept as safety net).
	if matchesInternalPackage(importKey, pkgSet) {
		return resolveToNamespace(importKey, pkgSet)
	}
	return ""
}

func (s *PythonScanner) extractPythonImports(dir string, _ *model.Namespace, importPath string, internalPkgs, _ map[string]bool, proj *model.Project, pyIndex map[string]string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}

	seen := make(map[string]bool)
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".py") {
			continue
		}

		fullPath := filepath.Join(dir, entry.Name())
		f, err := os.Open(fullPath)
		if err != nil {
			continue
		}

		sc := bufio.NewScanner(f)
		for sc.Scan() {
			line := strings.TrimSpace(sc.Text())
			module := parsePyImportModule(line)
			if module == "" || strings.HasPrefix(module, ".") {
				continue
			}

			topLevel := strings.SplitN(module, ".", 2)[0]
			internalKey := strings.ReplaceAll(module, ".", "/")
			if seen[internalKey] {
				continue
			}
			seen[internalKey] = true

			if target := resolvePyImport(internalKey, pyIndex, internalPkgs); target != "" {
				if target != importPath {
					proj.DependencyGraph.AddEdge(importPath, target, false)
				}
				continue
			}
			normalized := normalizePkgName(topLevel)
			if !seen["ext:"+normalized] {
				seen["ext:"+normalized] = true
				proj.DependencyGraph.AddEdge(importPath, normalized, true)
			}
		}
		f.Close()
	}
}

// parsePyImportModule extracts the module name from a Python import line.
// Returns "" for unrecognised lines.
func parsePyImportModule(line string) string {
	if m := rePyFromImport.FindStringSubmatch(line); m != nil {
		return m[1]
	}
	if m := rePyImport.FindStringSubmatch(line); m != nil {
		return m[1]
	}
	return ""
}

// resolveToNamespace maps an import key (e.g. "domain/entity") to its
// enclosing namespace (e.g. "domain") by finding the longest matching
// package prefix. This ensures dependency edges target known namespaces.
func resolveToNamespace(importKey string, pkgSet map[string]bool) string {
	if pkgSet[importKey] {
		return strings.ReplaceAll(importKey, "/", ".")
	}
	best := ""
	for pkg := range pkgSet {
		if strings.HasPrefix(importKey, pkg+"/") && len(pkg) > len(best) {
			best = pkg
		}
	}
	if best != "" {
		return strings.ReplaceAll(best, "/", ".")
	}
	return strings.ReplaceAll(importKey, "/", ".")
}

func matchesInternalPackage(importKey string, pkgSet map[string]bool) bool {
	if pkgSet[importKey] {
		return true
	}
	for pkg := range pkgSet {
		if strings.HasPrefix(importKey, pkg+"/") || strings.HasPrefix(pkg, importKey+"/") {
			return true
		}
	}
	return false
}

// ScanFile implements FileScanner for a single Python source file.
func (s *PythonScanner) ScanFile(path string) (*model.Project, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}

	base := filepath.Base(absPath)
	stem := strings.TrimSuffix(base, ".py")
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
		addPythonSymbolFromLine(ns, seen, sc.Text(), base, lineCount)
	}

	fileObj := model.NewFile(base, stem)
	fileObj.Lines = lineCount
	ns.AddFile(fileObj)

	proj := &model.Project{
		Path:            stem,
		Language:        model.LangPython,
		DependencyGraph: model.NewDependencyGraph(),
	}
	proj.AddNamespace(ns)
	return proj, nil
}
