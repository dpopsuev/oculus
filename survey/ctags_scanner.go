package survey

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/dpopsuev/oculus/v3/lang"
	"github.com/dpopsuev/oculus/v3/model"
)

var errCtagsNotFound = errors.New("ctags not found; install with: dnf install ctags")

// CtagsScanner uses Universal Ctags (--output-format=json) to extract
// symbols from C/C++ (or any ctags-supported language) projects.
// It populates model.Project with one Namespace per directory, one Symbol
// per tag, and extracts #include directives for dependency edges.
type CtagsScanner struct{}

type ctagsEntry struct {
	Type      string `json:"_type"`
	Name      string `json:"name"`
	Path      string `json:"path"`
	Language  string `json:"language"`
	Line      int    `json:"line"`
	Kind      string `json:"kind"`
	Scope     string `json:"scope"`
	ScopeKnd  string `json:"scopeKind"`
	Signature string `json:"signature"`
}

// ctagsScanTimeout is the maximum time ctags is allowed to run.
// Large workspaces with node_modules can take hours without --exclude;
// after adding excludes the typical scan takes under 60 s.
const ctagsScanTimeout = 5 * time.Minute

// ctagsExcludeArgs builds the --exclude= flags from CommonSkipDirs so that
// ctags never descends into node_modules, .git, vendor, etc.
func ctagsExcludeArgs() []string {
	args := make([]string, 0, len(lang.CommonSkipDirs))
	for dir := range lang.CommonSkipDirs {
		args = append(args, "--exclude="+dir)
	}
	// Also exclude hidden directories (dot-prefixed) that are not already listed.
	args = append(args, "--exclude=.*")
	return args
}

func (s *CtagsScanner) Scan(root string) (*model.Project, error) {
	if _, err := exec.LookPath("ctags"); err != nil {
		return nil, errCtagsNotFound
	}

	ctx, cancel := context.WithTimeout(context.Background(), ctagsScanTimeout)
	defer cancel()

	args := append(
		[]string{"--output-format=json", "--fields=*", "-o", "-"},
		ctagsExcludeArgs()...,
	)
	args = append(args, "-R", ".")

	cmd := exec.CommandContext(ctx, "ctags", args...)
	cmd.Dir = root
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("ctags pipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("ctags start: %w", err)
	}
	defer func() {
		// Always reap the child so no zombie is left behind.
		// cmd.Wait is called below on the success path; this handles
		// early-return error paths where Wait has not been reached.
		_ = cmd.Wait()
	}()

	proj := &model.Project{
		Path:     root,
		Language: DetectFromMarkers(root),
	}

	dirNS := make(map[string]*model.Namespace)
	fileSet := make(map[string]bool)

	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}
		var entry ctagsEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}
		if entry.Type != "tag" {
			continue
		}

		dir := filepath.Dir(entry.Path)
		if dir == "" {
			dir = "."
		}

		ns := dirNS[dir]
		if ns == nil {
			ns = model.NewNamespace(dir, dir)
			dirNS[dir] = ns
		}

		sym := &model.Symbol{
			Name:     entry.Name,
			Kind:     mapCtagsKind(entry.Kind),
			Exported: true,
			File:     entry.Path,
			Line:     entry.Line,
		}
		ns.AddSymbol(sym)

		if !fileSet[entry.Path] {
			fileSet[entry.Path] = true
			fileObj := model.NewFile(entry.Path, dir)
			// Count lines for LOC metric.
			if data, readErr := os.ReadFile(filepath.Join(root, entry.Path)); readErr == nil {
				fileObj.Lines = bytes.Count(data, []byte{'\n'})
				if len(data) > 0 && data[len(data)-1] != '\n' {
					fileObj.Lines++
				}
			}
			ns.AddFile(fileObj)
		}
	}

	if err := cmd.Wait(); err != nil {
		if ctx.Err() != nil {
			return nil, fmt.Errorf("ctags timed out after %s (is node_modules excluded?): %w", ctagsScanTimeout, ctx.Err())
		}
		// Non-zero exit from ctags is usually a parse warning — use whatever
		// symbols were collected rather than failing the whole scan.
	}

	for _, ns := range dirNS {
		proj.AddNamespace(ns)
	}

	// Start with C/C++ #include edges (no-op for non-C languages).
	deps := extractCIncludes(root)
	if deps == nil {
		deps = model.NewDependencyGraph()
	}

	// Merge language-specific import edges (Java, Kotlin, C#, Swift, Zig, Lua).
	// Always merged — not gated on zero C edges — so polyglot projects like
	// neovim (C + Lua) get edges from both extractors (LCS-BUG-74).
	if importDeps := extractLanguageImports(root, proj.Language, dirNS); importDeps != nil {
		for _, e := range importDeps.Edges {
			deps.AddEdge(e.From, e.To, e.External)
		}
	}

	if len(deps.Edges) > 0 {
		proj.DependencyGraph = deps
	}

	return proj, nil
}

