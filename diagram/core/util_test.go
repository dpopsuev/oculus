package core

import "testing"

func TestMermaidID(t *testing.T) {
	tests := []struct {
		input, want string
	}{
		{"simple", "simple"},
		{"with space", "with_space"},
		{"with-dash", "with_dash"},
		{"with.dot", "with_dot"},
		{"path/to/pkg", "path_to_pkg"},
		{"combo space-dash.dot/slash", "combo_space_dash_dot_slash"},
		{"", ""},
	}
	for _, tt := range tests {
		got := MermaidID(tt.input)
		if got != tt.want {
			t.Errorf("MermaidID(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
