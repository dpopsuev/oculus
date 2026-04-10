package analyzer

import (
	"context"
	"time"
	"github.com/dpopsuev/oculus"
	"github.com/dpopsuev/oculus/lsp"
)

// DeepFallbackAnalyzer chains registered deep analyzers by priority.
// Each method tries analyzers in order, stopping at the first non-empty result.
type DeepFallbackAnalyzer struct {
	analyzers []oculus.DeepAnalyzer // ordered by priority (highest first)
	root      string
	pool      lsp.Pool
}

// NewDeepFallback creates a DeepFallbackAnalyzer using the strategy registry.
// Analyzers are resolved by detected language and priority order.
func NewDeepFallback(root string, pool lsp.Pool) *DeepFallbackAnalyzer {
	return &DeepFallbackAnalyzer{
		analyzers: resolveDeepAnalyzers(root, pool),
		root:      root,
		pool:      pool,
	}
}

func (f *DeepFallbackAnalyzer) CallGraph(ctx context.Context, root string, opts oculus.CallGraphOpts) (*oculus.CallGraph, error) {
	for _, a := range f.analyzers {
		// Each analyzer gets its own timeout so a slow LSP doesn't starve GoAST.
		aCtx, cancel := context.WithTimeout(context.Background(), perAnalyzerTimeout)
		r, err := a.CallGraph(aCtx, root, opts)
		cancel()
		if err == nil && len(r.Edges) > 0 {
			// Universal enrichment: fill in types for any edges still missing them.
			// Individual analyzers may already populate types (GoAST, LSP hover),
			// but this catches gaps (Regex, partial TreeSitter).
			EnrichCallEdgeTypes(f.root, r.Edges)
			return r, nil
		}
	}
	return &oculus.CallGraph{}, nil
}

// perAnalyzerTimeout is the max time each analyzer gets before the fallback
// chain moves to the next one. Prevents a slow LSP from starving GoAST.
const perAnalyzerTimeout = 30 * time.Second

func (f *DeepFallbackAnalyzer) DataFlowTrace(ctx context.Context, root, entry string, depth int) (*oculus.DataFlow, error) {
	for _, a := range f.analyzers {
		r, err := a.DataFlowTrace(ctx, root, entry, depth)
		if err == nil && len(r.Edges) > 0 {
			return r, nil
		}
	}
	return &oculus.DataFlow{}, nil
}

func (f *DeepFallbackAnalyzer) DetectStateMachines(ctx context.Context, root string) ([]oculus.StateMachine, error) {
	for _, a := range f.analyzers {
		r, err := a.DetectStateMachines(ctx, root)
		if err == nil && len(r) > 0 {
			return r, nil
		}
	}
	return nil, nil
}
