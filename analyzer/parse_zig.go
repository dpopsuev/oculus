package analyzer

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/dpopsuev/oculus/v3"
	"github.com/dpopsuev/oculus/v3/lang"
	"github.com/dpopsuev/oculus/v3/lsp"
)

func init() {
	RegisterSource(lang.Zig, 80, func(root string, _ lsp.Pool) oculus.SymbolSource {
		if lang.DetectLanguage(root) != lang.Zig {
			return nil
		}
		funcs := ParseZigFunctions(root)
		if len(funcs) == 0 {
			return nil
		}
		return oculus.NewFuncIndexSource(funcs)
	})
}

// Zig function signature: pub? fn name(params) return_type {
var zigFuncRe = regexp.MustCompile(`(?m)^(?:pub\s+)?fn\s+(\w+)\s*\(([^)]*)\)\s*([^{]*)`)

// Zig function call: name(
var zigCallRe = regexp.MustCompile(`\b(\w+)\s*\(`)

// ParseZigFunctions parses .zig files via regex (no tree-sitter grammar available).
func ParseZigFunctions(root string) []oculus.Symbol {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return nil
	}

	var funcs []oculus.Symbol

	_ = filepath.WalkDir(absRoot, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if strings.HasPrefix(d.Name(), ".") || d.Name() == "zig-cache" || d.Name() == "zig-out" {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(d.Name()) != ".zig" {
			return nil
		}
		src, err := os.ReadFile(path)
		if err != nil {
			return nil
		}

		rel, _ := filepath.Rel(absRoot, path)
		pkg := filepath.ToSlash(filepath.Dir(rel))
		if pkg == "." {
			pkg = pkgRoot
		}

		lines := strings.Split(string(src), "\n")

		// First pass: find all function definitions.
		funcNames := make(map[string]bool)
		matches := zigFuncRe.FindAllStringSubmatchIndex(string(src), -1)
		for _, m := range matches {
			name := string(src[m[2]:m[3]])
			funcNames[name] = true
		}

		// Second pass: extract functions with params, return types, and callees.
		for _, m := range matches {
			name := string(src[m[2]:m[3]])
			paramStr := string(src[m[4]:m[5]])
			retStr := strings.TrimSpace(string(src[m[6]:m[7]]))

			// Line number (approximate from byte offset)
			line := 1 + strings.Count(string(src[:m[0]]), "\n")

			paramTypes := extractZigParamTypes(paramStr)

			var returnTypes []string
			if retStr != "" && retStr != "void" && retStr != "!" {
				// Clean up: remove trailing { and whitespace
				retStr = strings.TrimRight(retStr, " {")
				if retStr != "" && retStr != "void" {
					returnTypes = []string{retStr}
				}
			}

			// Find callees in function body
			endLine := findZigFuncEnd(lines, line-1)
			var body string
			if line-1 < len(lines) && endLine <= len(lines) {
				body = strings.Join(lines[line-1:endLine], "\n")
			}
			callees := extractZigCallees(body, funcNames)

			exported := !strings.HasPrefix(name, "_")

			funcs = append(funcs, oculus.Symbol{
				Name: name, Package: pkg, File: filepath.ToSlash(rel),
				Line: line, EndLine: endLine,
				ParamTypes: paramTypes, ReturnTypes: returnTypes,
				Callees: callees, Exported: exported,
			})
		}
		return nil
	})
	return funcs
}

func extractZigParamTypes(paramStr string) []string {
	paramStr = strings.TrimSpace(paramStr)
	if paramStr == "" {
		return nil
	}
	var types []string
	for _, p := range strings.Split(paramStr, ",") {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		// Zig param: name: type
		if colon := strings.Index(p, ":"); colon >= 0 {
			t := strings.TrimSpace(p[colon+1:])
			if t != "" {
				types = append(types, t)
			}
		}
	}
	return types
}

func findZigFuncEnd(lines []string, startLine int) int {
	depth := 0
	for i := startLine; i < len(lines); i++ {
		depth += strings.Count(lines[i], "{") - strings.Count(lines[i], "}")
		if depth <= 0 && i > startLine {
			return i + 1
		}
	}
	return startLine + 1
}

func extractZigCallees(body string, knownFuncs map[string]bool) []string {
	seen := make(map[string]bool)
	var callees []string
	for _, m := range zigCallRe.FindAllStringSubmatch(body, -1) {
		name := m[1]
		// Only include known functions, skip keywords
		if knownFuncs[name] && !seen[name] && !isZigKeyword(name) {
			seen[name] = true
			callees = append(callees, name)
		}
	}
	return callees
}

func isZigKeyword(s string) bool {
	switch s {
	case "if", "else", "while", "for", "return", "const", "var",
		"fn", "pub", "struct", "enum", "union", "switch", "try",
		"catch", "break", "continue", "defer", "errdefer":
		return true
	}
	return false
}
