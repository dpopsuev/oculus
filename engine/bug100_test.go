package engine

import (
	"context"
	"testing"
)

// TestBug100_SearchSymbols_PipePattern reproduces LCS-BUG-100:
// symbol_search with "write|render|compose" returns zero results because
// the pipe is treated as a literal character, not an alternation.
//
// Given symbols Run, Config, DB, Get, Put, Log in the test report
// When SearchSymbols is called with "run|config"
// Then both Run and Config are returned
func TestBug100_SearchSymbols_PipePattern(t *testing.T) {
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

// TestBug100_SearchSymbols_MultiTerm_SpaceSeparated verifies space-separated
// multi-term search as an alternative to pipe syntax.
//
// Given symbols Run, Config in the test report
// When SearchSymbols is called with "run config"
// Then both are returned
func TestBug100_SearchSymbols_SpacePattern(t *testing.T) {
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

// TestBug100_SearchSymbols_SingleTerm_Unchanged verifies backward compat:
// single-term search still works exactly as before.
func TestBug100_SearchSymbols_SingleTerm_Unchanged(t *testing.T) {
	eng, _ := newTestEngine()

	r, err := eng.SearchSymbols(context.Background(), "/tmp", "run")
	if err != nil {
		t.Fatalf("SearchSymbols: %v", err)
	}
	if len(r.Matches) != 1 || r.Matches[0].Symbol != "Run" {
		t.Errorf("single term 'run' should match only Run, got %v", r.Matches)
	}
}
