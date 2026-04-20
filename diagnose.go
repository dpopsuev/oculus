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

	if probe.FanOut > 6 {
		keywords = append(keywords, "fan-out", "god", "facade")
	}
	if probe.FanIn > 8 {
		keywords = append(keywords, "fan-in", "hub", "stability")
	}
	if probe.Circuits > 0 {
		keywords = append(keywords, "circular", "coupling")
	}

	// Always add the symbol's kind.
	if probe.Kind != "" {
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
