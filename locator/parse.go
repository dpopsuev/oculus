// Package locator parses and resolves agent-facing symbol locators.
//
// Grammar (leta-compatible):
//
//	Symbol              — bare name
//	Parent.Symbol       — qualified name (dots; Rust :: normalized to .)
//	path:Symbol         — file path filter + name
//	path:line:Symbol    — file + 1-based line + name
package locator

import (
	"fmt"
	"strconv"
	"strings"
)

// Parsed is a locator broken into optional path/line and a symbol name.
type Parsed struct {
	Raw  string `json:"raw"`
	Path string `json:"path,omitempty"`
	Line int    `json:"line,omitempty"` // 1-based; 0 = unset
	Name string `json:"name"`           // Symbol or Parent.Symbol
}

// Parse splits a locator string into path/line/name.
// Rust-style Foo::bar is normalized to Foo.bar before colon counting.
func Parse(raw string) (Parsed, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return Parsed{}, fmt.Errorf("empty locator")
	}
	normalized := strings.ReplaceAll(raw, "::", ".")
	switch strings.Count(normalized, ":") {
	case 0:
		return Parsed{Raw: raw, Name: normalized}, nil
	case 1:
		parts := strings.SplitN(normalized, ":", 2)
		if parts[0] == "" || parts[1] == "" {
			return Parsed{}, fmt.Errorf("invalid locator %q: need path:Symbol", raw)
		}
		return Parsed{Raw: raw, Path: parts[0], Name: parts[1]}, nil
	case 2:
		parts := strings.SplitN(normalized, ":", 3)
		line, err := strconv.Atoi(parts[1])
		if err != nil || line < 1 {
			return Parsed{}, fmt.Errorf("invalid line in locator %q", raw)
		}
		if parts[0] == "" || parts[2] == "" {
			return Parsed{}, fmt.Errorf("invalid locator %q: need path:line:Symbol", raw)
		}
		return Parsed{Raw: raw, Path: parts[0], Line: line, Name: parts[2]}, nil
	default:
		return Parsed{}, fmt.Errorf("invalid locator %q: too many ':' separators", raw)
	}
}

// Leaf returns the rightmost name segment (Symbol in Parent.Symbol).
func (p Parsed) Leaf() string {
	if i := strings.LastIndex(p.Name, "."); i >= 0 {
		return p.Name[i+1:]
	}
	return p.Name
}

// Parent returns the qualifier before the last dot, or "".
func (p Parsed) Parent() string {
	if i := strings.LastIndex(p.Name, "."); i >= 0 {
		return p.Name[:i]
	}
	return ""
}
