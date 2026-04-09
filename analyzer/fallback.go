package analyzer

import (
	"github.com/dpopsuev/oculus"
	"github.com/dpopsuev/oculus/lsp"
)

// FallbackAnalyzer chains registered TypeAnalyzers by priority.
type FallbackAnalyzer struct {
	analyzers []oculus.TypeAnalyzer
}

// NewFallback creates a FallbackAnalyzer using the strategy registry.
func NewFallback(root string, pool lsp.Pool) *FallbackAnalyzer {
	return &FallbackAnalyzer{
		analyzers: resolveTypeAnalyzers(root, pool),
	}
}

func (f *FallbackAnalyzer) Classes(root string) ([]oculus.ClassInfo, error) {
	for _, a := range f.analyzers {
		r, err := a.Classes(root)
		if err == nil && len(r) > 0 {
			return r, nil
		}
	}
	return nil, nil
}

func (f *FallbackAnalyzer) Implements(root string) ([]oculus.ImplEdge, error) {
	for _, a := range f.analyzers {
		r, err := a.Implements(root)
		if err == nil && len(r) > 0 {
			return r, nil
		}
	}
	return nil, nil
}

func (f *FallbackAnalyzer) CallChain(root, entry string, depth int) ([]oculus.Call, error) {
	for _, a := range f.analyzers {
		r, err := a.CallChain(root, entry, depth)
		if err == nil && len(r) > 0 {
			return r, nil
		}
	}
	return nil, nil
}

func (f *FallbackAnalyzer) EntryPoints(root string) ([]oculus.EntryPoint, error) {
	for _, a := range f.analyzers {
		r, err := a.EntryPoints(root)
		if err == nil && len(r) > 0 {
			return r, nil
		}
	}
	return nil, nil
}

func (f *FallbackAnalyzer) FieldRefs(root string) ([]oculus.FieldRef, error) {
	for _, a := range f.analyzers {
		r, err := a.FieldRefs(root)
		if err == nil && len(r) > 0 {
			return r, nil
		}
	}
	return nil, nil
}

func (f *FallbackAnalyzer) NestingDepth(root string) ([]oculus.NestingResult, error) {
	for _, a := range f.analyzers {
		r, err := a.NestingDepth(root)
		if err == nil && len(r) > 0 {
			return r, nil
		}
	}
	return nil, nil
}
