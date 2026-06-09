package engine

import (
	"context"
	"errors"
	"testing"
)

// TestBug99_GetCallers_SymbolNotFound reproduces LCS-BUG-99:
// GetCallers returns an empty slice with no error when the symbol
// does not exist in the call graph or source index. The agent cannot
// distinguish "no callers" from "symbol not indexed".
//
// Given a project with no call graph data
// When GetCallers is called with a symbol that cannot be found
// Then an error is returned mentioning the symbol name
func TestBug99_GetCallers_UnknownSymbol_ReturnsError(t *testing.T) {
	eng, _ := newTestEngine()

	_, err := eng.GetCallers(context.Background(), "/tmp", "_nonexistent_function_xyz")
	if err == nil {
		t.Fatal("expected error for unknown symbol, got nil — agent cannot distinguish from zero-callers")
	}
	if !errors.Is(err, ErrSymbolNotFound) {
		t.Errorf("expected ErrSymbolNotFound, got %v", err)
	}
}

// TestBug99_GetCallers_EmptyIsValid verifies that a known symbol
// with zero callers still returns a nil error (empty is valid).
//
// Given a symbol that exists in the index but has no callers
// When GetCallers is called
// Then an empty report is returned without error
func TestBug99_GetCallers_KnownSymbol_ZeroCallers_OK(t *testing.T) {
	eng, _ := newTestEngine()

	// "Log" exists in the testReport (pkg/logger) but nothing calls it
	// in the call graph edges (testReport has architecture edges, not call graph edges).
	// With a mock store, the call graph is empty, so callers = 0.
	// This should not be an error — zero callers for a real symbol is valid.
	r, err := eng.GetCallers(context.Background(), "/tmp", "Log")
	if err != nil {
		t.Errorf("known symbol with no callers should not error: %v", err)
	}
	if r == nil {
		t.Fatal("expected non-nil report")
	}
}