func mapCtagsKind(kind string) model.SymbolKind {
	switch kind {
	case "function":
		return model.SymbolFunction
	case "method":
		return model.SymbolMethod
	case "struct", "union":
		return model.SymbolStruct
	case "class":
		return model.SymbolClass
	case "enum":
		return model.SymbolEnum
	case "variable", "externvar":
		return model.SymbolVariable
	case "macro", "define":
		return model.SymbolConstant
	case "typedef":
		return model.SymbolTypeParameter
	case "member":
		return model.SymbolField
	default:
		return model.SymbolVariable
	}
}

// extractCIncludes scans .c and .h files for #include directives and
// builds a dependency graph mapping source directories to included header dirs.
func extractCIncludes(root string) *model.DependencyGraph {
	deps := model.NewDependencyGraph()

	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if ShouldSkipDir(d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		ext := filepath.Ext(path)
		if ext != ".c" && ext != ".h" && ext != ".cpp" && ext != ".hpp" && ext != ".cc" {
			return nil
		}

		f, err := os.Open(path)
		if err != nil {
			return nil
		}
		defer f.Close()

		relPath, _ := filepath.Rel(root, path)
		srcDir := filepath.Dir(relPath)

		sc := bufio.NewScanner(f)
		for sc.Scan() {
			line := strings.TrimSpace(sc.Text())
			if !strings.HasPrefix(line, "#include") {
				continue
			}
			inc := parseInclude(line)
			if inc == "" {
				continue
			}
			// Resolve include path using a cascade of search paths, mimicking
			// common -I compiler flags (LCS-BUG-74):
			//   1. <root>/<inc>          — -I <root>  (e.g. small projects)
			//   2. <root>/src/<inc>      — -I src     (e.g. neovim, Linux kernel)
			//   3. <root>/include/<inc>  — -I include (e.g. many C libs)
			//   4. <srcDir>/<inc>        — standard file-relative (fallback)
			var resolved string
			searchPaths := []string{"", "src", "include"}
			found := false
			for _, sp := range searchPaths {
				var candidate string
				if sp == "" {
					candidate = filepath.Join(root, inc)
				} else {
					candidate = filepath.Join(root, sp, inc)
				}
				if _, statErr := os.Stat(candidate); statErr == nil {
					rel, _ := filepath.Rel(root, candidate)
					resolved = filepath.ToSlash(rel)
					found = true
					break
				}
			}
			if !found {
				resolved = filepath.ToSlash(filepath.Clean(filepath.Join(srcDir, inc)))
			}
			incDir := filepath.Dir(resolved)
			if incDir == "." {
				incDir = srcDir
			}
			if incDir != srcDir {
				deps.AddEdge(srcDir, incDir, false)
			}
		}
		return nil
	})
	return deps
}

// Language-specific import regex patterns for non-C/C++ languages.
var (
	reJavaImport   = regexp.MustCompile(`^\s*import\s+(?:static\s+)?([a-zA-Z0-9_.]+)\.\w+\s*;`)
	reKotlinImport = regexp.MustCompile(`^\s*import\s+([a-zA-Z0-9_.]+)\.\w+`)
	reCSharpUsing  = regexp.MustCompile(`^\s*using\s+(?:static\s+)?([a-zA-Z0-9_.]+)\s*;`)
	reSwiftImport  = regexp.MustCompile(`^\s*import\s+(\w+)`)
	reZigImport    = regexp.MustCompile(`@import\("([^"]+)"\)`)
	// Lua: require("module.sub") or require('module.sub') with optional whitespace.
	reLuaRequire = regexp.MustCompile(`require\s*\(\s*["']([^"']+)["']\s*\)`)
)

