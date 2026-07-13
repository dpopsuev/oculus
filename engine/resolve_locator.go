package engine

import (
	"context"
	"fmt"

	"github.com/dpopsuev/oculus/v3/locator"
)

// ResolveLocator parses an agent locator and resolves it against the symbol
// graph (SearchSymbolsFiltered). Returns a unique hit, ambiguous candidates
// with escalations, or a not-found summary. Does not call LSP.
func (p *Engine) ResolveLocator(ctx context.Context, path, raw string, cacheKey ...string) (*locator.Result, error) {
	parsed, err := locator.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("resolve locator: %w", err)
	}
	path = p.resolvePath(path)

	report, err := p.SearchSymbolsFiltered(ctx, path, parsed.Leaf(), parsed.Path, cacheKey...)
	if err != nil {
		return nil, err
	}

	pool := make([]locator.Hit, 0, len(report.Matches))
	for _, m := range report.Matches {
		pool = append(pool, locator.Hit{
			Symbol:    m.Symbol,
			Kind:      m.Kind,
			Component: m.Component,
			File:      m.File,
			Line:      m.Line,
		})
	}

	// Path filter already applied in SearchSymbolsFiltered when Path set;
	// Match re-checks and applies Parent.Symbol / line ranking.
	r := locator.Resolve(parsed, pool)
	return &r, nil
}
