package engine

import "testing"

// TestCalleeMatches verifies NED-15's suffix-match logic for unqualified names.
//
// Given callee FQNs like "pkg.format" or "a/b/pkg.format"
// When calleeMatches is called with the unqualified name "format"
// Then it returns true
func TestCalleeMatches(t *testing.T) {
	tests := []struct {
		callee string
		symbol string
		want   bool
	}{
		// Exact FQN match.
		{"format", "format", true},
		{"pkg.format", "pkg.format", true},

		// Unqualified suffix via dot separator.
		{"pkg.format", "format", true},
		{"a/b/pkg.format", "format", true},

		// Unqualified suffix via slash separator.
		{"a/b/format", "format", true},

		// Private underscore names (Python-style).
		{"renderer._write_output", "_write_output", true},
		{"agents.renderer._write_output", "_write_output", true},

		// No match: name is a substring but not a suffix component.
		{"pkg.reformat", "format", false},
		{"pkg.formatExtra", "format", false},

		// No match: different name entirely.
		{"pkg.render", "format", false},
	}

	for _, tt := range tests {
		got := calleeMatches(tt.callee, tt.symbol)
		if got != tt.want {
			t.Errorf("calleeMatches(%q, %q) = %v, want %v", tt.callee, tt.symbol, got, tt.want)
		}
	}
}
