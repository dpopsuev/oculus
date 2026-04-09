package analyzer

import (
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

func (f *DeepFallbackAnalyzer) CallGraph(root string, opts oculus.CallGraphOpts) (*oculus.CallGraph, error) {
	for _, a := range f.analyzers {
		r, err := a.CallGraph(root, opts)
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

func (f *DeepFallbackAnalyzer) DataFlowTrace(root, entry string, depth int) (*oculus.DataFlow, error) {
	for _, a := range f.analyzers {
		r, err := a.DataFlowTrace(root, entry, depth)
		if err == nil && len(r.Edges) > 0 {
			return r, nil
		}
	}
	return &oculus.DataFlow{}, nil
}

func (f *DeepFallbackAnalyzer) DetectStateMachines(root string) ([]oculus.StateMachine, error) {
	for _, a := range f.analyzers {
		r, err := a.DetectStateMachines(root)
		if err == nil && len(r) > 0 {
			return r, nil
		}
	}
	return nil, nil
}
