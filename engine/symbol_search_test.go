package engine

import (
	"context"
	"testing"
)

// TestSearchSymbols_PipePattern verifies that symbol_search with
// "write|render|compose" treats the pipe as alternation, not a literal.
//
// Given symbols Run, Config, DB, Get, Put, Log in the test report
// When SearchSymbols is called with "run|config"
// Then both Run and Config are returned
func TestSearchSymbols_PipePattern(t *testing.T) {
	eng, _ := newTestEngine()

	r, err := eng.SearchSymbols(context.Background(), "/tmp", "run|config")
	if err != nil {
		t.Fatalf("SearchSymbols: %v", err)
	}
	if len(r.Matches) < 2 {
		names := make([]string, len(r.Matches))
		for i, m := range r.Matches {
			names[i] = m.Symbol
		}
		t.Errorf("pipe pattern 'run|config' should match Run and Config, got %v", names)
	}
}

// TestSearchSymbols_SpacePattern verifies space-separated multi-term
// search as an alternative to pipe syntax.
//
// Given symbols Run, Config in the test report
// When SearchSymbols is called with "run config"
// Then both are returned
func TestSearchSymbols_SpacePattern(t *testing.T) {
	eng, _ := newTestEngine()

	r, err := eng.SearchSymbols(context.Background(), "/tmp", "run config")
	if err != nil {
		t.Fatalf("SearchSymbols: %v", err)
	}
	if len(r.Matches) < 2 {
		names := make([]string, len(r.Matches))
		for i, m := range r.Matches {
			names[i] = m.Symbol
		}
		t.Errorf("space pattern 'run config' should match Run and Config, got %v", names)
	}
}

// TestSearchSymbols_SingleTerm_Unchanged verifies backward compat:
// single-term search still works exactly as before.
func TestSearchSymbols_SingleTerm_Unchanged(t *testing.T) {
	eng, _ := newTestEngine()

	r, err := eng.SearchSymbols(context.Background(), "/tmp", "run")
	if err != nil {
		t.Fatalf("SearchSymbols: %v", err)
	}
	if len(r.Matches) != 1 || r.Matches[0].Symbol != "Run" {
		t.Errorf("single term 'run' should match only Run, got %v", r.Matches)
	}
}
