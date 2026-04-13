package analyzer

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/dpopsuev/oculus"
	"github.com/dpopsuev/oculus/lsp"
)

// FallbackAnalyzer uses Racer to run TypeAnalyzers in parallel.
// First non-empty result returns immediately. Slower, higher-quality
// results cache in background for subsequent calls.
type FallbackAnalyzer struct {
	analyzers       []oculus.TypeAnalyzer
	classesRacer    *Racer[[]oculus.ClassInfo]
	implementsRacer *Racer[[]oculus.ImplEdge]
	fieldRefsRacer  *Racer[[]oculus.FieldRef]
}

// NewFallback creates a FallbackAnalyzer with Racer-backed parallel execution.
func NewFallback(root string, pool lsp.Pool) *FallbackAnalyzer {
	analyzers := resolveTypeAnalyzers(root, pool)

	// Build Racer attempts from registered analyzers.
	// Registry priority maps directly to QualityTier.
	var classAttempts []Attempt[[]oculus.ClassInfo]
	var implAttempts []Attempt[[]oculus.ImplEdge]
	var refAttempts []Attempt[[]oculus.FieldRef]

	for i, entry := range registry {
		if entry.typeA == nil {
			continue
		}
		a := entry.typeA(root, pool)
		if a == nil {
			continue
		}
		quality := QualityTier(entry.priority)
		name := fmt.Sprintf("type[%d]/%T", i, a)

		classAttempts = append(classAttempts, Attempt[[]oculus.ClassInfo]{
			Name: name, Quality: quality,
			Fn: func(ctx context.Context) ([]oculus.ClassInfo, error) {
				aCtx, cancel := context.WithTimeout(ctx, perAnalyzerTimeout)
				defer cancel()
				return a.Classes(aCtx, root)
			},
		})
		implAttempts = append(implAttempts, Attempt[[]oculus.ImplEdge]{
			Name: name, Quality: quality,
			Fn: func(ctx context.Context) ([]oculus.ImplEdge, error) {
				aCtx, cancel := context.WithTimeout(ctx, perAnalyzerTimeout)
				defer cancel()
				return a.Implements(aCtx, root)
			},
		})
		refAttempts = append(refAttempts, Attempt[[]oculus.FieldRef]{
			Name: name, Quality: quality,
			Fn: func(ctx context.Context) ([]oculus.FieldRef, error) {
				aCtx, cancel := context.WithTimeout(ctx, perAnalyzerTimeout)
				defer cancel()
				return a.FieldRefs(aCtx, root)
			},
		})
	}

	return &FallbackAnalyzer{
		analyzers:       analyzers,
		classesRacer:    NewRacer(func(r []oculus.ClassInfo) bool { return len(r) == 0 }, classAttempts...).WithMinQuality(QualityTreeSitter),
		implementsRacer: NewRacer(func(r []oculus.ImplEdge) bool { return len(r) == 0 }, implAttempts...).WithMinQuality(QualityTreeSitter),
		fieldRefsRacer:  NewRacer(func(r []oculus.FieldRef) bool { return len(r) == 0 }, refAttempts...),
	}
}

func (f *FallbackAnalyzer) Classes(ctx context.Context, _ string) ([]oculus.ClassInfo, error) {
	result, err := f.classesRacer.Race(ctx)
	if err != nil {
		return nil, err
	}
	if result.Winner != "" {
		slog.LogAttrs(ctx, slog.LevelInfo, "racer: Classes",
			slog.String("winner", result.Winner),
			slog.Int("quality", int(result.Quality)),
			slog.Duration("elapsed", result.Elapsed),
			slog.Bool("cached", result.Cached),
			slog.Int("count", len(result.Value)))
	}
	return result.Value, nil
}

func (f *FallbackAnalyzer) Implements(ctx context.Context, _ string) ([]oculus.ImplEdge, error) {
	result, err := f.implementsRacer.Race(ctx)
	if err != nil {
		return nil, err
	}
	if result.Winner != "" {
		slog.LogAttrs(ctx, slog.LevelInfo, "racer: Implements",
			slog.String("winner", result.Winner),
			slog.Int("quality", int(result.Quality)),
			slog.Duration("elapsed", result.Elapsed),
			slog.Bool("cached", result.Cached),
			slog.Int("count", len(result.Value)))
	}
	return result.Value, nil
}

func (f *FallbackAnalyzer) FieldRefs(ctx context.Context, _ string) ([]oculus.FieldRef, error) {
	result, err := f.fieldRefsRacer.Race(ctx)
	if err != nil {
		return nil, err
	}
	return result.Value, nil
}

// Sequential fallback for less-hot methods.

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
