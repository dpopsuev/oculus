package analyzer

import (
	"go/ast"

	"github.com/dpopsuev/oculus"
	"github.com/dpopsuev/oculus/lang"
	"github.com/dpopsuev/oculus/lsp"
)

func init() {
	RegisterSource(lang.Go, 90, func(root string, _ lsp.Pool) oculus.SymbolSource {
		funcs := ParseGoASTFunctions(root)
		if len(funcs) == 0 {
			return nil
		}
		return oculus.NewFuncIndexSource(funcs)
	})
}

// ParseGoASTFunctions parses Go source via go/ast and returns Symbols.
func ParseGoASTFunctions(root string) []oculus.Symbol {
	if lang.DetectLanguage(root) != lang.Go {
		return nil
	}
	a := &GoASTDeepAnalyzer{root: root}
	funcs, err := a.parseFunctions("")
	if err != nil || len(funcs) == 0 {
		return nil
	}
	// Set Exported field (parseFunctions doesn't set it).
	for i := range funcs {
		funcs[i].Exported = ast.IsExported(funcs[i].Name)
	}
	return funcs
}
