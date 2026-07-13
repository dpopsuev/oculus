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

// OutlineNode is a compact child outline entry under a shown symbol.
type OutlineNode struct {
	Name string `json:"name"`
	Kind string `json:"kind,omitempty"`
	Line int    `json:"line,omitempty"`
}

// ShowReport is a documentSymbol-backed body slice — not go-to-definition.
// Product invariant: show ≠ definition (body/outline vs type-accurate jump).
type ShowReport struct {
	Locator     string          `json:"locator"`
	Symbol      string          `json:"symbol,omitempty"`
	File        string          `json:"file,omitempty"`
	Kind        string          `json:"kind,omitempty"`
	StartLine   int             `json:"start_line,omitempty"`
	EndLine     int             `json:"end_line,omitempty"`
	Body        string          `json:"body,omitempty"`
	Outline     []OutlineNode   `json:"outline,omitempty"`
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

// GetShow resolves locator then returns a documentSymbol body slice for the
// matched symbol. Distinct from GetDefinition (locations vs source body).
func (p *Engine) GetShow(ctx context.Context, path, raw string, cacheKey ...string) (*ShowReport, error) {
	hit, unresolved, err := p.resolveUniqueHit(ctx, path, raw, cacheKey...)
	if err != nil {
		return nil, err
	}
	if unresolved != nil {
		return &ShowReport{
			Locator:     raw,
			Escalations: unresolved.Escalations,
			Candidates:  unresolved.Candidates,
			Summary:     unresolved.Summary,
		}, nil
	}

	file, line, _, err := absHit(path, hit)
	if err != nil {
		return nil, err
	}
	syms, err := p.lspDocumentSymbols(ctx, file)
	if err != nil {
		return &ShowReport{
			Locator: raw,
			Symbol:  hit.FQN,
			File:    file,
			Summary: "show unavailable: " + err.Error(),
		}, nil
	}
	match := matchDocSymbol(syms, leafName(hit.Symbol), line)
	if match == nil {
		return &ShowReport{
			Locator: raw,
			Symbol:  hit.FQN,
			File:    file,
			Summary: fmt.Sprintf("no documentSymbol match for %s at %s:%d", hit.FQN, file, line),
		}, nil
	}
	body, err := readLineRange(file, match.StartLine, match.EndLine)
	if err != nil {
		return &ShowReport{
			Locator:   raw,
			Symbol:    hit.FQN,
			File:      file,
			Kind:      symbolKindName(match.Kind),
			StartLine: match.StartLine,
			EndLine:   match.EndLine,
			Summary:   "show body read failed: " + err.Error(),
		}, nil
	}
	outline := make([]OutlineNode, 0, len(match.Children))
	for _, ch := range match.Children {
		outline = append(outline, OutlineNode{
			Name: ch.Name,
			Kind: symbolKindName(ch.Kind),
			Line: ch.SelectionLine,
		})
	}
	return &ShowReport{
		Locator:   raw,
		Symbol:    hit.FQN,
		File:      file,
		Kind:      symbolKindName(match.Kind),
		StartLine: match.StartLine,
		EndLine:   match.EndLine,
		Body:      body,
		Outline:   outline,
		Summary:   fmt.Sprintf("show %s (%s:%d-%d, %d child(ren))", hit.FQN, filepath.Base(file), match.StartLine, match.EndLine, len(outline)),
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

func (p *Engine) lspDocumentSymbols(ctx context.Context, file string) ([]lsp.DocSymbol, error) {
	if p.pool == nil {
		return nil, lsp.ErrNoPool
	}
	return p.pool.DocumentSymbols(ctx, file)
}

// matchDocSymbol picks the best documentSymbol for name + line.
// Prefer name+selection-line, then name+range-contains-line, then range-only.
func matchDocSymbol(syms []lsp.DocSymbol, name string, line int) *lsp.DocSymbol {
	var flat []*lsp.DocSymbol
	var walk func([]lsp.DocSymbol)
	walk = func(list []lsp.DocSymbol) {
		for i := range list {
			s := &list[i]
			flat = append(flat, s)
			walk(s.Children)
		}
	}
	walk(syms)

	var byNameLine, byNameRange, byRange *lsp.DocSymbol
	for _, s := range flat {
		inRange := line >= s.StartLine && line <= s.EndLine
		nameOK := name == "" || s.Name == name || strings.HasSuffix(s.Name, "."+name) || strings.HasSuffix(s.Name, ")."+name)
		if nameOK && s.SelectionLine == line {
			byNameLine = s
			break
		}
		if nameOK && inRange && byNameRange == nil {
			byNameRange = s
		}
		if inRange && byRange == nil {
			byRange = s
		}
	}
	switch {
	case byNameLine != nil:
		return byNameLine
	case byNameRange != nil:
		return byNameRange
	default:
		return byRange
	}
}

func readLineRange(file string, start, end int) (string, error) {
	if start < 1 {
		start = 1
	}
	if end < start {
		end = start
	}
	f, err := os.Open(file)
	if err != nil {
		return "", err
	}
	defer f.Close()
	var b strings.Builder
	sc := bufio.NewScanner(f)
	// Allow long lines in body slices.
	buf := make([]byte, 0, 64*1024)
	sc.Buffer(buf, 1024*1024)
	n := 0
	for sc.Scan() {
		n++
		if n < start {
			continue
		}
		if n > end {
			break
		}
		if b.Len() > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(sc.Text())
	}
	if err := sc.Err(); err != nil {
		return "", err
	}
	if n < start {
		return "", fmt.Errorf("file has %d lines, want start %d", n, start)
	}
	return b.String(), nil
}

func symbolKindName(kind int) string {
	// LSP SymbolKind
	names := map[int]string{
		1: "file", 2: "module", 3: "namespace", 4: "package", 5: "class",
		6: "method", 7: "property", 8: "field", 9: "constructor", 10: "enum",
		11: "interface", 12: "function", 13: "variable", 14: "constant",
		15: "string", 16: "number", 17: "boolean", 18: "array", 19: "object",
		20: "key", 21: "null", 22: "enumMember", 23: "struct", 24: "event",
		25: "operator", 26: "typeParameter",
	}
	if n, ok := names[kind]; ok {
		return n
	}
	return fmt.Sprintf("kind:%d", kind)
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
