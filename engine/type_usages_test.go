package engine

import (
	"context"
	"testing"
)

// TestGetTypeUsages_KnownType verifies that asking "which files use type X"
// returns files that reference that type in symbols, fields, or function
// signatures.
//
// Given a report with symbols whose types reference "Config"
// When GetTypeUsages("Config") is called
// Then the files containing that type reference are returned
func TestGetTypeUsages_KnownType(t *testing.T) {
	eng, _ := newTestEngine()

	r, err := eng.GetTypeUsages(context.Background(), "/tmp", "Config")
	if err != nil {
		t.Fatalf("GetTypeUsages: %v", err)
	}
	if r == nil {
		t.Fatal("expected non-nil result")
	}
	if len(r.Files) == 0 {
		t.Error("GetTypeUsages('Config') returned no files — at least its declaration file should appear")
	}
}

// TestGetTypeUsages_UnknownType returns empty (not an error).
// A type not found in the index simply has no usages.
func TestGetTypeUsages_UnknownType(t *testing.T) {
	eng, _ := newTestEngine()

	r, err := eng.GetTypeUsages(context.Background(), "/tmp", "NonExistentFormattedText")
	if err != nil {
		t.Fatalf("GetTypeUsages for unknown type should not error: %v", err)
	}
	if len(r.Files) != 0 {
		t.Errorf("unknown type should return zero files, got %d", len(r.Files))
	}
}
