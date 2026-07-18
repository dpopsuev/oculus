package locator

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
)

// Hit is one resolved symbol candidate (graph or search hit).
type Hit struct {
	Symbol    string `json:"symbol"`
	Kind      string `json:"kind,omitempty"`
	Component string `json:"component,omitempty"`
	File      string `json:"file,omitempty"`
	Line      int    `json:"line,omitempty"`
	Char      int    `json:"char,omitempty"` // 0-based; 0 when unknown
	FQN       string `json:"fqn,omitempty"`
}

// Result is the outcome of resolving a locator against a candidate pool.
type Result struct {
	Locator     Parsed   `json:"locator"`
	Hit         *Hit     `json:"hit,omitempty"`
	Candidates  []Hit    `json:"candidates,omitempty"`
	Escalations []string `json:"escalations,omitempty"`
	Summary     string   `json:"summary"`
}

// Match filters pool to hits that satisfy the parsed locator, ranked best-first.
func Match(p Parsed, pool []Hit) []Hit {
	var out []Hit
	for _, h := range pool {
		if p.Path != "" && !fileMatches(h.File, p.Path) {
			continue
		}
		// Only enforce line when both sides know it.
		if p.Line > 0 && h.Line > 0 && h.Line != p.Line {
			continue
		}
		if !nameMatches(p, h) {
			continue
		}
		out = append(out, withFQN(h))
	}
	sort.SliceStable(out, func(i, j int) bool {
		return rank(p, out[i]) > rank(p, out[j])
	})
	return out
}

// Resolve picks a unique hit or returns ambiguous/not-found Result.
func Resolve(p Parsed, pool []Hit) Result {
	hits := Match(p, pool)
	switch len(hits) {
	case 0:
		return Result{
			Locator: p,
			Summary: notFoundSummary(p),
		}
	case 1:
		h := hits[0]
		return Result{
			Locator: p,
			Hit:     &h,
			Summary: fmt.Sprintf("resolved %s → %s (%s:%d)", p.Raw, h.FQN, h.File, h.Line),
		}
	default:
		return Result{
			Locator:     p,
			Candidates:  hits,
			Escalations: Escalations(p, hits),
			Summary:     fmt.Sprintf("%d candidates for %q; escalate with path:Symbol or Parent.Symbol", len(hits), p.Raw),
		}
	}
}

// notFoundSummary explains a miss with actionable next steps for agents.
func notFoundSummary(p Parsed) string {
	base := fmt.Sprintf("no symbol matching %q", p.Raw)
	switch {
	case p.Path != "" && p.Line > 0 && p.Line <= 2:
		return base + " — line looks wrong; use the definition line, bare name, or analysis action=resolve"
	case p.Path != "" && p.Line > 0:
		return base + " — try bare name or correct file:line:Name; analysis action=resolve"
	case p.Path != "":
		return base + " — try path:line:Name or bare name; analysis action=resolve"
	case strings.Contains(p.Name, "."):
		return base + " — try file:line:Name or analysis action=resolve"
	default:
		return base + " — try file:line:Name (definition line) or analysis action=resolve"
	}
}

// Escalations suggests tighter locators for ambiguous hits.
func Escalations(p Parsed, hits []Hit) []string {
	seen := make(map[string]bool)
	var out []string
	add := func(s string) {
		if s == "" || seen[s] {
			return
		}
		seen[s] = true
		out = append(out, s)
	}
	for _, h := range hits {
		leaf := p.Leaf()
		if leaf == "" {
			leaf = h.Symbol
		}
		parent := effectiveParent(h)
		if parent != "" {
			add(parent + "." + leaf)
		}
		if h.File != "" {
			add(filepath.ToSlash(h.File) + ":" + leaf)
			if h.Line > 0 {
				add(fmt.Sprintf("%s:%d:%s", filepath.ToSlash(h.File), h.Line, leaf))
			}
		}
	}
	return out
}

func nameMatches(p Parsed, h Hit) bool {
	leaf := p.Leaf()
	parent := p.Parent()
	sym := h.Symbol

	if strings.EqualFold(sym, p.Name) || strings.EqualFold(sym, leaf) {
		if parent == "" {
			return true
		}
		return parentMatches(parent, h)
	}
	// Go methods: (*Engine).WarmLSP / (Engine).WarmLSP
	if goMethodLeaf(sym) == leaf {
		if parent == "" {
			return true
		}
		return parentMatches(parent, h) || strings.Contains(sym, parent)
	}
	// Qualified stored name Engine.WarmLSP
	if strings.HasSuffix(sym, "."+leaf) {
		if parent == "" {
			return true
		}
		return strings.Contains(sym, parent) || parentMatches(parent, h)
	}
	return false
}

func parentMatches(parent string, h Hit) bool {
	if parent == "" {
		return true
	}
	if strings.EqualFold(effectiveParent(h), parent) {
		return true
	}
	if strings.Contains(h.Component, parent) {
		return true
	}
	if strings.Contains(h.FQN, parent) {
		return true
	}
	// Go (*Parent).Method
	if strings.Contains(h.Symbol, "("+parent+")") || strings.Contains(h.Symbol, "(*"+parent+")") {
		return true
	}
	return false
}

func effectiveParent(h Hit) string {
	if p := goMethodParent(h.Symbol); p != "" {
		return p
	}
	if i := strings.LastIndex(h.Symbol, "."); i > 0 {
		return h.Symbol[:i]
	}
	base := filepath.Base(h.Component)
	if base == "." || base == "/" || base == "" {
		return ""
	}
	return base
}

func goMethodLeaf(sym string) string {
	// (*T).M or (T).M
	if i := strings.LastIndex(sym, ")."); i >= 0 && i+2 < len(sym) {
		return sym[i+2:]
	}
	return ""
}

func goMethodParent(sym string) string {
	start := strings.Index(sym, "(")
	end := strings.Index(sym, ")")
	if start < 0 || end <= start {
		return ""
	}
	recv := strings.TrimPrefix(sym[start+1:end], "*")
	return recv
}

func fileMatches(symFile, want string) bool {
	if symFile == "" || want == "" {
		return false
	}
	a := filepath.ToSlash(filepath.Clean(symFile))
	b := filepath.ToSlash(filepath.Clean(want))
	if a == b {
		return true
	}
	return strings.HasSuffix(a, "/"+b) || strings.HasSuffix(b, "/"+a) || strings.HasSuffix(a, b)
}

func withFQN(h Hit) Hit {
	if h.FQN != "" {
		return h
	}
	if h.Component != "" {
		h.FQN = h.Component + "." + h.Symbol
	} else {
		h.FQN = h.Symbol
	}
	return h
}

func rank(p Parsed, h Hit) int {
	score := 0
	leaf := p.Leaf()
	if strings.EqualFold(h.Symbol, p.Name) {
		score += 100
	} else if strings.EqualFold(h.Symbol, leaf) {
		score += 80
	} else if goMethodLeaf(h.Symbol) == leaf {
		score += 70
	}
	if p.Parent() != "" && parentMatches(p.Parent(), h) {
		score += 40
	}
	if p.Line > 0 && h.Line == p.Line {
		score += 30
	}
	if p.Path != "" && fileMatches(h.File, p.Path) {
		score += 20
	}
	return score
}
