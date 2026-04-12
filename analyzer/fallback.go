package analyzer

import (
	"context"

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

func (f *FallbackAnalyzer) Classes(ctx context.Context, root string) ([]oculus.ClassInfo, error) {
	for _, a := range f.analyzers {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		aCtx, cancel := context.WithTimeout(ctx, perAnalyzerTimeout)
		r, err := a.Classes(aCtx, root)
		cancel()
		if err == nil && len(r) > 0 {
			return r, nil
		}
	}
	return nil, nil
}

func (f *FallbackAnalyzer) Implements(ctx context.Context, root string) ([]oculus.ImplEdge, error) {
	for _, a := range f.analyzers {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		aCtx, cancel := context.WithTimeout(ctx, perAnalyzerTimeout)
		r, err := a.Implements(aCtx, root)
		cancel()
		if err == nil && len(r) > 0 {
			return r, nil
		}
	}
	return nil, nil
}

func (f *FallbackAnalyzer) CallChain(ctx context.Context, root, entry string, depth int) ([]oculus.Call, error) {
	for _, a := range f.analyzers {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		aCtx, cancel := context.WithTimeout(ctx, perAnalyzerTimeout)
		r, err := a.CallChain(aCtx, root, entry, depth)
		cancel()
		if err == nil && len(r) > 0 {
			return r, nil
		}
	}
	return nil, nil
}

func (f *FallbackAnalyzer) EntryPoints(ctx context.Context, root string) ([]oculus.EntryPoint, error) {
	for _, a := range f.analyzers {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		aCtx, cancel := context.WithTimeout(ctx, perAnalyzerTimeout)
		r, err := a.EntryPoints(aCtx, root)
		cancel()
		if err == nil && len(r) > 0 {
			return r, nil
		}
	}
	return nil, nil
}

func (f *FallbackAnalyzer) FieldRefs(ctx context.Context, root string) ([]oculus.FieldRef, error) {
	for _, a := range f.analyzers {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		aCtx, cancel := context.WithTimeout(ctx, perAnalyzerTimeout)
		r, err := a.FieldRefs(aCtx, root)
		cancel()
		if err == nil && len(r) > 0 {
			return r, nil
		}
	}
	return nil, nil
}

func (f *FallbackAnalyzer) NestingDepth(ctx context.Context, root string) ([]oculus.NestingResult, error) {
	for _, a := range f.analyzers {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		aCtx, cancel := context.WithTimeout(ctx, perAnalyzerTimeout)
		r, err := a.NestingDepth(aCtx, root)
		cancel()
		if err == nil && len(r) > 0 {
			return r, nil
		}
	}
	return nil, nil
}
