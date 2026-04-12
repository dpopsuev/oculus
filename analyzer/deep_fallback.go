package analyzer

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/dpopsuev/oculus"
	"github.com/dpopsuev/oculus/lsp"
)

// DeepFallbackAnalyzer uses Racer to run DeepAnalyzers in parallel.
// SymbolPipeline-backed sources and raw analyzers race simultaneously.
// First non-empty result returns immediately; higher-quality results
// cache in background for subsequent calls.
type DeepFallbackAnalyzer struct {
	rawAnalyzers []oculus.DeepAnalyzer
	root         string
	pool         lsp.Pool
}

// NewDeepFallback creates a DeepFallbackAnalyzer.
func NewDeepFallback(root string, pool lsp.Pool) *DeepFallbackAnalyzer {
	return &DeepFallbackAnalyzer{
		rawAnalyzers: resolveDeepAnalyzers(root, pool),
		root:         root,
		pool:         pool,
	}
}

// NewPipelineFallback is an alias for NewDeepFallback (unified path).
func NewPipelineFallback(root string, pool lsp.Pool) *DeepFallbackAnalyzer {
	return NewDeepFallback(root, pool)
}

func (f *DeepFallbackAnalyzer) CallGraph(ctx context.Context, root string, opts oculus.CallGraphOpts) (*oculus.CallGraph, error) {
	// Build attempts from SymbolSources + raw analyzers.
	var attempts []Attempt[*oculus.CallGraph]

	sources := resolveSymbolSources(f.root, f.pool, opts.Granularity)
	for i, src := range sources {
		name := fmt.Sprintf("source[%d]/%T", i, src)
		// Capture for closure.
		s := src
		attempts = append(attempts, Attempt[*oculus.CallGraph]{
			Name:    name,
			Quality: QualityTreeSitter, // SymbolSources are typically tree-sitter/GoAST level
			Fn: func(ctx context.Context) (*oculus.CallGraph, error) {
				p := &oculus.SymbolPipeline{
					Source:      s,
					Root:        f.root,
					Concurrency: oculus.DefaultPipelineConcurrency,
				}
				aCtx, cancel := context.WithTimeout(ctx, perAnalyzerTimeout)
				defer cancel()
				return p.CallGraph(aCtx, root, opts)
			},
		})
	}

	for i, a := range f.rawAnalyzers {
		name := fmt.Sprintf("raw[%d]/%T", i, a)
		analyzer := a
		quality := QualityGoAST // default for raw (GoAST, GoTools, etc.)
		if _, ok := analyzer.(*LSPDeepAnalyzer); ok {
			quality = QualityLSP
		}
		if _, ok := analyzer.(*RegexDeepAnalyzer); ok {
			continue // Regex is too low quality for racing — SymbolSources cover this tier
		}
		attempts = append(attempts, Attempt[*oculus.CallGraph]{
			Name:    name,
			Quality: quality,
			Fn: func(ctx context.Context) (*oculus.CallGraph, error) {
				aCtx, cancel := context.WithTimeout(ctx, perAnalyzerTimeout)
				defer cancel()
				return analyzer.CallGraph(aCtx, root, opts)
			},
		})
	}

	racer := NewRacer(func(cg *oculus.CallGraph) bool {
		return cg == nil || len(cg.Edges) == 0
	}, attempts...)

	start := time.Now()
	result, err := racer.Race(ctx)
	elapsed := time.Since(start)

	if err != nil {
		return &oculus.CallGraph{}, err
	}

	if result.Winner != "" {
		slog.LogAttrs(ctx, slog.LevelInfo, "deep racer: CallGraph",
			slog.String("winner", result.Winner),
			slog.Int("quality", int(result.Quality)),
			slog.Duration("elapsed", elapsed),
			slog.Bool("cached", result.Cached),
			slog.Int("edges", len(result.Value.Edges)))
		EnrichCallEdgeTypes(f.root, result.Value.Edges)
	}

	if result.Value == nil {
		return &oculus.CallGraph{}, nil
	}
	return result.Value, nil
}

// perAnalyzerTimeout is the max time each analyzer gets before the fallback
// chain moves to the next one. 5 minutes gives gopls time to index large
// repos with many external dependencies.
const perAnalyzerTimeout = 5 * time.Minute

func (f *DeepFallbackAnalyzer) DataFlowTrace(ctx context.Context, root, entry string, depth int) (*oculus.DataFlow, error) {
	sources := resolveSymbolSources(f.root, f.pool)
	for _, src := range sources {
		p := &oculus.SymbolPipeline{Source: src, Root: f.root, Concurrency: oculus.DefaultPipelineConcurrency}
		r, err := p.DataFlowTrace(ctx, root, entry, depth)
		if err == nil && len(r.Edges) > 0 {
			return r, nil
		}
	}
	for _, a := range f.rawAnalyzers {
		r, err := a.DataFlowTrace(ctx, root, entry, depth)
		if err == nil && len(r.Edges) > 0 {
			return r, nil
		}
	}
	return &oculus.DataFlow{}, nil
}

func (f *DeepFallbackAnalyzer) DetectStateMachines(ctx context.Context, root string) ([]oculus.StateMachine, error) {
	for _, a := range f.rawAnalyzers {
		r, err := a.DetectStateMachines(ctx, root)
		if err == nil && len(r) > 0 {
			return r, nil
		}
	}
	return nil, nil
}
