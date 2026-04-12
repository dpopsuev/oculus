package analyzer

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/dpopsuev/oculus"
	"github.com/dpopsuev/oculus/lsp"
)

// DeepFallbackAnalyzer chains registered deep analyzers by priority.
// Each method tries analyzers in order, stopping at the first non-empty result.
type DeepFallbackAnalyzer struct {
	rawAnalyzers []oculus.DeepAnalyzer // raw analyzer fallback (DetectStateMachines, etc.)
	root         string
	pool         lsp.Pool
}

// NewDeepFallback creates a DeepFallbackAnalyzer using Pipeline-backed
// SymbolSources (preferred) with raw analyzers as fallback.
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
	// Resolve sources matching the requested granularity (per-request, not cached).
	sources := resolveSymbolSources(f.root, f.pool, opts.Granularity)
	slog.LogAttrs(ctx, slog.LevelInfo, "fallback: CallGraph start", slog.Int("sources", len(sources)), slog.Int("raw_analyzers", len(f.rawAnalyzers)))
	for i, src := range sources {
		if ctx.Err() != nil {
			slog.LogAttrs(ctx, slog.LevelWarn, "fallback: context cancelled", slog.Any("error", ctx.Err()))
			return &oculus.CallGraph{}, ctx.Err()
		}
		name := fmt.Sprintf("source[%d]/%T", i, src)
		slog.LogAttrs(ctx, slog.LevelInfo, "fallback: trying source", slog.String("source", name))
		start := time.Now()
		p := &oculus.SymbolPipeline{
			Source:      src,
			Root:        f.root,
			Concurrency: oculus.DefaultPipelineConcurrency,
		}
		aCtx, cancel := context.WithTimeout(ctx, perAnalyzerTimeout)
		r, err := p.CallGraph(aCtx, root, opts)
		cancel()
		elapsed := time.Since(start)
		if err == nil && len(r.Edges) > 0 {
			slog.LogAttrs(ctx, slog.LevelInfo, "fallback: source succeeded", slog.String("source", name), slog.Duration("duration", elapsed), slog.Int("edges", len(r.Edges)))
			EnrichCallEdgeTypes(f.root, r.Edges)
			return r, nil
		}
		slog.LogAttrs(ctx, slog.LevelDebug, "fallback: source empty/failed", slog.String("source", name), slog.Duration("duration", elapsed), slog.Any("error", err))
	}

	// Raw analyzer fallback.
	for i, a := range f.rawAnalyzers {
		if ctx.Err() != nil {
			slog.LogAttrs(ctx, slog.LevelWarn, "fallback: context cancelled", slog.Any("error", ctx.Err()))
			return &oculus.CallGraph{}, ctx.Err()
		}
		name := fmt.Sprintf("raw[%d]/%T", i, a)
		slog.LogAttrs(ctx, slog.LevelInfo, "fallback: trying raw analyzer", slog.String("analyzer", name))
		start := time.Now()
		aCtx, cancel := context.WithTimeout(ctx, perAnalyzerTimeout)
		r, err := a.CallGraph(aCtx, root, opts)
		cancel()
		elapsed := time.Since(start)
		if err == nil && len(r.Edges) > 0 {
			slog.LogAttrs(ctx, slog.LevelInfo, "fallback: raw analyzer succeeded", slog.String("analyzer", name), slog.Duration("duration", elapsed), slog.Int("edges", len(r.Edges)))
			EnrichCallEdgeTypes(f.root, r.Edges)
			return r, nil
		}
		slog.LogAttrs(ctx, slog.LevelDebug, "fallback: raw analyzer empty/failed", slog.String("analyzer", name), slog.Duration("duration", elapsed), slog.Any("error", err))
	}
	slog.LogAttrs(ctx, slog.LevelWarn, "fallback: all analyzers exhausted, returning empty call graph")
	return &oculus.CallGraph{}, nil
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
