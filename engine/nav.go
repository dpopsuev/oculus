package engine

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/dpopsuev/oculus/v3/locator"
	"github.com/dpopsuev/oculus/v3/lsp"
)

// NavSite is a compact file:line hit for definition/references.
type NavSite struct {
	File string `json:"file"`
	Line int    `json:"line"`
	Char int    `json:"char,omitempty"`
}

// DefinitionReport is the result of go-to-definition via WarmLSP.
type DefinitionReport struct {
	Locator     string          `json:"locator"`
	Symbol      string          `json:"symbol,omitempty"`
	Definitions []NavSite       `json:"definitions,omitempty"`
	Escalations []string        `json:"escalations,omitempty"`
	Candidates  []locator.Hit   `json:"candidates,omitempty"`
	Summary     string          `json:"summary"`
}

// ReferencesReport is the result of find-references via WarmLSP.
type ReferencesReport struct {
	Locator     string          `json:"locator"`
	Symbol      string          `json:"symbol,omitempty"`
	References  []NavSite       `json:"references,omitempty"`
	Escalations []string        `json:"escalations,omitempty"`
	Candidates  []locator.Hit   `json:"candidates,omitempty"`
	Summary     string          `json:"summary"`
}

// GetDefinition resolves locator then calls textDocument/definition on WarmLSP.
func (p *Engine) GetDefinition(ctx context.Context, path, raw string, cacheKey ...string) (*DefinitionReport, error) {
	hit, unresolved, err := p.resolveUniqueHit(ctx, path, raw, cacheKey...)
	if err != nil {
		return nil, err
	}
	if unresolved != nil {
		return &DefinitionReport{
			Locator:     raw,
			Escalations: unresolved.Escalations,
			Candidates:  unresolved.Candidates,
			Summary:     unresolved.Summary,
		}, nil
	}

	file, line, char, err := absHit(path, hit)
	if err != nil {
		return nil, err
	}
	locs, err := p.lspDefinition(ctx, file, line, char)
	if err != nil {
		return &DefinitionReport{
			Locator: raw,
			Symbol:  hit.FQN,
			Summary: "definition unavailable: " + err.Error(),
		}, nil
	}
	sites := locationsToSites(locs)
	return &DefinitionReport{
		Locator:     raw,
		Symbol:      hit.FQN,
		Definitions: sites,
		Summary:     fmt.Sprintf("%d definition(s) for %s", len(sites), hit.FQN),
	}, nil
}

// GetReferencesByLocator resolves locator then calls textDocument/references.
func (p *Engine) GetReferencesByLocator(ctx context.Context, path, raw string, cacheKey ...string) (*ReferencesReport, error) {
	hit, unresolved, err := p.resolveUniqueHit(ctx, path, raw, cacheKey...)
	if err != nil {
		return nil, err
	}
	if unresolved != nil {
		return &ReferencesReport{
			Locator:     raw,
			Escalations: unresolved.Escalations,
			Candidates:  unresolved.Candidates,
			Summary:     unresolved.Summary,
		}, nil
	}

	file, line, char, err := absHit(path, hit)
	if err != nil {
		return nil, err
	}
	locs, err := p.lspReferences(ctx, file, line, char)
	if err != nil {
		return &ReferencesReport{
			Locator: raw,
			Symbol:  hit.FQN,
			Summary: "references unavailable: " + err.Error(),
		}, nil
	}
	sites := locationsToSites(locs)
	return &ReferencesReport{
		Locator:    raw,
		Symbol:     hit.FQN,
		References: sites,
		Summary:    fmt.Sprintf("%d reference(s) for %s across %d file(s)", len(sites), hit.FQN, distinctFiles(sites)),
	}, nil
}

func (p *Engine) resolveUniqueHit(ctx context.Context, path, raw string, cacheKey ...string) (*locator.Hit, *locator.Result, error) {
	r, err := p.ResolveLocator(ctx, path, raw, cacheKey...)
	if err != nil {
		return nil, nil, err
	}
	if r.Hit == nil {
		return nil, r, nil
	}
	return r.Hit, nil, nil
}

func absHit(workspace string, hit *locator.Hit) (file string, line, char int, err error) {
	if hit.File == "" || hit.Line <= 0 {
		return "", 0, 0, fmt.Errorf("resolved hit lacks file:line for LSP")
	}
	file = hit.File
	if !filepath.IsAbs(file) {
		file = filepath.Join(workspace, filepath.FromSlash(hit.File))
	}
	file = filepath.Clean(file)
	line = hit.Line
	char = hit.Char
	if char == 0 {
		char = symbolCharOnLine(file, line, leafName(hit.Symbol))
	}
	return file, line, char, nil
}

func leafName(sym string) string {
	if i := strings.LastIndex(sym, "."); i >= 0 {
		return sym[i+1:]
	}
	if i := strings.LastIndex(sym, ")."); i >= 0 {
		return sym[i+2:]
	}
	return sym
}

func symbolCharOnLine(file string, line int, name string) int {
	if name == "" {
		return 0
	}
	f, err := os.Open(file)
	if err != nil {
		return 0
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	n := 0
	for sc.Scan() {
		n++
		if n != line {
			continue
		}
		text := sc.Text()
		if i := strings.Index(text, name); i >= 0 {
			return i
		}
		return 0
	}
	return 0
}

func (p *Engine) lspDefinition(ctx context.Context, file string, line, char int) ([]lsp.Location, error) {
	if p.pool == nil {
		return nil, lsp.ErrNoPool
	}
	return p.pool.Definition(ctx, file, line, char)
}

func (p *Engine) lspReferences(ctx context.Context, file string, line, char int) ([]lsp.Location, error) {
	if p.pool == nil {
		return nil, lsp.ErrNoPool
	}
	return p.pool.References(ctx, file, line, char)
}

func locationsToSites(locs []lsp.Location) []NavSite {
	out := make([]NavSite, 0, len(locs))
	seen := map[string]bool{}
	for _, l := range locs {
		file := strings.TrimPrefix(l.URI, "file://")
		key := fmt.Sprintf("%s:%d", file, l.Line)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, NavSite{File: file, Line: l.Line})
	}
	return out
}

func distinctFiles(sites []NavSite) int {
	seen := map[string]bool{}
	for _, s := range sites {
		seen[s.File] = true
	}
	return len(seen)
}
