package oculus

import "github.com/dpopsuev/oculus/v3/book"

// DiagnoseResult combines symbol vitals with relevant knowledge entries.
type DiagnoseResult struct {
	Probe *ProbeResult    `json:"probe"`
	Book  *book.BookResult `json:"book"`
}

// Diagnose probes a symbol and queries the Book with signal-derived keywords.
func Diagnose(sg *SymbolGraph, bg *book.BookGraph, symbol string) *DiagnoseResult {
	probe := Probe(sg, symbol)
	if probe == nil {
		return nil
	}

	var keywords []string

	isTypeKind := probe.Kind == "struct" || probe.Kind == "interface"

	if probe.FanOut > 6 {
		keywords = append(keywords, "fan-out")
		if !isTypeKind {
			keywords = append(keywords, "god", "facade")
		}
	}
	if probe.FanIn > 8 {
		keywords = append(keywords, "fan-in", "hub", "stability")
	}
	if probe.Circuits > 0 {
		keywords = append(keywords, "circular", "coupling")
	}

	// TSK-178: struct/interface-specific keywords instead of generic ones.
	// Note: we intentionally do NOT add the raw kind "struct" because it
	// substring-matches "constructor" in Jaccard scoring, producing false
	// factory/constructor book hits for plain data types.
	if isTypeKind {
		keywords = append(keywords, "types", "cohesion", "data-class")
	} else if probe.Kind != "" {
		// Only add the symbol's kind for non-type symbols.
		keywords = append(keywords, probe.Kind)
	}

	var bookResult *book.BookResult
	if bg != nil && len(keywords) > 0 {
		bookResult = bg.Query(keywords, 1)
	}

	return &DiagnoseResult{
		Probe: probe,
		Book:  bookResult,
	}
}
