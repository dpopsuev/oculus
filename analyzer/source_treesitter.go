package analyzer

import (
	"strings"

	"github.com/dpopsuev/oculus/v3"
	"github.com/dpopsuev/oculus/v3/lang"
	"github.com/dpopsuev/oculus/v3/lsp"
)

func init() {
	RegisterSource(lang.Unknown, 50, func(root string, _ lsp.Pool) oculus.SymbolSource {
		funcs := ParseTreeSitterFunctions(root)
		if len(funcs) == 0 {
			return nil
		}
		return oculus.NewFuncIndexSource(funcs)
	})
}

// ParseTreeSitterFunctions parses all Go source files via tree-sitter and
// returns SourceFuncs with callees pre-extracted. Thread-safe: tree-sitter
// nodes are only accessed during construction, not during concurrent Pipeline walks.
func ParseTreeSitterFunctions(root string) []oculus.Symbol {
	pp, err := BuildParsedProject(root)
	if err != nil || pp == nil {
		return nil
	}

	a := &TreeSitterDeepAnalyzer{project: pp}
	allFuncs, _ := a.extractCallGraphFuncs(oculus.CallGraphOpts{})

	var funcs []oculus.Symbol
	for key, fd := range allFuncs {
		// Pre-extract callees from tree-sitter AST (single-threaded).
		var callees []string
		seen := make(map[string]bool)
		extractCalls(fd.body, fd.src, func(callee string, _ int) {
			calleeKey, _ := resolveCallee(callee, fd.pkg, allFuncs)
			if _, found := allFuncs[calleeKey]; found && !seen[callee] {
				seen[callee] = true
				callees = append(callees, callee)
			}
		})

		exported := isExported(fd.name)
		// Strip "pkg." prefix from key for the name if present.
		name := fd.name
		if dot := strings.LastIndex(key, "."); dot >= 0 && key[dot+1:] == name {
			// name is already just the function name
		}

		funcs = append(funcs, oculus.Symbol{
			Name:        name,
			Package:     fd.pkg,
			File:        fd.file,
			Line:        fd.line,
			EndLine:     fd.endLine,
			ParamTypes:  fd.paramTypes,
			ReturnTypes: fd.returnTypes,
			Callees:     callees,
			Exported:    exported,
		})
	}
	return funcs
}
