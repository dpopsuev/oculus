package analyzer

import (
	"context"
	"go/ast"

	"github.com/dpopsuev/oculus"
	"github.com/dpopsuev/oculus/lang"
	"github.com/dpopsuev/oculus/lsp"
)

func init() {
	RegisterSource(lang.Go, 90, func(root string, _ lsp.Pool) oculus.SymbolSource {
		return NewGoASTSymbolSource(root)
	})
}

// GoASTSymbolSource implements oculus.SymbolSource using Go's AST parser.
// All functions are parsed upfront; Roots/Children/Hover query the index.
type GoASTSymbolSource struct {
	root      string
	funcIndex map[string]*goFunc // keyed by function name
	allFuncs  []goFunc
}

// NewGoASTSymbolSource parses Go source and builds a SymbolSource.
// Returns nil if the root is not a Go project.
func NewGoASTSymbolSource(root string) *GoASTSymbolSource {
	if lang.DetectLanguage(root) != lang.Go {
		return nil
	}
	a := &GoASTDeepAnalyzer{root: root}
	funcs, err := a.parseFunctions("")
	if err != nil || len(funcs) == 0 {
		return nil
	}
	idx := make(map[string]*goFunc, len(funcs))
	for i := range funcs {
		idx[funcs[i].name] = &funcs[i]
	}
	return &GoASTSymbolSource{root: root, funcIndex: idx, allFuncs: funcs}
}

var _ oculus.SymbolSource = (*GoASTSymbolSource)(nil)

func (s *GoASTSymbolSource) Roots(_ context.Context, query string) ([]oculus.SourceSymbol, error) {
	if query != "" {
		fn, ok := s.funcIndex[query]
		if !ok {
			return nil, nil
		}
		return []oculus.SourceSymbol{s.funcToSymbol(fn)}, nil
	}
	var roots []oculus.SourceSymbol
	seen := make(map[string]bool)
	for _, f := range s.allFuncs {
		if !ast.IsExported(f.name) || seen[f.name] {
			continue
		}
		seen[f.name] = true
		roots = append(roots, s.funcToSymbol(&f))
	}
	return roots, nil
}

func (s *GoASTSymbolSource) Children(_ context.Context, sym oculus.SourceSymbol) ([]oculus.SourceRelation, error) {
	fn, ok := sym.Handle.(*goFunc)
	if !ok || fn == nil {
		fn = s.funcIndex[sym.Name]
		if fn == nil {
			return nil, nil
		}
	}
	var rels []oculus.SourceRelation
	for _, callee := range fn.callees {
		calleeFn, ok := s.funcIndex[callee]
		if !ok {
			continue
		}
		rels = append(rels, oculus.SourceRelation{
			Target:      s.funcToSymbol(calleeFn),
			Kind:        "call",
			InWorkspace: true,
		})
	}
	return rels, nil
}

func (s *GoASTSymbolSource) Hover(_ context.Context, sym oculus.SourceSymbol) (*oculus.SourceTypeInfo, error) {
	fn, ok := sym.Handle.(*goFunc)
	if !ok || fn == nil {
		fn = s.funcIndex[sym.Name]
		if fn == nil {
			return nil, nil
		}
	}
	if len(fn.paramTypes) == 0 && len(fn.returnTypes) == 0 {
		return nil, nil
	}
	return &oculus.SourceTypeInfo{
		ParamTypes:  fn.paramTypes,
		ReturnTypes: fn.returnTypes,
	}, nil
}

func (s *GoASTSymbolSource) funcToSymbol(fn *goFunc) oculus.SourceSymbol {
	return oculus.SourceSymbol{
		Name:    fn.name,
		Package: fn.pkg,
		File:    fn.file,
		Line:    fn.line,
		EndLine: fn.endLine,
		Kind:    12, // function
		Handle:  fn,
	}
}

