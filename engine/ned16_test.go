package engine

import (
	"context"
	"testing"
)

// TestNED16_GetTypeUsages_ReturnsFiles verifies LCS-NED-16:
// asking "which files use type FormattedText" returns files that
// reference that type in symbols, fields, or function signatures.
//
// Given a report with symbols whose types reference "Config"
// When GetTypeUsages is called with type_name="Config"
// Then the files containing that type reference are returned
func TestNED16_GetTypeUsages_KnownType(t *testing.T) {
	eng, _ := newTestEngine()

	// "Config" is a symbol in testReport() at internal/core.
	r, err := eng.GetTypeUsages(context.Background(), "/tmp", "Config")
	if err != nil {
		t.Fatalf("GetTypeUsages: %v", err)
	}
	if r == nil {
		t.Fatal("expected non-nil result")
	}
	// Config exists as a declared type — at minimum the file declaring it appears.
	if len(r.Files) == 0 {
		t.Error("NED-16: GetTypeUsages('Config') returned no files — at least its declaration file should appear")
	}
}

// TestNED16_GetTypeUsages_UnknownType returns empty (not an error).
// A type not found in the index simply has no usages.
func TestNED16_GetTypeUsages_UnknownType(t *testing.T) {
	eng, _ := newTestEngine()

	r, err := eng.GetTypeUsages(context.Background(), "/tmp", "NonExistentFormattedText")
	if err != nil {
		t.Fatalf("GetTypeUsages for unknown type should not error: %v", err)
	}
	if len(r.Files) != 0 {
		t.Errorf("unknown type should return zero files, got %d", len(r.Files))
	}
}
