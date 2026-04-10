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

// ParseGoASTFunctions parses Go source via go/ast and returns SourceFuncs.
func ParseGoASTFunctions(root string) []oculus.SourceFunc {
	if lang.DetectLanguage(root) != lang.Go {
		return nil
	}
	a := &GoASTDeepAnalyzer{root: root}
	goFuncs, err := a.parseFunctions("")
	if err != nil || len(goFuncs) == 0 {
		return nil
	}

	funcs := make([]oculus.SourceFunc, len(goFuncs))
	for i, f := range goFuncs {
		funcs[i] = oculus.SourceFunc{
			Name:        f.name,
			Package:     f.pkg,
			File:        f.file,
			Line:        f.line,
			EndLine:     f.endLine,
			ParamTypes:  f.paramTypes,
			ReturnTypes: f.returnTypes,
			Callees:     f.callees,
			Exported:    ast.IsExported(f.name),
		}
	}
	return funcs
}
