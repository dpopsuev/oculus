package survey

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/dpopsuev/oculus/v3/model"
)

// TypeScriptScanner extracts structural metadata from TypeScript/JavaScript
// projects by parsing package.json and scanning source files for import/export
// declarations via regex.
type TypeScriptScanner struct{}

type packageJSON struct {
	Name         string            `json:"name"`
	Dependencies map[string]string `json:"dependencies"`
	DevDeps      map[string]string `json:"devDependencies"`
}

// tsConfigFile is a minimal representation of tsconfig.json / tsconfig.base.json.
type tsConfigFile struct {
	Extends         string `json:"extends"`
	CompilerOptions struct {
		Paths map[string][]string `json:"paths"`
	} `json:"compilerOptions"`
}

// pathAlias holds a resolved tsconfig paths entry.
type pathAlias struct {
	aliasPrefix  string // leading non-wildcard part, e.g. "@scope/"
	dirPrefix    string // leading path to interpolate into, e.g. "packages/"
	dirSuffix    string // trailing path after the wildcard, e.g. "/src"
	exact        bool   // true = no wildcard, direct alias → dir mapping
	exactDir     string // resolved dir for exact aliases
}

// readPathAliases parses compilerOptions.paths from tsconfig.json and
// tsconfig.base.json in absRoot, resolving each entry to a local namespace
// directory. Both exact ("@scope/pkg") and glob ("@scope/*") patterns are
// supported. The extends chain is followed one level deep.
func readPathAliases(absRoot string) []pathAlias {
	var aliases []pathAlias
	seen := make(map[string]bool)

	var parseTsConfig func(path string)
	parseTsConfig = func(cfgPath string) {
		if seen[cfgPath] {
			return
		}
		seen[cfgPath] = true

		data, err := os.ReadFile(cfgPath)
		if err != nil {
			return
		}
		var cfg tsConfigFile
		if json.Unmarshal(data, &cfg) != nil {
			return
		}
		// Follow 'extends' one level (e.g. tsconfig.base.json).
		if cfg.Extends != "" {
			ext := cfg.Extends
			if !strings.HasSuffix(ext, ".json") {
				ext += ".json"
			}
			ext = strings.TrimPrefix(ext, "./")
			parseTsConfig(filepath.Join(absRoot, ext))
		}
		for alias, targets := range cfg.CompilerOptions.Paths {
			if len(targets) == 0 {
				continue
			}
			target := filepath.ToSlash(strings.TrimPrefix(targets[0], "./"))
			if strings.Contains(alias, "*") {
				// Glob alias: "@scope/*" → "packages/*/src/index.ts"
				aliasParts := strings.SplitN(alias, "*", 2)
				targetParts := strings.SplitN(target, "*", 2)
				if len(aliasParts) != 2 || len(targetParts) != 2 {
					continue
				}
				// Strip the trailing filename from the suffix so we get the dir.
				suffix := filepath.ToSlash(filepath.Dir(targetParts[1]))
				if suffix == "." {
					suffix = ""
				}
				aliases = append(aliases, pathAlias{
					aliasPrefix: aliasParts[0],
					dirPrefix:   targetParts[0],
					dirSuffix:   suffix,
				})
			} else {
				// Exact alias: "@scope/pkg" → "packages/pkg/src/index.ts"
				dir := filepath.ToSlash(filepath.Dir(target))
				if dir == "." {
					dir = nsRoot
				}
				aliases = append(aliases, pathAlias{
					aliasPrefix: alias,
					exact:       true,
					exactDir:    dir,
				})
			}
		}
	}

	// Parse tsconfig.json / jsconfig.json first, then tsconfig.base.json.
	// jsconfig.json is the JavaScript-only equivalent of tsconfig.json and
	// uses an identical compilerOptions.paths schema (LCS-BUG-80).
	for _, name := range []string{"tsconfig.json", "jsconfig.json", "tsconfig.base.json"} {
		parseTsConfig(filepath.Join(absRoot, name))
	}

	// Sort aliases so that the most specific pattern is tried first:
	//   1. exact matches (no wildcard) — highest priority
	//   2. glob patterns ordered by descending aliasPrefix length (longer prefix = more specific)
	//   3. catch-all (aliasPrefix == "", i.e. "*") — lowest priority
	//
	// Without this sort, Go map iteration produces non-deterministic order and
	// a catch-all "*": ["./*"] can shadow a more-specific exact entry like
	// "@scope/pkg": ["./packages/pkg/src/index.ts"] (LCS-BUG-79).
	sort.SliceStable(aliases, func(i, j int) bool {
		ai, aj := aliases[i], aliases[j]
		// Exact always beats glob.
		if ai.exact != aj.exact {
			return ai.exact
		}
		// Among globs, longer prefix is more specific.
		return len(ai.aliasPrefix) > len(aj.aliasPrefix)
	})
	return aliases
}

