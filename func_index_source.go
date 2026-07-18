package oculus

import (
	"context"
	"strings"
)

// FuncIndexSource implements SymbolSource from a pre-parsed function index.
// Any language that can produce []Symbol gets SymbolSource + DeepAnalyzer
// (via SymbolPipeline) for free — no bespoke struct needed.
//
// Usage:
//
//	RegisterSource(lang.Python, 80, func(root string, _ lsp.Pool) oculus.SymbolSource {
//	    return oculus.NewFuncIndexSource(parsePythonFunctions(root))
//	})
type FuncIndexSource struct {
	funcs       []Symbol
	index       map[string]*Symbol   // keyed by function name (legacy, kept for Roots)
	resolver    *CallResolver
	fileImports map[string][]string  // file path → imported module paths
}

// FileImports maps source file paths to their import module paths.
// Pass to NewFuncIndexSource to enable import-aware call resolution.
type FileImports map[string][]string

// NewFuncIndexSource creates a SymbolSource from a parsed function list.
// Optional FileImports enables the import_map resolution strategy.
func NewFuncIndexSource(funcs []Symbol, imports ...FileImports) *FuncIndexSource {
	idx := make(map[string]*Symbol, len(funcs))
	for i := range funcs {
		idx[funcs[i].Name] = &funcs[i]
	}
	var fi map[string][]string
	if len(imports) > 0 && imports[0] != nil {
		fi = imports[0]
	}
	return &FuncIndexSource{
		funcs:       funcs,
		index:       idx,
		resolver:    NewCallResolver(funcs),
		fileImports: fi,
	}
}

var _ SymbolSource = (*FuncIndexSource)(nil)

// Functions returns the raw function list for direct inspection.
func (s *FuncIndexSource) Functions() []Symbol {
	return s.funcs
}

func (s *FuncIndexSource) Roots(_ context.Context, query string) ([]Symbol, error) {
	if query != "" {
		fn, ok := s.index[query]
		if !ok {
			return nil, nil
		}
		return []Symbol{s.toSymbol(fn)}, nil
	}
	var roots []Symbol
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

func (s *FuncIndexSource) Children(_ context.Context, sym Symbol) ([]SourceRelation, error) {
	fn, ok := sym.Handle.(*Symbol)
	if !ok || fn == nil {
		fn = s.index[sym.Name]
		if fn == nil {
			return nil, nil
		}
	}

	var imports []string
	if s.fileImports != nil {
		imports = s.fileImports[fn.File]
	}

	var rels []SourceRelation
	rels = append(rels, s.resolveCallees(fn, fn.Callees, "call", imports, false)...)
	rels = append(rels, s.resolveCallees(fn, fn.MemberCallees, "call", imports, true)...)
	for callee, kind := range fn.AsyncCallees {
		rels = append(rels, s.resolveOne(fn, callee, kind, imports, false)...)
	}
	for callee, kind := range fn.MemberAsyncCallees {
		rels = append(rels, s.resolveOne(fn, callee, kind, imports, true)...)
	}
	return rels, nil
}

func (s *FuncIndexSource) resolveCallees(fn *Symbol, callees []string, kind string, imports []string, member bool) []SourceRelation {
	var rels []SourceRelation
	for _, callee := range callees {
		rels = append(rels, s.resolveOne(fn, callee, kind, imports, member)...)
	}
	return rels
}

func (s *FuncIndexSource) resolveOne(fn *Symbol, callee, kind string, imports []string, member bool) []SourceRelation {
	var res Resolution
	if member {
		res = s.resolver.ResolveMember(callee, fn.Package, fn.File, imports)
	} else {
		res = s.resolver.Resolve(callee, fn.Package, fn.File, imports)
	}
	if res.Symbol == nil {
		return nil
	}
	rel := SourceRelation{
		Target:      s.toSymbol(res.Symbol),
		Kind:        kind,
		InWorkspace: true,
		Confidence:  res.Confidence,
		Line:        fn.CallLines[callee],
	}
	if argExprs := fn.CallArgs[callee]; len(argExprs) > 0 {
		rel.Args = bindArgs(argExprs, res.Symbol.ParamTypes)
	}
	return []SourceRelation{rel}
}

func (s *FuncIndexSource) Hover(_ context.Context, sym Symbol) (*SourceTypeInfo, error) {
	fn, ok := sym.Handle.(*Symbol)
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

// bindArgs creates CallArg bindings from argument expressions and parameter types.
func bindArgs(argExprs, paramTypes []string) []CallArg {
	n := len(argExprs)
	if len(paramTypes) < n {
		n = len(paramTypes)
	}
	if n == 0 {
		return nil
	}
	args := make([]CallArg, n)
	for i := 0; i < n; i++ {
		args[i] = CallArg{
			Index:     i,
			Value:     argExprs[i],
			ParamName: paramTypes[i],
		}
		if strings.Contains(argExprs[i], ".") {
			args[i].FieldPath = argExprs[i]
		}
	}
	return args
}

func (s *FuncIndexSource) toSymbol(fn *Symbol) Symbol {
	return Symbol{
		Name:    fn.Name,
		Package: fn.Package,
		File:    fn.File,
		Line:    fn.Line,
		EndLine: fn.EndLine,
		Kind:    "function",
		Handle:  fn,
	}
}