// extractLanguageImports scans source files for language-specific import
// statements and builds a dependency graph mapping directory namespaces.
func extractLanguageImports(root string, lang model.Language, dirNS map[string]*model.Namespace) *model.DependencyGraph {
	var importRe *regexp.Regexp
	var resolver func(match []string, dirNS map[string]*model.Namespace) string

	switch lang {
	case model.LangJava:
		importRe = reJavaImport
		resolver = resolvePackageImport
	case model.LangKotlin:
		importRe = reKotlinImport
		resolver = resolvePackageImport
	case model.LangCSharp:
		importRe = reCSharpUsing
		resolver = resolvePackageImport
	case model.LangSwift:
		importRe = reSwiftImport
		resolver = resolveModuleImport
	case model.LangZig:
		importRe = reZigImport
		resolver = resolveZigImport
	case model.LangLua:
		importRe = reLuaRequire
		resolver = resolveLuaRequire
	default:
		return nil
	}

	graph := model.NewDependencyGraph()
	seen := make(map[[2]string]bool)

	for nsKey, ns := range dirNS {
		for _, f := range ns.Files {
			fullPath := filepath.Join(root, f.Path)
			data, err := os.ReadFile(fullPath)
			if err != nil {
				continue
			}
			for _, line := range strings.Split(string(data), "\n") {
				if m := importRe.FindStringSubmatch(line); m != nil {
					targetNS := resolver(m, dirNS)
					if targetNS != "" && targetNS != nsKey {
						key := [2]string{nsKey, targetNS}
						if !seen[key] {
							seen[key] = true
							graph.AddEdge(nsKey, targetNS, false)
						}
					}
				}
			}
		}
	}
	return graph
}

// resolvePackageImport maps a dotted package path (e.g. "domain.Entity" → captured "domain")
// to a known directory namespace by matching against namespace keys.
func resolvePackageImport(match []string, dirNS map[string]*model.Namespace) string {
	pkg := match[1]
	// Convert dots to slashes for path matching.
	pkgPath := strings.ReplaceAll(pkg, ".", "/")

	// Exact match.
	if _, ok := dirNS[pkgPath]; ok {
		return pkgPath
	}

	// Try progressively shorter prefixes.
	parts := strings.Split(pkgPath, "/")
	for i := len(parts); i > 0; i-- {
		candidate := strings.Join(parts[:i], "/")
		if _, ok := dirNS[candidate]; ok {
			return candidate
		}
	}

	// Try matching just the last segment (common for flat layouts).
	lastSeg := parts[len(parts)-1]
	for ns := range dirNS {
		if filepath.Base(ns) == lastSeg {
			return ns
		}
	}
	return ""
}

// resolveModuleImport maps a Swift module name to a directory namespace.
func resolveModuleImport(match []string, dirNS map[string]*model.Namespace) string {
	moduleName := match[1]
	for ns := range dirNS {
		if filepath.Base(ns) == moduleName {
			return ns
		}
	}
	return ""
}

// resolveZigImport maps a Zig @import path to a directory namespace.
func resolveZigImport(match []string, dirNS map[string]*model.Namespace) string {
	importPath := match[1]
	dir := filepath.Dir(importPath)
	if dir == "." {
		return ""
	}
	if _, ok := dirNS[dir]; ok {
		return dir
	}
	return ""
}

// resolveLuaRequire maps a Lua require() argument to a directory namespace.
// Lua uses dot notation: require("module.sub") → module/sub.
// Tries exact match, then progressively shorter prefixes, then the first segment.
func resolveLuaRequire(match []string, dirNS map[string]*model.Namespace) string {
	modPath := strings.ReplaceAll(match[1], ".", "/")

	// Exact match.
	if _, ok := dirNS[modPath]; ok {
		return modPath
	}
	// Longest-prefix match.
	parts := strings.Split(modPath, "/")
	for i := len(parts); i > 0; i-- {
		candidate := strings.Join(parts[:i], "/")
		if _, ok := dirNS[candidate]; ok {
			return candidate
		}
	}
	return ""
}

func parseInclude(line string) string {
	line = strings.TrimPrefix(line, "#include")
	line = strings.TrimSpace(line)
	if len(line) < 2 {
		return ""
	}
	if line[0] == '"' {
		if end := strings.Index(line[1:], "\""); end >= 0 {
			return line[1 : 1+end]
		}
	}
	return ""
}