// resolveAlias checks whether spec matches any tsconfig path alias and returns
// the resolved local namespace directory. Returns ("", false) for unknown
// specs that should fall through to external-package handling.
func resolveAlias(spec string, aliases []pathAlias) (string, bool) {
	for _, a := range aliases {
		if a.exact {
			if spec == a.aliasPrefix {
				return a.exactDir, true
			}
			continue
		}
		// Glob: spec must start with aliasPrefix.
		if strings.HasPrefix(spec, a.aliasPrefix) {
			wildcard := strings.TrimPrefix(spec, a.aliasPrefix)
			dir := filepath.ToSlash(fmt.Sprintf("%s%s%s", a.dirPrefix, wildcard, a.dirSuffix))
			return dir, true
		}
	}
	return "", false
}

func (s *TypeScriptScanner) Scan(root string) (*model.Project, error) {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}

	aliases := readPathAliases(absRoot)

	pkg := readPackageJSON(absRoot)

	projName := pkg.Name
	if projName == "" {
		projName = filepath.Base(absRoot)
	}

	proj := &model.Project{
		Path:            projName,
		Language:        model.LangTypeScript,
		DependencyGraph: model.NewDependencyGraph(),
	}

	externalPkgs := make(map[string]bool)
	for dep := range pkg.Dependencies {
		externalPkgs[dep] = true
	}
	for dep := range pkg.DevDeps {
		externalPkgs[dep] = true
	}

	nsMap := make(map[string]*model.Namespace)
	seen := make(map[string]map[string]bool)

	err = filepath.WalkDir(absRoot, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if ShouldSkipTSDir(d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if !isTSFile(d.Name()) {
			return nil
		}

		rel, relErr := filepath.Rel(absRoot, path)
		if relErr != nil {
			rel = path
		}
		rel = filepath.ToSlash(rel)

		dir := filepath.ToSlash(filepath.Dir(rel))
		if dir == "." {
			dir = nsRoot
		}

		ns := nsMap[dir]
		if ns == nil {
			ns = model.NewNamespace(dir, dir)
			nsMap[dir] = ns
			seen[dir] = make(map[string]bool)
		}
		fileObj := model.NewFile(rel, dir)

		f, fErr := os.Open(path)
		if fErr != nil {
			return nil
		}
		defer f.Close()

		lineCount := 0
		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			lineCount++
			line := scanner.Text()
			s.extractExports(line, ns, seen[dir], rel, lineCount)
			s.extractImportEdge(line, dir, aliases, proj.DependencyGraph)
		}
		fileObj.Lines = lineCount
		ns.AddFile(fileObj)
		return nil
	})
	if err != nil {
		return nil, err
	}

	keys := make([]string, 0, len(nsMap))
	for k := range nsMap {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		proj.AddNamespace(nsMap[k])
	}

	return proj, nil
}

