package analyzer

import (
	"context"

	"github.com/dpopsuev/oculus"
	"github.com/dpopsuev/oculus/lang"
	"github.com/dpopsuev/oculus/lsp"
)

func init() {
	RegisterSource(lang.Unknown, 50, func(root string, _ lsp.Pool) oculus.SymbolSource {
		return NewTreeSitterSymbolSource(root)
	})
}

// TreeSitterSymbolSource implements oculus.SymbolSource using tree-sitter.
// All files are parsed upfront via ParsedProject; Roots/Children/Hover
// query the cached ASTs and source bytes.
type TreeSitterSymbolSource struct {
	project   *ParsedProject
	funcIndex map[string]cgFuncDef // keyed by "pkg.Name"
}

// NewTreeSitterSymbolSource builds a SymbolSource from a parsed project.
func NewTreeSitterSymbolSource(root string) *TreeSitterSymbolSource {
	pp, err := BuildParsedProject(root)
	if err != nil || pp == nil {
		return nil
	}
	a := &TreeSitterDeepAnalyzer{project: pp}
	allFuncs, _ := a.extractCallGraphFuncs(oculus.CallGraphOpts{})
	return &TreeSitterSymbolSource{project: pp, funcIndex: allFuncs}
}

var _ oculus.SymbolSource = (*TreeSitterSymbolSource)(nil)

func (s *TreeSitterSymbolSource) Roots(_ context.Context, query string) ([]oculus.SourceSymbol, error) {
	if query != "" {
		for key, fd := range s.funcIndex {
			if fd.name == query {
				return []oculus.SourceSymbol{s.defToSymbol(key, &fd)}, nil
			}
		}
		return nil, nil
	}
	var roots []oculus.SourceSymbol
	for key, fd := range s.funcIndex {
		if isExported(fd.name) {
			roots = append(roots, s.defToSymbol(key, &fd))
		}
	}
	return roots, nil
}

func (s *TreeSitterSymbolSource) Children(_ context.Context, sym oculus.SourceSymbol) ([]oculus.SourceRelation, error) {
	key, ok := sym.Handle.(string)
	if !ok || key == "" {
		// Try to find by name.
		for k, fd := range s.funcIndex {
			if fd.name == sym.Name {
				key = k
				break
			}
		}
		if key == "" {
			return nil, nil
		}
	}

	fd, ok := s.funcIndex[key]
	if !ok {
		return nil, nil
	}

	var rels []oculus.SourceRelation
	extractCalls(fd.body, fd.src, func(callee string, _ int) {
		calleeKey, _ := resolveCallee(callee, fd.pkg, s.funcIndex)
		calleeDef, found := s.funcIndex[calleeKey]
		if !found {
			return
		}
		rels = append(rels, oculus.SourceRelation{
			Target:      s.defToSymbol(calleeKey, &calleeDef),
			Kind:        "call",
			InWorkspace: true,
		})
	})
	return rels, nil
}

func (s *TreeSitterSymbolSource) Hover(_ context.Context, sym oculus.SourceSymbol) (*oculus.SourceTypeInfo, error) {
	key, ok := sym.Handle.(string)
	if !ok || key == "" {
		for k, fd := range s.funcIndex {
			if fd.name == sym.Name {
				key = k
				_ = fd
				break
			}
		}
	}
	fd, ok := s.funcIndex[key]
	if !ok {
		return nil, nil
	}
	if len(fd.paramTypes) == 0 && len(fd.returnTypes) == 0 {
		return nil, nil
	}
	return &oculus.SourceTypeInfo{
		ParamTypes:  fd.paramTypes,
		ReturnTypes: fd.returnTypes,
	}, nil
}

func (s *TreeSitterSymbolSource) defToSymbol(key string, fd *cgFuncDef) oculus.SourceSymbol {
	return oculus.SourceSymbol{
		Name:    fd.name,
		Package: fd.pkg,
		File:    fd.file,
		Line:    fd.line,
		EndLine: fd.endLine,
		Kind:    12, // function
		Handle:  key,
	}
}
