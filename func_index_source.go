package oculus

import "context"

// FuncIndexSource implements SymbolSource from a pre-parsed function index.
// Any language that can produce []SourceFunc gets SymbolSource + DeepAnalyzer
// (via SymbolPipeline) for free — no bespoke struct needed.
//
// Usage:
//
//	RegisterSource(lang.Python, 80, func(root string, _ lsp.Pool) oculus.SymbolSource {
//	    return oculus.NewFuncIndexSource(parsePythonFunctions(root))
//	})
type FuncIndexSource struct {
	funcs []SourceFunc
	index map[string]*SourceFunc // keyed by function name
}

// NewFuncIndexSource creates a SymbolSource from a parsed function list.
func NewFuncIndexSource(funcs []SourceFunc) *FuncIndexSource {
	idx := make(map[string]*SourceFunc, len(funcs))
	for i := range funcs {
		idx[funcs[i].Name] = &funcs[i]
	}
	return &FuncIndexSource{funcs: funcs, index: idx}
}

var _ SymbolSource = (*FuncIndexSource)(nil)

func (s *FuncIndexSource) Roots(_ context.Context, query string) ([]SourceSymbol, error) {
	if query != "" {
		fn, ok := s.index[query]
		if !ok {
			return nil, nil
		}
		return []SourceSymbol{s.toSymbol(fn)}, nil
	}
	var roots []SourceSymbol
	seen := make(map[string]bool)
	for i := range s.funcs {
		f := &s.funcs[i]
		if !f.Exported || seen[f.Name] {
			continue
		}
		seen[f.Name] = true
		roots = append(roots, s.toSymbol(f))
	}
	return roots, nil
}

func (s *FuncIndexSource) Children(_ context.Context, sym SourceSymbol) ([]SourceRelation, error) {
	fn, ok := sym.Handle.(*SourceFunc)
	if !ok || fn == nil {
		fn = s.index[sym.Name]
		if fn == nil {
			return nil, nil
		}
	}
	var rels []SourceRelation
	for _, callee := range fn.Callees {
		cf, ok := s.index[callee]
		if !ok {
			continue
		}
		rels = append(rels, SourceRelation{
			Target:      s.toSymbol(cf),
			Kind:        "call",
			InWorkspace: true,
		})
	}
	return rels, nil
}

func (s *FuncIndexSource) Hover(_ context.Context, sym SourceSymbol) (*SourceTypeInfo, error) {
	fn, ok := sym.Handle.(*SourceFunc)
	if !ok || fn == nil {
		fn = s.index[sym.Name]
		if fn == nil {
			return nil, nil
		}
	}
	if len(fn.ParamTypes) == 0 && len(fn.ReturnTypes) == 0 {
		return nil, nil
	}
	return &SourceTypeInfo{
		ParamTypes:  fn.ParamTypes,
		ReturnTypes: fn.ReturnTypes,
	}, nil
}

func (s *FuncIndexSource) toSymbol(fn *SourceFunc) SourceSymbol {
	return SourceSymbol{
		Name:    fn.Name,
		Package: fn.Package,
		File:    fn.File,
		Line:    fn.Line,
		EndLine: fn.EndLine,
		Kind:    "function",
		Handle:  fn,
	}
}