var tsExportPatterns = []symbolPattern{
	{regexp.MustCompile(`^\s*export\s+(?:async\s+)?function\s+(\w+)`), model.SymbolFunction},
	{regexp.MustCompile(`^\s*export\s+(?:abstract\s+)?class\s+(\w+)`), model.SymbolClass},
	{regexp.MustCompile(`^\s*export\s+(?:const\s+)?enum\s+(\w+)`), model.SymbolEnum},
	{regexp.MustCompile(`^\s*export\s+(?:type\s+)?interface\s+(\w+)`), model.SymbolInterface},
	{regexp.MustCompile(`^\s*export\s+type\s+(\w+)\s*=`), model.SymbolTypeParameter},
	{regexp.MustCompile(`^\s*export\s+(?:const|let|var)\s+(\w+)`), model.SymbolVariable},
}

var (
	// Match value imports but NOT type-only imports.
	// `import type { X } from '...'` and `export type { X } from '...'` are
	// compile-time only (erased by TypeScript) and should not create dependency edges.
	reImportFrom     = regexp.MustCompile(`(?:import|export)\s+.*?\s+from\s+['"]([^'"]+)['"]`)
	reImportTypeOnly = regexp.MustCompile(`^\s*(?:import|export)\s+type\s+\{`)
	reImportSide     = regexp.MustCompile(`^\s*import\s+['"]([^'"]+)['"]`)
	reRequire        = regexp.MustCompile(`require\s*\(\s*['"]([^'"]+)['"]\s*\)`)
)

func (s *TypeScriptScanner) extractExports(line string, ns *model.Namespace, seen map[string]bool, filePath string, lineNum int) {
	matchSymbolPatterns(line, tsExportPatterns, ns, seen, true, filePath, lineNum)
}

func (s *TypeScriptScanner) extractImportEdge(line, fromDir string, aliases []pathAlias, graph *model.DependencyGraph) {
	// Skip type-only imports — they are erased at compile time and
	// don't create runtime dependencies. Prevents false-positive cycles.
	if reImportTypeOnly.MatchString(line) {
		return
	}

	var spec string
	if m := reImportFrom.FindStringSubmatch(line); m != nil {
		spec = m[1]
	} else if m := reImportSide.FindStringSubmatch(line); m != nil {
		spec = m[1]
	} else if m := reRequire.FindStringSubmatch(line); m != nil {
		spec = m[1]
	}
	if spec == "" {
		return
	}

	if strings.HasPrefix(spec, "./") || strings.HasPrefix(spec, "../") {
		resolved := resolveRelativeImport(fromDir, spec)
		if resolved != fromDir {
			graph.AddEdge(fromDir, resolved, false)
		}
	} else if dir, ok := resolveAlias(spec, aliases); ok {
		// Path alias resolved to a local namespace — internal edge.
		if dir != fromDir {
			graph.AddEdge(fromDir, dir, false)
		}
	} else {
		pkgName := barePackageName(spec)
		graph.AddEdge(fromDir, pkgName, true)
	}
}

func resolveRelativeImport(fromDir, spec string) string {
	base := fromDir
	if base == nsRoot {
		base = "."
	}
	resolved := filepath.ToSlash(filepath.Clean(filepath.Join(base, spec)))
	dir := filepath.ToSlash(filepath.Dir(resolved))
	if dir == "." {
		// resolved is a top-level bare name (e.g. "domain" from "../domain").
		// Return it directly as the target namespace rather than collapsing
		// to nsRoot — a directory-level import like '../domain' points to
		// the 'domain' component, not to the project root.
		if resolved == "." {
			return nsRoot
		}
		return resolved
	}
	return dir
}

func barePackageName(spec string) string {
	if strings.HasPrefix(spec, "@") {
		parts := strings.SplitN(spec, "/", 3)
		if len(parts) >= 2 {
			return parts[0] + "/" + parts[1]
		}
		return spec
	}
	parts := strings.SplitN(spec, "/", 2)
	return parts[0]
}

func isTSFile(name string) bool {
	ext := filepath.Ext(name)
	switch ext {
	case ".ts", ".tsx", ".js", ".jsx", ".mts", ".mjs":
		return true
	}
	return false
}

func readPackageJSON(root string) packageJSON {
	var pkg packageJSON
	data, err := os.ReadFile(filepath.Join(root, "package.json"))
	if err != nil {
		return pkg
	}
	_ = json.Unmarshal(data, &pkg)
	return pkg
}
