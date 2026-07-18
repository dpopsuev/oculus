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
		return oculus.NewFuncIndexSource(funcs, ExtractFileImports(root))
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
		var callees []string
		callArgs := make(map[string][]string)
		seen := make(map[string]bool)
		var memberCallees []string
		seenMember := make(map[string]bool)
		callLines := make(map[string]int)
		extractCallsWithArgs(fd.body, fd.src, func(callee string, line int, args []string, member bool) {
			calleeKey, _, resolved := resolveCallee(callee, fd.pkg, allFuncs, member)
			if !resolved {
				return
			}
			if _, found := allFuncs[calleeKey]; !found {
				return
			}
			callLines[callee] = line
			if member {
				if !seenMember[callee] {
					seenMember[callee] = true
					memberCallees = append(memberCallees, callee)
				}
				if len(args) > 0 {
					callArgs[callee] = args
				}
				return
			}
			if !seen[callee] {
				seen[callee] = true
				callees = append(callees, callee)
				if len(args) > 0 {
					callArgs[callee] = args
				}
			}
		})

		exported := isExported(fd.name)
		name := fd.name
		if dot := strings.LastIndex(key, "."); dot >= 0 && key[dot+1:] == name {
			// name is already just the function name
		}

		sym := oculus.Symbol{
			Name:          name,
			Package:       fd.pkg,
			File:          fd.file,
			Line:          fd.line,
			EndLine:       fd.endLine,
			ParamTypes:    fd.paramTypes,
			ReturnTypes:   fd.returnTypes,
			Callees:       callees,
			MemberCallees: memberCallees,
			Exported:      exported,
		}
		if len(callArgs) > 0 {
			sym.CallArgs = callArgs
		}
		if len(callLines) > 0 {
			sym.CallLines = callLines
		}
		funcs = append(funcs, sym)
	}
	return funcs
}
